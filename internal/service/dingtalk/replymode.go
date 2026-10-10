package dingtalk

import (
	"errors"
	"fmt"
	"time"

	"github.com/webapp/go-app/ai-agent/internal/config"
	"github.com/webapp/go-app/ai-agent/internal/model"
	"go.uber.org/zap"
	"gorm.io/gorm/clause"
)

const replyModeSettingKey = "dingtalk.reply_mode"

var (
	// ErrInvalidReplyMode 不是 chat、agent、web 之一。
	ErrInvalidReplyMode = errors.New("invalid reply mode")
	// ErrReplyModeStore 没有数据库，无法持久化回复模式。
	ErrReplyModeStore = errors.New("reply mode store unavailable")
)

// ReplyModeOption 控制台下拉项。
type ReplyModeOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
	Hint  string `json:"hint"`
}

// ReplyModeView 当前生效的钉钉回复模式。
type ReplyModeView struct {
	ReplyMode string            `json:"reply_mode"`
	FromStore bool              `json:"from_store"`
	Options   []ReplyModeOption `json:"options"`
}

// ReplyModeOptions 固定的三种回复模式。
func ReplyModeOptions() []ReplyModeOption {
	return []ReplyModeOption{
		{Value: config.DingTalkReplyChat, Label: "对话（CompleteStream）", Hint: "钉钉消息走对话 Completions，预检索结果注入 system。"},
		{Value: config.DingTalkReplyAgent, Label: "钉钉预检索 Agent", Hint: "先检索语料，再走 Agent.Run。"},
		{Value: config.DingTalkReplyWeb, Label: "同控制台 Agent 薄包装", Hint: "与控制台 Agent 页同一套 Run，不预注入检索结果。"},
	}
}

// ReplyMode 当前生效模式。未设置或非法值视为 chat。
func (b *Bot) ReplyMode() string {
	if b == nil {
		return config.DingTalkReplyChat
	}
	b.modeMu.RLock()
	defer b.modeMu.RUnlock()
	return canonicalReplyMode(b.cfg.ReplyMode)
}

// ReplyModeView 返回当前模式、是否来自数据库，以及下拉选项。
func (b *Bot) ReplyModeView() ReplyModeView {
	if b == nil {
		return ReplyModeView{ReplyMode: config.DingTalkReplyChat, Options: ReplyModeOptions()}
	}
	b.modeMu.RLock()
	defer b.modeMu.RUnlock()
	return ReplyModeView{
		ReplyMode: canonicalReplyMode(b.cfg.ReplyMode),
		FromStore: b.replyModeFromStore,
		Options:   ReplyModeOptions(),
	}
}

// SetReplyMode 校验并写入数据库，立即对后续钉钉消息生效。
func (b *Bot) SetReplyMode(mode string) error {
	if b == nil || b.db == nil {
		return ErrReplyModeStore
	}
	parsed, ok := config.ParseDingTalkReplyMode(mode)
	if !ok {
		return ErrInvalidReplyMode
	}
	if err := b.saveReplyMode(parsed); err != nil {
		return err
	}
	b.modeMu.Lock()
	b.cfg.ReplyMode = parsed
	b.replyModeFromStore = true
	b.modeMu.Unlock()
	if b.log != nil {
		b.log.Info("dingtalk reply mode updated", zap.String("reply_mode", parsed))
	}
	return nil
}

func (b *Bot) loadStoredReplyMode() {
	if b == nil || b.db == nil {
		return
	}
	var row model.AppSetting
	err := b.db.Where(&model.AppSetting{Key: replyModeSettingKey}).Limit(1).Find(&row).Error
	if err != nil {
		if b.log != nil {
			b.log.Warn("load dingtalk reply mode failed", zap.Error(err))
		}
		return
	}
	if row.Key == "" {
		return
	}
	mode, ok := config.ParseDingTalkReplyMode(row.Value)
	if !ok {
		if b.log != nil {
			b.log.Warn("stored dingtalk reply mode ignored", zap.String("reply_mode", row.Value))
		}
		return
	}
	b.modeMu.Lock()
	b.cfg.ReplyMode = mode
	b.replyModeFromStore = true
	b.modeMu.Unlock()
}

func (b *Bot) saveReplyMode(mode string) error {
	row := model.AppSetting{
		Key:       replyModeSettingKey,
		Value:     mode,
		UpdatedAt: time.Now(),
	}
	err := b.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "setting_key"}},
		DoUpdates: clause.AssignmentColumns([]string{"value", "updated_at"}),
	}).Create(&row).Error
	if err != nil {
		return fmt.Errorf("save dingtalk reply mode: %w", err)
	}
	return nil
}

func canonicalReplyMode(v string) string {
	mode, ok := config.ParseDingTalkReplyMode(v)
	if !ok {
		return config.DingTalkReplyChat
	}
	return mode
}
