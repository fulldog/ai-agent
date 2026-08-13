package middleware

import (
	"time"

	"github.com/gin-gonic/gin"
)

type ctxKey string

const (
	CtxRequestID ctxKey = "request_id"
	CtxAPIKey    ctxKey = "api_key"
	CtxAPIKeyID  ctxKey = "api_key_id"
	CtxIsAdmin   ctxKey = "is_admin"
	CtxUID       ctxKey = "uid" // X-User-Id
	CtxStartTime ctxKey = "start_time"
)

// ElapsedMs 自请求进入 RequestLog 起的耗时（毫秒）；未记录开始时间则返回 0。
func ElapsedMs(c *gin.Context) int64 {
	if c == nil {
		return 0
	}
	v, ok := c.Get(string(CtxStartTime))
	if !ok {
		return 0
	}
	start, ok := v.(time.Time)
	if !ok || start.IsZero() {
		return 0
	}
	return time.Since(start).Milliseconds()
}
