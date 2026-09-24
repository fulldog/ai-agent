package dingtalk

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestReplyWebhook(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method=%s", r.Method)
		}
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		var body map[string]any
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Fatal(err)
		}
		if body["msgtype"] != "markdown" {
			t.Fatalf("msgtype=%v", body["msgtype"])
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	if err := replyWebhook(context.Background(), srv.URL, "AI 回复", "hello", nil); err != nil {
		t.Fatal(err)
	}
	if err := replyWebhook(context.Background(), "", "t", "x", nil); err == nil {
		t.Fatal("empty webhook should fail")
	}
}

func TestReplyWebhookAt(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		var body map[string]any
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Fatal(err)
		}
		at, ok := body["at"].(map[string]any)
		if !ok {
			t.Fatalf("missing at: %s", raw)
		}
		ids, _ := at["atUserIds"].([]any)
		if len(ids) != 1 || ids[0] != "staff-1" {
			t.Fatalf("atUserIds=%v", ids)
		}
		md, _ := body["markdown"].(map[string]any)
		if text, _ := md["text"].(string); text != "hi\n\n@staff-1" {
			t.Fatalf("text=%q", text)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	text := appendMarkdownAt("hi", "staff-1")
	if err := replyWebhook(context.Background(), srv.URL, "t", text, []string{"staff-1", "", "staff-1"}); err != nil {
		t.Fatal(err)
	}
}

func TestAppendMarkdownAt(t *testing.T) {
	t.Parallel()
	if got := appendMarkdownAt("hello", "u1"); got != "hello\n\n@u1" {
		t.Fatalf("got %q", got)
	}
	if got := appendMarkdownAt("hello\n\n@u1", "u1"); got != "hello\n\n@u1" {
		t.Fatalf("idempotent: %q", got)
	}
	if got := appendMarkdownAt("x", "  "); got != "x" {
		t.Fatalf("empty id: %q", got)
	}
}
