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

func TestSchemaUsesCorpus(t *testing.T) {
	t.Parallel()
	c := &Client{}
	out, err := c.Schema(context.Background(), "", "Srm_VendorInfo", sampleCorpus)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Srm_VendorInfo") || !strings.Contains(out, "供应商名称") {
		t.Fatalf("got:\n%s", out)
	}
	if strings.Contains(out, "开户行") {
		t.Fatalf("exact table must not include other sections:\n%s", out)
	}
}

func TestSchemaCorpusKeywordAndCatalog(t *testing.T) {
	t.Parallel()
	c := &Client{}
	out, err := c.Schema(context.Background(), "开户行", "", sampleCorpus)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Srm_PayInfo") || strings.Contains(out, "VendorName") {
		t.Fatalf("got:\n%s", out)
	}
	out, err = c.Schema(context.Background(), "", "", sampleCorpus)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "语料目录:") || !strings.Contains(out, "## Srm_PayInfo") {
		t.Fatalf("got:\n%s", out)
	}
	if strings.Contains(out, "开户行") {
		t.Fatalf("catalog must not include section body:\n%s", out)
	}
}

func TestSchemaCorpusRejectsBadTable(t *testing.T) {
	t.Parallel()
	c := &Client{}
	_, err := c.Schema(context.Background(), "", "vendor", sampleCorpus)
	if err == nil || !strings.Contains(err.Error(), "前缀") {
		t.Fatalf("prefix: %v", err)
	}
	_, err = c.Schema(context.Background(), "", "srm_vendorinfo", sampleCorpus)
	if err == nil || !strings.Contains(err.Error(), "前缀") {
		t.Fatalf("case: %v", err)
	}
	_, err = c.Schema(context.Background(), "", "Srm_Missing", sampleCorpus)
	if err == nil || !strings.Contains(err.Error(), "未找到表") {
		t.Fatalf("missing: %v", err)
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

func TestCorpusExcerptTruncates(t *testing.T) {
	t.Parallel()
	var b strings.Builder
	b.WriteString("# 长文\nSrm_VendorInfo\n")
	for i := 0; i < maxCorpusRunes; i++ {
		b.WriteString("甲")
	}
	out, ok := excerptCorpus(b.String(), "Srm_VendorInfo", true)
	if !ok || !strings.Contains(out, "结果已截断") || !strings.Contains(out, "Srm_VendorInfo") {
		t.Fatalf("ok=%v got tail %q", ok, tail(out, 40))
	}
}

func tail(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[len(r)-n:])
}
