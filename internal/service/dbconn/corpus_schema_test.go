package dbconn

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const sampleCorpus = `# SRM 数据字典

查有效数据加 IsDelete = 0。

## Srm_VendorInfo

供应商基本信息表。

- VendorName 供应商名称

## Srm_PayInfo

- BankName 开户行
`

func TestSchemaUsesCorpusSections(t *testing.T) {
	t.Parallel()
	c := &Client{}
	overview, err := c.Schema(context.Background(), "", "", sampleCorpus)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(overview, "IsDelete = 0") || !strings.Contains(overview, "Srm_VendorInfo") || !strings.Contains(overview, "Srm_PayInfo") {
		t.Fatalf("overview:\n%s", overview)
	}
	if strings.Contains(overview, "VendorName") {
		t.Fatalf("overview should omit columns:\n%s", overview)
	}
	vendor, err := c.Schema(context.Background(), "", "srm_vendorinfo", sampleCorpus)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(vendor, "VendorName") || strings.Contains(vendor, "BankName") {
		t.Fatalf("vendor section:\n%s", vendor)
	}
	pay, err := c.Schema(context.Background(), "开户行", "", sampleCorpus)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(pay, "Srm_PayInfo") || strings.Contains(pay, "VendorName") {
		t.Fatalf("keyword:\n%s", pay)
	}
	if _, err := c.Schema(context.Background(), "", "NoSuchTable", sampleCorpus); err == nil || !strings.Contains(err.Error(), "不在数据字典中") {
		t.Fatalf("missing table: %v", err)
	}
}

func TestSchemaCorpusNotTruncated(t *testing.T) {
	t.Parallel()
	body := strings.Repeat("甲", 8000)
	out, err := (&Client{}).Schema(context.Background(), "甲", "Srm_VendorInfo", body)
	if err != nil {
		t.Fatal(err)
	}
	if out != body {
		t.Fatalf("len=%d", len([]rune(out)))
	}
}

func TestPromptDictionaryKeepsRulesAndDropsColumns(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "docs", "SRM_AI_DICTIONARY.md"))
	if err != nil {
		t.Fatal(err)
	}
	body, full := PromptDictionary(string(raw))
	if full {
		t.Fatal("SRM dictionary should not be inlined in full")
	}
	if !strings.Contains(body, "写 SQL 前先看") || !strings.Contains(body, "Srm_VendorInfo") {
		t.Fatal("rules and table catalog missing")
	}
	if strings.Contains(body, "VendorNameShort") {
		t.Fatal("column detail leaked into the prompt excerpt")
	}
	one, err := lookupCorpus(string(raw), "", "Srm_VendorInfo")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(one, "VendorNameShort") || strings.Contains(one, "Srm_PayInfo") {
		t.Fatal("table lookup should return one table")
	}
}

func TestSchemaWithoutCorpusNeedsDB(t *testing.T) {
	t.Parallel()
	_, err := (&Client{}).Schema(context.Background(), "", "", "")
	if err == nil || !strings.Contains(err.Error(), "未配置") {
		t.Fatalf("got %v", err)
	}
	_, err = (&Client{}).Schema(context.Background(), "", "srm_vendorinfo", "  ")
	if err == nil || !strings.Contains(err.Error(), "前缀") {
		t.Fatalf("bad prefix should fail before db: %v", err)
	}
}
