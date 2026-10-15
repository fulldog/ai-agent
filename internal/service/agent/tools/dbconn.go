package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/webapp/go-app/ai-agent/internal/service/dbconn"
	"github.com/webapp/go-app/ai-agent/internal/service/llm"
)

const queryExportHint = "导出这份结果时调用 generate_file，只传 filename 和 from_query=true。不要复制上面的 columns 和 rows；需要中文列名请写在 SELECT 别名里。"

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
			Description: "查询独立业务 MySQL。会话配置了数据字典时，全文已在系统提示的【数据字典】中，写 SQL 直接用其中的表名和列名，不必先 schema。未配置时 schema 从业务库实时读取 information_schema，可用 keyword 或 table 过滤，仅 Srm 前缀且区分大小写，如 Srm_VendorInfo。query 执行只读 SELECT。仅在需要业务数据时调用。不要编造表名或列名。query 的完整结果会留给 generate_file，导出时不要把行再抄进参数。",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"action": map[string]any{
						"type":        "string",
						"enum":        []string{"schema", "query"},
						"description": "schema=读取数据字典全文，未配置字典时查表结构；query=执行只读 SELECT",
					},
					"keyword": map[string]any{
						"type":        "string",
						"description": "未配置数据字典时，action=schema 且未指定 table，按表名、表注释、字段名或字段注释过滤。已配置数据字典时忽略",
					},
					"table": map[string]any{
						"type":        "string",
						"description": "未配置数据字典时，action=schema 指定表名查看列注释，仅允许 Srm 前缀且区分大小写，如 Srm_VendorInfo。已配置数据字典时忽略",
					},
					"sql": map[string]any{
						"type":        "string",
						"description": "action=query 时的单条 SELECT / WITH ... SELECT。表名和列名必须来自系统提示中的【数据字典】；没有该段时先 schema",
					},
				},
				"required": []string{"action"},
			},
		},
	}
}

func (t DBConn) Exec(ctx context.Context, args json.RawMessage, env *Env) (string, error) {
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
		corpus := ""
		if env != nil {
			corpus = env.Dictionary
		}
		return t.Client.Schema(ctx, p.Keyword, p.Table, corpus)
	case "query":
		if strings.TrimSpace(p.SQL) == "" {
			return "", fmt.Errorf("sql 不能为空")
		}
		text, table, err := t.Client.QueryTable(ctx, p.SQL)
		if env != nil {
			env.RecordExecutedSQL(table.SQL)
		}
		if err != nil {
			return "", err
		}
		if env != nil {
			env.SetQueryTable(QueryTable{
				Columns:   table.Columns,
				Rows:      table.Rows,
				Truncated: table.Truncated,
			})
		}
		text += "\n" + queryExportHint
		if table.Truncated {
			text += "本次查询已达到行数上限，导出文件会包含本次返回的全部行。"
		}
		return text, nil
	case "":
		return "", fmt.Errorf("action 不能为空")
	default:
		return "", fmt.Errorf("未知 action: %s", p.Action)
	}
}
