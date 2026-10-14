package llmog

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/webapp/go-app/ai-agent/internal/model"
	"github.com/webapp/go-app/ai-agent/internal/service/llm"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func TestLoadPayloadsReadsLLMLog(t *testing.T) {
	dir := t.TempDir()
	id := uuid.New()
	long := strings.Repeat("语料", 40_000)
	var buf bytes.Buffer
	encCfg := zap.NewProductionEncoderConfig()
	encCfg.TimeKey = "ts"
	encCfg.EncodeTime = zapcore.ISO8601TimeEncoder
	core := zapcore.NewCore(zapcore.NewJSONEncoder(encCfg), zapcore.AddSync(&buf), zap.InfoLevel)
	log := zap.New(core)
	Save(nil, log, &model.LLMCallLog{
		ID: id, RequestID: "req-1", Provider: "qwen", Model: "qwen-plus", Status: "ok",
		RequestSummary: "agent_step=1 tools=1",
	}, &Payload{
		Messages: []llm.Message{
			{Role: "system", Content: "你是助手"},
			{Role: "user", Content: long},
		},
		Response:     "查完了",
		ToolCalls:    []llm.ToolCall{{ID: "c1", Type: "function", Function: llm.ToolFunction{Name: "dbconn", Arguments: `{"sql":"select 1"}`}}},
		FinishReason: "tool_calls",
	})
	if err := log.Sync(); err != nil {
		t.Fatalf("sync: %v", err)
	}
	name := filepath.Join(dir, "llm-"+time.Now().Format("2006-01-02")+".001.log")
	if err := os.WriteFile(name, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("write log: %v", err)
	}

	got := LoadPayloads(dir, []model.LLMCallLog{{ID: id, CreatedAt: time.Now()}})
	hit, ok := got[id]
	if !ok {
		t.Fatal("payload not found")
	}
	if !bytes.Contains(hit.Messages, []byte("你是助手")) || !bytes.Contains(hit.Messages, []byte("语料")) {
		t.Fatalf("messages missing prompt text, len=%d", len(hit.Messages))
	}
	if hit.Response != "查完了" || hit.FinishReason != "tool_calls" {
		t.Fatalf("response=%q finish=%q", hit.Response, hit.FinishReason)
	}
	if !bytes.Contains(hit.ToolCalls, []byte("dbconn")) {
		t.Fatalf("tool_calls: %s", hit.ToolCalls)
	}

	missing := LoadPayloads(dir, []model.LLMCallLog{{ID: uuid.New(), CreatedAt: time.Now()}})
	if len(missing) != 0 {
		t.Fatalf("unexpected hit: %#v", missing)
	}
}
