package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/webapp/go-app/ai-agent/internal/config"
)

func APIKey(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		key := c.GetHeader("X-API-Key")
		if key == "" || !cfg.IsAPIKey(key) {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": gin.H{"code": "unauthorized", "message": "invalid api key"},
			})
			return
		}
		c.Set(string(CtxAPIKey), key)
		c.Set(string(CtxAPIKeyID), cfg.APIKeyID(key))
		c.Set(string(CtxIsAdmin), cfg.IsAdminAPIKey(key))
		c.Next()
	}
}

func AdminAPIKey(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		key := c.GetHeader("X-API-Key")
		if key == "" || !cfg.IsAdminAPIKey(key) {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": gin.H{"code": "unauthorized", "message": "admin api key required"},
			})
			return
		}
		c.Set(string(CtxAPIKey), key)
		c.Set(string(CtxAPIKeyID), cfg.APIKeyID(key))
		c.Set(string(CtxIsAdmin), true)
		c.Next()
	}
}
