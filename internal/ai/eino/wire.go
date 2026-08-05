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

// Runtime 持有多厂商 Client 池；Eino ChatModel 仅在默认厂商为 deepseek 且有 key 时装配。
type Runtime struct {
	ChatModel model.ToolCallingChatModel
	Pool      *llm.Pool
	Client    *llm.Client // 默认厂商客户端（兼容旧代码）
	Provider  string
	Model     string
}

func NewRuntime(cfg *config.Config) (*Runtime, error) {
	pool := llm.NewPool(cfg)
	client := pool.DefaultClient()
	rt := &Runtime{
		Pool:     pool,
		Client:   client,
		Provider: cfg.LLM.DefaultProvider,
		Model:    cfg.LLM.DefaultModel,
	}

	// Eino DeepSeek 仅默认厂商为 deepseek 时启用
	name, pc, err := cfg.ResolveLLM(cfg.LLM.DefaultProvider)
	if err != nil || name != "deepseek" {
		return rt, nil
	}
	timeout := time.Duration(cfg.LLM.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 2 * time.Minute
	}
	cm, err := deepseek.NewChatModel(context.Background(), &deepseek.ChatModelConfig{
		APIKey:  pc.APIKey,
		BaseURL: pc.BaseURL,
		Model:   pc.DefaultModel,
		Timeout: timeout,
	})
	if err != nil {
		return nil, fmt.Errorf("eino deepseek chat model: %w", err)
	}
	rt.ChatModel = cm
	return rt, nil
}
