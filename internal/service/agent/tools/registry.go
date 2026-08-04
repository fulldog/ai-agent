package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/webapp/go-app/ai-agent/internal/service/llm"
)

// Registry 按名称注册与查找工具。
type Registry struct {
	byName map[string]Tool
}

func NewRegistry(list ...Tool) *Registry {
	r := &Registry{byName: make(map[string]Tool, len(list))}
	for _, t := range list {
		r.byName[t.Name()] = t
	}
	return r
}

// Default 内置工具集（knowledge_search / current_time / calculator）。
func Default() *Registry {
	return NewRegistry(
		KnowledgeSearch{},
		CurrentTime{},
		Calculator{},
	)
}

func (r *Registry) Specs(names []string) []llm.ToolSpec {
	out := make([]llm.ToolSpec, 0, len(names))
	for _, n := range names {
		if t, ok := r.byName[n]; ok {
			out = append(out, t.Spec())
		}
	}
	return out
}

func (r *Registry) Exec(ctx context.Context, name string, args string, env *Env) (string, error) {
	t, ok := r.byName[name]
	if !ok {
		return "", fmt.Errorf("未知工具: %s", name)
	}
	return t.Exec(ctx, json.RawMessage(args), env)
}
