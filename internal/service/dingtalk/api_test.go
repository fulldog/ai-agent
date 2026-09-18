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

	if err := replyWebhook(context.Background(), srv.URL, "AI 回复", "hello"); err != nil {
		t.Fatal(err)
	}
	if err := replyWebhook(context.Background(), "", "t", "x"); err == nil {
		t.Fatal("empty webhook should fail")
	}
}
