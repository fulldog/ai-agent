package dbconn

import (
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestLogQueryWritesSQL(t *testing.T) {
	t.Parallel()
	core, recorded := observer.New(zap.InfoLevel)
	c := &Client{log: zap.New(core)}
	sqlText := "SELECT VendorId FROM Srm_VendorInfo WHERE VendorName = '北京字节跳动科技有限公司'"

	c.logQuery(sqlText, 20, false, 15*time.Millisecond, nil)

	entries := recorded.All()
	if len(entries) != 1 {
		t.Fatalf("entries=%d", len(entries))
	}
	if entries[0].Level != zap.InfoLevel || entries[0].Message != "dbconn query" {
		t.Fatalf("entry=%+v", entries[0])
	}
	if got := entries[0].ContextMap()["sql"]; got != sqlText {
		t.Fatalf("sql=%v", got)
	}
	if got := entries[0].ContextMap()["rows"]; got != int64(20) {
		t.Fatalf("rows=%v", got)
	}
}

func TestLogQueryKeepsSQLOnError(t *testing.T) {
	t.Parallel()
	core, recorded := observer.New(zap.InfoLevel)
	c := &Client{log: zap.New(core)}
	c.logQuery("SELECT 1", 0, false, time.Millisecond, errQuery)

	entries := recorded.All()
	if len(entries) != 1 || entries[0].Level != zap.ErrorLevel {
		t.Fatalf("entries=%v", entries)
	}
	if got := entries[0].ContextMap()["sql"]; got != "SELECT 1" {
		t.Fatalf("sql=%v", got)
	}
}

var errQuery = errString("查询失败")

type errString string

func (e errString) Error() string { return string(e) }
