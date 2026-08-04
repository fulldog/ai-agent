package app

import (
	"github.com/webapp/go-app/ai-agent/internal/ai/eino"
	"github.com/webapp/go-app/ai-agent/internal/config"
	"github.com/webapp/go-app/ai-agent/internal/service/agent"
	"github.com/webapp/go-app/ai-agent/internal/service/chat"
	"github.com/webapp/go-app/ai-agent/internal/service/corpus"
	"github.com/webapp/go-app/ai-agent/internal/service/embed"
	"github.com/webapp/go-app/ai-agent/internal/service/llm"
	"github.com/webapp/go-app/ai-agent/internal/service/rag"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type App struct {
	Config *config.Config
	DB     *gorm.DB
	Log    *zap.Logger
	LLM    *llm.Client
	Embed  *embed.Client
	RAG    *rag.Service
	Corpus *corpus.Service
	Chat   *chat.Service
	Agent  *agent.Service
	Eino   *eino.Runtime
}

func New(cfg *config.Config, db *gorm.DB, log *zap.Logger) (*App, error) {
	rt, err := eino.NewRuntime(cfg)
	if err != nil {
		return nil, err
	}
	embedClient := embed.NewClient(cfg.Embed.BaseURL, cfg.Embed.APIKey, cfg.Embed.Model, cfg.Embed.Dimensions)
	ragSvc := rag.New(db, embedClient)
	corpusSvc := corpus.New(db, cfg, embedClient)
	chatSvc := chat.New(db, cfg, rt.Client, ragSvc)
	agentSvc := agent.New(db, cfg, rt.Client, ragSvc)
	return &App{
		Config: cfg,
		DB:     db,
		Log:    log,
		LLM:    rt.Client,
		Embed:  embedClient,
		RAG:    ragSvc,
		Corpus: corpusSvc,
		Chat:   chatSvc,
		Agent:  agentSvc,
		Eino:   rt,
	}, nil
}
