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
	r.Use(middleware.RequestLog(cfg, application.DB, application.Log))
	if cfg.Metrics.Enabled {
		r.Use(middleware.Metrics())
	}

	// ---------- 公开路由（无需 X-API-Key）----------
	healthH := &handler.HealthHandler{DB: application.DB}
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
	chatH := &handler.ChatHandler{Chat: application.Chat}
	agentH := &handler.AgentHandler{Agent: application.Agent}
	corpusH := &handler.CorpusHandler{Corpus: application.Corpus, Extract: application.Extract}
	ragH := &handler.RAGHandler{RAG: application.RAG}
	logsH := &handler.LogsHandler{DB: application.DB}
	modelsH := &handler.ModelsHandler{Pool: application.LLMPool}

	// ---------- /api/v1（需 X-API-Key）----------
	v1 := r.Group("/api/v1")
	v1.Use(middleware.APIKey(cfg))
	{
		v1.GET("/models", modelsH.List) // 已配置的 LLM 厂商列表（不含密钥）

		// 会话管理
		v1.POST("/conversations", convH.Create)               // 创建会话
		v1.GET("/conversations", convH.List)                  // 会话列表
		v1.GET("/conversations/:id", convH.Get)               // 会话详情
		v1.DELETE("/conversations/:id", convH.Delete)         // 删除会话
		v1.GET("/conversations/:id/messages", convH.Messages) // 会话消息列表

		// 聊天（同步 / SSE 流式）
		v1.POST("/chat/completions", chatH.Completions)              // 同步补全
		v1.POST("/chat/completions/stream", chatH.CompletionsStream) // SSE 流式补全

		// Agent 运行（同步 / SSE；含 Tool Calling）
		v1.POST("/agent/runs", agentH.Run)              // 同步运行
		v1.POST("/agent/runs/stream", agentH.RunStream) // SSE 流式运行
		v1.GET("/agent/runs/:id", agentH.Get)           // 查询运行记录与步骤

		// 语料库 / 文档索引（RAG 数据源）
		v1.POST("/corpora", corpusH.Create)                                 // 创建语料库
		v1.GET("/corpora", corpusH.List)                                    // 语料库列表
		v1.GET("/corpora/:id", corpusH.Get)                                 // 语料库详情
		v1.DELETE("/corpora/:id", corpusH.Delete)                           // 删除语料库
		v1.POST("/corpora/:id/documents", corpusH.AddDocument)              // 上传/写入文档并索引
		v1.GET("/corpora/:id/documents", corpusH.ListDocuments)             // 文档列表
		v1.DELETE("/corpora/:id/documents/:doc_id", corpusH.DeleteDocument) // 删除文档
		v1.POST("/corpora/:id/reindex", corpusH.Reindex)                    // 重建向量索引

		// RAG 检索调试
		v1.POST("/rag/search", ragH.Search) // 按语料库做向量检索

		// 请求日志查询
		v1.GET("/logs/requests", logsH.ListRequests)   // 列表（可筛选）
		v1.GET("/logs/requests/:id", logsH.GetRequest) // 单条详情
	}

	// 未匹配路由
	r.NoRoute(func(c *gin.Context) {
		c.JSON(http.StatusNotFound, gin.H{"error": gin.H{"code": "not_found", "message": "route not found"}})
	})
	return r
}
