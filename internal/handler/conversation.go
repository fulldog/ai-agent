package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/webapp/go-app/ai-agent/internal/service/chat"
)

type ConversationHandler struct {
	Chat *chat.Service
}

func (h *ConversationHandler) Create(c *gin.Context) {
	var req struct {
		Title        string  `json:"title"`
		SystemPrompt string  `json:"system_prompt"`
		CorpusID     *string `json:"corpus_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	var corpusID *uuid.UUID
	if req.CorpusID != nil && *req.CorpusID != "" {
		id, err := uuid.Parse(*req.CorpusID)
		if err != nil {
			writeError(c, http.StatusBadRequest, "bad_request", "invalid corpus_id")
			return
		}
		corpusID = &id
	}
	conv, err := h.Chat.CreateConversation(chat.CreateConversationInput{
		Title: req.Title, SystemPrompt: req.SystemPrompt, CorpusID: corpusID,
	})
	if err != nil {
		writeError(c, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	c.JSON(http.StatusCreated, conv)
}

func (h *ConversationHandler) List(c *gin.Context) {
	rows, err := h.Chat.ListConversations(20, 0)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": rows})
}

func (h *ConversationHandler) Get(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		writeError(c, http.StatusBadRequest, "bad_request", "invalid id")
		return
	}
	conv, err := h.Chat.GetConversation(id)
	if err != nil {
		writeError(c, http.StatusNotFound, "not_found", "conversation not found")
		return
	}
	c.JSON(http.StatusOK, conv)
}

func (h *ConversationHandler) Delete(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		writeError(c, http.StatusBadRequest, "bad_request", "invalid id")
		return
	}
	if err := h.Chat.DeleteConversation(id); err != nil {
		writeError(c, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *ConversationHandler) Messages(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		writeError(c, http.StatusBadRequest, "bad_request", "invalid id")
		return
	}
	rows, err := h.Chat.ListMessages(id, 200)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": rows})
}
