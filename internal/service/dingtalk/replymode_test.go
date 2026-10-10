package dingtalk

import (
	"errors"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/webapp/go-app/ai-agent/internal/config"
	"github.com/webapp/go-app/ai-agent/internal/model"
	"go.uber.org/zap"
	"gorm.io/gorm"
	glogger "gorm.io/gorm/logger"
)

func openReplyModeDB(t *testing.T) *gorm.DB {
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
	if err := db.AutoMigrate(&model.AppSetting{}); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestSetReplyModePersistsAndReloads(t *testing.T) {
	db := openReplyModeDB(t)
	b := &Bot{
		cfg: config.DingTalkConfig{ReplyMode: config.DingTalkReplyChat},
		db:  db,
		log: zap.NewNop(),
	}
	if b.ReplyMode() != config.DingTalkReplyChat || b.ReplyModeView().FromStore {
		t.Fatalf("initial: %#v", b.ReplyModeView())
	}
	if err := b.SetReplyMode("AGENT"); err != nil {
		t.Fatal(err)
	}
	if b.ReplyMode() != config.DingTalkReplyAgent || !b.ReplyModeView().FromStore {
		t.Fatalf("after set: %#v", b.ReplyModeView())
	}

	reloaded := &Bot{
		cfg: config.DingTalkConfig{ReplyMode: config.DingTalkReplyWeb},
		db:  db,
		log: zap.NewNop(),
	}
	reloaded.loadStoredReplyMode()
	view := reloaded.ReplyModeView()
	if view.ReplyMode != config.DingTalkReplyAgent || !view.FromStore {
		t.Fatalf("reloaded: %#v", view)
	}
}

func TestSetReplyModeRejectsUnknownAndKeepsCurrent(t *testing.T) {
	db := openReplyModeDB(t)
	b := &Bot{
		cfg: config.DingTalkConfig{ReplyMode: config.DingTalkReplyWeb},
		db:  db,
		log: zap.NewNop(),
	}
	err := b.SetReplyMode("weird")
	if !errors.Is(err, ErrInvalidReplyMode) {
		t.Fatalf("err: %v", err)
	}
	if b.ReplyMode() != config.DingTalkReplyWeb || b.ReplyModeView().FromStore {
		t.Fatalf("unchanged: %#v", b.ReplyModeView())
	}
}

func TestSetReplyModeRequiresDatabase(t *testing.T) {
	b := &Bot{cfg: config.DingTalkConfig{ReplyMode: config.DingTalkReplyWeb}, log: zap.NewNop()}
	err := b.SetReplyMode(config.DingTalkReplyChat)
	if !errors.Is(err, ErrReplyModeStore) {
		t.Fatalf("err: %v", err)
	}
	if b.ReplyMode() != config.DingTalkReplyWeb {
		t.Fatalf("mode: %s", b.ReplyMode())
	}
}

func TestLoadStoredReplyModeIgnoresIllegalValue(t *testing.T) {
	db := openReplyModeDB(t)
	if err := db.Create(&model.AppSetting{Key: replyModeSettingKey, Value: "nope"}).Error; err != nil {
		t.Fatal(err)
	}
	b := &Bot{
		cfg: config.DingTalkConfig{ReplyMode: config.DingTalkReplyAgent},
		db:  db,
		log: zap.NewNop(),
	}
	b.loadStoredReplyMode()
	view := b.ReplyModeView()
	if view.ReplyMode != config.DingTalkReplyAgent || view.FromStore {
		t.Fatalf("view: %#v", view)
	}
}
