package handler

import (
	"net/http"
	"strconv"
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
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	q := h.DB.Model(&model.RequestLog{}).Order("created_at desc")
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
	var rows []model.RequestLog
	if err := q.Limit(limit).Offset(offset).Find(&rows).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": rows})
}

func (h *LogsHandler) GetRequest(c *gin.Context) {
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
	c.JSON(http.StatusOK, row)
}
