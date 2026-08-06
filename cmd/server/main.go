package main

import (
	"context"
	"flag"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/webapp/go-app/ai-agent/internal/app"
	"github.com/webapp/go-app/ai-agent/internal/config"
	"github.com/webapp/go-app/ai-agent/internal/database"
	"github.com/webapp/go-app/ai-agent/internal/logger"
	"github.com/webapp/go-app/ai-agent/internal/router"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

func main() {
	configPath := flag.String("config", "configs/config.yaml", "path to config file")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		panic(err)
	}

	alsoStdout := cfg.Log.AlsoStdout == nil || *cfg.Log.AlsoStdout
	bundle, err := logger.NewBundle(logger.Options{
		Level:      cfg.Log.Level,
		Encoding:   cfg.Log.Encoding,
		Dir:        cfg.Log.Dir,
		Filename:   cfg.Log.Filename,
		MaxSizeMB:  cfg.Log.MaxSizeMB,
		MaxBackups: cfg.Log.MaxBackups,
		MaxAgeDays: cfg.Log.MaxAgeDays,
		AlsoStdout: alsoStdout,
	})
	if err != nil {
		panic(err)
	}
	log := bundle.App
	defer log.Sync()           //nolint:errcheck
	defer bundle.Access.Sync() //nolint:errcheck
	defer bundle.LLM.Sync()    //nolint:errcheck

	var db *gorm.DB
	if cfg.Database.IsEnabled() {
		opened, err := database.Open(cfg.Database, log)
		if err != nil {
			log.Fatal("open database", zap.Error(err))
		}
		db = opened
		if err := database.EnsureChunkEmbeddingColumn(db, cfg.Embed.Dimensions); err != nil {
			log.Fatal("ensure embedding column", zap.Error(err))
		}
		_ = database.EnsureVectorIndex(db, cfg.RAG.VectorIndex)
	} else {
		log.Warn("database disabled; minimal mode (analyze without DB)")
	}

	application, err := app.New(cfg, db, log, bundle.Access, bundle.LLM)
	if err != nil {
		log.Fatal("init app", zap.Error(err))
	}
	engine := router.Setup(application)

	srv := &http.Server{
		Addr:              cfg.Server.Addr,
		Handler:           engine,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Info("server listening",
			zap.String("addr", cfg.Server.Addr),
			zap.String("mode", cfg.Server.Mode),
			zap.Bool("log_stdout", alsoStdout),
			zap.Bool("database_enabled", cfg.Database.IsEnabled()),
			zap.String("llm_default_provider", cfg.LLM.DefaultProvider),
			zap.String("llm_default_model", cfg.LLM.DefaultModel),
			zap.Bool("llm_default_key_set", cfg.LLM.APIKey != ""),
		)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal("server error", zap.Error(err))
		}
	}()

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	<-ch
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
	log.Info("server stopped")
}
