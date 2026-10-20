package dingtalk

import (
	"errors"
	"testing"

	"github.com/webapp/go-app/ai-agent/internal/config"
	"github.com/webapp/go-app/ai-agent/internal/model"
	"go.uber.org/zap"
)

func TestSetChatLLM(t *testing.T) {
	db := openChatDB(t)
	b := &Bot{
		db:  db,
		log: zap.NewNop(),
		llm: config.LLMConfig{
			Providers: map[string]config.LLMProviderConfig{
				"qwen": {BaseURL: "https://example.com/v1", APIKey: "secret", DefaultModel: "qwen-plus"},
			},
		},
	}
	row := model.DingTalkChat{ConversationID: "cid-llm", ConversationType: "2"}
	if err := db.Create(&row).Error; err != nil {
		t.Fatal(err)
	}

	item, err := b.SetChatLLM(row.ID, "DashScope", "qwen-plus")
	if err != nil {
		t.Fatal(err)
	}
	if item.LLMProvider != "qwen" || item.LLMModel != "qwen-plus" {
		t.Fatalf("stored %#v", item)
	}
	provider, modelName := b.chatLLM("cid-llm")
	if provider != "qwen" || modelName != "qwen-plus" {
		t.Fatalf("loaded %s %s", provider, modelName)
	}

	if _, err := b.SetChatLLM(row.ID, "nope", "qwen-plus"); !errors.Is(err, ErrUnknownLLM) {
		t.Fatalf("unknown provider: %v", err)
	}
	provider, modelName = b.chatLLM("cid-llm")
	if provider != "qwen" || modelName != "qwen-plus" {
		t.Fatalf("unknown provider changed selection: %s %s", provider, modelName)
	}

	if _, err := b.SetChatLLM(row.ID, "qwen", "bad model"); !errors.Is(err, ErrInvalidLLMModel) {
		t.Fatalf("bad model: %v", err)
	}
	if _, err := b.SetChatLLM(row.ID, "qwen", ""); !errors.Is(err, ErrInvalidLLMModel) {
		t.Fatalf("half empty: %v", err)
	}

	if _, err := b.SetChatLLM(row.ID, "", ""); err != nil {
		t.Fatal(err)
	}
	provider, modelName = b.chatLLM("cid-llm")
	if provider != "" || modelName != "" {
		t.Fatalf("cleared: %s %s", provider, modelName)
	}
}
