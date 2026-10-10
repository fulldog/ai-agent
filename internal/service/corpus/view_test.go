package corpus

import (
	"strings"
	"testing"

	"github.com/webapp/go-app/ai-agent/pkg/chunker"
)

func TestResolveKind(t *testing.T) {
	cases := []struct {
		kind, source, want string
	}{
		{"", "排障与问题定位指南.md", "file"},
		{"", "如何判断供应商能否付款", "text"},
		{"text", "sla.md", "text"},
		{"file", "标题", "file"},
		{"FILE", "a.pdf", "file"},
	}
	for _, tc := range cases {
		if got := ResolveKind(tc.kind, tc.source); got != tc.want {
			t.Fatalf("ResolveKind(%q, %q)=%q, want %q", tc.kind, tc.source, got, tc.want)
		}
	}
}

func TestJoinChunksStripsConfiguredOverlap(t *testing.T) {
	const text = "供应商能否付款需要先看合同状态，再核对付款计划与发票。"
	parts := chunker.Split(text, 12, 4)
	if len(parts) < 2 {
		t.Fatalf("expected multiple chunks, got %d", len(parts))
	}
	if got := joinChunks(parts, 4); got != text {
		t.Fatalf("joinChunks()=%q, want %q", got, text)
	}
}

func TestJoinChunksRepeatedRunes(t *testing.T) {
	text := strings.Repeat("a", 30)
	parts := chunker.Split(text, 10, 3)
	if got := joinChunks(parts, 3); got != text {
		t.Fatalf("joinChunks() len=%d, want %d", len([]rune(got)), len([]rune(text)))
	}
}

func TestJoinChunksFallbackWhenOverlapMismatch(t *testing.T) {
	got := joinChunks([]string{"你好", "世界"}, 4)
	if got != "你好\n世界" {
		t.Fatalf("got %q", got)
	}
}
