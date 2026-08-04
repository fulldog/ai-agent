package chunker

import "testing"

func TestSplit(t *testing.T) {
	parts := Split("abcdefghijklmnopqrstuvwxyz", 10, 2)
	if len(parts) < 2 {
		t.Fatalf("expected multiple chunks, got %d", len(parts))
	}
	if parts[0] != "abcdefghij" {
		t.Fatalf("unexpected first chunk: %q", parts[0])
	}
}

func TestSplitShort(t *testing.T) {
	parts := Split("hello", 100, 10)
	if len(parts) != 1 || parts[0] != "hello" {
		t.Fatalf("unexpected: %#v", parts)
	}
}
