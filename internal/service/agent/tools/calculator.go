package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/webapp/go-app/ai-agent/internal/service/llm"
)

// Calculator 简单四则运算。
type Calculator struct{}

func (Calculator) Name() string { return "calculator" }

func (Calculator) Spec() llm.ToolSpec {
	return llm.ToolSpec{
		Type: "function",
		Function: llm.ToolSpecFunc{
			Name:        "calculator",
			Description: "计算简单四则运算：a op b。op 为 +、-、*、/ 之一",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"a":  map[string]any{"type": "number", "description": "左操作数"},
					"b":  map[string]any{"type": "number", "description": "右操作数"},
					"op": map[string]any{"type": "string", "enum": []string{"+", "-", "*", "/"}, "description": "运算符"},
				},
				"required": []string{"a", "b", "op"},
			},
		},
	}
}

func (Calculator) Exec(_ context.Context, args json.RawMessage, _ *Env) (string, error) {
	var p struct {
		A  float64 `json:"a"`
		B  float64 `json:"b"`
		Op string  `json:"op"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("参数无效: %w", err)
	}
	var v float64
	switch p.Op {
	case "+":
		v = p.A + p.B
	case "-":
		v = p.A - p.B
	case "*":
		v = p.A * p.B
	case "/":
		if p.B == 0 {
			return "", fmt.Errorf("除数不能为 0")
		}
		v = p.A / p.B
	default:
		return "", fmt.Errorf("不支持的运算符: %s", p.Op)
	}
	return strconv.FormatFloat(v, 'f', -1, 64), nil
}
