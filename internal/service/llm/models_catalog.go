package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/webapp/go-app/ai-agent/internal/config"
)

const availableModelsTTL = 5 * time.Minute

// ModelGroup 一个已配置厂商及其可选用的模型 id。
type ModelGroup struct {
	Provider     string   `json:"provider"`
	Models       []string `json:"models"`
	DefaultModel string   `json:"default_model"`
	Error        string   `json:"error,omitempty"`
}

// ListModels 请求 OpenAI 兼容的 GET {base}/models，返回去重后的模型 id。
// 不跟随重定向，避免把密钥带到别的主机。
func (c *Client) ListModels(ctx context.Context) ([]string, error) {
	if c == nil || strings.TrimSpace(c.baseURL) == "" {
		return nil, fmt.Errorf("llm client not configured")
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/models", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	client := &http.Client{
		Timeout: 8 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("list models: status %d", resp.StatusCode)
	}
	return parseModelIDs(raw)
}

func parseModelIDs(raw []byte) ([]string, error) {
	var parsed struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("parse model list: %w", err)
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(parsed.Data))
	for _, item := range parsed.Data {
		id := strings.TrimSpace(item.ID)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	sort.Strings(out)
	return out, nil
}

// AvailableModels 按厂商分组返回可用模型。结果缓存 5 分钟。
// 某一家上游失败时该组只保留 default_model，并带上错误说明。
func (p *Pool) AvailableModels(ctx context.Context) []ModelGroup {
	if p == nil || p.cfg == nil {
		return []ModelGroup{}
	}
	p.availMu.Lock()
	if p.available != nil && time.Since(p.availableAt) < availableModelsTTL {
		out := cloneModelGroups(p.available)
		p.availMu.Unlock()
		return out
	}
	p.availMu.Unlock()

	groups := p.fetchAvailableModels(ctx)
	if ctx.Err() == nil {
		p.availMu.Lock()
		p.available = groups
		p.availableAt = time.Now()
		p.availMu.Unlock()
	}
	return cloneModelGroups(groups)
}

func (p *Pool) fetchAvailableModels(ctx context.Context) []ModelGroup {
	names := make([]string, 0, len(p.cfg.LLM.Providers))
	for name, pc := range p.cfg.LLM.Providers {
		if !pc.ProviderEnabled() || strings.TrimSpace(pc.APIKey) == "" || strings.TrimSpace(pc.BaseURL) == "" {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]ModelGroup, len(names))
	var wg sync.WaitGroup
	for i, name := range names {
		pc := p.cfg.LLM.Providers[name]
		wg.Add(1)
		go func(i int, name string, pc config.LLMProviderConfig) {
			defer wg.Done()
			out[i] = p.loadProviderModels(ctx, name, pc)
		}(i, name, pc)
	}
	wg.Wait()
	return out
}

func (p *Pool) loadProviderModels(ctx context.Context, name string, pc config.LLMProviderConfig) ModelGroup {
	group := ModelGroup{
		Provider:     name,
		Models:       []string{},
		DefaultModel: strings.TrimSpace(pc.DefaultModel),
	}
	client := p.newClient(name, pc)
	ids, err := client.ListModels(ctx)
	if err != nil {
		group.Error = "上游模型列表暂时不可用"
		if group.DefaultModel != "" {
			group.Models = []string{group.DefaultModel}
		}
		return group
	}
	group.Models = mergeModelIDs(ids, group.DefaultModel)
	return group
}

func mergeModelIDs(ids []string, defaultModel string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(ids)+1)
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	if defaultModel != "" {
		if _, ok := seen[defaultModel]; !ok {
			out = append(out, defaultModel)
		}
	}
	sort.Strings(out)
	return out
}

func cloneModelGroups(in []ModelGroup) []ModelGroup {
	if in == nil {
		return []ModelGroup{}
	}
	out := make([]ModelGroup, len(in))
	for i, g := range in {
		out[i] = g
		out[i].Models = append([]string{}, g.Models...)
	}
	return out
}
