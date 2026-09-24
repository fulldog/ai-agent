package middleware

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"time"
	"unicode/utf8"

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
	held       *bytes.Buffer // 非流式：暂存完整响应，结束后注入 elapsed_ms
	stream     bool
	sseCount   int
	previewMax int
}

func (w *bodyWriter) Write(b []byte) (int, error) {
	if w.stream {
		w.sseCount++
		w.appendPreview(b)
		return w.ResponseWriter.Write(b)
	}
	if w.held != nil {
		_, _ = w.held.Write(b)
	}
	w.appendPreview(b)
	return len(b), nil
}

// appendPreview 写入响应预览，按字节上限截断且不切断 UTF-8 多字节序列。
func (w *bodyWriter) appendPreview(b []byte) {
	if w == nil || w.buf == nil || w.previewMax <= 0 || w.buf.Len() >= w.previewMax {
		return
	}
	remain := w.previewMax - w.buf.Len()
	if len(b) > remain {
		b = b[:remain]
	}
	b = trimIncompleteUTF8(b)
	if len(b) > 0 {
		_, _ = w.buf.Write(b)
	}
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
			zap.String("uid", UIDFromContext(c)),
			zap.String("body", bodyStr),
			zap.Int("status", c.Writer.Status()),
			zap.Duration("latency", latency),
			zap.Int64("elapsed_ms", elapsedMs),
			zap.Any("api_key_id", apiKeyID),
			zap.Bool("stream", stream),
			zap.Int("bytes_out_preview", bw.buf.Len()),
		)

		if db != nil && strings.HasPrefix(pathTemplate, "/api/v1") && !strings.HasPrefix(pathTemplate, "/api/v1/logs") {
			apiKeyIDStr, _ := apiKeyID.(string)
			row := model.RequestLog{
				RequestID:       reqID,
				APIKeyID:        apiKeyIDStr,
				UID:             UIDFromContext(c),
				Method:          c.Request.Method,
				Path:            c.Request.URL.Path,
				PathTemplate:    pathTemplate,
				Status:          c.Writer.Status(),
				LatencyMs:       elapsedMs,
				RequestBody:     bodyStr,
				ResponsePreview: truncate(bw.buf.String(), cfg.Log.BodyPreviewMax),
				Stream:          stream,
				SSEEventCount:   bw.sseCount,
				ConversationID:  parseBodyUUID(reqBody, "conversation_id"),
				AgentRunID:      parseJSONUUID(bw.buf.Bytes(), "run_id"),
			}
			go func(r model.RequestLog) {
				_ = db.Create(&r).Error
			}(row)
		}
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

// truncate 按字节上限截断，并保证结果为合法 UTF-8（避免 PostgreSQL SQLSTATE 22021）。
func truncate(s string, max int) string {
	if max > 0 && len(s) > max {
		s = string(trimIncompleteUTF8([]byte(s[:max])))
	}
	return strings.ToValidUTF8(s, "\uFFFD")
}

func trimIncompleteUTF8(b []byte) []byte {
	for len(b) > 0 {
		r, size := utf8.DecodeLastRune(b)
		if r != utf8.RuneError || size > 1 {
			break
		}
		// size==1 且 RuneError：末尾是不完整多字节序列或孤立非法字节。
		b = b[:len(b)-1]
	}
	return b
}

func endsWith(s, suffix string) bool {
	return len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix
}

func parseBodyUUID(body []byte, key string) *uuid.UUID {
	return parseJSONUUID(body, key)
}

func parseJSONUUID(raw []byte, key string) *uuid.UUID {
	if len(raw) == 0 {
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil
	}
	v, ok := m[key]
	if !ok || v == nil {
		return nil
	}
	s, ok := v.(string)
	if !ok || strings.TrimSpace(s) == "" {
		return nil
	}
	id, err := uuid.Parse(s)
	if err != nil {
		return nil
	}
	return &id
}
