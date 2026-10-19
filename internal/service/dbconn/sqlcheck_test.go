package dbconn

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const miniDict = `
## 写 SQL 前先看

- 有 IsDelete 的表加 IsDelete = 0。

### Srm_VendorInfo

- VendorId varchar(64) NN PK | 供应商Id
- VendorName varchar(256) NN | 供应商名称
- VendorLabel int(11) NN PK | 标签
- IsDelete bit(1) NN PK | 是否删除

### Srm_Vendor_Main_Info

- vendorId varchar(64) NN | 供应商Id
- VendorLabel int(11) NN | 标签
- main_business varchar(512) NN | 主营业务
- IsDelete bit(1) NN | 是否删除

### Srm_PayInfo

- VendorId varchar(64) NN | 供应商Id
- BankName varchar(128) NN | 开户行
- IsDelete bit(1) NN | 是否删除

### Srm_Vendor_Draft

- business varchar(64) NN | 业务类型
- is_delete bit(1) NN | 是否删除
`

func TestCheckSQLRewritesIdentifiers(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		sql  string
		want string
	}{
		{
			name: "case",
			sql:  "SELECT vendorname, vendorid FROM srm_vendorinfo WHERE isdelete = 0",
			want: "SELECT VendorName, VendorId FROM Srm_VendorInfo WHERE IsDelete = 0",
		},
		{
			name: "qualified and main info case",
			sql: `SELECT m.VendorId, m.main_business
FROM Srm_VendorInfo v
JOIN Srm_Vendor_Main_Info m ON m.vendorid = v.VendorId AND m.VendorLabel = v.VendorLabel
WHERE v.IsDelete = 0 AND m.IsDelete = 0`,
			want: `SELECT m.vendorId, m.main_business
FROM Srm_VendorInfo v
JOIN Srm_Vendor_Main_Info m ON m.vendorId = v.VendorId AND m.VendorLabel = v.VendorLabel
WHERE v.IsDelete = 0 AND m.IsDelete = 0`,
		},
		{
			name: "alias and backticks",
			sql:  "SELECT `vendorname` FROM `srm_vendorinfo` AS v WHERE v.`isdelete` = 0",
			want: "SELECT `VendorName` FROM `Srm_VendorInfo` AS v WHERE v.`IsDelete` = 0",
		},
		{
			name: "string and comment stay",
			sql:  "SELECT 'srm_vendorinfo' AS name /* vendorname */ FROM Srm_VendorInfo",
			want: "SELECT 'srm_vendorinfo' AS name /* vendorname */ FROM Srm_VendorInfo",
		},
		{
			name: "chinese alias",
			sql:  "SELECT vendorname 名称 FROM srm_vendorinfo",
			want: "SELECT VendorName 名称 FROM Srm_VendorInfo",
		},
		{
			name: "star and schema",
			sql:  "SELECT v.* FROM crm.srm_vendorinfo v",
			want: "SELECT v.* FROM crm.Srm_VendorInfo v",
		},
		{
			name: "fuzzy column and table",
			sql:  "SELECT VendorNam FROM Srm_VendorInf",
			want: "SELECT VendorName FROM Srm_VendorInfo",
		},
		{
			name: "draft delete column",
			sql:  "SELECT business FROM Srm_Vendor_Draft WHERE isdelete = 0",
			want: "SELECT business FROM Srm_Vendor_Draft WHERE is_delete = 0",
		},
		{
			name: "subquery",
			sql:  "SELECT v.VendorName FROM Srm_VendorInfo v WHERE v.VendorId IN (SELECT vendorid FROM Srm_Vendor_Main_Info WHERE isdelete = 0)",
			want: "SELECT v.VendorName FROM Srm_VendorInfo v WHERE v.VendorId IN (SELECT vendorId FROM Srm_Vendor_Main_Info WHERE IsDelete = 0)",
		},
		{
			name: "derived",
			sql:  "SELECT d.VendorName FROM (SELECT vendorname FROM srm_vendorinfo) d",
			want: "SELECT d.VendorName FROM (SELECT VendorName FROM Srm_VendorInfo) d",
		},
		{
			name: "cte",
			sql:  "WITH c AS (SELECT VendorName FROM Srm_VendorInfo) SELECT VendorName FROM c",
			want: "WITH c AS (SELECT VendorName FROM Srm_VendorInfo) SELECT VendorName FROM c",
		},
		{
			name: "comma join",
			sql:  "SELECT a.vendorname, b.bankname FROM srm_vendorinfo a, srm_payinfo b WHERE a.vendorid = b.vendorid",
			want: "SELECT a.VendorName, b.BankName FROM Srm_VendorInfo a, Srm_PayInfo b WHERE a.VendorId = b.VendorId",
		},
		{
			name: "order by alias",
			sql:  "SELECT VendorName name FROM Srm_VendorInfo ORDER BY name",
			want: "SELECT VendorName name FROM Srm_VendorInfo ORDER BY name",
		},
		{
			name: "semicolon stripped only",
			sql:  "SELECT VendorName FROM Srm_VendorInfo;",
			want: "SELECT VendorName FROM Srm_VendorInfo",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := CheckSQL(tt.sql, miniDict)
			if err != nil {
				t.Fatalf("CheckSQL: %v", err)
			}
			if got != tt.want {
				t.Fatalf("got %q\nwant %q", got, tt.want)
			}
		})
	}
}

