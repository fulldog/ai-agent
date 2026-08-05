package remote

import "testing"

func TestSanitizeUploadFilename(t *testing.T) {
	got := sanitizeUploadFilename("哈药合同.pdf")
	if got != "upload.pdf" {
		t.Fatalf("got %q", got)
	}
	if sanitizeUploadFilename("a b.txt") != "a b.txt" {
		t.Fatal("ascii name changed")
	}
}

func TestParseFileObjectNested(t *testing.T) {
	raw := []byte(`{"data":{"id":"file-fe-abc","status":"processed","filename":"x.pdf"}}`)
	fo, err := parseFileObject(raw)
	if err != nil || fo.ID != "file-fe-abc" {
		t.Fatalf("got %#v err=%v", fo, err)
	}
	raw2 := []byte(`{"file_id":"file-fe-xyz","status":"ok"}`)
	fo2, err := parseFileObject(raw2)
	if err != nil || fo2.ID != "file-fe-xyz" {
		t.Fatalf("got %#v err=%v", fo2, err)
	}
}
