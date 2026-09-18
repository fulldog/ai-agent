package dingtalk

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestParseOpenResponse(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		status   int
		body     string
		wantErr  string
		endpoint string
		ticket   string
	}{
		{
			name:     "ok",
			status:   200,
			body:     `{"endpoint":"wss://wss-open-connection.dingtalk.com:443/connect","ticket":"abc-1"}`,
			endpoint: "wss://wss-open-connection.dingtalk.com:443/connect",
			ticket:   "abc-1",
		},
		{
			name:    "system error",
			status:  400,
			body:    `{"code":"systemError","message":"系统错误"}`,
			wantErr: "code=systemError",
		},
		{
			name:    "empty ticket",
			status:  200,
			body:    `{"endpoint":"wss://x","ticket":""}`,
			wantErr: "http_200",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ep, ticket, err := parseOpenResponse(tt.status, []byte(tt.body))
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err=%v want substring %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if ep != tt.endpoint || ticket != tt.ticket {
				t.Fatalf("got %q %q", ep, ticket)
			}
		})
	}
}

func TestStreamWSURL(t *testing.T) {
	t.Parallel()
	got, err := streamWSURL("wss://wss-open-connection.dingtalk.com:443/connect", "tick et")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "ticket=tick+et") && !strings.Contains(got, "ticket=tick%20et") {
		t.Fatalf("ticket not escaped: %s", got)
	}
}

func TestAckForFrame(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name           string
		raw            string
		wantDisconnect bool
		wantCode       int
		wantData       string
	}{
		{
			name: "ping",
			raw: `{
				"type":"SYSTEM",
				"headers":{"topic":"ping","messageId":"m1","contentType":"application/json"},
				"data":"{\"opaque\":\"123-dsfs\"}"
			}`,
			wantCode: 200,
			wantData: `{"opaque":"123-dsfs"}`,
		},
		{
			name: "disconnect",
			raw: `{
				"type":"SYSTEM",
				"headers":{"topic":"disconnect","contentType":"application/json"},
				"data":"{\"reason\":\"connection is expired\"}"
			}`,
			wantDisconnect: true,
		},
		{
			name: "bot callback",
			raw: `{
				"type":"CALLBACK",
				"headers":{"topic":"/v1.0/im/bot/messages/get","messageId":"m2","time":1690362102194},
				"data":"{\"msgId\":\"msg1\",\"senderStaffId\":\"u1\",\"text\":{\"content\":\"hi\"}}"
			}`,
			wantCode: 200,
			wantData: `{"response":null}`,
		},
		{
			name: "unknown topic",
			raw: `{
				"type":"CALLBACK",
				"headers":{"topic":"/v1.0/card/instances/callback","messageId":"m3"}
			}`,
			wantCode: 404,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			frame, err := decodeFrame([]byte(tt.raw))
			if err != nil {
				t.Fatal(err)
			}
			ack, disconnect := ackForFrame(frame)
			if disconnect != tt.wantDisconnect {
				t.Fatalf("disconnect=%v", disconnect)
			}
			if tt.wantDisconnect {
				if ack != nil {
					t.Fatal("disconnect must not ack")
				}
				return
			}
			if ack == nil {
				t.Fatal("expected ack")
			}
			if ack.Code != tt.wantCode {
				t.Fatalf("code=%d want %d", ack.Code, tt.wantCode)
			}
			if tt.wantData != "" && ack.Data != tt.wantData {
				t.Fatalf("data=%q want %q", ack.Data, tt.wantData)
			}
			if frame.Headers.get("messageId") != "" && ack.Headers["messageId"] != frame.Headers.get("messageId") {
				t.Fatalf("messageId=%q", ack.Headers["messageId"])
			}
		})
	}
}

func TestDecodeBotCallbackData(t *testing.T) {
	t.Parallel()
	raw := `{"conversationId":"cidA","senderStaffId":"16650","isInAtList":true,"conversationType":"2","sessionWebhook":"https://oapi.dingtalk.com/x","text":{"content":" 测试数据"},"msgtype":"text","msgId":"msg1"}`
	var data botCallback
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		t.Fatal(err)
	}
	if data.SenderStaffID != "16650" || data.ConversationID != "cidA" || !data.IsInAtList || data.MsgType != "text" {
		t.Fatalf("%+v", data)
	}
}
