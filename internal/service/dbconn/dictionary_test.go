package dbconn

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func loadFixture(t *testing.T) *Dictionary {
	t.Helper()
	d, err := LoadDictionary(filepath.Join("testdata", "dictionary.json"))
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestDictionaryExactTable(t *testing.T) {
	t.Parallel()
	out, err := loadFixture(t).lookup("", "Srm_VendorInfo")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"供应商基本信息表", "VendorId, VendorLabel", "供应商名称", "Srm_PayInfo(MUL)", "是否删除"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
	if strings.Contains(out, "开户行") {
		t.Fatalf("exact table must not include other tables' columns:\n%s", out)
	}
}

func TestDictionaryQualifiedName(t *testing.T) {
	t.Parallel()
	out, err := loadFixture(t).lookup("", "crm.Srm_VendorInfo")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "表 Srm_VendorInfo") {
		t.Fatalf("got:\n%s", out)
	}
}

func TestDictionaryKeywordHitsColumnComment(t *testing.T) {
	t.Parallel()
	out, err := loadFixture(t).lookup("开户行", "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Srm_PayInfo") || !strings.Contains(out, "BankName") {
		t.Fatalf("got:\n%s", out)
	}
	if strings.Contains(out, "VendorName") {
		t.Fatalf("keyword must not dump unrelated columns:\n%s", out)
	}
}

func TestDictionaryCatalog(t *testing.T) {
	t.Parallel()
	out, err := loadFixture(t).lookup("", "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "查有效数据加 IsDelete = 0") || !strings.Contains(out, "Srm_PayInfo") {
		t.Fatalf("got:\n%s", out)
	}
	if strings.Contains(out, "开户行") || strings.Contains(out, "供应商名称") {
		t.Fatalf("catalog must not include column comments:\n%s", out)
	}
}

func TestDictionaryRejectsBadTable(t *testing.T) {
	t.Parallel()
	d := loadFixture(t)
	_, err := d.lookup("", "vendor")
	if err == nil || !strings.Contains(err.Error(), "前缀") {
		t.Fatalf("prefix: %v", err)
	}
	_, err = d.lookup("", "srm_vendorinfo")
	if err == nil || !strings.Contains(err.Error(), "前缀") {
		t.Fatalf("case: %v", err)
	}
	_, err = d.lookup("", "Srm_Missing")
	if err == nil || !strings.Contains(err.Error(), "未找到表") {
		t.Fatalf("missing: %v", err)
	}
}

func TestDictionaryKeywordTruncates(t *testing.T) {
	t.Parallel()
	cols := make([]DictColumn, 0, maxSchemaRows+5)
	for i := 0; i < maxSchemaRows+5; i++ {
		cols = append(cols, DictColumn{Name: "Col", Type: "int", Comment: "命中项"})
	}
	d := &Dictionary{Tables: []DictTable{{
		Name:    "Srm_Wide",
		Comment: "宽表",
		Columns: cols,
	}}}
	if err := d.index(); err != nil {
		t.Fatal(err)
	}
	out := d.lookupKeyword("命中")
	if !strings.Contains(out, "结果已截断") {
		t.Fatalf("got:\n%s", out)
	}
	if got := strings.Count(out, "命中项"); got != maxSchemaRows {
		t.Fatalf("hits=%d", got)
	}
}

func TestLoadDictionaryErrors(t *testing.T) {
	t.Parallel()
	if _, err := LoadDictionary(filepath.Join(t.TempDir(), "missing.json")); err == nil {
		t.Fatal("missing file")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(path, []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadDictionary(path); err == nil || !strings.Contains(err.Error(), "解析数据字典") {
		t.Fatalf("bad json: %v", err)
	}
}
