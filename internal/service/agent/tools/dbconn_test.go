package tools_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/webapp/go-app/ai-agent/internal/service/agent/tools"
	"github.com/webapp/go-app/ai-agent/internal/service/dbconn"
)

func TestDBConnNilClient(t *testing.T) {
	t.Parallel()
	tool := tools.DBConn{}
	if tool.Name() != "dbconn" {
		t.Fatalf("name=%s", tool.Name())
	}
	if tool.Spec().Function.Description == "" {
		t.Fatal("empty description")
	}
	_, err := tool.Exec(context.Background(), json.RawMessage(`{"action":"schema"}`), nil)
	if err == nil || !strings.Contains(err.Error(), "未配置") {
		t.Fatalf("want 未配置, got %v", err)
	}
}

func TestDBConnArgs(t *testing.T) {
	t.Parallel()
	tool := tools.DBConn{Client: &dbconn.Client{}}
	_, err := tool.Exec(context.Background(), json.RawMessage(`{`), nil)
	if err == nil || !strings.Contains(err.Error(), "参数无效") {
		t.Fatalf("bad json: %v", err)
	}
	_, err = tool.Exec(context.Background(), json.RawMessage(`{"action":""}`), nil)
	if err == nil || !strings.Contains(err.Error(), "action 不能为空") {
		t.Fatalf("empty action: %v", err)
	}
	_, err = tool.Exec(context.Background(), json.RawMessage(`{"action":"nope"}`), nil)
	if err == nil || !strings.Contains(err.Error(), "未知 action") {
		t.Fatalf("unknown action: %v", err)
	}
	_, err = tool.Exec(context.Background(), json.RawMessage(`{"action":"query"}`), nil)
	if err == nil || !strings.Contains(err.Error(), "sql 不能为空") {
		t.Fatalf("empty sql: %v", err)
	}
	_, err = tool.Exec(context.Background(), json.RawMessage(`{"action":"schema"}`), nil)
	if err == nil || !strings.Contains(err.Error(), "未配置") {
		t.Fatalf("schema without db: %v", err)
	}
}

func TestRegistryDBConn(t *testing.T) {
	t.Parallel()
	if n := len(tools.Default().Specs([]string{"dbconn", "knowledge_search"})); n != 1 {
		t.Fatalf("default must hide dbconn, got %d specs", n)
	}
	if n := len(tools.WithDBConn(nil).Specs([]string{"dbconn"})); n != 0 {
		t.Fatal("nil client must not register dbconn")
	}
	specs := tools.WithDBConn(&dbconn.Client{}).Specs([]string{"dbconn"})
	if len(specs) != 1 || specs[0].Function.Name != "dbconn" {
		t.Fatalf("expected dbconn spec, got %#v", specs)
	}
}
