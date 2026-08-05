package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/webapp/go-app/ai-agent/internal/service/llm"
)

type ModelsHandler struct {
	Pool *llm.Pool
}

// List GET /api/v1/models — 列出 yaml 中配置的厂商（不含 api_key）。
func (h *ModelsHandler) List(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"providers": h.Pool.List()})
}
