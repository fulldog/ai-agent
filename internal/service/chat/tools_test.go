package chat

import (
	"strings"
	"testing"

	"github.com/webapp/go-app/ai-agent/internal/config"
	"github.com/webapp/go-app/ai-agent/internal/service/agent/tools"
	"github.com/webapp/go-app/ai-agent/internal/service/llm"
)

func TestToolSpecsSwitch(t *testing.T) {
	t.Parallel()
	off := false
	s := &Service{cfg: &config.Config{}, registry: tools.Default()}
	s.cfg.Agent.DefaultTools = []string{"knowledge_search"}

	if got := s.toolSpecs(); len(got) != 1 || got[0].Function.Name != "knowledge_search" {
		t.Fatalf("default on: %+v", got)
	}
	s.cfg.Chat.ToolsEnabled = &off
	if got := s.toolSpecs(); got != nil {
		t.Fatalf("disabled: %+v", got)
	}
}

func TestToolSpecsPrefersChatTools(t *testing.T) {
	t.Parallel()
	s := &Service{cfg: &config.Config{}, registry: tools.Default()}
	s.cfg.Agent.DefaultTools = []string{"knowledge_search"}
	s.cfg.Chat.Tools = []string{"not_registered"}
	if got := s.toolSpecs(); len(got) != 0 {
		t.Fatalf("chat.tools should win: %+v", got)
	}
}

func TestMaxToolSteps(t *testing.T) {
	t.Parallel()
	if got := (&Service{}).maxToolSteps(); got != 4 {
		t.Fatalf("nil cfg: %d", got)
	}
	s := &Service{cfg: &config.Config{}}
	s.cfg.Chat.MaxToolSteps = 2
	if got := s.maxToolSteps(); got != 2 {
		t.Fatalf("configured: %d", got)
	}
}

func TestToolSystemPrompt(t *testing.T) {
	t.Parallel()
	if got := toolSystemPrompt(nil); got != "" {
		t.Fatalf("no tools: %q", got)
	}
	spec := func(name string) llm.ToolSpec {
		return llm.ToolSpec{Type: "function", Function: llm.ToolSpecFunc{Name: name}}
	}
	got := toolSystemPrompt([]llm.ToolSpec{spec("knowledge_search")})
	if got != baseToolPrompt {
		t.Fatalf("without dbconn: %q", got)
	}
	got = toolSystemPrompt([]llm.ToolSpec{spec("knowledge_search"), spec("dbconn")})
	if !strings.Contains(got, baseToolPrompt) || !strings.Contains(got, dbconnToolPrompt) {
		t.Fatalf("with dbconn: %q", got)
	}
}
