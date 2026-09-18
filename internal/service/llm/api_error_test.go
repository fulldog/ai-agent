package llm

import (
	"strings"
	"testing"
)

func TestParseAPIErrorInspection(t *testing.T) {
	t.Parallel()
	raw := []byte(`{"error":{"message":"Input data may contain inappropriate content.","type":"data_inspection_failed","code":"data_inspection_failed"}}`)
	err := parseAPIError(400, raw)
	if !IsInspectionFailed(err) {
		t.Fatalf("expected inspection: %v", err)
	}
	msg := PublicMessage(err)
	if strings.Contains(msg, "data_inspection_failed") || strings.Contains(msg, "{") {
		t.Fatalf("public message leaked upstream json: %q", msg)
	}
}

func TestPublicMessagePassthrough(t *testing.T) {
	t.Parallel()
	err := parseAPIError(500, []byte(`{"error":{"code":"internal","message":"boom"}}`))
	if IsInspectionFailed(err) {
		t.Fatal("not inspection")
	}
	if !strings.Contains(PublicMessage(err), "500") {
		t.Fatalf("got %q", PublicMessage(err))
	}
}
