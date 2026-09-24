package model

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

func TestConversationMarshalJSONDeletedAt(t *testing.T) {
	t.Parallel()
	id := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	c := Conversation{ID: id, UID: "u1", Title: "t"}
	b, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "deleted_at") {
		t.Fatalf("active should omit deleted_at: %s", b)
	}
	if strings.Contains(string(b), `"Valid"`) {
		t.Fatalf("must not emit gorm DeletedAt object: %s", b)
	}

	ts := time.Date(2026, 9, 24, 8, 0, 0, 0, time.UTC)
	c.DeletedAt = gorm.DeletedAt{Time: ts, Valid: true}
	b, err = json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	raw, ok := got["deleted_at"].(string)
	if !ok || !strings.HasPrefix(raw, "2026-09-24") {
		t.Fatalf("deleted_at string: %#v in %s", got["deleted_at"], b)
	}
}
