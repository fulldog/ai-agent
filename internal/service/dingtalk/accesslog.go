package dingtalk

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/webapp/go-app/ai-agent/internal/model"
	"github.com/webapp/go-app/ai-agent/internal/service/llm"
	"github.com/webapp/go-app/ai-agent/internal/service/rag"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

const (
	accessMethod = "STREAM"
	accessPath   = "/dingtalk/bot/messages"

	stepReceive     = 1
	stepRAG         = 2
	stepLLMRequest  = 3
	stepResult      = 4
	eventReceive    = "dingtalk.receive"
	eventRAG        = "dingtalk.rag"
	eventLLMRequest = "dingtalk.llm_request"
	eventAgent      = "dingtalk.agent"
	eventResult     = "dingtalk.result"
)

type msgTrace struct {
	start          time.Time
	data           *botCallback
	requestID      string
	uid            string
	query          string
	ragQuery       string
	forceOnline    bool
	enableSearch   bool
	hitCount       int
	hitScores      []float64
	corpusID       *uuid.UUID
	conversationID *uuid.UUID
	outcome        string
	reply          string
	status         int
	errMsg         string
	steps          []pipelineStep
}

type pipelineStep struct {
	Step   int    `json:"step"`
	Event  string `json:"event"`
	At     string `json:"at"`
	Detail any    `json:"detail,omitempty"`
}

type pipelinePayload struct {
	Channel      string         `json:"channel"`
	RequestID    string         `json:"request_id"`
	Query        string         `json:"query,omitempty"`
	RAGQuery     string         `json:"rag_query,omitempty"`
	ForceOnline  bool           `json:"force_online,omitempty"`
	EnableSearch bool           `json:"enable_search,omitempty"`
	Outcome      string         `json:"outcome,omitempty"`
	Steps        []pipelineStep `json:"steps"`
}

type ragHitLog struct {
	ChunkID  string  `json:"chunk_id"`
	CorpusID string  `json:"corpus_id,omitempty"`
	Score    float64 `json:"score"`
	Content  string  `json:"content"`
}

type llmTurnLog struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func newMsgTrace(data *botCallback) *msgTrace {
	reqID := ""
	if data != nil {
		reqID = data.MsgID
	}
	if reqID == "" {
		reqID = uuid.NewString()
	}
	return &msgTrace{
		start:     time.Now(),
		data:      data,
		requestID: reqID,
		status:    200,
		steps:     []pipelineStep{},
	}
}

func (b *Bot) stepLog(requestID string, step int, event string, fields ...zap.Field) {
	if b == nil || b.log == nil {
		return
	}
	all := make([]zap.Field, 0, 4+len(fields))
	all = append(all,
		zap.String("request_id", requestID),
		zap.Int("step", step),
		zap.String("event", event),
		zap.String("channel", channelDingTalk),
	)
	all = append(all, fields...)
	b.log.Info(event, all...)
	if b.access != nil && b.access != b.log {
		b.access.Info(event, all...)
	}
}

func (b *Bot) recordStep(tr *msgTrace, step int, event string, detail any, fields ...zap.Field) {
	if tr == nil {
		return
	}
	tr.steps = append(tr.steps, pipelineStep{
		Step:   step,
		Event:  event,
		At:     time.Now().UTC().Format(time.RFC3339Nano),
		Detail: detail,
	})
	b.stepLog(tr.requestID, step, event, fields...)
	b.upsertRequestLog(tr)
}

func (b *Bot) emitMsgLog(tr *msgTrace) {
	if tr == nil || tr.data == nil {
		return
	}
	if tr.outcome == "" {
		if tr.status >= 500 {
			tr.outcome = "error"
		} else if tr.status >= 400 {
			tr.outcome = "rejected"
		} else {
			tr.outcome = "ok"
		}
	}
	elapsedMs := time.Since(tr.start).Milliseconds()
	detail := map[string]any{
		"outcome":         tr.outcome,
		"status":          tr.status,
		"elapsed_ms":      elapsedMs,
		"rag_hits":        tr.hitCount,
		"enable_search":   tr.enableSearch,
		"conversation_id": uuidString(tr.conversationID),
		"reply":           previewText(tr.reply, b.previewMax),
	}
	fields := []zap.Field{
		zap.String("uid", tr.uid),
		zap.String("outcome", tr.outcome),
		zap.Int("status", tr.status),
		zap.Int64("elapsed_ms", elapsedMs),
		zap.Int("rag_hits", tr.hitCount),
		zap.Bool("enable_search", tr.enableSearch),
		zap.String("conversation_id", uuidString(tr.conversationID)),
		zap.String("reply", previewText(tr.reply, b.previewMax)),
	}
	if tr.errMsg != "" {
		detail["error_message"] = tr.errMsg
		fields = append(fields, zap.String("error_message", tr.errMsg))
	}
	b.recordStep(tr, stepResult, eventResult, detail, fields...)
}

