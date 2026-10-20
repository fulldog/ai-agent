package handler

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/webapp/go-app/ai-agent/internal/middleware"
	"github.com/webapp/go-app/ai-agent/internal/service/llm"
)

type ModelsHandler struct {
	Pool *llm.Pool
}

// List GET /api/v1/models — 列出 yaml 中配置的厂商（不含 api_key）。
func (h *ModelsHandler) List(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"providers": h.Pool.List(), "is_admin": middleware.IsAdminContext(c)})
}

// Available GET /api/v1/models/available — 按厂商分组返回上游模型列表。
func (h *ModelsHandler) Available(c *gin.Context) {
	if h == nil || h.Pool == nil {
		c.JSON(http.StatusOK, gin.H{"groups": []llm.ModelGroup{}})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 12*time.Second)
	defer cancel()
	groups := h.Pool.AvailableModels(ctx)
	if groups == nil {
		groups = []llm.ModelGroup{}
	}
	c.JSON(http.StatusOK, gin.H{"groups": groups})
}
