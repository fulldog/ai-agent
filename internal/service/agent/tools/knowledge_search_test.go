package tools

import (
	"context"
	"strings"
	"testing"
)

func TestKnowledgeQueryUsesCurrentTurn(t *testing.T) {
	t.Parallel()
	env := &Env{RAGQuery: "本轮用户问题"}
	if got := knowledgeQuery("模型根据历史改写的问题", env); got != "本轮用户问题" {
		t.Fatalf("got %q", got)
	}
}

func TestKnowledgeQueryFallsBackToToolArgument(t *testing.T) {
	t.Parallel()
	if got := knowledgeQuery("  查询词  ", &Env{}); got != "查询词" {
		t.Fatalf("got %q", got)
	}
}

func TestKnowledgeSearchDisabledSkipsRetrieval(t *testing.T) {
	t.Parallel()
	got, err := KnowledgeSearch{}.Exec(context.Background(), []byte(`{"query":"供应商"}`), &Env{RAGDisabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "未绑定语料库") {
		t.Fatalf("got %q", got)
	}
}
