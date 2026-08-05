package llmog

import (
	"github.com/google/uuid"
	"github.com/webapp/go-app/ai-agent/internal/model"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// Payload 完整 prompt / 回复（仅写文本日志，不入库）。
type Payload struct {
	Messages     any    // 通常为 []llm.Message
	Response     string // 模型回复正文
	ToolCalls    any    // 可选 tool_calls
	FinishReason string
}

// Save 写入 llm_call_logs 摘要行，并在文本日志留下完整 messages/response。
// 关联键：文本日志字段 llm_call_id == 表 id；另含 request_id。
func Save(db *gorm.DB, log *zap.Logger, row *model.LLMCallLog, payload *Payload) {
	if row == nil {
		return
	}
	if row.ID == uuid.Nil {
		row.ID = uuid.New()
	}
	if db != nil {
		_ = db.Create(row).Error
	}
	writeTextLog(log, row, payload)
}

func writeTextLog(log *zap.Logger, row *model.LLMCallLog, payload *Payload) {
	if log == nil {
		return
	}
	fields := []zap.Field{
		zap.String("event", "llm_call"),
		zap.String("llm_call_id", row.ID.String()),
		zap.String("request_id", row.RequestID),
		zap.String("provider", row.Provider),
		zap.String("model", row.Model),
		zap.Bool("stream", row.Stream),
		zap.String("status", row.Status),
		zap.Int("prompt_tokens", row.PromptTokens),
		zap.Int("completion_tokens", row.CompletionTokens),
		zap.Int64("latency_ms", row.LatencyMs),
		zap.String("request_summary", row.RequestSummary),
	}
	if row.ConversationID != nil {
		fields = append(fields, zap.String("conversation_id", row.ConversationID.String()))
	}
	if row.AgentRunID != nil {
		fields = append(fields, zap.String("agent_run_id", row.AgentRunID.String()))
	}
	if row.ErrorMessage != "" {
		fields = append(fields, zap.String("error_message", row.ErrorMessage))
	}
	if payload != nil {
		if payload.Messages != nil {
			fields = append(fields, zap.Any("messages", payload.Messages))
		}
		fields = append(fields, zap.String("response", payload.Response))
		if payload.ToolCalls != nil {
			fields = append(fields, zap.Any("tool_calls", payload.ToolCalls))
		}
		if payload.FinishReason != "" {
			fields = append(fields, zap.String("finish_reason", payload.FinishReason))
		}
	}
	log.Info("llm_call", fields...)
}

func PtrUUID(id uuid.UUID) *uuid.UUID {
	if id == uuid.Nil {
		return nil
	}
	return &id
}
