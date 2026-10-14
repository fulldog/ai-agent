package dingtalk

import (
	"errors"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/webapp/go-app/ai-agent/internal/model"
	"go.uber.org/zap"
	"gorm.io/gorm"
	glogger "gorm.io/gorm/logger"
)

func openChatDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := "file:" + strings.ReplaceAll(t.Name(), "/", "_") + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: glogger.Default.LogMode(glogger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&model.DingTalkChat{}, &model.DingTalkChatCorpus{}, &model.Corpus{}); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestSetChatPrefixRoundTrip(t *testing.T) {
	db := openChatDB(t)
	b := &Bot{db: db, log: zap.NewNop()}
	row := model.DingTalkChat{ConversationID: "cid-srm", Title: "SRM AI 助手测试群", ConversationType: "2"}
	if err := db.Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	const prompt = "查询供应商必须同时有名称和标签。只给名称时先提示补充标签，不要查库。"
	item, err := b.SetChatPrefix(row.ID, "  "+prompt+"  ")
	if err != nil {
		t.Fatal(err)
	}
	if item.PrefixPrompt != prompt {
		t.Fatalf("stored %q", item.PrefixPrompt)
	}
	got, skip := b.chatPrefix("cid-srm")
	if !skip || !strings.Contains(got, prompt) || !strings.Contains(got, "必须先调用工具") {
		t.Fatalf("injected skip=%v %q", skip, got)
	}
	const queryFirst = "供应商问题必须先调dbconn工具查询。没有标签时按名称查，多条再追问标签。"
	if _, err := b.SetChatPrefix(row.ID, queryFirst); err != nil {
		t.Fatal(err)
	}
	got, skip = b.chatPrefix("cid-srm")
	if skip || !strings.Contains(got, queryFirst) || !strings.Contains(got, "必须先调用工具") {
		t.Fatalf("query-first skip=%v %q", skip, got)
	}
	if _, err := b.SetChatPrefix(row.ID, "  "); err != nil {
		t.Fatal(err)
	}
	got, skip = b.chatPrefix("cid-srm")
	if got != "" || skip {
		t.Fatal("expected cleared prefix")
	}
}

func TestSetChatPrefixTooLong(t *testing.T) {
	db := openChatDB(t)
	b := &Bot{db: db, log: zap.NewNop()}
	row := model.DingTalkChat{ConversationID: "cid-long", ConversationType: "2"}
	if err := db.Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	_, err := b.SetChatPrefix(row.ID, strings.Repeat("字", 4001))
	if !errors.Is(err, ErrPrefixTooLong) {
		t.Fatalf("err: %v", err)
	}
}

func TestWrapPrefixPromptEmpty(t *testing.T) {
	t.Parallel()
	if wrapPrefixPrompt("  ") != "" {
		t.Fatal("blank prompt should stay empty")
	}
}
