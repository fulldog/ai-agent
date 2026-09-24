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
