package dbconn

import (
	"context"
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

func TestSchemaReturnsWholeCorpus(t *testing.T) {
	t.Parallel()
	c := &Client{}
	cases := []struct {
		keyword string
		table   string
	}{
		{keyword: "", table: ""},
		{keyword: "开户行", table: ""},
		{keyword: "", table: "Srm_VendorInfo"},
		{keyword: "", table: "vendor"},
	}
	want := strings.TrimSpace(sampleCorpus)
	for _, tc := range cases {
		out, err := c.Schema(context.Background(), tc.keyword, tc.table, sampleCorpus)
		if err != nil {
			t.Fatalf("keyword=%q table=%q: %v", tc.keyword, tc.table, err)
		}
		if out != want {
			t.Fatalf("keyword=%q table=%q got:\n%s", tc.keyword, tc.table, out)
		}
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
