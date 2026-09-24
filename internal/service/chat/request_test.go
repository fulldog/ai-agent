package chat

import (
	"testing"

	"github.com/google/uuid"
	"github.com/webapp/go-app/ai-agent/internal/config"
)

func TestChatRequestEnableSearchOnlyQwen(t *testing.T) {
	t.Parallel()
	in := CompleteInput{EnableSearch: true}
	req := chatRequest(in, "qwen", "qwen-plus", nil)
	if !req.EnableSearch {
		t.Fatal("qwen should pass enable_search")
	}
	req = chatRequest(in, "deepseek", "deepseek-v4-flash", nil)
	if req.EnableSearch {
		t.Fatal("non-qwen must not send enable_search")
	}
}

func TestGetByChannelRequiresKeys(t *testing.T) {
	t.Parallel()
	s := &Service{}
	got, err := s.GetByChannel("", "dingtalk", "cid")
	if got != nil || err != nil {
		t.Fatalf("empty uid: %#v %v", got, err)
	}
	ok, err := s.HasMessages(uuid.Nil)
	if ok || err != nil {
		t.Fatalf("nil id: %v %v", ok, err)
	}
}

func TestMaxHistory(t *testing.T) {
	t.Parallel()
	if got := (&Service{}).maxHistory(); got != 10 {
		t.Fatalf("nil cfg: got %d", got)
	}
	s := &Service{cfg: &config.Config{}}
	s.cfg.LLM.MaxHistory = 6
	if got := s.maxHistory(); got != 6 {
		t.Fatalf("want 6, got %d", got)
	}
}
