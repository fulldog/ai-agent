package llm

import (
	"fmt"
	"strings"
	"sync"

	"github.com/webapp/go-app/ai-agent/internal/config"
)

// ProviderInfo 对外列出已配置厂商（不含密钥）。
type ProviderInfo struct {
	Name         string `json:"name"`
	BaseURL      string `json:"base_url"`
	DefaultModel string `json:"default_model"`
	Configured   bool   `json:"configured"` // 是否已配置 api_key
	Enabled      bool   `json:"enabled"`
}

// Pool 多厂商 OpenAI 兼容客户端池。
type Pool struct {
	mu       sync.RWMutex
	clients  map[string]*Client
	cfg      *config.Config
	timeout  int
	defaultP string
}

func NewPool(cfg *config.Config) *Pool {
	return &Pool{
		clients:  make(map[string]*Client),
		cfg:      cfg,
		timeout:  cfg.LLM.TimeoutSeconds,
		defaultP: cfg.LLM.DefaultProvider,
	}
}

// DefaultClient 返回默认厂商客户端（无 key 时仍返回 Client，调用会失败）。
func (p *Pool) DefaultClient() *Client {
	client, _, _, err := p.Resolve(p.defaultP, "")
	if err != nil {
		// 无 key 时也建一个空客户端，保持启动不崩
		name := p.defaultP
		pc := p.cfg.LLM.Providers[name]
		return NewClient(pc.BaseURL, pc.APIKey, p.timeout)
	}
	return client
}

// Resolve 按 provider + 可选 model 解析客户端。
func (p *Pool) Resolve(provider, model string) (client *Client, providerName, modelName string, err error) {
	name, pc, err := p.cfg.ResolveLLM(provider)
	if err != nil {
		return nil, "", "", err
	}
	modelName = strings.TrimSpace(model)
	if modelName == "" {
		modelName = pc.DefaultModel
	}
	if modelName == "" {
		return nil, "", "", fmt.Errorf("厂商 %s 未指定 model，且无 default_model", name)
	}

	p.mu.RLock()
	client, ok := p.clients[name]
	p.mu.RUnlock()
	if ok {
		return client, name, modelName, nil
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	if client, ok = p.clients[name]; ok {
		return client, name, modelName, nil
	}
	client = NewClient(pc.BaseURL, pc.APIKey, p.timeout)
	p.clients[name] = client
	return client, name, modelName, nil
}

// List 返回配置中的厂商列表。
func (p *Pool) List() []ProviderInfo {
	out := make([]ProviderInfo, 0, len(p.cfg.LLM.Providers))
	for name, pc := range p.cfg.LLM.Providers {
		out = append(out, ProviderInfo{
			Name:         name,
			BaseURL:      pc.BaseURL,
			DefaultModel: pc.DefaultModel,
			Configured:   strings.TrimSpace(pc.APIKey) != "",
			Enabled:      pc.ProviderEnabled(),
		})
	}
	return out
}
