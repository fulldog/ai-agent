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
			Description: "查询独立业务 MySQL。query 执行只读 SELECT。会话绑定了数据字典时，执行前会按字典预检并修正表名和列名，不通过则返回原因、不会查库。action=schema：指定 table 返回该表字段，指定 keyword 筛选，都不指定则返回规则和表目录。未配置字典时 schema 从业务库实时读取 information_schema，可用 keyword 或 table 过滤，仅 Srm 前缀且区分大小写，如 Srm_VendorInfo。仅在需要业务数据时调用。不要编造表名或列名。query 的完整结果会留给 generate_file，导出时不要把行再抄进参数。",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"action": map[string]any{
						"type":        "string",
						"enum":        []string{"schema", "query"},
						"description": "schema=按数据字典或业务库查看表结构；query=执行只读 SELECT",
					},
					"keyword": map[string]any{
						"type":        "string",
						"description": "action=schema 且未指定 table 时，按表名、表注释、字段名或字段注释筛选",
					},
					"table": map[string]any{
						"type":        "string",
						"description": "action=schema 指定表名查看该表字段。已配置数据字典时按字典里的表名匹配。未配置字典时仅允许 Srm 前缀且区分大小写，如 Srm_VendorInfo",
					},
					"sql": map[string]any{
						"type":        "string",
						"description": "action=query 时的单条 SELECT / WITH ... SELECT。绑定了数据字典时，执行前会按字典校正表名和列名；没有字典时先 schema",
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
		sqlText, rewritten, err := prepareQuerySQL(p.SQL, env)
		if err != nil {
			return "", err
		}
		text, table, err := t.Client.QueryTable(ctx, sqlText)
		if env != nil {
			env.RecordExecutedSQL(table.SQL)
		}
		if err != nil {
			if rewritten && table.SQL != "" {
				return "", fmt.Errorf("已按数据字典修正为 %s 后执行失败: %w", table.SQL, err)
			}
			return "", err
		}
		if env != nil {
			env.SetQueryTable(QueryTable{
				Columns:   table.Columns,
				Rows:      table.Rows,
				Truncated: table.Truncated,
			})
		}
		if rewritten && table.SQL != "" {
			text = "已按数据字典修正 SQL：\n" + table.SQL + "\n\n" + text
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

// prepareQuerySQL 在真正查库前，用会话绑定的数据字典校正表名和列名。
// 没有字典、或字典里解析不出表时，原样返回。rewritten 表示字典改过标识符。
func prepareQuerySQL(sqlText string, env *Env) (string, bool, error) {
	if env == nil || strings.TrimSpace(env.Dictionary) == "" {
		return sqlText, false, nil
	}
	fixed, err := dbconn.CheckSQL(sqlText, env.Dictionary)
	if err != nil {
		return "", false, err
	}
	// 字典没解析出表时 CheckSQL 原样返回，不能当成修正。
	if fixed == sqlText {
		return sqlText, false, nil
	}
	clean, err := dbconn.ValidateSelect(sqlText)
	if err != nil {
		return fixed, false, nil
	}
	return fixed, fixed != clean, nil
}
