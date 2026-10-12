package fileextract

import (
	"context"
	"strings"
	"testing"

	"github.com/webapp/go-app/ai-agent/internal/config"
	"github.com/webapp/go-app/ai-agent/pkg/extract"
)

func TestContentHashStable(t *testing.T) {
	a := ContentHash([]byte("hello"))
	b := ContentHash([]byte("hello"))
	c := ContentHash([]byte("world"))
	if a != b {
		t.Fatalf("hash not stable")
	}
	if a == c {
		t.Fatalf("different content same hash")
	}
	if len(a) != 64 {
		t.Fatalf("want sha256 hex len 64, got %d", len(a))
	}
}

func TestPreparePlainTextSkipsRemote(t *testing.T) {
	s := New(nil, extract.New(extract.OCRConfig{}), t.TempDir(), &config.Config{
		LLM: config.LLMConfig{DefaultProvider: "qwen"},
	})
	got, err := s.Prepare(context.Background(), PrepareInput{
		Filename: "note.md",
		Data:     []byte("# 标题\n正文"),
		Provider: "qwen",
		NeedText: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.ExtractBackend != "local" || got.RemoteFileID != "" || !strings.Contains(got.Text, "正文") {
		t.Fatalf("%#v", got)
	}
}

func TestQwenReuseRequiresTextWhenNeeded(t *testing.T) {
	if !qwenReuseOK(false, "") {
		t.Fatal("callers that only need a file id can reuse it")
	}
	if qwenReuseOK(true, "") || qwenReuseOK(true, " \n\t") {
		t.Fatal("indexing must not accept a file id without text")
	}
	if !qwenReuseOK(true, "正文") {
		t.Fatal("non-empty text should satisfy indexing")
	}
}
