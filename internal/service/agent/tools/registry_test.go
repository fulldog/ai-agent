package tools

import "testing"

func TestRegistryListOrder(t *testing.T) {
	t.Parallel()
	r := Default()
	got := r.List()
	if len(got) != 3 {
		t.Fatalf("default list len=%d: %+v", len(got), got)
	}
	if got[0].Name != "knowledge_search" || got[1].Name != "current_time" || got[2].Name != "calculator" {
		t.Fatalf("default order: %+v", got)
	}
	specs := r.Specs([]string{"missing", "knowledge_search", "calculator"})
	if len(specs) != 2 || specs[0].Function.Name != "knowledge_search" || specs[1].Function.Name != "calculator" {
		t.Fatalf("specs: %+v", specs)
	}
}

func TestMergeNamesKeepsDefaultsFirst(t *testing.T) {
	t.Parallel()
	got := MergeNames([]string{"knowledge_search", "dbconn"}, []string{"dbconn", "calculator"}, []string{"current_time"})
	want := []string{"knowledge_search", "dbconn", "calculator", "current_time"}
	if len(got) != len(want) {
		t.Fatalf("got %#v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %#v want %#v", got, want)
		}
	}
	if got := MergeNames(nil, []string{"a"}); len(got) != 1 || got[0] != "a" {
		t.Fatalf("extras only: %#v", got)
	}
	if got := MergeNames([]string{"a"}, nil); len(got) != 1 || got[0] != "a" {
		t.Fatalf("base only: %#v", got)
	}
}
