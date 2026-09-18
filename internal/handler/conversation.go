package handler

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/webapp/go-app/ai-agent/internal/middleware"
	"github.com/webapp/go-app/ai-agent/internal/service/chat"
	"gorm.io/gorm"
)

type ConversationHandler struct {
	Chat *chat.Service
}

func (h *ConversationHandler) Create(c *gin.Context) {
	uid, ok := requireUID(c)
	if !ok {
		return
	}
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
		UID: uid, Title: req.Title, SystemPrompt: req.SystemPrompt, CorpusID: corpusID,
	})
	if err != nil {
		writeError(c, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	c.JSON(http.StatusCreated, conv)
}

func (h *ConversationHandler) List(c *gin.Context) {
	admin := middleware.IsAdminContext(c)
	filter := ""
	if admin {
		filter = strings.TrimSpace(c.Query("uid"))
	} else {
		uid, ok := requireUID(c)
		if !ok {
			return
		}
		filter = uid
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	rows, total, err := h.Chat.QueryConversations(filter, admin, limit, offset)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": rows, "total": total, "limit": clampLimit(limit), "offset": max0(offset), "scope_admin": admin})
}

func (h *ConversationHandler) Get(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		writeError(c, http.StatusBadRequest, "bad_request", "invalid id")
		return
	}
	uid, admin, ok := bindUID(c, false)
	if !ok {
		return
	}
	if admin {
		row, err := h.Chat.GetConversationByID(id)
		if err != nil {
			writeError(c, http.StatusNotFound, "not_found", "conversation not found")
			return
		}
		c.JSON(http.StatusOK, row)
		return
	}
	row, err := h.Chat.GetConversation(id, uid)
	if err != nil {
		writeError(c, http.StatusNotFound, "not_found", "conversation not found")
		return
	}
	c.JSON(http.StatusOK, row)
}

func (h *ConversationHandler) Delete(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		writeError(c, http.StatusBadRequest, "bad_request", "invalid id")
		return
	}
	uid, admin, ok := bindUID(c, false)
	if !ok {
		return
	}
	var delErr error
	if admin {
		delErr = h.Chat.DeleteConversationByID(id)
	} else {
		delErr = h.Chat.DeleteConversation(id, uid)
	}
	if delErr != nil {
		if errors.Is(delErr, gorm.ErrRecordNotFound) {
			writeError(c, http.StatusNotFound, "not_found", "conversation not found")
			return
		}
		writeError(c, http.StatusInternalServerError, "internal_error", delErr.Error())
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
	uid, admin, ok := bindUID(c, false)
	if !ok {
		return
	}
	var msgs interface{}
	var listErr error
	if admin {
		msgs, listErr = h.Chat.ListMessagesByID(id, 500)
	} else {
		msgs, listErr = h.Chat.ListMessages(id, uid, 500)
	}
	if listErr != nil {
		if errors.Is(listErr, gorm.ErrRecordNotFound) {
			writeError(c, http.StatusNotFound, "not_found", "conversation not found")
			return
		}
		writeError(c, http.StatusInternalServerError, "internal_error", listErr.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": msgs})
}

func requireUID(c *gin.Context) (string, bool) {
	uid, _, ok := bindUID(c, true)
	return uid, ok
}

// bindUID 读取 X-User-Id。admin 密钥可不带头（forceUID=false）；创建会话等仍须 forceUID=true。
func bindUID(c *gin.Context, forceUID bool) (uid string, admin bool, ok bool) {
	admin = middleware.IsAdminContext(c)
	uid = middleware.UIDFromContext(c)
	if forceUID || !admin {
		if uid == "" {
			writeError(c, http.StatusBadRequest, "uid_required", "missing X-User-Id header")
			return "", admin, false
		}
	}
	return uid, admin, true
}

func clampLimit(limit int) int {
	if limit <= 0 {
		return 20
	}
	if limit > 200 {
		return 200
	}
	return limit
}

func max0(n int) int {
	if n < 0 {
		return 0
	}
	return n
}
