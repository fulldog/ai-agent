package agent

import (
	"strings"
	"testing"
)

func TestMaskSchemaLeak(t *testing.T) {
	t.Parallel()
	in := strings.Join([]string{
		"已按 VendorLabel = 8（中后台）完成三条件校验：",
		"Srm_VendorInfo：IsEnable = 1，IsDelete = 0 → 启用",
		"Srm_VendorPolicy：存在 1 条AuditStatus = 4 且 IsDelete = 0 的有效政策 → 满足",
		"Srm_VendorContract：存在 2 条AuditStatus = 4 且 IsDelete = 0 的有效合同 → 满足",
		"结论：可以出款。",
		"所有前置条件均满足，系统将允许发起付款申请。",
		"来源：SRM供应商系统（1.判断供应商能否付款 2.01-供应商管理.md 3.业务开关与前置校验.md）",
	}, "\n")
	got := MaskSchemaLeak(in)
	for _, banned := range []string{"Srm_VendorInfo", "Srm_VendorPolicy", "Srm_VendorContract", "VendorLabel", "IsEnable", "IsDelete", "AuditStatus"} {
		if strings.Contains(got, banned) {
			t.Fatalf("still contains %s: %q", banned, got)
		}
	}
	for _, want := range []string{"中后台", "启用", "有效政策", "有效合同", "可以出款", "SRM供应商系统", "供应商管理.md"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %s: %q", want, got)
		}
	}
}

func TestMaskSchemaLeakKeepsOrdinaryReply(t *testing.T) {
	t.Parallel()
	const in = "乐推可以付款，且合同有效。"
	if got := MaskSchemaLeak(in); got != in {
		t.Fatalf("got %q", got)
	}
}
