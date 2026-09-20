package tools

import "testing"

func TestRegistryListOrder(t *testing.T) {
	t.Parallel()
	r := Default()
	got := r.List()
	if len(got) != 1 || got[0].Name != "knowledge_search" || got[0].Description == "" {
		t.Fatalf("default list: %+v", got)
	}
	if len(r.Specs([]string{"current_time", "knowledge_search"})) != 1 {
		t.Fatal("unregistered names must be skipped")
	}
}
