package rag

import (
	"strings"
	"testing"
)

func TestFilterByMaxDistance(t *testing.T) {
	t.Parallel()
	hits := []Hit{
		{Content: "close", Score: 0.2},
		{Content: "edge", Score: 0.55},
		{Content: "far", Score: 0.9},
	}
	tests := []struct {
		name     string
		max      float64
		wantN    int
		wantLast string
	}{
		{name: "threshold keeps close and edge", max: 0.55, wantN: 2, wantLast: "edge"},
		{name: "zero disables filter", max: 0, wantN: 3, wantLast: "far"},
		{name: "negative disables filter", max: -1, wantN: 3, wantLast: "far"},
		{name: "strict drops all but closest", max: 0.2, wantN: 1, wantLast: "close"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := FilterByMaxDistance(hits, tc.max)
			if len(got) != tc.wantN {
				t.Fatalf("len=%d want %d (%#v)", len(got), tc.wantN, got)
			}
			if tc.wantN > 0 && got[len(got)-1].Content != tc.wantLast {
				t.Fatalf("last=%q want %q", got[len(got)-1].Content, tc.wantLast)
			}
		})
	}
	if FilterByMaxDistance(nil, 0.55) != nil {
		t.Fatal("nil hits should stay nil")
	}
}

func TestHitsPrompt(t *testing.T) {
	t.Parallel()
	if HitsPrompt(nil) != "" {
		t.Fatal("empty")
	}
	got := HitsPrompt([]Hit{{Content: "报销需发票"}})
	if !strings.Contains(got, HitsPromptHeader) || !strings.Contains(got, "[1] 报销需发票") {
		t.Fatalf("got %q", got)
	}
	if !strings.Contains(got, "前置条件") {
		t.Fatal("header should require following corpus workflow")
	}
}

func TestMaxDistanceGetter(t *testing.T) {
	t.Parallel()
	if (&Service{}).MaxDistance() != 0 {
		t.Fatal("zero service")
	}
	if (*Service)(nil).MaxDistance() != 0 {
		t.Fatal("nil service")
	}
	s := New(nil, nil, 0.55)
	if s.MaxDistance() != 0.55 {
		t.Fatalf("got %v", s.MaxDistance())
	}
}

func TestExpandDocumentsNilSafe(t *testing.T) {
	t.Parallel()
	hits := []Hit{{Content: "a", Score: 0.1}}
	if got := (*Service)(nil).expandDocuments(t.Context(), hits, 4); len(got) != 1 || got[0].Content != "a" {
		t.Fatalf("%+v", got)
	}
	if got := (&Service{}).expandDocuments(t.Context(), hits, 4); len(got) != 1 {
		t.Fatalf("%+v", got)
	}
	if got := (&Service{}).expandDocuments(t.Context(), nil, 4); got != nil {
		t.Fatalf("%+v", got)
	}
}
