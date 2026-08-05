package extract

import (
	"strings"
	"testing"
)

func TestIsSupportedExtension(t *testing.T) {
	cases := map[string]bool{
		"a.pdf":  true,
		"a.docx": true,
		"a.png":  true,
		"a.jpg":  true,
		"a.doc":  false,
		"a.exe":  false,
	}
	for name, want := range cases {
		if got := IsSupportedExtension(name); got != want {
			t.Fatalf("%s: got %v want %v", name, got, want)
		}
	}
}

func TestFromBytesText(t *testing.T) {
	e := New(OCRConfig{Enabled: false})
	text, err := e.FromBytes("note.md", []byte("# hello\nworld"))
	if err != nil {
		t.Fatal(err)
	}
	if text == "" {
		t.Fatal("empty text")
	}
}

func TestCollapseCJKSpaces(t *testing.T) {
	in := "合 同 编 号 : LT-HYJK\n自 【2024】 年 5 月 【 】 日 起"
	got := collapseCJKSpaces(in)
	if !strings.Contains(got, "合同编号") {
		t.Fatalf("want 合同编号 collapsed, got %q", got)
	}
	if strings.Contains(got, "合 同") {
		t.Fatalf("CJK spaces not collapsed: %q", got)
	}
	// 英文词间空格保留；英文与汉字之间的空格会去掉（合同 OCR 常见）
	eng := collapseCJKSpaces("hello world 中 文")
	if eng != "hello world中文" {
		t.Fatalf("got %q", eng)
	}
}
