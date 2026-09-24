package dingtalk

import (
	"encoding/json"
	"strings"
)

// botCallback is the JSON payload of Stream topic /v1.0/im/bot/messages/get.
// See https://open.dingtalk.com/document/orgapp/receive-message
type botCallback struct {
	ConversationID    string       `json:"conversationId"`
	MsgID             string       `json:"msgId"`
	SenderNick        string       `json:"senderNick"`
	SenderStaffID     string       `json:"senderStaffId"`
	ConversationType  string       `json:"conversationType"`
	ConversationTitle string       `json:"conversationTitle"`
	IsInAtList        flexibleBool `json:"isInAtList"`
	AtUsers           []botAtUser  `json:"atUsers"`
	SessionWebhook    string       `json:"sessionWebhook"`
	Text              botText      `json:"text"`
	MsgType           string       `json:"msgtype"`
}

type botAtUser struct {
	DingTalkID string `json:"dingtalkId"`
	StaffID    string `json:"staffId"`
}

type botText struct {
	Content string `json:"content"`
}

// flexibleBool accepts JSON bool, 0/1, or "true"/"false" from DingTalk payloads.
type flexibleBool bool

func (b *flexibleBool) UnmarshalJSON(raw []byte) error {
	if b == nil {
		return nil
	}
	s := strings.TrimSpace(string(raw))
	if s == "" || s == "null" {
		*b = false
		return nil
	}
	var v bool
	if err := json.Unmarshal(raw, &v); err == nil {
		*b = flexibleBool(v)
		return nil
	}
	var n json.Number
	if err := json.Unmarshal(raw, &n); err == nil {
		*b = n.String() != "0"
		return nil
	}
	var str string
	if err := json.Unmarshal(raw, &str); err == nil {
		switch strings.ToLower(strings.TrimSpace(str)) {
		case "true", "1", "yes":
			*b = true
		default:
			*b = false
		}
		return nil
	}
	*b = false
	return nil
}

func (d *botCallback) inAtList() bool {
	return d != nil && bool(d.IsInAtList)
}

func isBotMention(data *botCallback) bool {
	if data == nil {
		return false
	}
	if !isGroup(data.ConversationType) {
		return true
	}
	if data.inAtList() {
		return true
	}
	return len(data.AtUsers) > 0
}
