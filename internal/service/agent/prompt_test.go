package agent

import (
	"strings"
	"testing"

	"github.com/webapp/go-app/ai-agent/internal/model"
	"github.com/webapp/go-app/ai-agent/internal/service/llm"
	"github.com/webapp/go-app/ai-agent/internal/service/rag"
)

func TestAgentSystemPrompt(t *testing.T) {
	t.Parallel()
	got := agentSystemPrompt(nil)
	if got != baseAgentPrompt {
		t.Fatalf("no tools: %q", got)
	}
	got = agentSystemPrompt([]llm.ToolSpec{{Function: llm.ToolSpecFunc{Name: "calculator"}}})
	if !strings.Contains(got, toolsReadyPrompt) || !strings.Contains(got, "calculator") || strings.Contains(got, dbconnAgentPrompt) {
		t.Fatalf("calculator only: %q", got)
	}
	got = agentSystemPrompt([]llm.ToolSpec{
		{Function: llm.ToolSpecFunc{Name: "knowledge_search"}},
		{Function: llm.ToolSpecFunc{Name: "dbconn"}},
	})
	if !strings.Contains(got, "dbconn") || !strings.Contains(got, "knowledge_search") || !strings.Contains(got, toolsReadyPrompt) {
		t.Fatalf("dbconn prompt: %q", got)
	}
	if !strings.Contains(got, "工具函数管理") {
		t.Fatal("should explicitly forbid 工具函数管理 phrasing")
	}
}

func TestMergeAgentSystemAndInitialMessages(t *testing.T) {
	t.Parallel()
	if mergeAgentSystem("base", nil) != "base" {
		t.Fatal("no hits")
	}
	got := mergeAgentSystem(baseAgentPrompt, []rag.Hit{{Content: "请到工具函数管理开启"}})
	if !strings.Contains(got, baseAgentPrompt) || !strings.Contains(got, ragOnlyHint) || !strings.Contains(got, "工具函数管理") {
		t.Fatalf("rag: %q", got)
	}
	hist := []model.Message{
		{Role: "user", Content: "hi"},
		{Role: "assistant", Content: "hello"},
		{Role: "system", Content: "skip"},
	}
	msgs := initialMessages("sys", hist, "now")
	if len(msgs) != 4 {
		t.Fatalf("len=%d", len(msgs))
	}
	if msgs[0].Role != "system" || msgs[1].Role != "user" || msgs[2].Role != "assistant" || msgs[3].Content != "now" {
		t.Fatalf("%+v", msgs)
	}
}
