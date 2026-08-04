package tools_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/webapp/go-app/ai-agent/internal/service/agent/tools"
)

func TestCalculator(t *testing.T) {
	c := tools.Calculator{}
	args, _ := json.Marshal(map[string]any{"a": 123.45, "b": 6.7, "op": "*"})
	got, err := c.Exec(context.Background(), args, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got != "827.115" {
		t.Fatalf("got %q", got)
	}
}

func TestRegistrySpecsAndUnknown(t *testing.T) {
	reg := tools.Default()
	specs := reg.Specs([]string{"calculator", "missing", "current_time"})
	if len(specs) != 2 {
		t.Fatalf("len=%d", len(specs))
	}
	if specs[0].Function.Name != "calculator" {
		t.Fatalf("name=%s", specs[0].Function.Name)
	}
	if specs[0].Function.Description == "" {
		t.Fatal("empty chinese description")
	}
	_, err := reg.Exec(context.Background(), "nope", "{}", nil)
	if err == nil {
		t.Fatal("expected unknown tool error")
	}
}

func TestCurrentTime(t *testing.T) {
	got, err := tools.CurrentTime{}.Exec(context.Background(), nil, nil)
	if err != nil || got == "" {
		t.Fatalf("got %q err=%v", got, err)
	}
}
