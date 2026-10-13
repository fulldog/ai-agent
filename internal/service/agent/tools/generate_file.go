package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/webapp/go-app/ai-agent/internal/service/filegen"
	"github.com/webapp/go-app/ai-agent/internal/service/llm"
)

// GenerateFile 把已整理的表格写成 Excel，并返回下载链接。
type GenerateFile struct {
	Store *filegen.Store
}

func (GenerateFile) Name() string { return "generate_file" }

func (GenerateFile) Spec() llm.ToolSpec {
	return llm.ToolSpec{
		Type: "function",
		Function: llm.ToolSpecFunc{
			Name:        "generate_file",
			Description: "生成 Excel 并返回下载链接。导出 dbconn 查询结果时只传 filename 和 from_query=true，工具会写入本次查询的全部行，不要再传 columns 或 rows。只有不是查询结果的小表才自己传 columns 和 rows。",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"filename": map[string]any{"type": "string", "description": "下载时显示的文件名，不含路径，可不带扩展名"},
					"from_query": map[string]any{
						"type":        "boolean",
						"description": "为 true 时直接导出本次对话最近一次 dbconn query 的完整结果，不要同时传 columns 和 rows",
					},
					"columns": map[string]any{
						"type":        "array",
						"description": "非查询结果时的表头。from_query 为 true 时不要传",
						"items":       map[string]any{"type": "string"},
					},
					"rows": map[string]any{
						"type":        "array",
						"description": "非查询结果时的数据行。from_query 为 true 时不要传",
						"items":       map[string]any{},
					},
				},
			},
		},
	}
}

func (g GenerateFile) Exec(_ context.Context, args json.RawMessage, env *Env) (string, error) {
	if g.Store == nil {
		return "", fmt.Errorf("文件生成服务未就绪")
	}
	var p struct {
		Filename  string            `json:"filename"`
		FromQuery bool              `json:"from_query"`
		Columns   []string          `json:"columns"`
		Rows      []json.RawMessage `json:"rows"`
	}
	table, hasQuery := env.QueryTable()
	if err := json.Unmarshal(args, &p); err != nil {
		if !hasQuery {
			return "", fmt.Errorf("参数无效: %w", err)
		}
		return g.saveTable(filenameFromArgs(args), table)
	}
	if p.FromQuery || (hasQuery && len(p.Columns) == 0 && len(p.Rows) == 0) {
		if !hasQuery {
			return "", fmt.Errorf("还没有可导出的查询结果，请先调用 dbconn 的 query")
		}
		return g.saveTable(p.Filename, table)
	}
	columns := make([]string, 0, len(p.Columns))
	for _, col := range p.Columns {
		col = strings.TrimSpace(col)
		if col == "" {
			return "", fmt.Errorf("列名不能为空")
		}
		columns = append(columns, col)
	}
	rows := make([][]string, 0, len(p.Rows))
	for i, raw := range p.Rows {
		row, err := parseRow(raw, columns)
		if err != nil {
			return "", fmt.Errorf("第 %d 行无效: %w", i+1, err)
		}
		rows = append(rows, row)
	}
	file, err := g.Store.SaveXLSX(p.Filename, columns, rows)
	if err != nil {
		return "", err
	}
	return formatFileResult(file, ""), nil
}

func (g GenerateFile) saveTable(filename string, table QueryTable) (string, error) {
	cols := append([]string(nil), table.Columns...)
	rows := make([][]string, len(table.Rows))
	for i, row := range table.Rows {
		rows[i] = append([]string(nil), row...)
	}
	file, err := g.Store.SaveXLSX(filename, cols, rows)
	if err != nil {
		return "", err
	}
	note := ""
	if table.Truncated {
		note = "查询已达到行数上限，文件包含本次查询返回的全部行。"
	}
	return formatFileResult(file, note), nil
}

func formatFileResult(file filegen.File, note string) string {
	var b strings.Builder
	b.WriteString("文件已生成。\n")
	b.WriteString("文件名：")
	b.WriteString(file.Name)
	b.WriteString("\n下载链接：")
	b.WriteString(file.URL)
	if !strings.HasPrefix(file.URL, "http://") && !strings.HasPrefix(file.URL, "https://") {
		b.WriteString("\n当前未配置 server.public_base_url，上面是站内路径。")
	}
	if note != "" {
		b.WriteString("\n")
		b.WriteString(note)
	}
	b.WriteString("\n请在回复中把这个下载链接发给用户。")
	return b.String()
}

// filenameFromArgs 只读取已经写完的文件名。参数其余部分被截断时不解析行。
func filenameFromArgs(raw []byte) string {
	key := []byte(`"filename"`)
	i := bytes.Index(raw, key)
	if i < 0 {
		return ""
	}
	rest := bytes.TrimSpace(raw[i+len(key):])
	if len(rest) == 0 || rest[0] != ':' {
		return ""
	}
	rest = bytes.TrimSpace(rest[1:])
	var name string
	if err := json.NewDecoder(bytes.NewReader(rest)).Decode(&name); err != nil {
		return ""
	}
	return name
}

func parseRow(raw json.RawMessage, columns []string) ([]string, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return make([]string, len(columns)), nil
	}
	switch raw[0] {
	case '[':
		var items []json.RawMessage
		if err := json.Unmarshal(raw, &items); err != nil {
			return nil, err
		}
		out := make([]string, len(columns))
		for i := range out {
			if i < len(items) {
				out[i] = formatCell(items[i])
			}
		}
		return out, nil
	case '{':
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(raw, &obj); err != nil {
			return nil, err
		}
		out := make([]string, len(columns))
		for i, col := range columns {
			if v, ok := obj[col]; ok {
				out[i] = formatCell(v)
			}
		}
		return out, nil
	default:
		return nil, fmt.Errorf("行必须是数组或对象")
	}
}

func formatCell(raw json.RawMessage) string {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return ""
	}
	switch raw[0] {
	case '"':
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return ""
		}
		return s
	case 't', 'f':
		var b bool
		if err := json.Unmarshal(raw, &b); err != nil {
			return string(raw)
		}
		if b {
			return "是"
		}
		return "否"
	default:
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.UseNumber()
		var n json.Number
		if err := dec.Decode(&n); err != nil {
			return string(raw)
		}
		return n.String()
	}
}
