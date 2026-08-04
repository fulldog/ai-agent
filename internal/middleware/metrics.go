package middleware

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/webapp/go-app/ai-agent/internal/metrics"
)

func Metrics() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		path := c.FullPath()
		if path == "" {
			path = "unknown"
		}
		metrics.ObserveHTTP(c.Request.Method, path, c.Writer.Status(), time.Since(start).Seconds())
		_ = strconv.Itoa(c.Writer.Status())
	}
}
