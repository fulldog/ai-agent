package intent_test

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/webapp/go-app/ai-agent/internal/service/intent"
)

func TestParseResultJSON(t *testing.T) {
	raw := "```json\n{\"code\":0,\"msg\":\"\",\"data\":[{\"media_account_id\":\"1\",\"media_account_id_in\":\"\",\"icon_amount\":13,\"TransferTryBest\":false,\"media_account_ids\":[],\"KeyWordType\":10}]}\n```"
	r, err := intent.ParseResultJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	if r.Code != 0 || len(r.Data) != 1 || r.Data[0].KeyWordType != 10 {
		t.Fatalf("%+v", r)
	}
	if !r.Data[0].IconAmount.Equal(decimal.NewFromInt(13)) {
		t.Fatalf("amount=%v", r.Data[0].IconAmount)
	}
}

func TestParseResultJSONPhone(t *testing.T) {
	raw := `{"code":0,"msg":"","data":[{"media_account_id":"","media_account_id_in":"","phone":"13800138000","icon_amount":0,"TransferTryBest":false,"media_account_ids":[],"KeyWordType":100,"KeyWordTypeStr":"短信授权","remark":""}]}`
	r, err := intent.ParseResultJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	if r.Data[0].Phone != "13800138000" || r.Data[0].MediaAccountID != "" {
		t.Fatalf("%+v", r.Data[0])
	}
}

func TestParseResultJSONRefundNegativeAmount(t *testing.T) {
	raw := `{"code":0,"msg":"","data":[{"media_account_id":"12345678","media_account_id_in":"","phone":"","icon_amount":100,"TransferTryBest":false,"media_account_ids":[],"KeyWordType":12,"KeyWordTypeStr":"退币","remark":""}]}`
	r, err := intent.ParseResultJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Data[0].IconAmount.Equal(decimal.NewFromInt(-100)) {
		t.Fatalf("refund amount want -100, got %v", r.Data[0].IconAmount)
	}

	rawNeg := `{"code":0,"msg":"","data":[{"media_account_id":"12345678","media_account_id_in":"","phone":"","icon_amount":-50,"TransferTryBest":true,"media_account_ids":[],"KeyWordType":20,"KeyWordTypeStr":"尽可能退","remark":""}]}`
	r2, err := intent.ParseResultJSON(rawNeg)
	if err != nil {
		t.Fatal(err)
	}
	if !r2.Data[0].IconAmount.Equal(decimal.NewFromInt(-50)) {
		t.Fatalf("already negative want -50, got %v", r2.Data[0].IconAmount)
	}
}

func TestParseResultJSONExpandNonBanAccountLists(t *testing.T) {
	raw := `{"code":0,"msg":"","data":[{"media_account_id":"","media_account_id_in":"","phone":"","icon_amount":100,"TransferTryBest":false,"media_account_ids":["12121212","343434343"],"KeyWordType":20,"KeyWordTypeStr":"尽可能退","remark":""}]}`
	r, err := intent.ParseResultJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Data) != 2 {
		t.Fatalf("want 2 data, got %+v", r.Data)
	}
	if r.Data[0].MediaAccountID != "12121212" || len(r.Data[0].MediaAccountIDs) != 0 {
		t.Fatalf("%+v", r.Data[0])
	}
	if r.Data[1].MediaAccountID != "343434343" || !r.Data[1].IconAmount.Equal(decimal.NewFromInt(-100)) {
		t.Fatalf("%+v", r.Data[1])
	}

	ban := `{"code":0,"msg":"","data":[{"media_account_id":"","media_account_id_in":"","phone":"","icon_amount":0,"TransferTryBest":false,"media_account_ids":["12345678","23456789"],"KeyWordType":30,"KeyWordTypeStr":"永久封停","remark":"素材问题"}]}`
	r2, err := intent.ParseResultJSON(ban)
	if err != nil {
		t.Fatal(err)
	}
	if len(r2.Data) != 1 || len(r2.Data[0].MediaAccountIDs) != 2 {
		t.Fatalf("ban should keep media_account_ids: %+v", r2.Data)
	}
}

func TestParseResultJSONFreeAnswer(t *testing.T) {
	raw := `{"code":1,"msg":"今天多云，气温 22℃ 左右，出门记得带伞。","data":[]}`
	r, err := intent.ParseResultJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	if r.Code != 1 || r.Msg == "" || r.Msg == "联网检索答案" {
		t.Fatalf("%+v", r)
	}
}
