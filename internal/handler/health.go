package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type HealthHandler struct {
	DB *gorm.DB
}

func (h *HealthHandler) Health(c *gin.Context) {
	dbStatus := "up"
	if h.DB != nil {
		sqlDB, err := h.DB.DB()
		if err != nil || sqlDB.Ping() != nil {
			dbStatus = "down"
		}
	}
	status := http.StatusOK
	if dbStatus != "up" {
		status = http.StatusServiceUnavailable
	}
	c.JSON(status, gin.H{"status": map[bool]string{true: "ok", false: "degraded"}[dbStatus == "up"], "db": dbStatus})
}
