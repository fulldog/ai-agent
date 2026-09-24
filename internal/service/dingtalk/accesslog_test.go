package dingtalk

import (
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"github.com/webapp/go-app/ai-agent/internal/model"
	"github.com/webapp/go-app/ai-agent/internal/service/llm"
	"github.com/webapp/go-app/ai-agent/internal/service/rag"
	"go.uber.org/zap"
	"gorm.io/gorm"
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
		data: &botCallback{
			ConversationID:    "cid-group-1",
			ConversationTitle: "财务群",
			ConversationType:  "2",
		},
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
	if !strings.Contains(raw, `"ding_conversation_id":"cid-group-1"`) {
		t.Fatalf("missing ding_conversation_id: %s", raw)
	}
	if !strings.Contains(raw, `"conversation_title":"财务群"`) {
		t.Fatalf("missing conversation_title: %s", raw)
	}
	if !strings.Contains(raw, `"group":true`) {
		t.Fatalf("missing group: %s", raw)
	}
	if !strings.Contains(raw, eventReceive) || !strings.Contains(raw, eventRAG) ||
		!strings.Contains(raw, eventLLMRequest) || !strings.Contains(raw, eventResult) {
		t.Fatalf("missing steps: %s", raw)
	}
}

func TestReceiveDetailFillsGroupFields(t *testing.T) {
	t.Parallel()
	got := receiveDetail(&botCallback{
		ConversationID:   "cid-1",
		ConversationType: "2",
		SenderNick:       "张三",
		MsgType:          "text",
		IsInAtList:       false,
	}, "staff-1")
	if got["ding_conversation_id"] != "cid-1" {
		t.Fatalf("ding id: %#v", got)
	}
	if got["group"] != true {
		t.Fatalf("group: %#v", got)
	}
	if got["conversation_title"] != "钉钉群聊" {
		t.Fatalf("default title: %#v", got)
	}
	if got["is_in_at_list"] != false {
		t.Fatalf("at list: %#v", got)
	}
}

func TestRecordSkipWritesOutcome(t *testing.T) {
	t.Parallel()
	b := &Bot{log: zap.NewNop(), access: zap.NewNop(), previewMax: 100, dedup: newMsgDeduper(0)}
	data := &botCallback{
		MsgID:            "skip-1",
		ConversationID:   "cid-skip",
		ConversationType: "2",
		SenderStaffID:    "u1",
		SenderNick:       "李四",
		MsgType:          "text",
		IsInAtList:       false,
		Text:             botText{Content: "hello"},
	}
	b.recordSkip(data, "not_in_at_list")
}

func TestIsBotMention(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		data *botCallback
		want bool
	}{
		{name: "nil", want: false},
		{name: "dm", data: &botCallback{ConversationType: "1"}, want: true},
		{name: "group no at", data: &botCallback{ConversationType: "2"}, want: false},
		{name: "group in at list", data: &botCallback{ConversationType: "2", IsInAtList: true}, want: true},
		{name: "group atUsers", data: &botCallback{ConversationType: "2", AtUsers: []botAtUser{{StaffID: "bot"}}}, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := isBotMention(tt.data); got != tt.want {
				t.Fatalf("got %v want %v", got, tt.want)
			}
		})
	}
}

func testRequestLogDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.RequestLog{}); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestBeginMentionTraceWritesRequestLog(t *testing.T) {
	t.Parallel()
	b := &Bot{
		log: zap.NewNop(), access: zap.NewNop(), previewMax: 100,
		db: testRequestLogDB(t), dedup: newMsgDeduper(0),
	}
	data := &botCallback{
		MsgID:            "msg-at-1",
		ConversationID:   "cid-at",
		ConversationType: "2",
		SenderStaffID:    "staff-9",
		MsgType:          "text",
		IsInAtList:       true,
		Text:             botText{Content: "@助手 年假几天"},
	}
	tr := b.beginMentionTrace(data)
	if tr.requestID != "msg-at-1" {
		t.Fatalf("request_id=%s", tr.requestID)
	}
	var row model.RequestLog
	if err := b.db.Where("request_id = ?", "msg-at-1").First(&row).Error; err != nil {
		t.Fatal(err)
	}
	if row.Method != accessMethod || row.Path != accessPath {
		t.Fatalf("method/path: %s %s", row.Method, row.Path)
	}
	if row.UID != "staff-9" || !strings.Contains(row.RequestBody, "dingtalk.receive") {
		t.Fatalf("row=%#v body=%s", row, row.RequestBody)
	}
}

func TestRecordSkipWritesRequestLog(t *testing.T) {
	t.Parallel()
	b := &Bot{
		log: zap.NewNop(), access: zap.NewNop(), previewMax: 100,
		db: testRequestLogDB(t),
	}
	b.recordSkip(&botCallback{
		MsgID: "msg-skip-db", ConversationType: "2", SenderStaffID: "u2",
		Text: botText{Content: "旁白"},
	}, "not_in_at_list")
	var n int64
	if err := b.db.Model(&model.RequestLog{}).Where("request_id = ?", "msg-skip-db").Count(&n).Error; err != nil || n != 1 {
		t.Fatalf("n=%d err=%v", n, err)
	}
}

func TestEnsureMentionLoggedFillsMissingRow(t *testing.T) {
	t.Parallel()
	b := &Bot{
		log: zap.NewNop(), access: zap.NewNop(), previewMax: 100,
		db: testRequestLogDB(t),
	}
	data := &botCallback{
		MsgID: "msg-dup-at", ConversationType: "2", IsInAtList: true, SenderStaffID: "u3",
		Text: botText{Content: "@助手 hi"},
	}
	if b.hasRequestLog(data.MsgID) {
		t.Fatal("expected missing")
	}
	b.ensureMentionLogged(data)
	if !b.hasRequestLog(data.MsgID) {
		t.Fatal("expected request_logs row for @ mention")
	}
}
