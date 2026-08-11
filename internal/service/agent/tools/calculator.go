package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/webapp/go-app/ai-agent/internal/service/llm"
)

// Calculator 简单四则运算（Agent 内置 Tool Calling）。
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
	return Calc(p.A, p.B, p.Op)
}

// Calc 执行 a op b，返回可读数字字符串。供内置 Tool 与 MCP Server 共用。
func Calc(a, b float64, op string) (string, error) {
	var v float64
	switch op {
	case "+":
		v = a + b
	case "-":
		v = a - b
	case "*":
		v = a * b
	case "/":
		if b == 0 {
			return "", fmt.Errorf("除数不能为 0")
		}
		v = a / b
	default:
		return "", fmt.Errorf("不支持的运算符: %s", op)
	}
	return strconv.FormatFloat(v, 'f', -1, 64), nil
}
