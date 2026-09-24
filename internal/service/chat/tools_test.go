package chat

import (
	"strings"
	"testing"

	"github.com/webapp/go-app/ai-agent/internal/config"
	"github.com/webapp/go-app/ai-agent/internal/service/agent/tools"
	"github.com/webapp/go-app/ai-agent/internal/service/llm"
)

func TestToolSpecsAlwaysIncludesDefaultTools(t *testing.T) {
	t.Parallel()
	off := false
	s := &Service{cfg: &config.Config{}, registry: tools.Default()}
	s.cfg.Agent.DefaultTools = []string{"knowledge_search"}

	if got := s.toolSpecs(); len(got) < 1 || got[0].Function.Name != "knowledge_search" {
		t.Fatalf("default on: %+v", got)
	}
	s.cfg.Chat.ToolsEnabled = &off
	got := s.toolSpecs()
	if len(got) < 1 || got[0].Function.Name != "knowledge_search" {
		t.Fatalf("tools_enabled=false still keeps default_tools: %+v", got)
	}
}

func TestToolSpecsMergesChatTools(t *testing.T) {
	t.Parallel()
	s := &Service{cfg: &config.Config{}, registry: tools.Default()}
	s.cfg.Agent.DefaultTools = []string{"knowledge_search"}
	s.cfg.Chat.Tools = []string{"not_registered", "calculator"}
	got := s.toolSpecs()
	names := make([]string, 0, len(got))
	for _, sp := range got {
		names = append(names, sp.Function.Name)
	}
	if len(names) < 2 || names[0] != "knowledge_search" {
		t.Fatalf("should keep default then extras: %+v", names)
	}
	foundCalc := false
	for _, n := range names {
		if n == "calculator" {
			foundCalc = true
		}
		if n == "not_registered" {
			t.Fatal("unregistered name must be dropped")
		}
	}
	if !foundCalc {
		t.Fatalf("missing calculator: %+v", names)
	}
}

func TestWantRAG(t *testing.T) {
	t.Parallel()
	if !(&Service{}).wantRAG(CompleteInput{}) {
		t.Fatal("nil cfg should default RAG on")
	}
	s := &Service{cfg: &config.Config{}}
	if !s.wantRAG(CompleteInput{}) {
		t.Fatal("unset chat.rag_enabled should default on")
	}
	off := false
	s.cfg.Chat.RAGEnabled = &off
	if s.wantRAG(CompleteInput{}) {
		t.Fatal("config off")
	}
	if !s.wantRAG(CompleteInput{RAGExplicit: true, RAGEnabled: true}) {
		t.Fatal("explicit on should win over config off")
	}
	if s.wantRAG(CompleteInput{RAGExplicit: true, RAGEnabled: false}) {
		t.Fatal("explicit off")
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
	if !strings.Contains(got, "知识摘录") {
		t.Fatal("dbconn prompt should follow corpus hits first")
	}
}
