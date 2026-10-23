package dingtalk

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/webapp/go-app/ai-agent/internal/config"
	"github.com/webapp/go-app/ai-agent/internal/model"
	"github.com/webapp/go-app/ai-agent/internal/service/rag"
	"go.uber.org/zap"
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

func TestUnboundDirectChat(t *testing.T) {
	t.Parallel()
	if !unboundDirectChat("1", 0) {
		t.Fatal("direct chat without a corpus should skip rag")
	}
	if unboundDirectChat("1", 2) {
		t.Fatal("bound direct chat should still retrieve")
	}
	if unboundDirectChat("2", 0) {
		t.Fatal("unbound group should still retrieve")
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

func TestStopForCorpusMiss(t *testing.T) {
	t.Parallel()
	if !stopForCorpusMiss(nil, false, false, false) {
		t.Fatal("first turn without dbconn should stop")
	}
	if stopForCorpusMiss(nil, false, false, true) {
		t.Fatal("dbconn should keep a data question alive")
	}
	if stopForCorpusMiss(nil, false, true, false) {
		t.Fatal("history should continue")
	}
	if stopForCorpusMiss(nil, true, false, false) {
		t.Fatal("forced online should continue")
	}
	if stopForCorpusMiss([]rag.Hit{{Content: "x"}}, false, false, false) {
		t.Fatal("hits should continue")
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

func TestSanitizeOutboundBlocksEvasion(t *testing.T) {
	t.Parallel()
	b := &Bot{log: zap.NewNop()}
	got := b.sanitizeOutbound(`当前未启用任何工具，且历史对话中无相关依据。请先在「工具函数管理」中开启对应工具后再问。`)
	if got == "" || strings.Contains(got, "工具函数管理") {
		t.Fatalf("should replace evasion: %q", got)
	}
	if got := b.sanitizeOutbound("乐推可以付款"); got != "乐推可以付款" {
		t.Fatalf("normal reply: %q", got)
	}
}

func TestAppendSourceNote(t *testing.T) {
	t.Parallel()
	got := appendSourceNote("答案", "来源：SOP（1.合同）")
	if got != "答案\n\n来源：SOP（1.合同）" {
		t.Fatalf("got %q", got)
	}
	if got := appendSourceNote("答案\n\n来源：SOP（1.合同）", "来源：SOP（1.合同）"); !strings.HasSuffix(got, "来源：SOP（1.合同）") || strings.Count(got, "来源：") != 1 {
		t.Fatalf("idempotent: %q", got)
	}
	if got := appendSourceNote("x", ""); got != "x" {
		t.Fatalf("empty note: %q", got)
	}
}

func TestMergeSourceLinesKeepsTagLast(t *testing.T) {
	t.Parallel()
	in := strings.Join([]string{
		"SRM 模块不支持查询余额。",
		"SRM 仅管理供应商建档、评级、政策、合同及付款申请流程，不涉及资金账户或余额数据。余额相关数据存储在 T_VendorMoneyEx（大账户备款）等外部表中，不在 SRM 的 33 张表范围内，需通过账单或天玑充值等其他系统查询。",
		"来源：SRM模块说明 §1、§5.6；SRM数据字典-表目录",
		"来源：SRM供应商系统-鸡哥（1.SRM模块说明.md 2.SRM接口调用链路.md 3.SRM接口请求响应.md）",
		"〔lianshan_test〕",
		"来源：SRM供应商系统-鸡哥（1.SRM模块说明.md 2.SRM接口调用链路.md）",
		"来源：SRM供应商系统-鸡哥（1.供应商动态管理规则V1.0 2.供应商管理规则 V2.0 3.SRM模块说明.md）",
	}, "\n")
	b := &Bot{cfg: config.DingTalkConfig{ReplyTag: "lianshan_test"}}
	got := b.decorateOutbound(&msgTrace{sourceNote: "来源：SRM供应商系统-鸡哥（1.SRM模块说明.md）"}, in)
	wantNote := "来源：SRM模块说明 §1、§5.6；SRM数据字典-表目录；SRM供应商系统-鸡哥（1.SRM模块说明.md 2.SRM接口调用链路.md 3.SRM接口请求响应.md 4.供应商动态管理规则V1.0 5.供应商管理规则 V2.0）"
	if strings.Count(got, "来源：") != 1 || !strings.Contains(got, wantNote) {
		t.Fatalf("merged note:\n%s", got)
	}
	if !strings.HasSuffix(got, "〔lianshan_test〕") {
		t.Fatalf("tag should be the last line:\n%s", got)
	}
	if strings.Count(got, "〔lianshan_test〕") != 1 {
		t.Fatalf("tag once:\n%s", got)
	}
	if !strings.HasPrefix(got, "SRM 模块不支持查询余额。") {
		t.Fatalf("body:\n%s", got)
	}
}

func TestFormatCorpusSourceNote(t *testing.T) {
	t.Parallel()
	got := formatCorpusSourceNote([]corpusSourceGroup{
		{Name: "付款SOP", Titles: []string{"付款条件", "合同模板"}},
	})
	if got != "来源：付款SOP（1.付款条件 2.合同模板）" {
		t.Fatalf("got %q", got)
	}
	got = formatCorpusSourceNote([]corpusSourceGroup{
		{Name: "财务制度", Titles: []string{"年假"}},
		{Name: "人事手册", Titles: []string{"入职"}},
	})
	if got != "来源：财务制度（1.年假）；人事手册（1.入职）" {
		t.Fatalf("multi: %q", got)
	}
}

func TestBuildCorpusSourceNote(t *testing.T) {
	t.Parallel()
	b := &Bot{}
	if got := b.buildCorpusSourceNote(nil); got != "" {
		t.Fatalf("empty hits: %q", got)
	}
	id1 := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	id2 := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	got := b.buildCorpusSourceNote([]rag.Hit{{CorpusID: id1}, {CorpusID: id1}, {CorpusID: id2}})
	if got != "来源：语料库" {
		t.Fatalf("no db: %q", got)
	}
}
