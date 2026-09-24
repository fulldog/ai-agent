package handler

import (
	"testing"

	"github.com/google/uuid"
)

func TestParseCorpusIDs(t *testing.T) {
	t.Parallel()
	got, err := parseCorpusIDs(nil)
	if err != nil || len(got) != 0 {
		t.Fatalf("nil: %#v %v", got, err)
	}
	id := uuid.New()
	got, err = parseCorpusIDs([]string{id.String()})
	if err != nil || len(got) != 1 || got[0] != id {
		t.Fatalf("one: %#v %v", got, err)
	}
	if _, err = parseCorpusIDs([]string{"not-a-uuid"}); err == nil {
		t.Fatal("want invalid uuid error")
	}
}
