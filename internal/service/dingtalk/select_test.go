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

func TestCorporaForRetrieve(t *testing.T) {
	t.Parallel()
	a := uuid.New()
	b := uuid.New()
	c := uuid.New()
	all := []model.Corpus{
		{ID: a, Name: "财务制度"},
		{ID: b, Name: "人事手册"},
		{ID: c, Name: "供应商付款"},
	}
	bound := []model.Corpus{all[1], all[2]}

	got := corporaForRetrieve(all, "随便问问")
	if len(got) != 3 {
		t.Fatalf("unbound should search all: %#v", got)
	}
	got = corporaForRetrieve(bound, "随便问问")
	if len(got) != 2 || got[0].ID != b || got[1].ID != c {
		t.Fatalf("bound should search bound set: %#v", got)
	}
	got = corporaForRetrieve(bound, "人事手册里年假几天")
	if len(got) != 1 || got[0].ID != b {
		t.Fatalf("name match within bound: %#v", got)
	}
	got = corporaForRetrieve(bound, "财务制度怎么报销")
	if len(got) != 2 {
		t.Fatalf("name outside bound should not expand: %#v", got)
	}
}

func TestChatDisplayTitle(t *testing.T) {
	t.Parallel()
	if got := chatDisplayTitle(&botCallback{ConversationTitle: " 采购群 ", ConversationType: "2"}); got != "采购群" {
		t.Fatalf("got %q", got)
	}
	if got := chatDisplayTitle(&botCallback{ConversationType: "2"}); got != "钉钉群聊" {
		t.Fatalf("got %q", got)
	}
	if got := chatDisplayTitle(&botCallback{ConversationType: "1"}); got != "钉钉单聊" {
		t.Fatalf("got %q", got)
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

func TestContinueWithHistory(t *testing.T) {
	t.Parallel()
	if continueWithHistory(nil, false, false) {
		t.Fatal("first turn miss should not continue")
	}
	if !continueWithHistory(nil, false, true) {
		t.Fatal("follow-up miss should continue")
	}
	if continueWithHistory([]rag.Hit{{Content: "x"}}, false, true) {
		t.Fatal("hits are not a miss")
	}
}

func TestSessionTitleIncludesNick(t *testing.T) {
	t.Parallel()
	got := sessionTitle(&botCallback{ConversationTitle: "采购群", ConversationType: "2", SenderNick: "张三"})
	if got != "采购群 · 张三" {
		t.Fatalf("got %q", got)
	}
}
