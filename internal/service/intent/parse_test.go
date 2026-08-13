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
