package middleware

import (
	"strings"

	"github.com/gin-gonic/gin"
)

const HeaderUserID = "X-User-Id"

// UserID 将 X-User-Id 写入 context（可为空；强制校验由 handler.requireUID 负责）。
func UserID() gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := strings.TrimSpace(c.GetHeader(HeaderUserID))
		if uid != "" {
			c.Set(string(CtxUID), uid)
		}
		c.Next()
	}
}

// UIDFromContext 读取已写入的 uid；无则返回空串。
func UIDFromContext(c *gin.Context) string {
	if v, ok := c.Get(string(CtxUID)); ok {
		if s, ok := v.(string); ok {
			return strings.TrimSpace(s)
		}
	}
	return strings.TrimSpace(c.GetHeader(HeaderUserID))
}
