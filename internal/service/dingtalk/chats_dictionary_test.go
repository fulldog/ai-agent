package dingtalk

import (
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/webapp/go-app/ai-agent/internal/model"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

func TestSetChatDictionaryRoundTrip(t *testing.T) {
	db := openChatDB(t)
	if err := db.AutoMigrate(&model.Document{}); err != nil {
		t.Fatal(err)
	}
	b := &Bot{db: db, log: zap.NewNop()}
	row := model.DingTalkChat{ConversationID: "cid-dict", Title: "字典群", ConversationType: "2"}
	if err := db.Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	doc := model.Document{CorpusID: uuid.New(), Title: "SRM 数据字典", Source: "srm.md", Kind: "text", Status: "indexed"}
	if err := db.Create(&doc).Error; err != nil {
		t.Fatal(err)
	}

	item, err := b.SetChatDictionary(row.ID, &doc.ID)
	if err != nil {
		t.Fatal(err)
	}
	if item.DictionaryDocumentID != doc.ID.String() {
		t.Fatalf("stored %q", item.DictionaryDocumentID)
	}
	var stored model.DingTalkChat
	if err := db.First(&stored, "id = ?", row.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.DictionaryDocumentID == nil || *stored.DictionaryDocumentID != doc.ID {
		t.Fatalf("db %#v", stored.DictionaryDocumentID)
	}

	listed, err := b.ListChats()
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || listed[0].DictionaryDocumentID != doc.ID.String() {
		t.Fatalf("list %#v", listed)
	}

	cleared, err := b.SetChatDictionary(row.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if cleared.DictionaryDocumentID != "" {
		t.Fatalf("cleared %q", cleared.DictionaryDocumentID)
	}
	var after model.DingTalkChat
	if err := db.First(&after, "id = ?", row.ID).Error; err != nil {
		t.Fatal(err)
	}
	if after.DictionaryDocumentID != nil {
		t.Fatalf("expected nil, got %v", after.DictionaryDocumentID)
	}
}

func TestChatDictionaryLoadsConfiguredDocument(t *testing.T) {
	db := openChatDB(t)
	if err := db.AutoMigrate(&model.Document{}); err != nil {
		t.Fatal(err)
	}
	b := &Bot{db: db, log: zap.NewNop()}
	doc := model.Document{
		CorpusID: uuid.New(), Title: "SRM", Source: "srm.md", Kind: "text", Status: "indexed",
		Content: "## Srm_VendorInfo\n供应商名称\n",
	}
	if err := db.Create(&doc).Error; err != nil {
		t.Fatal(err)
	}
	row := model.DingTalkChat{
		ConversationID: "cid-dict-text", ConversationType: "2", DictionaryDocumentID: &doc.ID,
	}
	if err := db.Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	if got := b.chatDictionary("cid-dict-text"); !strings.Contains(got, "Srm_VendorInfo") {
		t.Fatalf("got %q", got)
	}
	if got := b.chatDictionary("missing"); got != "" {
		t.Fatalf("missing chat %q", got)
	}
	row.DictionaryDocumentID = nil
	if err := db.Model(&model.DingTalkChat{}).Where("id = ?", row.ID).Update("dictionary_document_id", gorm.Expr("NULL")).Error; err != nil {
		t.Fatal(err)
	}
	if got := b.chatDictionary("cid-dict-text"); got != "" {
		t.Fatalf("cleared %q", got)
	}
}

func TestSetChatDictionaryMissingDocument(t *testing.T) {
	db := openChatDB(t)
	if err := db.AutoMigrate(&model.Document{}); err != nil {
		t.Fatal(err)
	}
	b := &Bot{db: db, log: zap.NewNop()}
	row := model.DingTalkChat{ConversationID: "cid-missing-doc", ConversationType: "2"}
	if err := db.Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	missing := uuid.New()
	_, err := b.SetChatDictionary(row.ID, &missing)
	if !errors.Is(err, ErrDocumentNotFound) {
		t.Fatalf("err: %v", err)
	}
}
