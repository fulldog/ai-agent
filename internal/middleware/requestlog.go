package middleware

import (
	"bytes"
	"encoding/json"
	"io"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/webapp/go-app/ai-agent/internal/config"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type bodyWriter struct {
	gin.ResponseWriter
	buf        *bytes.Buffer
	held       *bytes.Buffer // 非流式：暂存完整响应，结束后注入 elapsed_ms
	stream     bool
	sseCount   int
	previewMax int
}

func (w *bodyWriter) Write(b []byte) (int, error) {
	if w.stream {
		w.sseCount++
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
	if w.held != nil {
		_, _ = w.held.Write(b)
	}
	if w.buf.Len() < w.previewMax {
		remain := w.previewMax - w.buf.Len()
		if len(b) > remain {
			w.buf.Write(b[:remain])
		} else {
			w.buf.Write(b)
		}
	}
	return len(b), nil
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

		start := time.Now()
		c.Set(string(CtxStartTime), start)

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
			held:           &bytes.Buffer{},
			stream:         stream,
			previewMax:     cfg.Log.BodyPreviewMax,
		}
		c.Writer = bw

		c.Next()
		latency := time.Since(start)
		elapsedMs := latency.Milliseconds()
		c.Writer.Header().Set("X-Elapsed-Ms", itoa64(elapsedMs))

		if !stream && bw.held != nil && bw.held.Len() > 0 {
			out := injectElapsedMS(bw.held.Bytes(), elapsedMs)
			c.Writer.Header().Del("Content-Length")
			_, _ = bw.ResponseWriter.Write(out)
		}

		apiKeyID, _ := c.Get(string(CtxAPIKeyID))
		pathTemplate := c.FullPath()
		if pathTemplate == "" {
			pathTemplate = c.Request.URL.Path
		}

		bodyStr := ""
		if cfg.RequestLog.PersistBody {
			bodyStr = truncate(string(reqBody), cfg.Log.BodyPreviewMax)
		}

		log.Info("http_access",
			zap.String("request_id", reqID),
			zap.String("method", c.Request.Method),
			zap.String("path", c.Request.URL.Path),
			zap.String("path_template", pathTemplate),
			zap.String("body", bodyStr),
			zap.Int("status", c.Writer.Status()),
			zap.Duration("latency", latency),
			zap.Int64("elapsed_ms", elapsedMs),
			zap.Any("api_key_id", apiKeyID),
			zap.Bool("stream", stream),
			zap.Int("bytes_out_preview", bw.buf.Len()),
		)
	}
}

// injectElapsedMS 向 JSON 对象响应注入 elapsed_ms；非对象则原样返回。
func injectElapsedMS(body []byte, ms int64) []byte {
	var v any
	if err := json.Unmarshal(body, &v); err != nil {
		return body
	}
	m, ok := v.(map[string]any)
	if !ok {
		return body
	}
	m["elapsed_ms"] = ms
	out, err := json.Marshal(m)
	if err != nil {
		return body
	}
	return out
}

func itoa64(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
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
