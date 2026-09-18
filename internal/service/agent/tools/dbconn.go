package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/webapp/go-app/ai-agent/internal/service/dbconn"
	"github.com/webapp/go-app/ai-agent/internal/service/llm"
)

// DBConn 查询独立 MySQL 业务库的数据字典或只读 SELECT。
type DBConn struct {
	Client *dbconn.Client
}

func (DBConn) Name() string { return "dbconn" }

func (DBConn) Spec() llm.ToolSpec {
	return llm.ToolSpec{
		Type: "function",
		Function: llm.ToolSpecFunc{
			Name:        "dbconn",
			Description: "查询独立业务 MySQL：schema 按表名/注释检索数据字典；query 执行只读 SELECT。仅在需要业务数据时调用；先对照语料口径与表/列注释再写 SQL。不要编造表名。",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"action": map[string]any{
						"type":        "string",
						"enum":        []string{"schema", "query"},
						"description": "schema=查表/列注释；query=执行只读 SELECT",
					},
					"keyword": map[string]any{
						"type":        "string",
						"description": "action=schema 且未指定 table 时，按表名或表注释过滤",
					},
					"table": map[string]any{
						"type":        "string",
						"description": "action=schema 时指定表名以查看列注释，如 vendor 或 schema.table",
					},
					"sql": map[string]any{
						"type":        "string",
						"description": "action=query 时的单条 SELECT / WITH ... SELECT",
					},
				},
				"required": []string{"action"},
			},
		},
	}
}

func (t DBConn) Exec(ctx context.Context, args json.RawMessage, env *Env) (string, error) {
	_ = env
	if t.Client == nil {
		return "", fmt.Errorf("业务库未配置")
	}
	var p struct {
		Action  string `json:"action"`
		Keyword string `json:"keyword"`
		Table   string `json:"table"`
		SQL     string `json:"sql"`
	}
	if len(args) > 0 {
		if err := json.Unmarshal(args, &p); err != nil {
			return "", fmt.Errorf("参数无效: %w", err)
		}
	}
	switch strings.ToLower(strings.TrimSpace(p.Action)) {
	case "schema":
		return t.Client.Schema(ctx, p.Keyword, p.Table)
	case "query":
		if strings.TrimSpace(p.SQL) == "" {
			return "", fmt.Errorf("sql 不能为空")
		}
		return t.Client.Query(ctx, p.SQL)
	case "":
		return "", fmt.Errorf("action 不能为空")
	default:
		return "", fmt.Errorf("未知 action: %s", p.Action)
	}
}
