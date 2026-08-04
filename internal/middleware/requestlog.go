package middleware

import (
	"bytes"
	"io"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/webapp/go-app/ai-agent/internal/config"
	"github.com/webapp/go-app/ai-agent/internal/model"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type bodyWriter struct {
	gin.ResponseWriter
	buf        *bytes.Buffer
	stream     bool
	sseCount   int
	previewMax int
}

func (w *bodyWriter) Write(b []byte) (int, error) {
	if w.stream {
		w.sseCount++
	}
	if w.buf.Len() < w.previewMax {
		remain := w.previewMax - w.buf.Len()
		if len(b) > remain {
			w.buf.Write(b[:remain])
		} else {
			w.buf.Write(b)
		}
	}
	return w.ResponseWriter.Write(b)
}

func (w *bodyWriter) WriteString(s string) (int, error) {
	return w.Write([]byte(s))
}

func RequestLog(cfg *config.Config, db *gorm.DB, log *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		reqID := c.GetHeader("X-Request-ID")
		if reqID == "" {
			reqID = uuid.NewString()
		}
		c.Set(string(CtxRequestID), reqID)
		c.Writer.Header().Set("X-Request-ID", reqID)

		var reqBody []byte
		if c.Request.Body != nil {
			reqBody, _ = io.ReadAll(c.Request.Body)
			c.Request.Body = io.NopCloser(bytes.NewReader(reqBody))
		}

		stream := c.GetHeader("Accept") == "text/event-stream" ||
			c.FullPath() == "/api/v1/chat/completions/stream" ||
			c.FullPath() == "/api/v1/agent/runs/stream" ||
			endsWith(c.Request.URL.Path, "/stream")

		bw := &bodyWriter{
			ResponseWriter: c.Writer,
			buf:            &bytes.Buffer{},
			stream:         stream,
			previewMax:     cfg.Log.BodyPreviewMax,
		}
		c.Writer = bw

		start := time.Now()
		c.Next()
		latency := time.Since(start)

		apiKeyID, _ := c.Get(string(CtxAPIKeyID))
		pathTemplate := c.FullPath()
		if pathTemplate == "" {
			pathTemplate = c.Request.URL.Path
		}

		bodyStr := ""
		if cfg.RequestLog.PersistBody {
			bodyStr = truncate(string(reqBody), cfg.Log.BodyPreviewMax)
		}

		log.Info("http_request",
			zap.String("request_id", reqID),
			zap.String("method", c.Request.Method),
			zap.String("path", c.Request.URL.Path),
			zap.String("path_template", pathTemplate),
			zap.Int("status", c.Writer.Status()),
			zap.Duration("latency", latency),
			zap.Any("api_key_id", apiKeyID),
			zap.Bool("stream", stream),
			zap.Int("bytes_out_preview", bw.buf.Len()),
		)

		if !cfg.RequestLog.Enabled || db == nil {
			return
		}
		keyID, _ := apiKeyID.(string)
		row := model.RequestLog{
			RequestID:       reqID,
			APIKeyID:        keyID,
			Method:          c.Request.Method,
			Path:            c.Request.URL.Path,
			PathTemplate:    pathTemplate,
			Status:          c.Writer.Status(),
			LatencyMs:       latency.Milliseconds(),
			RequestBody:     bodyStr,
			ResponsePreview: truncate(bw.buf.String(), cfg.Log.BodyPreviewMax),
			Stream:          stream,
			SSEEventCount:   bw.sseCount,
		}
		if err := db.Create(&row).Error; err != nil {
			log.Warn("persist request_log failed", zap.Error(err), zap.String("request_id", reqID))
		}
	}
}

func truncate(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	return s[:max]
}

func endsWith(s, suffix string) bool {
	return len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix
}