func pipelineBody(tr *msgTrace) string {
	if tr == nil {
		return "{}"
	}
	steps := tr.steps
	if steps == nil {
		steps = []pipelineStep{}
	}
	raw, err := json.Marshal(pipelinePayload{
		Channel:      channelDingTalk,
		RequestID:    tr.requestID,
		Query:        tr.query,
		RAGQuery:     tr.ragQuery,
		ForceOnline:  tr.forceOnline,
		EnableSearch: tr.enableSearch,
		Outcome:      tr.outcome,
		Steps:        steps,
	})
	if err != nil {
		return "{}"
	}
	return string(raw)
}

func (b *Bot) upsertRequestLog(tr *msgTrace) {
	if b == nil || b.db == nil || tr == nil || tr.requestID == "" {
		return
	}
	elapsedMs := time.Since(tr.start).Milliseconds()
	body := pipelineBody(tr)
	reply := previewText(tr.reply, b.previewMax)
	var existing model.RequestLog
	err := b.db.Where("request_id = ?", tr.requestID).First(&existing).Error
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			b.log.Warn("load dingtalk request_log", zap.Error(err), zap.String("request_id", tr.requestID))
			return
		}
		row := model.RequestLog{
			RequestID:       tr.requestID,
			UID:             tr.uid,
			Method:          accessMethod,
			Path:            accessPath,
			PathTemplate:    accessPath,
			Status:          tr.status,
			LatencyMs:       elapsedMs,
			RequestBody:     body,
			ResponsePreview: reply,
			Stream:          true,
			SSEEventCount:   len(tr.steps),
			ConversationID:  tr.conversationID,
			ErrorMessage:    tr.errMsg,
		}
		if cerr := b.db.Create(&row).Error; cerr != nil {
			b.log.Warn("persist dingtalk request_log", zap.Error(cerr), zap.String("request_id", tr.requestID))
		}
		return
	}
	updates := map[string]any{
		"uid":              tr.uid,
		"status":           tr.status,
		"latency_ms":       elapsedMs,
		"request_body":     body,
		"response_preview": reply,
		"sse_event_count":  len(tr.steps),
		"error_message":    tr.errMsg,
	}
	if tr.conversationID != nil {
		updates["conversation_id"] = *tr.conversationID
	}
	if err := b.db.Model(&existing).Updates(updates).Error; err != nil {
		b.log.Warn("update dingtalk request_log", zap.Error(err), zap.String("request_id", tr.requestID))
	}
}

func hitScores(hits []rag.Hit) []float64 {
	out := make([]float64, 0, len(hits))
	for _, h := range hits {
		out = append(out, h.Score)
	}
	return out
}

func ragHitLogs(hits []rag.Hit, max int) []ragHitLog {
	out := make([]ragHitLog, 0, len(hits))
	for _, h := range hits {
		out = append(out, ragHitLog{
			ChunkID:  h.ChunkID.String(),
			CorpusID: h.CorpusID.String(),
			Score:    h.Score,
			Content:  previewText(h.Content, max),
		})
	}
	return out
}

func llmTurnLogs(messages []llm.Message, max int) []llmTurnLog {
	out := make([]llmTurnLog, 0, len(messages))
	for _, m := range messages {
		out = append(out, llmTurnLog{
			Role:    m.Role,
			Content: previewText(m.Content, max),
		})
	}
	return out
}

func uuidString(id *uuid.UUID) string {
	if id == nil {
		return ""
	}
	return id.String()
}

func previewText(s string, max int) string {
	if max <= 0 {
		return s
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "..."
}