func TestCheckSQLRejectsUnknownIdentifiers(t *testing.T) {
	t.Parallel()
	_, err := CheckSQL("SELECT VendorName FROM Srm_NoSuch", miniDict)
	if err == nil || !strings.Contains(err.Error(), "Srm_NoSuch") || !strings.Contains(err.Error(), "数据字典预检") {
		t.Fatalf("table: %v", err)
	}
	_, err = CheckSQL("SELECT v.NotARealCol FROM Srm_VendorInfo v", miniDict)
	if err == nil || !strings.Contains(err.Error(), "NotARealCol") {
		t.Fatalf("column: %v", err)
	}
	_, err = CheckSQL("SELECT z.VendorName FROM Srm_VendorInfo v", miniDict)
	if err == nil || !strings.Contains(err.Error(), "未知表或别名 z") {
		t.Fatalf("alias: %v", err)
	}
	_, err = CheckSQL("DELETE FROM Srm_VendorInfo", miniDict)
	if err == nil || !strings.Contains(err.Error(), "仅允许") || strings.Contains(err.Error(), "预检") {
		t.Fatalf("write: %v", err)
	}
}

func TestCheckSQLSkipsWithoutCatalog(t *testing.T) {
	t.Parallel()
	raw := "SELECT a FROM t;"
	got, err := CheckSQL(raw, "")
	if err != nil || got != raw {
		t.Fatalf("empty dict: %q %v", got, err)
	}
	got, err = CheckSQL(raw, "这段说明里没有表结构")
	if err != nil || got != raw {
		t.Fatalf("prose: %q %v", got, err)
	}
}

func TestCheckSQLParsesOtherDictionaryShapes(t *testing.T) {
	t.Parallel()
	pipe := "### `Srm_VendorInfo`\n\n| 1 | `VendorName` | varchar(256) | NO |\n"
	got, err := CheckSQL("SELECT vendorname FROM srm_vendorinfo", pipe)
	if err != nil || got != "SELECT VendorName FROM Srm_VendorInfo" {
		t.Fatalf("pipe: %q %v", got, err)
	}
	js := `{"tables":[{"name":"Srm_VendorInfo","columns":[{"name":"VendorName"},{"name":"IsDelete"}]}]}`
	got, err = CheckSQL("select isdelete from srm_vendorinfo", js)
	if err != nil || got != "select IsDelete from Srm_VendorInfo" {
		t.Fatalf("json: %q %v", got, err)
	}
}

func TestCheckSQLAgainstAIDictionary(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "docs", "SRM_AI_DICTIONARY.md"))
	if err != nil {
		t.Fatal(err)
	}
	cat := parseSQLCatalog(string(raw))
	if len(cat.byName) != 33 {
		t.Fatalf("tables=%d", len(cat.byName))
	}
	mainInfo := cat.byName["Srm_Vendor_Main_Info"]
	if mainInfo == nil || len(mainInfo.cols["vendorid"]) != 1 || mainInfo.cols["vendorid"][0] != "vendorId" {
		t.Fatalf("vendorId %#v", mainInfo)
	}
	invite := cat.byName["Srm_VendorEvaluateInvite"]
	if invite == nil || len(invite.cols["comment"]) != 1 || invite.cols["comment"][0] != "Comment" {
		t.Fatalf("Comment %#v", invite)
	}
	got, err := CheckSQL(`SELECT v.vendorname, m.VendorId, i.comment
FROM srm_vendorinfo v
JOIN srm_vendor_main_info m ON m.vendorid = v.vendorid AND m.vendorlabel = v.vendorlabel
JOIN Srm_VendorEvaluateInvite i ON i.IsDelete = 0
WHERE v.isdelete = 0 AND m.isdelete = 0`, string(raw))
	if err != nil {
		t.Fatal(err)
	}
	want := `SELECT v.VendorName, m.vendorId, i.Comment
FROM Srm_VendorInfo v
JOIN Srm_Vendor_Main_Info m ON m.vendorId = v.VendorId AND m.VendorLabel = v.VendorLabel
JOIN Srm_VendorEvaluateInvite i ON i.IsDelete = 0
WHERE v.IsDelete = 0 AND m.IsDelete = 0`
	if got != want {
		t.Fatalf("got %q\nwant %q", got, want)
	}
	got, err = CheckSQL("SELECT isdelete FROM Srm_Vendor_Draft", string(raw))
	if err != nil || got != "SELECT is_delete FROM Srm_Vendor_Draft" {
		t.Fatalf("draft: %q %v", got, err)
	}
}
