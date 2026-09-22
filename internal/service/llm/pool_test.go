package llm

import (
	"testing"

	"github.com/webapp/go-app/ai-agent/internal/config"
)

func TestPoolNewClientThinkingFlag(t *testing.T) {
	t.Parallel()
	off := false
	on := true
	cfg := &config.Config{}
	cfg.LLM.EnableThinking = &off
	cfg.LLM.TimeoutSeconds = 10
	p := NewPool(cfg)

	qwen := p.newClient("qwen", config.LLMProviderConfig{BaseURL: "http://q", APIKey: "k"})
	if !qwen.sendThinking || qwen.enableThinking {
		t.Fatalf("qwen off: send=%v think=%v", qwen.sendThinking, qwen.enableThinking)
	}
	ds := p.newClient("deepseek", config.LLMProviderConfig{BaseURL: "http://d", APIKey: "k"})
	if ds.sendThinking {
		t.Fatal("deepseek should omit enable_thinking")
	}

	cfg.LLM.EnableThinking = &on
	qwenOn := p.newClient("qwen", config.LLMProviderConfig{BaseURL: "http://q", APIKey: "k"})
	if !qwenOn.sendThinking || !qwenOn.enableThinking {
		t.Fatalf("qwen on: send=%v think=%v", qwenOn.sendThinking, qwenOn.enableThinking)
	}
}
