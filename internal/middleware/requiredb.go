package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// RequireDB 最小化部署（无数据库）时拒绝依赖库的接口。
func RequireDB(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		if db == nil {
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{
				"error": gin.H{
					"code":    "db_unavailable",
					"message": "数据库未启用（最小化部署）；文件分析请用 /api/v1/chat/analyze",
				},
			})
			return
		}
		c.Next()
	}
}
