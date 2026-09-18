package agent

import (
	"strings"
	"testing"

	"github.com/webapp/go-app/ai-agent/internal/service/llm"
)

func TestAgentSystemPrompt(t *testing.T) {
	t.Parallel()
	got := agentSystemPrompt(nil)
	if got != baseAgentPrompt {
		t.Fatalf("no tools: %q", got)
	}
	got = agentSystemPrompt([]llm.ToolSpec{{Function: llm.ToolSpecFunc{Name: "calculator"}}})
	if got != baseAgentPrompt {
		t.Fatalf("no dbconn: %q", got)
	}
	got = agentSystemPrompt([]llm.ToolSpec{{Function: llm.ToolSpecFunc{Name: "dbconn"}}})
	if got == baseAgentPrompt || !strings.Contains(got, "dbconn") || !strings.Contains(got, "knowledge_search") {
		t.Fatalf("dbconn prompt: %q", got)
	}
}
