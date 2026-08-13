package handler

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/webapp/go-app/ai-agent/internal/service/intent"
)

type IntentHandler struct {
	Intent *intent.Service
}

// AnalyzeIntent POST /api/v1/chat/intent
// 自然语言意图分析（默认 qwen，不依赖 DB）。
func (h *IntentHandler) AnalyzeIntent(c *gin.Context) {
	if h.Intent == nil {
		writeError(c, http.StatusServiceUnavailable, "unavailable", "intent service not configured")
		return
	}
	var req struct {
		Text      string `json:"text" binding:"required"`
		Provider  string `json:"provider"`
		Model     string `json:"model"`
		MaxTokens int    `json:"max_tokens"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if strings.TrimSpace(req.Text) == "" {
		writeError(c, http.StatusBadRequest, "bad_request", "text is required")
		return
	}

	out, err := h.Intent.Analyze(c.Request.Context(), intent.AnalyzeInput{
		Text:      req.Text,
		Provider:  req.Provider,
		Model:     req.Model,
		MaxTokens: req.MaxTokens,
		RequestID: requestID(c),
	})
	if err != nil {
		writeError(c, http.StatusBadGateway, "llm_error", err.Error())
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":              out.Code,
		"msg":               out.Msg,
		"data":              out.Data,
		"provider":          out.Provider,
		"model":             out.Model,
		"prompt_tokens":     out.PromptTokens,
		"completion_tokens": out.CompletionTokens,
		"request_id":        requestID(c),
	})
}
