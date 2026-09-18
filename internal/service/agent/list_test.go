package agent

import (
	"errors"
	"testing"

	"github.com/webapp/go-app/ai-agent/internal/model"
)

func TestNormalizeListRuns(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		uid        string
		all        bool
		limit      int
		offset     int
		wantUID    string
		wantLimit  int
		wantOffset int
		wantErr    error
	}{
		{name: "user missing uid", wantErr: errUIDRequired},
		{name: "admin empty uid", all: true, limit: 0, offset: -1, wantLimit: 20, wantOffset: 0},
		{name: "clamp high limit", uid: "u1", limit: 999, wantUID: "u1", wantLimit: 200},
		{name: "trim uid", uid: "  u1  ", wantUID: "u1", wantLimit: 20},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			uid, limit, offset, err := normalizeListRuns(tt.uid, tt.all, tt.limit, tt.offset)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("err=%v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if uid != tt.wantUID || limit != tt.wantLimit || offset != tt.wantOffset {
				t.Fatalf("got uid=%q limit=%d offset=%d", uid, limit, offset)
			}
		})
	}
}

func TestRunUIDMatch(t *testing.T) {
	t.Parallel()
	run := &model.AgentRun{UID: "u1"}
	if !runUIDMatch(run, "u1", false) {
		t.Fatal("owner should see own run")
	}
	if runUIDMatch(run, "u2", false) {
		t.Fatal("other user must not see run")
	}
	if !runUIDMatch(run, "u2", true) {
		t.Fatal("admin should see any run")
	}
	if runUIDMatch(&model.AgentRun{}, "u1", false) {
		t.Fatal("empty uid run is not owned")
	}
}
