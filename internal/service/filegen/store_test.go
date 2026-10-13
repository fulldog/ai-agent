package filegen

import (
	"bytes"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"
)

func TestSaveAndReadXLSX(t *testing.T) {
	store, err := Open(t.TempDir(), "http://example.test", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	file, err := store.SaveXLSX("供应商 / 名单", []string{"名称", "金额"}, [][]string{
		{"甲公司", "12.5"},
		{"乙公司", "=1+1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if file.URL != "http://example.test/api/v1/exports/"+file.ID {
		t.Fatalf("url %s", file.URL)
	}
	if file.Name != "供应商 _ 名单.xlsx" {
		t.Fatalf("name %s", file.Name)
	}
	name, body, err := store.Read(file.ID)
	if err != nil {
		t.Fatal(err)
	}
	if name != file.Name {
		t.Fatalf("read name %s", name)
	}
	book, err := excelize.OpenReader(bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer book.Close()
	sheet := book.GetSheetName(0)
	got, err := book.GetCellValue(sheet, "B3")
	if err != nil {
		t.Fatal(err)
	}
	if got != "'=1+1" {
		t.Fatalf("formula cell %q", got)
	}
	amount, err := book.GetCellValue(sheet, "B2")
	if err != nil {
		t.Fatal(err)
	}
	if amount != "12.5" {
		t.Fatalf("amount %q", amount)
	}
	if _, _, err := store.Read("../" + file.ID); err != ErrNotFound {
		t.Fatalf("traversal err %v", err)
	}
}

func TestReadExpired(t *testing.T) {
	store, err := Open(t.TempDir(), "", time.Nanosecond)
	if err != nil {
		t.Fatal(err)
	}
	file, err := store.SaveXLSX("过期", []string{"列"}, [][]string{{"值"}})
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(2 * time.Millisecond)
	if _, _, err := store.Read(file.ID); err != ErrNotFound {
		t.Fatalf("err %v", err)
	}
}
