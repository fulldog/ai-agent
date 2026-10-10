package handler

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/webapp/go-app/ai-agent/internal/service/dbconn"
)

// DictionaryHandler 从业务库重建 Srm 数据字典。
type DictionaryHandler struct {
	DBConn *dbconn.Client
}

// Update PUT /api/v1/dbconn/dictionary — 重新连接业务库，读取 Srm 前缀表并重建字典。
func (h *DictionaryHandler) Update(c *gin.Context) {
	if h == nil || h.DBConn == nil {
		writeError(c, http.StatusServiceUnavailable, "dbconn_disabled", "业务库未配置")
		return
	}
	tables, columns, err := h.DBConn.RebuildDictionary(c.Request.Context())
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, dbconn.ErrDBNotConnected) || strings.Contains(err.Error(), "重新连接数据库") {
			status = http.StatusServiceUnavailable
		}
		writeError(c, status, "dictionary_rebuild_failed", err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"tables":  tables,
		"columns": columns,
	})
}
