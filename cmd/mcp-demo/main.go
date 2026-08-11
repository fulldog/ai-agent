// Command mcp-demo 以 MCP（stdio）暴露与 Agent 内置 calculator 相同的四则运算工具。
//
// 构建:
//
//	go build -o bin/mcp-demo ./cmd/mcp-demo
//
// Cursor / Claude Desktop 等 MCP 客户端配置示例见 docs/MCP.md。
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/webapp/go-app/ai-agent/internal/service/agent/tools"
)

type calcInput struct {
	A  float64 `json:"a" jsonschema:"左操作数"`
	B  float64 `json:"b" jsonschema:"右操作数"`
	Op string  `json:"op" jsonschema:"运算符，必须是 +、-、*、/ 之一"`
}

type calcOutput struct {
	Result string `json:"result" jsonschema:"运算结果（十进制字符串）"`
}

func calculate(_ context.Context, _ *mcp.CallToolRequest, in calcInput) (*mcp.CallToolResult, calcOutput, error) {
	got, err := tools.Calc(in.A, in.B, in.Op)
	if err != nil {
		return nil, calcOutput{}, err
	}
	return nil, calcOutput{Result: got}, nil
}

func main() {
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "ai-agent-mcp",
		Version: "v1.0.0",
	}, nil)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "calculator",
		Description: "计算简单四则运算：a op b。op 为 +、-、*、/ 之一。与 ai-agent 内置 Agent 工具 calculator 语义一致。",
	}, calculate)

	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		fmt.Fprintf(os.Stderr, "ai-agent-mcp: %v\n", err)
		log.Fatal(err)
	}
}
