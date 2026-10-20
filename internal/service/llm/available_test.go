package llm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/webapp/go-app/ai-agent/internal/config"
)

func TestAvailableModels(t *testing.T) {
	t.Parallel()
	var hits atomic.Int32
	var badAuth atomic.Bool
	okSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.URL.Path != "/models" || r.Header.Get("Authorization") != "Bearer ok-key" {
			badAuth.Store(true)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"m-b"},{"id":"m-a"},{"id":"m-a"},{"id":" "}]}`))
	}))
	t.Cleanup(okSrv.Close)
	badSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "secret-body", http.StatusInternalServerError)
	}))
	t.Cleanup(badSrv.Close)

	off := false
	cfg := &config.Config{}
	cfg.LLM.Providers = map[string]config.LLMProviderConfig{
		"zeta":     {BaseURL: badSrv.URL, APIKey: "bad-key", DefaultModel: "zeta-default"},
		"alpha":    {BaseURL: okSrv.URL, APIKey: "ok-key", DefaultModel: "m-default"},
		"skip":     {BaseURL: okSrv.URL, APIKey: "", DefaultModel: "hidden"},
		"disabled": {BaseURL: okSrv.URL, APIKey: "k", DefaultModel: "x", Enabled: &off},
	}
	p := NewPool(cfg)
	groups := p.AvailableModels(context.Background())
	if badAuth.Load() {
		t.Fatal("model list request was not GET /models with the provider key")
	}
	if len(groups) != 2 {
		t.Fatalf("groups: %#v", groups)
	}
	if groups[0].Provider != "alpha" || groups[0].Error != "" {
		t.Fatalf("alpha: %#v", groups[0])
	}
	if got := strings.Join(groups[0].Models, ","); got != "m-a,m-b,m-default" {
		t.Fatalf("alpha models %s", got)
	}
	if groups[1].Provider != "zeta" || groups[1].Error == "" {
		t.Fatalf("zeta: %#v", groups[1])
	}
	if got := strings.Join(groups[1].Models, ","); got != "zeta-default" {
		t.Fatalf("zeta models %s", got)
	}
	if strings.Contains(groups[1].Error, "secret-body") || strings.Contains(groups[1].Error, "bad-key") {
		t.Fatalf("error leaked: %s", groups[1].Error)
	}

	again := p.AvailableModels(context.Background())
	if hits.Load() != 1 {
		t.Fatalf("cache missed, hits=%d", hits.Load())
	}
	if strings.Join(again[0].Models, ",") != "m-a,m-b,m-default" {
		t.Fatalf("cached %#v", again)
	}
}
