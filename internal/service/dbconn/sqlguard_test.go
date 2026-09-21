package dbconn

import (
	"strings"
	"testing"
)

func TestValidateSelect(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		sql     string
		wantErr string
	}{
		{name: "select", sql: "SELECT id, name FROM vendor WHERE name = 'a'"},
		{name: "select trailing semicolon", sql: "SELECT 1;"},
		{name: "cte", sql: "WITH x AS (SELECT 1 AS n) SELECT n FROM x"},
		{name: "empty", sql: "   ", wantErr: "sql 不能为空"},
		{name: "multi statement", sql: "SELECT 1; DELETE FROM t", wantErr: "禁止多条"},
		{name: "insert", sql: "INSERT INTO t VALUES (1)", wantErr: "仅允许 SELECT"},
		{name: "update", sql: "UPDATE t SET a=1", wantErr: "仅允许"},
		{name: "delete", sql: "DELETE FROM t", wantErr: "仅允许"},
		{name: "drop", sql: "DROP TABLE t", wantErr: "仅允许"},
		{name: "select into outfile", sql: "SELECT * FROM t INTO OUTFILE '/tmp/x'", wantErr: "只读"},
		{name: "for update", sql: "SELECT * FROM t FOR UPDATE", wantErr: "只读"},
		{name: "comment hide delete", sql: "SELECT 1; /* */ DELETE FROM t", wantErr: "禁止多条"},
		{name: "line comment then write", sql: "SELECT 1 --\n; DROP TABLE t", wantErr: "禁止多条"},
		{name: "executable comment", sql: "SELECT /*!50000 1 */", wantErr: "可执行注释"},
		{name: "keyword in string allowed", sql: "SELECT 'DROP TABLE t' AS x"},
		{name: "set", sql: "SET names utf8", wantErr: "仅允许"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := ValidateSelect(tt.sql)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected err: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("want error containing %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("err=%q want substring %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestSplitTableIdent(t *testing.T) {
	t.Parallel()
	schema, table, err := splitTableIdent("vendor")
	if err != nil || schema != "" || table != "vendor" {
		t.Fatalf("got %q %q %v", schema, table, err)
	}
	schema, table, err = splitTableIdent("`biz`.`vendor`")
	if err != nil || schema != "biz" || table != "vendor" {
		t.Fatalf("got %q %q %v", schema, table, err)
	}
	if _, _, err = splitTableIdent("a;b"); err == nil {
		t.Fatal("expected illegal table")
	}
	if _, _, err = splitTableIdent(""); err == nil {
		t.Fatal("expected empty table")
	}
}

func TestHasTablePrefix(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		ok   bool
	}{
		{name: "Srm_VendorInfo", ok: true},
		{name: "SrmOrder", ok: true},
		{name: "  Srm_x  ", ok: true},
		{name: "srm_vendor", ok: false},
		{name: "SRM_VENDOR", ok: false},
		{name: "vendor", ok: false},
		{name: "asrm_x", ok: false},
		{name: "", ok: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := hasTablePrefix(tt.name); got != tt.ok {
				t.Fatalf("hasTablePrefix(%q)=%v want %v", tt.name, got, tt.ok)
			}
		})
	}
}

func TestLikeContains(t *testing.T) {
	t.Parallel()
	got := likeContains(`a%b_c`)
	if got != `%a\%b\_c%` {
		t.Fatalf("got %q", got)
	}
}
