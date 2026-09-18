package dingtalk

// botCallback is the JSON payload of Stream topic /v1.0/im/bot/messages/get.
// See https://open.dingtalk.com/document/orgapp/receive-message
type botCallback struct {
	ConversationID    string  `json:"conversationId"`
	MsgID             string  `json:"msgId"`
	SenderNick        string  `json:"senderNick"`
	SenderStaffID     string  `json:"senderStaffId"`
	ConversationType  string  `json:"conversationType"`
	ConversationTitle string  `json:"conversationTitle"`
	IsInAtList        bool    `json:"isInAtList"`
	SessionWebhook    string  `json:"sessionWebhook"`
	Text              botText `json:"text"`
	MsgType           string  `json:"msgtype"`
}

type botText struct {
	Content string `json:"content"`
}
