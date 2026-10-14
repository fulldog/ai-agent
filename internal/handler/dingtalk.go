package handler

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/webapp/go-app/ai-agent/internal/service/dingtalk"
	"gorm.io/gorm"
)

type DingTalkHandler struct {
	Bot *dingtalk.Bot
}

func (h *DingTalkHandler) GetReplyMode(c *gin.Context) {
	if h == nil || h.Bot == nil {
		writeError(c, http.StatusServiceUnavailable, "unavailable", "dingtalk not configured")
		return
	}
	c.JSON(http.StatusOK, h.Bot.ReplyModeView())
}

func (h *DingTalkHandler) SetReplyMode(c *gin.Context) {
	if h == nil || h.Bot == nil {
		writeError(c, http.StatusServiceUnavailable, "unavailable", "dingtalk not configured")
		return
	}
	var req struct {
		ReplyMode string `json:"reply_mode"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if err := h.Bot.SetReplyMode(req.ReplyMode); err != nil {
		if errors.Is(err, dingtalk.ErrInvalidReplyMode) {
			writeError(c, http.StatusBadRequest, "bad_request", "reply_mode must be chat, agent, or web")
			return
		}
		if errors.Is(err, dingtalk.ErrReplyModeStore) {
			writeError(c, http.StatusServiceUnavailable, "unavailable", "database required to save reply mode")
			return
		}
		writeError(c, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	c.JSON(http.StatusOK, h.Bot.ReplyModeView())
}

func (h *DingTalkHandler) ListChats(c *gin.Context) {
	if h == nil || h.Bot == nil {
		writeError(c, http.StatusServiceUnavailable, "unavailable", "dingtalk not configured")
		return
	}
	rows, err := h.Bot.ListChats()
	if err != nil {
		writeError(c, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": rows})
}

func (h *DingTalkHandler) SetChatCorpora(c *gin.Context) {
	if h == nil || h.Bot == nil {
		writeError(c, http.StatusServiceUnavailable, "unavailable", "dingtalk not configured")
		return
	}
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		writeError(c, http.StatusBadRequest, "bad_request", "invalid id")
		return
	}
	var req struct {
		CorpusIDs []string `json:"corpus_ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	ids, err := parseCorpusIDs(req.CorpusIDs)
	if err != nil {
		writeError(c, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	item, err := h.Bot.SetChatCorpora(id, ids)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeError(c, http.StatusNotFound, "not_found", "dingtalk chat not found")
			return
		}
		if isClientCorpusErr(err) {
			writeError(c, http.StatusBadRequest, "bad_request", err.Error())
			return
		}
		writeError(c, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	c.JSON(http.StatusOK, item)
}

func (h *DingTalkHandler) SetChatDictionary(c *gin.Context) {
	if h == nil || h.Bot == nil {
		writeError(c, http.StatusServiceUnavailable, "unavailable", "dingtalk not configured")
		return
	}
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		writeError(c, http.StatusBadRequest, "bad_request", "invalid id")
		return
	}
	var req struct {
		DocumentID string `json:"document_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	docID, err := parseOptionalUUID(req.DocumentID)
	if err != nil {
		writeError(c, http.StatusBadRequest, "bad_request", "invalid document_id")
		return
	}
	item, err := h.Bot.SetChatDictionary(id, docID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeError(c, http.StatusNotFound, "not_found", "dingtalk chat not found")
			return
		}
		if errors.Is(err, dingtalk.ErrDocumentNotFound) {
			writeError(c, http.StatusBadRequest, "bad_request", "document not found")
			return
		}
		writeError(c, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	c.JSON(http.StatusOK, item)
}

func (h *DingTalkHandler) SetChatPrefix(c *gin.Context) {
	if h == nil || h.Bot == nil {
		writeError(c, http.StatusServiceUnavailable, "unavailable", "dingtalk not configured")
		return
	}
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		writeError(c, http.StatusBadRequest, "bad_request", "invalid id")
		return
	}
	var req struct {
		PrefixPrompt string `json:"prefix_prompt"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	item, err := h.Bot.SetChatPrefix(id, req.PrefixPrompt)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeError(c, http.StatusNotFound, "not_found", "dingtalk chat not found")
			return
		}
		if errors.Is(err, dingtalk.ErrPrefixTooLong) {
			writeError(c, http.StatusBadRequest, "bad_request", "前置提示词不能超过 4000 字")
			return
		}
		writeError(c, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	c.JSON(http.StatusOK, item)
}

func (h *DingTalkHandler) SetChatHistoryLimit(c *gin.Context) {
	if h == nil || h.Bot == nil {
		writeError(c, http.StatusServiceUnavailable, "unavailable", "dingtalk not configured")
		return
	}
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		writeError(c, http.StatusBadRequest, "bad_request", "invalid id")
		return
	}
	var req struct {
		HistoryLimit int `json:"history_limit"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	item, err := h.Bot.SetChatHistoryLimit(id, req.HistoryLimit)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeError(c, http.StatusNotFound, "not_found", "dingtalk chat not found")
			return
		}
		if errors.Is(err, dingtalk.ErrHistoryLimit) {
			writeError(c, http.StatusBadRequest, "bad_request", "上下文条数须为 0 到 200 的整数")
			return
		}
		writeError(c, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	c.JSON(http.StatusOK, item)
}

func (h *DingTalkHandler) SetChatTools(c *gin.Context) {
	if h == nil || h.Bot == nil {
		writeError(c, http.StatusServiceUnavailable, "unavailable", "dingtalk not configured")
		return
	}
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		writeError(c, http.StatusBadRequest, "bad_request", "invalid id")
		return
	}
	var req struct {
		Tools []string `json:"tools"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	item, err := h.Bot.SetChatTools(id, req.Tools)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeError(c, http.StatusNotFound, "not_found", "dingtalk chat not found")
			return
		}
		if errors.Is(err, dingtalk.ErrUnknownTool) {
			writeError(c, http.StatusBadRequest, "bad_request", err.Error())
			return
		}
		writeError(c, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	c.JSON(http.StatusOK, item)
}

func parseOptionalUUID(raw string) (*uuid.UUID, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	id, err := uuid.Parse(raw)
	if err != nil || id == uuid.Nil {
		if err == nil {
			return nil, errors.New("invalid id")
		}
		return nil, err
	}
	return &id, nil
}

func parseCorpusIDs(raw []string) ([]uuid.UUID, error) {
	if raw == nil {
		return []uuid.UUID{}, nil
	}
	out := make([]uuid.UUID, 0, len(raw))
	for _, s := range raw {
		id, err := uuid.Parse(s)
		if err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, nil
}

func isClientCorpusErr(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return msg == "invalid corpus_id" || strings.HasPrefix(msg, "corpus not found")
}
