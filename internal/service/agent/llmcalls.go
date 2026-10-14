package agent

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/webapp/go-app/ai-agent/internal/model"
	"github.com/webapp/go-app/ai-agent/internal/service/llmog"
)

// LLMCallDetail 一次上游调用。报文从 logs/llm-*.log 按 id 回填，不入库。
type LLMCallDetail struct {
	ID               uuid.UUID       `json:"id"`
	Provider         string          `json:"provider"`
	Model            string          `json:"model"`
	Stream           bool            `json:"stream"`
	Status           string          `json:"status"`
	PromptTokens     int             `json:"prompt_tokens"`
	CompletionTokens int             `json:"completion_tokens"`
	LatencyMs        int64           `json:"latency_ms"`
	RequestSummary   string          `json:"request_summary,omitempty"`
	ErrorMessage     string          `json:"error_message,omitempty"`
	CreatedAt        time.Time       `json:"created_at"`
	Messages         json.RawMessage `json:"messages,omitempty"`
	Response         string          `json:"response,omitempty"`
	ToolCalls        json.RawMessage `json:"tool_calls,omitempty"`
	FinishReason     string          `json:"finish_reason,omitempty"`
	PayloadFound     bool            `json:"payload_found"`
}

// LLMCallsForRun 列出某次运行的上游调用，并附上文本日志里的请求报文。
func (s *Service) LLMCallsForRun(runID uuid.UUID) ([]LLMCallDetail, error) {
	if s == nil || s.db == nil || runID == uuid.Nil {
		return []LLMCallDetail{}, nil
	}
	var rows []model.LLMCallLog
	if err := s.db.Where("agent_run_id = ?", runID).Order("created_at asc, id asc").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list llm calls: %w", err)
	}
	hits := llmog.LoadPayloads(s.logDir(), rows)
	out := make([]LLMCallDetail, 0, len(rows))
	for _, row := range rows {
		item := LLMCallDetail{
			ID:               row.ID,
			Provider:         row.Provider,
			Model:            row.Model,
			Stream:           row.Stream,
			Status:           row.Status,
			PromptTokens:     row.PromptTokens,
			CompletionTokens: row.CompletionTokens,
			LatencyMs:        row.LatencyMs,
			RequestSummary:   row.RequestSummary,
			ErrorMessage:     row.ErrorMessage,
			CreatedAt:        row.CreatedAt,
		}
		if hit, ok := hits[row.ID]; ok {
			item.Messages = hit.Messages
			item.Response = hit.Response
			item.ToolCalls = hit.ToolCalls
			item.FinishReason = hit.FinishReason
			item.PayloadFound = true
		}
		out = append(out, item)
	}
	return out, nil
}

func (s *Service) logDir() string {
	if s != nil && s.cfg != nil {
		if dir := strings.TrimSpace(s.cfg.Log.Dir); dir != "" {
			return dir
		}
	}
	return "logs"
}
