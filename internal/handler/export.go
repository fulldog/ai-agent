package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/webapp/go-app/ai-agent/internal/service/filegen"
)

// ExportHandler 提供生成文件的下载。链接里的 UUID 即访问凭证，不要求 API Key。
type ExportHandler struct {
	Files *filegen.Store
}

func (h *ExportHandler) Download(c *gin.Context) {
	if h == nil || h.Files == nil {
		writeError(c, http.StatusNotFound, "not_found", "文件不存在或已过期")
		return
	}
	name, body, err := h.Files.Read(c.Param("id"))
	if err != nil {
		if errors.Is(err, filegen.ErrNotFound) {
			writeError(c, http.StatusNotFound, "not_found", "文件不存在或已过期")
			return
		}
		writeError(c, http.StatusInternalServerError, "internal_error", "读取文件失败")
		return
	}
	c.Header("Content-Disposition", contentDisposition(name, false))
	c.Header("Cache-Control", "private")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Data(http.StatusOK, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", body)
}
