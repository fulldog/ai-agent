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
			Description: "把已经查询并整理好的表格生成 Excel 文件，返回下载链接。用户要求导出、下载或生成表格文件时，先整理列和行，再调用本工具。不要传入 SQL 或未整理的原始结果。",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"filename": map[string]any{"type": "string", "description": "下载时显示的文件名，不含路径，可不带扩展名"},
					"columns": map[string]any{
						"type":        "array",
						"description": "表头，使用用户能看懂的中文列名",
						"items":       map[string]any{"type": "string"},
					},
					"rows": map[string]any{
						"type":        "array",
						"description": "数据行。每行是与 columns 顺序一致的数组，或按列名取值的对象",
						"items":       map[string]any{},
					},
				},
				"required": []string{"columns", "rows"},
			},
		},
	}
}

func (g GenerateFile) Exec(_ context.Context, args json.RawMessage, _ *Env) (string, error) {
	if g.Store == nil {
		return "", fmt.Errorf("文件生成服务未就绪")
	}
	var p struct {
		Filename string            `json:"filename"`
		Columns  []string          `json:"columns"`
		Rows     []json.RawMessage `json:"rows"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("参数无效: %w", err)
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
	var b strings.Builder
	b.WriteString("文件已生成。\n")
	b.WriteString("文件名：")
	b.WriteString(file.Name)
	b.WriteString("\n下载链接：")
	b.WriteString(file.URL)
	if !strings.HasPrefix(file.URL, "http://") && !strings.HasPrefix(file.URL, "https://") {
		b.WriteString("\n当前未配置 server.public_base_url，上面是站内路径。")
	}
	b.WriteString("\n请在回复中把这个下载链接发给用户。")
	return b.String(), nil
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
