package dbconn

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
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
	c := &Client{dict: loadFixture(t)}
	out, err := c.Schema(context.Background(), "", "Srm_VendorInfo")
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
	c := &Client{dict: loadFixture(t)}
	out, err := c.Schema(context.Background(), "", "crm.Srm_VendorInfo")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "表 Srm_VendorInfo") {
		t.Fatalf("got:\n%s", out)
	}
}

func TestDictionaryKeywordHitsColumnComment(t *testing.T) {
	t.Parallel()
	c := &Client{dict: loadFixture(t)}
	out, err := c.Schema(context.Background(), "开户行", "")
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
	c := &Client{dict: loadFixture(t)}
	out, err := c.Schema(context.Background(), "", "")
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
	c := &Client{dict: loadFixture(t)}
	_, err := c.Schema(context.Background(), "", "vendor")
	if err == nil || !strings.Contains(err.Error(), "前缀") {
		t.Fatalf("prefix: %v", err)
	}
	_, err = c.Schema(context.Background(), "", "srm_vendorinfo")
	if err == nil || !strings.Contains(err.Error(), "前缀") {
		t.Fatalf("case: %v", err)
	}
	_, err = c.Schema(context.Background(), "", "Srm_Missing")
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

func TestUpdateDictionaryReplacesFileAndMemory(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "srm_dictionary.json")
	old, err := parseDictionary([]byte(`{"conventions":["旧约定"],"tables":[{"name":"Srm_Old","comment":"旧表","columns":[{"name":"Id","type":"int","nullable":false}]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	c := &Client{dictPath: path, dict: old}
	if _, err := c.Schema(context.Background(), "", "Srm_Old"); err != nil {
		t.Fatal(err)
	}
	next := []byte(`{"conventions":["新约定"],"tables":[{"name":"Srm_New","comment":"新表","columns":[{"name":"Name","type":"varchar(32)","nullable":true,"comment":"名称"}]}]}`)
	tables, cols, err := c.UpdateDictionary(next)
	if err != nil || tables != 1 || cols != 1 {
		t.Fatalf("tables=%d cols=%d err=%v", tables, cols, err)
	}
	out, err := c.Schema(context.Background(), "", "Srm_New")
	if err != nil || !strings.Contains(out, "新表") || !strings.Contains(out, "名称") {
		t.Fatalf("schema after update: %v\n%s", err, out)
	}
	if _, err := c.Schema(context.Background(), "", "Srm_Old"); err == nil || !strings.Contains(err.Error(), "未找到表") {
		t.Fatalf("old table: %v", err)
	}
	loaded, err := LoadDictionary(path)
	if err != nil || loaded.byName["Srm_New"] == nil {
		t.Fatalf("file reload: %v", err)
	}
	if _, _, err := c.UpdateDictionary([]byte(`{`)); err == nil {
		t.Fatal("invalid json should fail")
	}
	if _, err := c.Schema(context.Background(), "", "Srm_New"); err != nil {
		t.Fatalf("invalid update must keep previous dictionary: %v", err)
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

func TestProductionDictionary(t *testing.T) {
	t.Parallel()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("caller")
	}
	path := filepath.Join(filepath.Dir(file), "..", "..", "..", "docs", "srm_dictionary.json")
	d, err := LoadDictionary(path)
	if err != nil {
		t.Fatal(err)
	}
	c := &Client{dict: d}
	out, err := c.Schema(context.Background(), "", "Srm_VendorInfo")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "10 创节业务") {
		t.Fatalf("VendorLabel comment missing:\n%s", out)
	}
	if len(d.Tables) != 33 {
		t.Fatalf("tables=%d", len(d.Tables))
	}
}
