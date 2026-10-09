package handler

import (
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/webapp/go-app/ai-agent/internal/service/dbconn"
)

const maxDictionaryBytes = 8 << 20

// DictionaryHandler 更新本地 Srm 数据字典文件，并立刻替换进程内副本。
type DictionaryHandler struct {
	DBConn *dbconn.Client
}

// Update PUT /api/v1/dbconn/dictionary — 请求体为完整的 srm_dictionary.json。需管理员 API Key。
func (h *DictionaryHandler) Update(c *gin.Context) {
	if h == nil || h.DBConn == nil {
		writeError(c, http.StatusServiceUnavailable, "dbconn_disabled", "业务库未配置")
		return
	}
	raw, err := io.ReadAll(io.LimitReader(c.Request.Body, maxDictionaryBytes+1))
	if err != nil {
		writeError(c, http.StatusBadRequest, "invalid_body", "读取请求体失败")
		return
	}
	if len(raw) > maxDictionaryBytes {
		writeError(c, http.StatusRequestEntityTooLarge, "body_too_large", "字典不能超过 8MB")
		return
	}
	tables, columns, err := h.DBConn.UpdateDictionary(raw)
	if err != nil {
		writeError(c, http.StatusBadRequest, "invalid_dictionary", err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"tables":  tables,
		"columns": columns,
	})
}
