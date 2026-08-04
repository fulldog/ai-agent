package router

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/webapp/go-app/ai-agent/internal/app"
	"github.com/webapp/go-app/ai-agent/internal/handler"
	"github.com/webapp/go-app/ai-agent/internal/middleware"
)

func Setup(application *app.App) *gin.Engine {
	cfg := application.Config
	if cfg.Server.Mode == "release" {
		gin.SetMode(gin.ReleaseMode)
	}
	r := gin.New()
	r.Use(middleware.Recover(application.Log))
	r.Use(middleware.RequestLog(cfg, application.DB, application.Log))
	if cfg.Metrics.Enabled {
		r.Use(middleware.Metrics())
	}

	healthH := &handler.HealthHandler{DB: application.DB}
	r.GET("/health", healthH.Health)

	if cfg.Metrics.Enabled {
		metricsHandler := promhttp.Handler()
		if cfg.Metrics.Protect {
			r.GET(cfg.Metrics.Path, middleware.AdminAPIKey(cfg), gin.WrapH(metricsHandler))
		} else {
			r.GET(cfg.Metrics.Path, gin.WrapH(metricsHandler))
		}
	}

	convH := &handler.ConversationHandler{Chat: application.Chat}
	chatH := &handler.ChatHandler{Chat: application.Chat}
	agentH := &handler.AgentHandler{Agent: application.Agent}
	corpusH := &handler.CorpusHandler{Corpus: application.Corpus}
	ragH := &handler.RAGHandler{RAG: application.RAG}
	logsH := &handler.LogsHandler{DB: application.DB}

	v1 := r.Group("/api/v1")
	v1.Use(middleware.APIKey(cfg))
	{
		v1.POST("/conversations", convH.Create)
		v1.GET("/conversations", convH.List)
		v1.GET("/conversations/:id", convH.Get)
		v1.DELETE("/conversations/:id", convH.Delete)
		v1.GET("/conversations/:id/messages", convH.Messages)

		v1.POST("/chat/completions", chatH.Completions)
		v1.POST("/chat/completions/stream", chatH.CompletionsStream)

		v1.POST("/agent/runs", agentH.Run)
		v1.POST("/agent/runs/stream", agentH.RunStream)
		v1.GET("/agent/runs/:id", agentH.Get)

		v1.POST("/corpora", corpusH.Create)
		v1.GET("/corpora", corpusH.List)
		v1.GET("/corpora/:id", corpusH.Get)
		v1.DELETE("/corpora/:id", corpusH.Delete)
		v1.POST("/corpora/:id/documents", corpusH.AddDocument)
		v1.GET("/corpora/:id/documents", corpusH.ListDocuments)
		v1.DELETE("/corpora/:id/documents/:doc_id", corpusH.DeleteDocument)
		v1.POST("/corpora/:id/reindex", corpusH.Reindex)

		v1.POST("/rag/search", ragH.Search)

		v1.GET("/logs/requests", logsH.ListRequests)
		v1.GET("/logs/requests/:id", logsH.GetRequest)
	}

	r.NoRoute(func(c *gin.Context) {
		c.JSON(http.StatusNotFound, gin.H{"error": gin.H{"code": "not_found", "message": "route not found"}})
	})
	return r
}
