package fileextract

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSafeAbs(t *testing.T) {
	root := t.TempDir()
	s := &Service{root: root}
	inside := filepath.Join(root, "a.txt")
	if err := os.WriteFile(inside, []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := s.safeAbs(inside)
	if err != nil {
		t.Fatal(err)
	}
	if got != inside {
		t.Fatalf("got %s", got)
	}
	if _, err := s.safeAbs(filepath.Join(root, "..", "secret.txt")); err == nil {
		t.Fatal("expected path escape to fail")
	}
	if _, err := s.safeAbs(""); err == nil {
		t.Fatal("expected empty path to fail")
	}
}
