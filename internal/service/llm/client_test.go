package llm

import "testing"

func TestBuildBodyEnableSearch(t *testing.T) {
	t.Parallel()
	c := NewClient("http://example.test", "k", 10)
	body := c.buildBody(ChatRequest{Model: "qwen-plus", EnableSearch: true}, false)
	if body["enable_search"] != true {
		t.Fatalf("enable_search: %#v", body)
	}
	opts, ok := body["search_options"].(map[string]any)
	if !ok || opts["forced_search"] != true {
		t.Fatalf("search_options: %#v", body["search_options"])
	}
	off := c.buildBody(ChatRequest{Model: "qwen-plus"}, false)
	if _, exists := off["enable_search"]; exists {
		t.Fatal("enable_search should be omitted when disabled")
	}
}

func TestBuildBodyEnableThinking(t *testing.T) {
	t.Parallel()
	c := NewClient("http://example.test", "k", 10)
	omit := c.buildBody(ChatRequest{Model: "qwen-plus"}, false)
	if _, exists := omit["enable_thinking"]; exists {
		t.Fatal("omit when sendThinking is false")
	}
	c.sendThinking = true
	c.enableThinking = false
	body := c.buildBody(ChatRequest{Model: "qwen-plus"}, false)
	if body["enable_thinking"] != false {
		t.Fatalf("want false, got %#v", body["enable_thinking"])
	}
	c.enableThinking = true
	on := c.buildBody(ChatRequest{Model: "qwen-plus"}, false)
	if on["enable_thinking"] != true {
		t.Fatalf("want true, got %#v", on["enable_thinking"])
	}
}
