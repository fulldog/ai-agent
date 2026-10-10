package dbconn

import (
	"context"
	"strings"
	"testing"
)

func TestAssembleDictionary(t *testing.T) {
	t.Parallel()
	d := assembleDictionary(nil, []scannedTable{
		{name: "Srm_VendorInfo", comment: "供应商基本信息表", rows: 10},
		{name: "Srm_PayInfo", comment: "供应商收款信息表", rows: 3},
	}, []scannedColumn{
		{table: "Srm_VendorInfo", name: "VendorId", typ: "varchar(64)", key: "PRI", comment: "供应商Id"},
		{table: "Srm_VendorInfo", name: "SysNo", typ: "int(11)", key: "UNI", extra: "auto_increment"},
		{table: "Srm_PayInfo", name: "SysNo", typ: "int(11)", key: "PRI"},
		{table: "Srm_PayInfo", name: "VendorId", typ: "varchar(64)", key: "MUL", comment: "供应商Id"},
		{table: "Srm_PayInfo", name: "BankName", typ: "varchar(64)", nullable: true, comment: "开户行"},
	}, []scannedIndex{
		{table: "Srm_VendorInfo", name: "PRIMARY", unique: true, seq: 1, column: "VendorId"},
		{table: "Srm_PayInfo", name: "PRIMARY", unique: true, seq: 1, column: "SysNo"},
		{table: "Srm_PayInfo", name: "fk_VendorId", unique: false, seq: 1, column: "VendorId"},
	})
	if err := d.index(); err != nil {
		t.Fatal(err)
	}
	if len(d.Conventions) == 0 {
		t.Fatal("expected default conventions")
	}
	vendor := d.byName["Srm_VendorInfo"]
	if vendor == nil || len(vendor.PrimaryKey) != 1 || vendor.PrimaryKey[0] != "VendorId" {
		t.Fatalf("primary key: %#v", vendor)
	}
	if len(d.Relations) != 1 || d.Relations[0].Column != "VendorId" {
		t.Fatalf("relations: %#v", d.Relations)
	}
	if strings.Contains(relationTables(d, "SysNo"), "Srm_") {
		t.Fatal("SysNo must not be a cross-table relation")
	}
	out, err := (&Client{dict: d}).Schema(context.Background(), "", "Srm_PayInfo")
	if err != nil || !strings.Contains(out, "开户行") {
		t.Fatalf("schema: %v\n%s", err, out)
	}
}

func relationTables(d *Dictionary, column string) string {
	for _, rel := range d.Relations {
		if rel.Column == column {
			return formatRelationTables(rel.Tables)
		}
	}
	return ""
}
