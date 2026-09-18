package dingtalk

import (
	"testing"

	"github.com/google/uuid"
	"github.com/webapp/go-app/ai-agent/internal/model"
	"github.com/webapp/go-app/ai-agent/internal/service/rag"
)

func TestMatchCorporaByName(t *testing.T) {
	t.Parallel()
	fin := uuid.New()
	hr := uuid.New()
	rows := []model.Corpus{
		{ID: fin, Name: "财务制度"},
		{ID: hr, Name: "人事手册"},
	}
	got := matchCorpora("人事手册里年假几天", rows)
	if len(got) != 1 || got[0].ID != hr {
		t.Fatalf("name match: %#v", got)
	}
	if matchCorpora("随便问问", rows) != nil {
		t.Fatal("no keyword should not pin a corpus")
	}
}

func TestBestCorpusID(t *testing.T) {
	t.Parallel()
	id := uuid.New()
	got := bestCorpusID([]rag.Hit{{CorpusID: id}})
	if got == nil || *got != id {
		t.Fatalf("got %#v", got)
	}
}

func TestFilterRelevant(t *testing.T) {
	t.Parallel()
	hits := []rag.Hit{
		{Content: "close", Score: 0.2},
		{Content: "far", Score: 0.9},
	}
	got := filterRelevant(hits, 0.55)
	if len(got) != 1 || got[0].Content != "close" {
		t.Fatalf("got %#v", got)
	}
	if filterRelevant(hits, 0) == nil || len(filterRelevant(hits, 0)) != 2 {
		t.Fatal("maxDistance<=0 should keep all hits")
	}
}

func TestIsCorpusMiss(t *testing.T) {
	t.Parallel()
	if !isCorpusMiss(nil, false) {
		t.Fatal("empty hits without online should miss")
	}
	if isCorpusMiss(nil, true) {
		t.Fatal("empty hits with forced online should not miss")
	}
	if isCorpusMiss([]rag.Hit{{Content: "x"}}, false) {
		t.Fatal("hits should not miss")
	}
}
