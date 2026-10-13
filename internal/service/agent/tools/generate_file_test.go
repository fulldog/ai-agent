package tools_test

import (
	"bytes"
	"context"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"

	"github.com/webapp/go-app/ai-agent/internal/service/agent/tools"
	"github.com/webapp/go-app/ai-agent/internal/service/filegen"
)

func TestGenerateFileWritesLink(t *testing.T) {
	store, err := filegen.Open(t.TempDir(), "https://files.example", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	got, err := (tools.GenerateFile{Store: store}).Exec(context.Background(), []byte(`{
		"filename": "广告线供应商",
		"columns": ["名称", "有效"],
		"rows": [
			["甲公司", true],
			{"名称": "乙公司", "有效": false}
		]
	}`), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "https://files.example/api/v1/exports/") || !strings.Contains(got, "广告线供应商.xlsx") {
		t.Fatalf("result %s", got)
	}
}

func TestGenerateFileExportsQueryTable(t *testing.T) {
	store, err := filegen.Open(t.TempDir(), "https://files.example", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	env := &tools.Env{}
	env.SetQueryTable(tools.QueryTable{
		Columns: []string{"供应商名称", "VendorId"},
		Rows: [][]string{
			{"北京字节跳动科技有限公司", "10000001"},
			{"上海伊姐房地产经纪有限公司", "10000002"},
			{"哈尔滨北极光数字传媒有限公司", "10000020"},
		},
	})
	tool := tools.GenerateFile{Store: store}

	got, err := tool.Exec(context.Background(), []byte(`{"filename":"启用的广告线供应商基础信息","from_query":true}`), env)
	if err != nil {
		t.Fatal(err)
	}
	rows := readExportRows(t, store, got)
	if len(rows) != 4 || rows[1][0] != "北京字节跳动科技有限公司" || rows[3][0] != "哈尔滨北极光数字传媒有限公司" || rows[3][1] != "10000020" {
		t.Fatalf("from_query rows %+v", rows)
	}

	// 模型把行抄进参数时会在末尾被截断。这时仍导出查询结果，不解析半截行。
	truncated := []byte(`{"filename":"启用的广告线供应商基础信息","columns":["供应商名称"],"rows":[["只有半行`)
	got, err = tool.Exec(context.Background(), truncated, env)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "启用的广告线供应商基础信息.xlsx") {
		t.Fatalf("result %s", got)
	}
	rows = readExportRows(t, store, got)
	if len(rows) != 4 || rows[1][0] != "北京字节跳动科技有限公司" || rows[3][0] != "哈尔滨北极光数字传媒有限公司" {
		t.Fatalf("truncated args rows %+v", rows)
	}

	got, err = tool.Exec(context.Background(), []byte(`{"filename":"手工","columns":["名称"],"rows":[["甲公司"]]}`), env)
	if err != nil {
		t.Fatal(err)
	}
	rows = readExportRows(t, store, got)
	if len(rows) != 2 || rows[1][0] != "甲公司" {
		t.Fatalf("manual rows %+v", rows)
	}
}

func TestGenerateFileFromQueryRequiresResult(t *testing.T) {
	store, err := filegen.Open(t.TempDir(), "https://files.example", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	_, err = (tools.GenerateFile{Store: store}).Exec(context.Background(), []byte(`{"filename":"供应商","from_query":true}`), nil)
	if err == nil || !strings.Contains(err.Error(), "还没有可导出的查询结果") {
		t.Fatalf("got %v", err)
	}
	_, err = (tools.GenerateFile{Store: store}).Exec(context.Background(), []byte(`{"filename":"供应商","rows":[["半行`), nil)
	if err == nil || !strings.Contains(err.Error(), "参数无效") {
		t.Fatalf("got %v", err)
	}
}

func readExportRows(t *testing.T, store *filegen.Store, result string) [][]string {
	t.Helper()
	id := regexp.MustCompile(`/exports/([0-9a-f-]{36})`).FindStringSubmatch(result)
	if id == nil {
		t.Fatalf("result %s", result)
	}
	_, body, err := store.Read(id[1])
	if err != nil {
		t.Fatal(err)
	}
	book, err := excelize.OpenReader(bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer book.Close()
	sheets := book.GetSheetList()
	if len(sheets) == 0 {
		t.Fatal("no sheet")
	}
	rows, err := book.GetRows(sheets[0])
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

func TestGenerateFileRejectsEmpty(t *testing.T) {
	store, err := filegen.Open(t.TempDir(), "https://files.example", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	_, err = (tools.GenerateFile{Store: store}).Exec(context.Background(), []byte(`{"columns":["名称"],"rows":[]}`), nil)
	if err == nil {
		t.Fatal("expected empty rows error")
	}
}
