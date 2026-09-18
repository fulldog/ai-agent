package dingtalk

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/webapp/go-app/ai-agent/internal/service/llm"
	"github.com/webapp/go-app/ai-agent/internal/service/rag"
)

func TestPreviewText(t *testing.T) {
	t.Parallel()
	if got := previewText("abc", 10); got != "abc" {
		t.Fatalf("got %q", got)
	}
	if got := previewText("abcdefghij", 4); got != "abcd..." {
		t.Fatalf("got %q", got)
	}
	if got := previewText("你好世界", 2); got != "你好..." {
		t.Fatalf("got %q", got)
	}
}

func TestRagHitLogs(t *testing.T) {
	t.Parallel()
	id := uuid.New()
	got := ragHitLogs([]rag.Hit{{ChunkID: id, Content: "abcdefghij", Score: 0.2}}, 4)
	if len(got) != 1 || got[0].ChunkID != id.String() || got[0].Content != "abcd..." {
		t.Fatalf("got %#v", got)
	}
}

func TestLLMTurnLogs(t *testing.T) {
	t.Parallel()
	got := llmTurnLogs([]llm.Message{{Role: "user", Content: "hello world"}}, 5)
	if len(got) != 1 || got[0].Role != "user" || got[0].Content != "hello..." {
		t.Fatalf("got %#v", got)
	}
}

func TestPipelineBody(t *testing.T) {
	t.Parallel()
	tr := &msgTrace{
		requestID: "msg-1",
		query:     "年假几天",
		ragQuery:  "年假几天",
		outcome:   "llm",
		steps: []pipelineStep{
			{Step: 1, Event: eventReceive, Detail: map[string]any{"raw_text": "年假几天"}},
			{Step: 2, Event: eventRAG, Detail: map[string]any{"hits": 1}},
			{Step: 3, Event: eventLLMRequest, Detail: map[string]any{"model": "qwen-plus"}},
			{Step: 4, Event: eventResult, Detail: map[string]any{"reply": "5天"}},
		},
	}
	raw := pipelineBody(tr)
	if raw == "{}" {
		t.Fatal("empty pipeline body")
	}
	if !strings.Contains(raw, `"request_id":"msg-1"`) {
		t.Fatalf("missing request_id: %s", raw)
	}
	if !strings.Contains(raw, eventReceive) || !strings.Contains(raw, eventRAG) ||
		!strings.Contains(raw, eventLLMRequest) || !strings.Contains(raw, eventResult) {
		t.Fatalf("missing steps: %s", raw)
	}
}
