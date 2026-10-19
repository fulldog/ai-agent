package tools_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/webapp/go-app/ai-agent/internal/service/agent/tools"
	"github.com/webapp/go-app/ai-agent/internal/service/dbconn"
)

func TestRecordExecutedSQL(t *testing.T) {
	t.Parallel()
	var got string
	env := &tools.Env{SaveSQL: func(sql string) { got = sql }}
	env.RecordExecutedSQL("  SELECT VendorId FROM Srm_VendorInfo  ")
	if got != "SELECT VendorId FROM Srm_VendorInfo" {
		t.Fatalf("got %q", got)
	}
	env.RecordExecutedSQL("   ")
	if got != "SELECT VendorId FROM Srm_VendorInfo" {
		t.Fatalf("blank sql replaced history: %q", got)
	}
}

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

func TestDBConnQueryPrecheckUsesDictionary(t *testing.T) {
	t.Parallel()
	tool := tools.DBConn{Client: &dbconn.Client{}}
	dict := "### Srm_VendorInfo\n\n- VendorName varchar(256) NN | 供应商名称\n"
	env := &tools.Env{Dictionary: dict}
	_, err := tool.Exec(context.Background(), json.RawMessage(`{"action":"query","sql":"SELECT VendorName FROM Srm_NoSuch"}`), env)
	if err == nil || !strings.Contains(err.Error(), "数据字典预检") || strings.Contains(err.Error(), "未配置") {
		t.Fatalf("bad table should fail before the database: %v", err)
	}
	_, err = tool.Exec(context.Background(), json.RawMessage(`{"action":"query","sql":"SELECT VendorName FROM Srm_VendorInfo"}`), env)
	if err == nil || !strings.Contains(err.Error(), "未配置") || strings.Contains(err.Error(), "预检") {
		t.Fatalf("known table should reach the database: %v", err)
	}
	_, err = tool.Exec(context.Background(), json.RawMessage(`{"action":"query","sql":"DELETE FROM Srm_VendorInfo"}`), env)
	if err == nil || !strings.Contains(err.Error(), "仅允许") {
		t.Fatalf("write: %v", err)
	}
}

func TestDBConnSchemaUsesConfiguredCorpus(t *testing.T) {
	t.Parallel()
	tool := tools.DBConn{Client: &dbconn.Client{}}
	env := &tools.Env{Dictionary: "# 字典\n\n## Srm_VendorInfo\n\n供应商名称\n"}
	out, err := tool.Exec(context.Background(), json.RawMessage(`{"action":"schema","table":"Srm_VendorInfo"}`), env)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "供应商名称") {
		t.Fatalf("got:\n%s", out)
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
