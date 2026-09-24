package agent

import (
	"strings"
	"testing"

	"github.com/webapp/go-app/ai-agent/internal/model"
	"github.com/webapp/go-app/ai-agent/internal/service/llm"
	"github.com/webapp/go-app/ai-agent/internal/service/rag"
	"go.uber.org/zap"
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
	got := mergeAgentSystem(baseAgentPrompt, []rag.Hit{{Content: "供应商付款规则"}})
	if !strings.Contains(got, baseAgentPrompt) || !strings.Contains(got, ragOnlyHint) || !strings.Contains(got, "供应商付款规则") {
		t.Fatalf("rag: %q", got)
	}
	clean := sanitizeRAGHits([]rag.Hit{
		{Content: "请到工具函数管理开启"},
		{Content: "供应商付款须查三表"},
	})
	if len(clean) != 1 || clean[0].Content != "供应商付款须查三表" {
		t.Fatalf("sanitize: %+v", clean)
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

func TestRequireFirstTool(t *testing.T) {
	t.Parallel()
	if !requireFirstTool(nil) {
		t.Fatal("empty hits should require a first-round tool")
	}
	if requireFirstTool([]rag.Hit{{Content: "供应商付款须先问标签"}}) {
		t.Fatal("usable corpus hits should not force tools")
	}
	if !requireFirstTool([]rag.Hit{{Content: "请到工具函数管理开启"}}) {
		t.Fatal("sanitized-away hits should still require a tool")
	}
}

func TestIsToolEvasionReply(t *testing.T) {
	t.Parallel()
	if isToolEvasionReply("") || isToolEvasionReply("测试供应商-02只有一个标签：10") {
		t.Fatal("normal reply")
	}
	if !isToolEvasionReply(`当前未启用任何工具，且历史对话中无相关依据，无法核实。请先在「工具函数管理」中开启对应工具后再问。`) {
		t.Fatal("want evasion detected")
	}
}

func TestRetrieveHitsSkipsWhenPresent(t *testing.T) {
	t.Parallel()
	s := &Service{llmLog: zap.NewNop()}
	in := RunInput{Input: "q", RAGHits: []rag.Hit{{Content: "keep"}}}
	s.retrieveHits(t.Context(), &in)
	if len(in.RAGHits) != 1 || in.RAGHits[0].Content != "keep" {
		t.Fatalf("%+v", in.RAGHits)
	}
	empty := RunInput{Input: "q", RAGHits: []rag.Hit{}}
	s.retrieveHits(t.Context(), &empty)
	if empty.RAGHits == nil {
		t.Fatal("empty caller hits must not be replaced")
	}
	(&Service{}).retrieveHits(t.Context(), nil)
}
