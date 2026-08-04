package tools

import (
	"context"
	"encoding/json"
	"time"

	"github.com/webapp/go-app/ai-agent/internal/service/llm"
)

// CurrentTime 返回服务器当前时间。
type CurrentTime struct{}

func (CurrentTime) Name() string { return "current_time" }

func (CurrentTime) Spec() llm.ToolSpec {
	return llm.ToolSpec{
		Type: "function",
		Function: llm.ToolSpecFunc{
			Name:        "current_time",
			Description: "获取服务器当前时间（RFC3339 格式）",
			Parameters: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
	}
}

func (CurrentTime) Exec(_ context.Context, _ json.RawMessage, _ *Env) (string, error) {
	return time.Now().Format(time.RFC3339), nil
}
