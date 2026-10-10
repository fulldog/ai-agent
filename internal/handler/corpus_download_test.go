package handler

import (
	"strings"
	"testing"
)

func TestExtractedDownloadName(t *testing.T) {
	if got := extractedDownloadName("排障与问题定位指南.md", ""); got != "排障与问题定位指南.md" {
		t.Fatalf("md: %s", got)
	}
	if got := extractedDownloadName("扫描件.pdf", "扫描件"); got != "扫描件.txt" {
		t.Fatalf("pdf: %s", got)
	}
	if got := extractedDownloadName("", "如何判断供应商能否付款"); got != "如何判断供应商能否付款.txt" {
		t.Fatalf("title: %s", got)
	}
}

func TestInlinePreview(t *testing.T) {
	if !inlinePreview("扫描件.pdf") || !inlinePreview("图.png") {
		t.Fatal("pdf and images should open inline")
	}
	if inlinePreview("合同.docx") || inlinePreview("说明.md") {
		t.Fatal("docx downloads; markdown opens as text")
	}
	inline := contentDisposition("图.png", true)
	if !strings.HasPrefix(inline, "inline;") {
		t.Fatalf("inline: %s", inline)
	}
	attachment := contentDisposition("合同.docx", false)
	if !strings.HasPrefix(attachment, "attachment;") {
		t.Fatalf("attachment: %s", attachment)
	}
}
