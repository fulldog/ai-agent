package dbconn

import (
	"strings"
	"testing"
)

func TestAssembleLiveDictionary(t *testing.T) {
	t.Parallel()
	d := assembleLiveDictionary([]liveTable{
		{name: "Srm_VendorInfo", comment: "供应商基本信息表", rows: 12},
		{name: "Srm_PayInfo", comment: "付款信息", rows: 3},
	}, []liveColumn{
		{table: "Srm_VendorInfo", name: "VendorId", typ: "int", key: "PRI", comment: "供应商ID"},
		{table: "Srm_VendorInfo", name: "VendorName", typ: "varchar(64)", nullable: true, comment: "供应商名称"},
		{table: "Srm_PayInfo", name: "BankName", typ: "varchar(64)", comment: "开户行"},
	}, []liveIndex{
		{table: "Srm_VendorInfo", name: "PRIMARY", unique: true, seq: 1, column: "VendorId"},
		{table: "Srm_PayInfo", name: "idx_bank", unique: false, seq: 1, column: "BankName"},
	})
	if err := d.index(); err != nil {
		t.Fatal(err)
	}
	out, err := d.lookup("", "Srm_VendorInfo")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"供应商基本信息表", "约行数: 12", "主键: VendorId", "VendorName", "供应商名称"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
	if strings.Contains(out, "开户行") {
		t.Fatalf("exact table leaked other columns:\n%s", out)
	}
	keyword, err := d.lookup("开户行", "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(keyword, "Srm_PayInfo") || !strings.Contains(keyword, "BankName") {
		t.Fatalf("got:\n%s", keyword)
	}
	if strings.Contains(keyword, "VendorName") {
		t.Fatalf("keyword dumped unrelated columns:\n%s", keyword)
	}
}
