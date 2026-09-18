package middleware

import (
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/webapp/go-app/ai-agent/internal/config"
)

// CORS 允许前端跨域调用 API。默认允许所有来源；cors_origins 填具体 Origin 时改为白名单。
func CORS(cfg *config.Config) gin.HandlerFunc {
	cc := cors.Config{
		AllowMethods: []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"},
		AllowHeaders: []string{"Origin", "Content-Type", "Accept", "X-API-Key", "X-User-Id", "X-Request-ID"},
		ExposeHeaders: []string{
			"X-Request-ID",
			"X-Elapsed-Ms",
		},
		MaxAge: 12 * time.Hour,
	}
	if cfg.Server.CORSAllowAll() {
		cc.AllowAllOrigins = true
	} else {
		cc.AllowOrigins = cfg.Server.CORSAllowOrigins()
	}
	return cors.New(cc)
}
