package app

import (
	"github.com/webapp/go-app/ai-agent/internal/ai/eino"
	"github.com/webapp/go-app/ai-agent/internal/config"
	"github.com/webapp/go-app/ai-agent/internal/service/agent"
	"github.com/webapp/go-app/ai-agent/internal/service/chat"
	"github.com/webapp/go-app/ai-agent/internal/service/corpus"
	"github.com/webapp/go-app/ai-agent/internal/service/embed"
	"github.com/webapp/go-app/ai-agent/internal/service/fileextract"
	"github.com/webapp/go-app/ai-agent/internal/service/intent"
	"github.com/webapp/go-app/ai-agent/internal/service/llm"
	"github.com/webapp/go-app/ai-agent/internal/service/rag"
	"github.com/webapp/go-app/ai-agent/pkg/extract"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type App struct {
	Config      *config.Config
	DB          *gorm.DB
	Log         *zap.Logger // 应用日志：info→info 文件，error→error 文件
	AccessLog   *zap.Logger // HTTP access 分类
	LLMLog      *zap.Logger // 大模型完整 prompt/回复 → llm 文件
	LLM         *llm.Client
	LLMPool     *llm.Pool
	Embed       *embed.Client
	RAG         *rag.Service
	Corpus      *corpus.Service
	Chat        *chat.Service
	Intent      *intent.Service
	Agent       *agent.Service
	Eino        *eino.Runtime
	Extract     *extract.Extractor
	FileExtract *fileextract.Service
}

func New(cfg *config.Config, db *gorm.DB, log, accessLog, llmLog *zap.Logger) (*App, error) {
	if accessLog == nil {
		accessLog = log
	}
	if llmLog == nil {
		llmLog = zap.NewNop()
	}
	rt, err := eino.NewRuntime(cfg)
	if err != nil {
		return nil, err
	}
	embedClient := embed.NewClient(cfg.Embed.BaseURL, cfg.Embed.APIKey, cfg.Embed.Model, cfg.Embed.Dimensions)
	ragSvc := rag.New(db, embedClient)
	corpusSvc := corpus.New(db, cfg, embedClient)
	chatSvc := chat.New(db, cfg, rt.Pool, ragSvc, llmLog)
	intentSvc := intent.New(cfg, rt.Pool, db, llmLog)
	agentSvc := agent.New(db, cfg, rt.Pool, ragSvc, llmLog)
	extractor := extract.New(extract.OCRConfig{
		Enabled:           cfg.OCR.Enabled,
		TesseractPath:     cfg.OCR.TesseractPath,
		Languages:         cfg.OCR.Languages,
		PDFToPPMPath:      cfg.OCR.PDFToPPMPath,
		PDFToTextPath:     cfg.OCR.PDFToTextPath,
		MinPDFTextLen:     cfg.OCR.MinPDFTextLen,
		TimeoutSeconds:    cfg.OCR.TimeoutSeconds,
		DPI:               cfg.OCR.DPI,
		PSM:               cfg.OCR.PSM,
		OEM:               cfg.OCR.OEM,
		PDFToPPMGray:      cfg.OCR.PDFToPPMGray,
		CollapseCJKSpaces: cfg.OCR.CollapseCJKSpaces,
	})
	fileExtractSvc := fileextract.New(db, extractor, cfg.Storage.AttachmentsDir, cfg)
	return &App{
		Config:      cfg,
		DB:          db,
		Log:         log,
		AccessLog:   accessLog,
		LLMLog:      llmLog,
		LLM:         rt.Client,
		LLMPool:     rt.Pool,
		Embed:       embedClient,
		RAG:         ragSvc,
		Corpus:      corpusSvc,
		Chat:        chatSvc,
		Intent:      intentSvc,
		Agent:       agentSvc,
		Eino:        rt,
		Extract:     extractor,
		FileExtract: fileExtractSvc,
	}, nil
}
