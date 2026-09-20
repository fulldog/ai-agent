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
	order  []string
}

func NewRegistry(list ...Tool) *Registry {
	r := &Registry{byName: make(map[string]Tool, len(list)), order: make([]string, 0, len(list))}
	for _, t := range list {
		name := t.Name()
		if _, ok := r.byName[name]; ok {
			continue
		}
		r.byName[name] = t
		r.order = append(r.order, name)
	}
	return r
}

func builtin(c *dbconn.Client) []Tool {
	list := []Tool{
		KnowledgeSearch{},
		//CurrentTime{},
		//Calculator{},
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

type Info struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

func (r *Registry) List() []Info {
	if r == nil {
		return nil
	}
	out := make([]Info, 0, len(r.order))
	for _, name := range r.order {
		t := r.byName[name]
		out = append(out, Info{Name: name, Description: t.Spec().Function.Description})
	}
	return out
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
