package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/webapp/go-app/ai-agent/internal/service/chat"
)

type ChatHandler struct {
	Chat *chat.Service
}

func (h *ChatHandler) parseInput(c *gin.Context) (chat.CompleteInput, bool) {
	var req struct {
		ConversationID string  `json:"conversation_id" binding:"required"`
		Message        string  `json:"message" binding:"required"`
		Model          string  `json:"model"`
		Temperature    float64 `json:"temperature"`
		MaxTokens      int     `json:"max_tokens"`
		RAG            *struct {
			Enabled  bool   `json:"enabled"`
			CorpusID string `json:"corpus_id"`
			TopK     int    `json:"top_k"`
		} `json:"rag"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "bad_request", err.Error())
		return chat.CompleteInput{}, false
	}
	cid, err := uuid.Parse(req.ConversationID)
	if err != nil {
		writeError(c, http.StatusBadRequest, "bad_request", "invalid conversation_id")
		return chat.CompleteInput{}, false
	}
	in := chat.CompleteInput{
		ConversationID: cid,
		Message:        req.Message,
		Model:          req.Model,
		Temperature:    req.Temperature,
		MaxTokens:      req.MaxTokens,
		RequestID:      requestID(c),
	}
	if req.RAG != nil {
		in.RAGEnabled = req.RAG.Enabled
		in.TopK = req.RAG.TopK
		if req.RAG.CorpusID != "" {
			id, err := uuid.Parse(req.RAG.CorpusID)
			if err != nil {
				writeError(c, http.StatusBadRequest, "bad_request", "invalid corpus_id")
				return chat.CompleteInput{}, false
			}
			in.CorpusID = &id
		}
	}
	return in, true
}

func (h *ChatHandler) Completions(c *gin.Context) {
	in, ok := h.parseInput(c)
	if !ok {
		return
	}
	res, err := h.Chat.Complete(c.Request.Context(), in)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"conversation_id": res.ConversationID,
		"message_id":      res.MessageID,
		"content":         res.Content,
		"usage": gin.H{
			"prompt_tokens":     res.PromptTokens,
			"completion_tokens": res.CompletionTokens,
			"total_tokens":      res.PromptTokens + res.CompletionTokens,
		},
	})
}

func (h *ChatHandler) CompletionsStream(c *gin.Context) {
	in, ok := h.parseInput(c)
	if !ok {
		return
	}
	setupSSE(c)
	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		writeError(c, http.StatusInternalServerError, "internal_error", "streaming unsupported")
		return
	}
	_ = flusher
	_, err := h.Chat.CompleteStream(c.Request.Context(), in, func(delta string) error {
		return writeSSE(c, "delta", gin.H{"content": delta})
	})
	if err != nil {
		_ = writeSSE(c, "error", gin.H{"message": err.Error()})
		return
	}
	_ = writeSSE(c, "done", gin.H{"status": "ok"})
}
