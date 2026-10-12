package remote

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSanitizeUploadFilename(t *testing.T) {
	got := sanitizeUploadFilename("哈药合同.pdf")
	if got != "upload.pdf" {
		t.Fatalf("got %q", got)
	}
	if sanitizeUploadFilename("a b.txt") != "a b.txt" {
		t.Fatal("ascii name changed")
	}
}

func TestChatWithFileIDStreamsPastClientTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			http.NotFound(w, r)
			return
		}
		time.Sleep(80 * time.Millisecond)
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"合同\"}}]}\n\n"))
		if flusher != nil {
			flusher.Flush()
		}
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"正文\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer srv.Close()

	cli := NewFilesClient(srv.URL, "test-key", 20*time.Millisecond)
	got, err := cli.ChatWithFileID(t.Context(), "qwen-long", "file-1", "输出全文")
	if err != nil {
		t.Fatal(err)
	}
	if got != "合同正文" {
		t.Fatalf("got %q", got)
	}
}

func TestChatWithFileIDJSONFallback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"全文"}}]}`))
	}))
	defer srv.Close()

	cli := NewFilesClient(srv.URL, "test-key", time.Second)
	got, err := cli.ChatWithFileID(t.Context(), "qwen-long", "file-1", "")
	if err != nil {
		t.Fatal(err)
	}
	if got != "全文" {
		t.Fatalf("got %q", got)
	}
}

func TestChatWithFileIDStreamError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"error\":{\"message\":\"upstream busy\"}}\n\n"))
	}))
	defer srv.Close()

	cli := NewFilesClient(srv.URL, "test-key", time.Second)
	_, err := cli.ChatWithFileID(t.Context(), "qwen-long", "file-1", "输出全文")
	if err == nil || !strings.Contains(err.Error(), "upstream busy") {
		t.Fatalf("err=%v", err)
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
