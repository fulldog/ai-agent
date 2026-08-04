package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/webapp/go-app/ai-agent/internal/service/rag"
)

type RAGHandler struct {
	RAG *rag.Service
}

func (h *RAGHandler) Search(c *gin.Context) {
	var req struct {
		CorpusID string `json:"corpus_id" binding:"required"`
		Query    string `json:"query" binding:"required"`
		TopK     int    `json:"top_k"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	cid, err := uuid.Parse(req.CorpusID)
	if err != nil {
		writeError(c, http.StatusBadRequest, "bad_request", "invalid corpus_id")
		return
	}
	hits, err := h.RAG.Search(c.Request.Context(), cid, req.Query, req.TopK)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"results": hits})
}
