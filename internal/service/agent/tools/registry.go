package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/webapp/go-app/ai-agent/internal/service/dbconn"
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

func builtin(c *dbconn.Client) []Tool {
	list := []Tool{
		KnowledgeSearch{},
		CurrentTime{},
		Calculator{},
	}
	if c != nil {
		list = append(list, DBConn{Client: c})
	}
	return list
}

// Default 内置工具集（不含业务库）。
func Default() *Registry {
	return NewRegistry(builtin(nil)...)
}

// WithDBConn 内置工具并注册 dbconn。
func WithDBConn(c *dbconn.Client) *Registry {
	return NewRegistry(builtin(c)...)
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
