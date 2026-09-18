package chat

import (
	"strings"
	"testing"

	"github.com/webapp/go-app/ai-agent/internal/config"
	"github.com/webapp/go-app/ai-agent/internal/service/rag"
)

func TestReplyStyleDefaultAndOverride(t *testing.T) {
	t.Parallel()
	if got := (&Service{}).replyStyle(); got != defaultReplyStyle {
		t.Fatalf("nil cfg: %q", got)
	}
	s := &Service{cfg: &config.Config{}}
	s.cfg.LLM.SystemPrompt = "  只答要点  "
	if got := s.replyStyle(); got != "只答要点" {
		t.Fatalf("override: %q", got)
	}
}

func TestMergeSystem(t *testing.T) {
	t.Parallel()
	got := mergeSystem(defaultReplyStyle, "", "")
	if got != defaultReplyStyle {
		t.Fatalf("style only: %q", got)
	}
	got = mergeSystem(defaultReplyStyle, "你是财务助手", "【知识摘录】\n[1] 年假 5 天")
	if !strings.Contains(got, "你是财务助手") || !strings.Contains(got, defaultReplyStyle) || !strings.Contains(got, "年假") {
		t.Fatalf("merged: %q", got)
	}
}

func TestRAGContext(t *testing.T) {
	t.Parallel()
	if ragContext(nil) != "" {
		t.Fatal("empty hits should be empty")
	}
	got := ragContext([]rag.Hit{{Content: "报销需发票"}})
	if !strings.Contains(got, ragContextHeader) || !strings.Contains(got, "[1] 报销需发票") {
		t.Fatalf("got %q", got)
	}
}
