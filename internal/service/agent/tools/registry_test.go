package tools

import "testing"

func TestRegistryListOrder(t *testing.T) {
	t.Parallel()
	r := Default()
	got := r.List()
	if len(got) != 1 || got[0].Name != "knowledge_search" {
		t.Fatalf("default list: %+v", got)
	}
	specs := r.Specs([]string{"missing", "knowledge_search"})
	if len(specs) != 1 || specs[0].Function.Name != "knowledge_search" {
		t.Fatalf("specs: %+v", specs)
	}
}

func TestMergeNamesKeepsDefaultsFirst(t *testing.T) {
	t.Parallel()
	got := MergeNames([]string{"knowledge_search", "dbconn"}, []string{"dbconn", "extra"}, []string{"later"})
	want := []string{"knowledge_search", "dbconn", "extra", "later"}
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
