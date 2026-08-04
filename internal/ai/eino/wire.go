package eino

import (
	"context"
	"fmt"
	"time"

	"github.com/cloudwego/eino-ext/components/model/deepseek"
	"github.com/cloudwego/eino/components/model"
	"github.com/webapp/go-app/ai-agent/internal/config"
	"github.com/webapp/go-app/ai-agent/internal/service/llm"
)

// Runtime holds DeepSeek model wiring for Chat/Agent.
// - ChatModel: CloudWeGo Eino DeepSeek ChatModel (M4); may be nil if API key unset
// - Client: OpenAI-compatible HTTP client used by service layer
type Runtime struct {
	ChatModel model.ToolCallingChatModel
	Client    *llm.Client
	Provider  string
	Model     string
}

// NewRuntime builds Eino DeepSeek ChatModel (when key present) + service LLM client.
func NewRuntime(cfg *config.Config) (*Runtime, error) {
	client := llm.NewClient(cfg.LLM.BaseURL, cfg.LLM.APIKey, cfg.LLM.TimeoutSeconds)
	rt := &Runtime{
		Client:   client,
		Provider: "deepseek",
		Model:    cfg.LLM.DefaultModel,
	}
	if cfg.LLM.APIKey == "" {
		// Allow bootstrapping DB/metrics without LLM credentials.
		return rt, nil
	}
	timeout := time.Duration(cfg.LLM.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 2 * time.Minute
	}
	cm, err := deepseek.NewChatModel(context.Background(), &deepseek.ChatModelConfig{
		APIKey:  cfg.LLM.APIKey,
		BaseURL: cfg.LLM.BaseURL,
		Model:   cfg.LLM.DefaultModel,
		Timeout: timeout,
	})
	if err != nil {
		return nil, fmt.Errorf("eino deepseek chat model: %w", err)
	}
	rt.ChatModel = cm
	return rt, nil
}
