package middleware

import (
	"bytes"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestTruncateKeepsValidUTF8(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   string
		max  int
	}{
		{name: "ascii within", in: "hello", max: 10},
		{name: "ascii cut", in: "hello world", max: 5},
		{name: "cjk cut mid rune", in: "你好世界", max: 5}, // 你=3 bytes，截到 5 会切到「好」中间
		{name: "emoji cut", in: "ok👍done", max: 5},
		{name: "already invalid", in: "a\xe2", max: 10},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := truncate(tt.in, tt.max)
			if !utf8.ValidString(got) {
				t.Fatalf("invalid utf8: %q bytes=%v", got, []byte(got))
			}
			if tt.max > 0 && len(got) > tt.max {
				t.Fatalf("len=%d > max=%d", len(got), tt.max)
			}
		})
	}
	if got := truncate("你好", 3); got != "你" {
		t.Fatalf("want 你, got %q", got)
	}
	if got := truncate("a\xe2b", 10); strings.ContainsRune(got, utf8.RuneError) || !utf8.ValidString(got) {
		// ToValidUTF8 会把非法字节换成 U+FFFD；结果仍须 ValidString。
		if !utf8.ValidString(got) {
			t.Fatalf("invalid after sanitize: %q", got)
		}
	}
}

func TestBodyWriterPreviewDoesNotSplitUTF8(t *testing.T) {
	t.Parallel()
	w := &bodyWriter{buf: &bytes.Buffer{}, previewMax: 5}
	// 「你好」= 6 bytes；previewMax=5 只能完整留下「你」(3 bytes)
	n, err := w.Write([]byte("你好世界"))
	if err != nil || n != len("你好世界") {
		t.Fatalf("Write n=%d err=%v", n, err)
	}
	got := w.buf.String()
	if !utf8.ValidString(got) {
		t.Fatalf("invalid preview: %q", got)
	}
	if got != "你" {
		t.Fatalf("got %q want 你", got)
	}
}
