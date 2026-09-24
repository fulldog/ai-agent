package chat

import (
	"errors"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"github.com/webapp/go-app/ai-agent/internal/model"
	"gorm.io/gorm"
)

func testChatDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Conversation{}, &model.Message{}); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestDeleteConversationKeepsMessages(t *testing.T) {
	s := &Service{db: testChatDB(t)}
	conv, err := s.CreateConversation(CreateConversationInput{UID: "staff-1", Title: "t"})
	if err != nil {
		t.Fatal(err)
	}
	msg := model.Message{ConversationID: conv.ID, Role: "user", Content: "hi"}
	if err := s.db.Create(&msg).Error; err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteConversation(conv.ID, "staff-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetConversation(conv.ID, "staff-1"); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("live get should miss deleted: %v", err)
	}
	got, err := s.GetConversationIncludingDeleted(conv.ID, "staff-1")
	if err != nil || !got.DeletedAt.Valid {
		t.Fatalf("including deleted: %#v %v", got, err)
	}
	rows, total, err := s.QueryConversations("staff-1", false, false, 20, 0)
	if err != nil || total != 0 || len(rows) != 0 {
		t.Fatalf("default list: n=%d total=%d err=%v", len(rows), total, err)
	}
	rows, total, err = s.QueryConversations("staff-1", false, true, 20, 0)
	if err != nil || total != 1 || len(rows) != 1 || !rows[0].DeletedAt.Valid {
		t.Fatalf("include deleted: n=%d total=%d err=%v", len(rows), total, err)
	}
	msgs, err := s.ListMessages(conv.ID, "staff-1", 50)
	if err != nil || len(msgs) != 1 || msgs[0].Content != "hi" {
		t.Fatalf("messages kept: %#v %v", msgs, err)
	}
}

func TestFindOrCreateByChannelSkipsDeletedAndReusesLatest(t *testing.T) {
	s := &Service{db: testChatDB(t)}
	in := CreateConversationInput{
		UID: "staff-1", Title: "g · nick", Channel: "dingtalk", ChannelSessionID: "cid-9",
	}
	first, err := s.FindOrCreateByChannel(in)
	if err != nil {
		t.Fatal(err)
	}
	again, err := s.FindOrCreateByChannel(in)
	if err != nil || again.ID != first.ID {
		t.Fatalf("reuse live: %#v %v", again, err)
	}
	if err := s.DeleteConversation(first.ID, "staff-1"); err != nil {
		t.Fatal(err)
	}
	next, err := s.FindOrCreateByChannel(in)
	if err != nil {
		t.Fatal(err)
	}
	if next.ID == first.ID {
		t.Fatal("must not reuse deleted conversation")
	}
	live, err := s.GetByChannel("staff-1", "dingtalk", "cid-9")
	if err != nil || live == nil || live.ID != next.ID {
		t.Fatalf("get latest live: %#v %v", live, err)
	}
}

func TestLatestByChannelOrdersByCreatedAt(t *testing.T) {
	s := &Service{db: testChatDB(t)}
	old := &model.Conversation{
		UID: "staff-1", Title: "old", Channel: "dingtalk", ChannelSessionID: "cid-1",
		CreatedAt: time.Now().Add(-time.Hour),
	}
	newer := &model.Conversation{
		UID: "staff-1", Title: "new", Channel: "dingtalk", ChannelSessionID: "cid-1",
		CreatedAt: time.Now(),
	}
	if err := s.db.Create(old).Error; err != nil {
		t.Fatal(err)
	}
	if err := s.db.Create(newer).Error; err != nil {
		t.Fatal(err)
	}
	got, err := s.FindOrCreateByChannel(CreateConversationInput{
		UID: "staff-1", Channel: "dingtalk", ChannelSessionID: "cid-1",
	})
	if err != nil || got.ID != newer.ID {
		t.Fatalf("want newest live %s, got %#v %v", newer.ID, got, err)
	}
}

func TestGetConversationEmptyUID(t *testing.T) {
	t.Parallel()
	s := &Service{}
	if _, err := s.GetConversation(uuid.New(), ""); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("empty uid: %v", err)
	}
}
