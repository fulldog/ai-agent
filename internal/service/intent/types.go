package intent

import (
	"encoding/json"

	"github.com/shopspring/decimal"
)

// KeyWordType 对齐 C# EnumWeChat_KeyWordType（微信助手关键字意图）。
type KeyWordType int

const (
	Balance              KeyWordType = 1
	BalanceMax           KeyWordType = 2
	Recharge             KeyWordType = 10
	Return               KeyWordType = 12
	ReturnAll            KeyWordType = 14
	BatchRecharge        KeyWordType = 16
	BatchReturn          KeyWordType = 18
	TransferTryBestType  KeyWordType = 20 // 枚举名 TransferTryBest，避免与字段混淆
	Transfer             KeyWordType = 22
	TransferAll          KeyWordType = 24
	WxCopy               KeyWordType = 26
	WxCopyQuery          KeyWordType = 28
	WxForbiddenPermanent KeyWordType = 30
	WxForbiddenConfirm   KeyWordType = 32
	WxForbiddenCancel    KeyWordType = 34
	SMSAuth              KeyWordType = 100
	SMSAuthCheck         KeyWordType = 102
)

// Item 单条解析结果（字段名与下游 .NET 约定对齐）。
type Item struct {
	MediaAccountID   string          `json:"media_account_id"`
	MediaAccountIDIn string          `json:"media_account_id_in"`
	Mobile           string          `json:"Mobile"` // 短信授权手机号，与 media_account_id 互不混用
	IconAmount       decimal.Decimal `json:"icon_amount"`
	TransferTryBest  bool            `json:"TransferTryBest"`
	MediaAccountIDs  []string        `json:"media_account_ids"`
	KeyWordType      int             `json:"KeyWordType"`
	KeyWordTypeStr   string          `json:"KeyWordTypeStr"`
	ForbiddenReason  string          `json:"ForbiddenReason"`  // 封停原因（业务问题|素材问题）
	AuthCode         string          `json:"AuthCode"`         // 短信授权验证码
	CopyNumber       int             `json:"CopyNumber"`       // 复制账户数量
	CopyTaskNo       string          `json:"CopyTaskNo"`       // 复制账户任务编号
	Remark           string          `json:"remark,omitempty"` // 兼容旧输出；规范化后清空
}

// UnmarshalJSON 兼容旧字段 phone → Mobile。
func (it *Item) UnmarshalJSON(b []byte) error {
	type alias Item
	aux := struct {
		Phone string `json:"phone"`
		*alias
	}{alias: (*alias)(it)}
	if err := json.Unmarshal(b, &aux); err != nil {
		return err
	}
	if it.Mobile == "" && aux.Phone != "" {
		it.Mobile = aux.Phone
	}
	return nil
}

// Result 大模型固定 JSON 外壳。
type Result struct {
	Code int    `json:"code"` // 0 成功，1 失败
	Msg  string `json:"msg"`
	Data []Item `json:"data"`
}
