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
