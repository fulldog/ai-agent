package handler

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/webapp/go-app/ai-agent/internal/middleware"
)

func requestID(c *gin.Context) string {
	if v, ok := c.Get(string(middleware.CtxRequestID)); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func writeError(c *gin.Context, status int, code, message string) {
	c.JSON(status, gin.H{
		"error":      gin.H{"code": code, "message": message},
		"request_id": requestID(c),
	})
}

func writeSSE(c *gin.Context, event string, payload any) error {
	if event == "done" || event == "error" {
		payload = withElapsedMS(c, payload)
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if event != "" {
		if _, err := fmt.Fprintf(c.Writer, "event: %s\n", event); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(c.Writer, "data: %s\n\n", data); err != nil {
		return err
	}
	c.Writer.Flush()
	return nil
}

func withElapsedMS(c *gin.Context, payload any) any {
	ms := middleware.ElapsedMs(c)
	switch p := payload.(type) {
	case gin.H:
		p["elapsed_ms"] = ms
		return p
	case map[string]any:
		p["elapsed_ms"] = ms
		return p
	default:
		return payload
	}
}

func setupSSE(c *gin.Context) {
	c.Writer.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)
}
