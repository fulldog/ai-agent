package handler

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/webapp/go-app/ai-agent/internal/model"
	"gorm.io/gorm"
)

type LogsHandler struct {
	DB *gorm.DB
}

func (h *LogsHandler) ListRequests(c *gin.Context) {
	uid, admin, ok := bindUID(c, false)
	if !ok {
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	q := h.DB.Model(&model.RequestLog{}).Order("created_at desc")
	if admin {
		if v := strings.TrimSpace(c.Query("uid")); v != "" {
			q = q.Where("uid = ?", v)
		}
	} else {
		q = q.Where("uid = ?", uid)
	}
	if v := c.Query("request_id"); v != "" {
		q = q.Where("request_id = ?", v)
	}
	if v := c.Query("path"); v != "" {
		q = q.Where("path_template = ? OR path = ?", v, v)
	}
	if v := c.Query("conversation_id"); v != "" {
		if id, err := uuid.Parse(v); err == nil {
			q = q.Where("conversation_id = ?", id)
		}
	}
	if v := c.Query("agent_run_id"); v != "" {
		if id, err := uuid.Parse(v); err == nil {
			q = q.Where("agent_run_id = ?", id)
		}
	}
	if v := c.Query("from"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			q = q.Where("created_at >= ?", t)
		}
	}
	if v := c.Query("to"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			q = q.Where("created_at <= ?", t)
		}
	}
	var total int64
	if err := q.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	var rows []model.RequestLog
	if err := q.Limit(limit).Offset(offset).Find(&rows).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": rows, "total": total, "scope_admin": admin})
}

type requestLogDetail struct {
	model.RequestLog
	LLMCalls []model.LLMCallLog `json:"llm_calls"`
}

func (h *LogsHandler) GetRequest(c *gin.Context) {
	uid, admin, ok := bindUID(c, false)
	if !ok {
		return
	}
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		writeError(c, http.StatusBadRequest, "bad_request", "invalid id")
		return
	}
	var row model.RequestLog
	if err := h.DB.First(&row, "id = ?", id).Error; err != nil {
		writeError(c, http.StatusNotFound, "not_found", "log not found")
		return
	}
	if !admin && row.UID != uid {
		writeError(c, http.StatusNotFound, "not_found", "log not found")
		return
	}
	out := requestLogDetail{RequestLog: row, LLMCalls: []model.LLMCallLog{}}
	if row.RequestID != "" {
		_ = h.DB.Where("request_id = ?", row.RequestID).Order("created_at asc").Find(&out.LLMCalls).Error
	}
	c.JSON(http.StatusOK, out)
}
