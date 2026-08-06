package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type HealthHandler struct {
	DB        *gorm.DB
	DBEnabled bool // 配置是否启用数据库；false 时为最小化部署
}

func (h *HealthHandler) Health(c *gin.Context) {
	if !h.DBEnabled || h.DB == nil {
		c.JSON(http.StatusOK, gin.H{
			"status": "ok",
			"db":     "disabled",
			"mode":   "minimal",
		})
		return
	}
	dbStatus := "up"
	sqlDB, err := h.DB.DB()
	if err != nil || sqlDB.Ping() != nil {
		dbStatus = "down"
	}
	status := http.StatusOK
	if dbStatus != "up" {
		status = http.StatusServiceUnavailable
	}
	c.JSON(status, gin.H{
		"status": map[bool]string{true: "ok", false: "degraded"}[dbStatus == "up"],
		"db":     dbStatus,
	})
}
