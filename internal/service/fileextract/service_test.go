package fileextract

import "testing"

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
