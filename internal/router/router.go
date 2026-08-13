package router

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/webapp/go-app/ai-agent/internal/app"
	"github.com/webapp/go-app/ai-agent/internal/handler"
	"github.com/webapp/go-app/ai-agent/internal/middleware"
)

// Setup 注册全局中间件与 HTTP 路由，返回可 ListenAndServe 的 Gin Engine。
func Setup(application *app.App) *gin.Engine {
	cfg := application.Config
	if cfg.Server.Mode == "release" {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()
	// 全局中间件：panic 恢复 → 请求日志落库/zap → Prometheus HTTP 指标
	r.Use(middleware.Recover(application.Log))
	accessLog := application.AccessLog
	if accessLog == nil {
		accessLog = application.Log
	}
	r.Use(middleware.RequestLog(cfg, application.DB, accessLog))
	if cfg.Metrics.Enabled {
		r.Use(middleware.Metrics())
	}

	// ---------- 公开路由（无需 X-API-Key）----------
	healthH := &handler.HealthHandler{DB: application.DB, DBEnabled: cfg.Database.IsEnabled()}
	r.GET("/health", healthH.Health) // 健康检查，含 DB 连通性

	// Prometheus 抓取端点；protect=true 时需 admin API Key
	if cfg.Metrics.Enabled {
		metricsHandler := promhttp.Handler()
		if cfg.Metrics.Protect {
			r.GET(cfg.Metrics.Path, middleware.AdminAPIKey(cfg), gin.WrapH(metricsHandler))
		} else {
			r.GET(cfg.Metrics.Path, gin.WrapH(metricsHandler))
		}
	}

	// ---------- 业务 Handler ----------
	convH := &handler.ConversationHandler{Chat: application.Chat}
	chatH := &handler.ChatHandler{Chat: application.Chat, FileExtract: application.FileExtract, DB: application.DB}
	intentH := &handler.IntentHandler{Intent: application.Intent}
	agentH := &handler.AgentHandler{Agent: application.Agent}
	corpusH := &handler.CorpusHandler{Corpus: application.Corpus, FileExtract: application.FileExtract}
	ragH := &handler.RAGHandler{RAG: application.RAG}
	logsH := &handler.LogsHandler{DB: application.DB}
	modelsH := &handler.ModelsHandler{Pool: application.LLMPool}

	needDB := middleware.RequireDB(application.DB)

	// ---------- /api/v1（需 X-API-Key）----------
	v1 := r.Group("/api/v1")
	v1.Use(middleware.APIKey(cfg))
	v1.Use(middleware.UserID())
	{
		v1.GET("/models", modelsH.List) // 已配置的 LLM 厂商列表（不含密钥）

		// 无库可用：文件分析、意图分析
		v1.POST("/chat/analyze", chatH.Analyze)
		v1.POST("/chat/analyze/stream", chatH.AnalyzeStream)
		v1.POST("/chat/intent", intentH.AnalyzeIntent)

		// 以下依赖 PostgreSQL
		dbGroup := v1.Group("")
		dbGroup.Use(needDB)
		{
			dbGroup.POST("/conversations", convH.Create)
			dbGroup.GET("/conversations", convH.List)
			dbGroup.GET("/conversations/:id", convH.Get)
			dbGroup.DELETE("/conversations/:id", convH.Delete)
			dbGroup.GET("/conversations/:id/messages", convH.Messages)

			dbGroup.POST("/chat/completions", chatH.Completions)
			dbGroup.POST("/chat/completions/stream", chatH.CompletionsStream)

			dbGroup.POST("/agent/runs", agentH.Run)
			dbGroup.POST("/agent/runs/stream", agentH.RunStream)
			dbGroup.GET("/agent/runs/:id", agentH.Get)

			dbGroup.POST("/corpora", corpusH.Create)
			dbGroup.GET("/corpora", corpusH.List)
			dbGroup.GET("/corpora/:id", corpusH.Get)
			dbGroup.DELETE("/corpora/:id", corpusH.Delete)
			dbGroup.POST("/corpora/:id/documents", corpusH.AddDocument)
			dbGroup.GET("/corpora/:id/documents", corpusH.ListDocuments)
			dbGroup.DELETE("/corpora/:id/documents/:doc_id", corpusH.DeleteDocument)
			dbGroup.POST("/corpora/:id/reindex", corpusH.Reindex)

			dbGroup.POST("/rag/search", ragH.Search)

			dbGroup.GET("/logs/requests", logsH.ListRequests)
			dbGroup.GET("/logs/requests/:id", logsH.GetRequest)
		}
	}

	r.NoRoute(func(c *gin.Context) {
		c.JSON(http.StatusNotFound, gin.H{
			"error":      gin.H{"code": "not_found", "message": "route not found"},
			"request_id": c.Writer.Header().Get("X-Request-ID"),
		})
	})
	return r
}
