package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/webapp/go-app/ai-agent/internal/config"
)

func APIKey(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		_ = cfg
		// 鉴权已跳过：控制台不填写 X-API-Key，请求按管理员处理。
		c.Set(string(CtxIsAdmin), true)
		c.Next()
	}
}

func AdminAPIKey(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		_ = cfg
		c.Set(string(CtxIsAdmin), true)
		c.Next()
	}
}
