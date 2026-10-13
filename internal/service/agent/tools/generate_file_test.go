package tools_test

import (
	"context"
	"strings"
	"testing"
	"time"

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
