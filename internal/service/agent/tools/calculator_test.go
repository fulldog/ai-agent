package tools_test

import (
	"context"
	"testing"

	"github.com/webapp/go-app/ai-agent/internal/service/agent/tools"
)

func TestCalc(t *testing.T) {
	got, err := tools.Calc(123.45, 6.7, "*")
	if err != nil {
		t.Fatal(err)
	}
	if got != "827.115" {
		t.Fatalf("got %q", got)
	}
}

func TestRegistrySpecsAndUnknown(t *testing.T) {
	reg := tools.Default()
	specs := reg.Specs([]string{"knowledge_search", "missing"})
	if len(specs) != 1 {
		t.Fatalf("len=%d", len(specs))
	}
	if specs[0].Function.Name != "knowledge_search" {
		t.Fatalf("names=%s", specs[0].Function.Name)
	}
	if specs[0].Function.Description == "" {
		t.Fatal("empty chinese description")
	}
	_, err := reg.Exec(context.Background(), "nope", "{}", nil)
	if err == nil {
		t.Fatal("expected unknown tool error")
	}
}
