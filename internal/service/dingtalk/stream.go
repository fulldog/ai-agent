package dingtalk

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

const (
	streamOpenURL     = "https://api.dingtalk.com/v1.0/gateway/connections/open"
	streamUA          = "ai-agent-go/1.0"
	topicBotMessages  = "/v1.0/im/bot/messages/get"
	frameTypeSystem   = "SYSTEM"
	frameTypeCallback = "CALLBACK"
	topicPing         = "ping"
	topicDisconnect   = "disconnect"
	ackContentType    = "application/json"
	streamReadIdle    = 10 * time.Minute
)

var errStreamDisconnect = errors.New("dingtalk stream disconnect")

type streamFrame struct {
	SpecVersion string          `json:"specVersion"`
	Type        string          `json:"type"`
	Headers     streamHeaders   `json:"headers"`
	Data        json.RawMessage `json:"data"`
}

func (f *streamFrame) dataString() string {
	if f == nil || len(f.Data) == 0 || string(f.Data) == "null" {
		return ""
	}
	if f.Data[0] == '"' {
		var s string
		if err := json.Unmarshal(f.Data, &s); err == nil {
			return s
		}
	}
	return string(f.Data)
}

func parseBotCallback(data json.RawMessage) (*botCallback, error) {
	raw := strings.TrimSpace(string(data))
	if raw == "" || raw == "null" {
		return nil, errors.New("empty bot callback")
	}
	payload := data
	if data[0] == '"' {
		var s string
		if err := json.Unmarshal(data, &s); err != nil {
			return nil, err
		}
		payload = []byte(s)
	}
	var out botCallback
	if err := json.Unmarshal(payload, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

type streamHeaders map[string]json.RawMessage

func (h streamHeaders) get(key string) string {
	if h == nil {
		return ""
	}
	raw, ok := h[key]
	if !ok || len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var n json.Number
	if err := json.Unmarshal(raw, &n); err == nil {
		return n.String()
	}
	return strings.TrimSpace(string(raw))
}

type streamAck struct {
	Code    int               `json:"code"`
	Headers map[string]string `json:"headers"`
	Message string            `json:"message"`
	Data    string            `json:"data"`
}

func newStreamAck(code int, messageID, message, data string) streamAck {
	return streamAck{
		Code:    code,
		Message: message,
		Headers: map[string]string{
			"messageId":   messageID,
			"contentType": ackContentType,
		},
		Data: data,
	}
}

type streamSession struct {
	mu     sync.Mutex
	closed bool
	conn   *websocket.Conn
	log    *zap.Logger
}

func (s *streamSession) writeJSON(v any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.conn == nil {
		return errors.New("dingtalk stream disconnected")
	}
	return s.conn.WriteJSON(v)
}

func (s *streamSession) close() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	s.closed = true
	if s.conn != nil {
		_ = s.conn.Close()
	}
}

type openResponse struct {
	Endpoint string `json:"endpoint"`
	Ticket   string `json:"ticket"`
	Code     string `json:"code"`
	Message  string `json:"message"`
}

func registerStream(ctx context.Context, clientID, clientSecret string) (endpoint, ticket string, err error) {
	body, err := json.Marshal(map[string]any{
		"clientId":     clientID,
		"clientSecret": clientSecret,
		"ua":           streamUA,
		"subscriptions": []map[string]string{
			{"type": frameTypeCallback, "topic": topicBotMessages},
		},
	})
	if err != nil {
		return "", "", fmt.Errorf("marshal stream open: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, streamOpenURL, bytes.NewReader(body))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		return "", "", fmt.Errorf("stream connections/open: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", fmt.Errorf("read stream connections/open: %w", err)
	}
	return parseOpenResponse(resp.StatusCode, raw)
}

func parseOpenResponse(status int, raw []byte) (endpoint, ticket string, err error) {
	var out openResponse
	if uerr := json.Unmarshal(raw, &out); uerr != nil {
		return "", "", fmt.Errorf("parse stream connections/open: status=%d body=%s: %w", status, string(raw), uerr)
	}
	if status >= 300 || out.Endpoint == "" || out.Ticket == "" {
		code := out.Code
		if code == "" {
			code = fmt.Sprintf("http_%d", status)
		}
		msg := out.Message
		if msg == "" {
			msg = string(raw)
		}
		return "", "", fmt.Errorf("stream connections/open: code=%s message=%s", code, msg)
	}
	return out.Endpoint, out.Ticket, nil
}

func openStream(ctx context.Context, clientID, clientSecret string) (*streamSession, error) {
	endpoint, ticket, err := registerStream(ctx, clientID, clientSecret)
	if err != nil {
		return nil, err
	}
	wsURL, err := streamWSURL(endpoint, ticket)
	if err != nil {
		return nil, err
	}
	dialer := websocket.Dialer{HandshakeTimeout: 15 * time.Second}
	conn, resp, err := dialer.DialContext(ctx, wsURL, nil)
	if resp != nil && resp.Body != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		if resp != nil {
			raw, _ := io.ReadAll(resp.Body)
			return nil, fmt.Errorf("stream websocket: %w body=%s", err, string(raw))
		}
		return nil, fmt.Errorf("stream websocket: %w", err)
	}
	return &streamSession{conn: conn}, nil
}

func streamWSURL(endpoint, ticket string) (string, error) {
	u, err := url.Parse(endpoint)
	if err != nil {
		return "", fmt.Errorf("stream endpoint: %w", err)
	}
	q := u.Query()
	q.Set("ticket", ticket)
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func (s *streamSession) serve(ctx context.Context, onBot func(*botCallback)) error {
	errCh := make(chan error, 1)
	go func() {
		errCh <- s.readLoop(onBot)
	}()
	select {
	case <-ctx.Done():
		s.close()
		<-errCh
		return ctx.Err()
	case err := <-errCh:
		return err
	}
}

func (s *streamSession) readLoop(onBot func(*botCallback)) error {
	for {
		if err := s.conn.SetReadDeadline(time.Now().Add(streamReadIdle)); err != nil {
			return err
		}
		_, msg, err := s.conn.ReadMessage()
		if err != nil {
			return err
		}
		if err := s.handleFrame(msg, onBot); err != nil {
			return err
		}
	}
}

func (s *streamSession) handleFrame(raw []byte, onBot func(*botCallback)) error {
	frame, err := decodeFrame(raw)
	if err != nil {
		if s != nil && s.log != nil {
			s.log.Warn("dingtalk stream frame decode", zap.Error(err), zap.Int("bytes", len(raw)))
		}
		return nil
	}
	topic := frame.Headers.get("topic")
	isBotMsg := frame.Type == frameTypeCallback && topic == topicBotMessages

	// 机器人消息：先交给业务落 receive 日志，再 ACK。
	// 若先 ACK 后进程崩溃，钉钉不再重投 → 用户看到异常回复或无回复，且本机无 request_log。
	if isBotMsg {
		data, perr := parseBotCallback(frame.Data)
		if perr != nil {
			if s != nil && s.log != nil {
				s.log.Warn("dingtalk bot callback unmarshal", zap.Error(perr), zap.Int("bytes", len(frame.Data)))
			}
			// 解析失败仍 ACK，避免毒消息死循环；内容已打 warn。
			ack := newStreamAck(200, frame.Headers.get("messageId"), "OK", `{"response":null}`)
			return s.writeJSON(ack)
		}
		if onBot != nil {
			onBot(data)
		}
		ack := newStreamAck(200, frame.Headers.get("messageId"), "OK", `{"response":null}`)
		if err := s.writeJSON(ack); err != nil {
			return fmt.Errorf("stream ack: %w", err)
		}
		return nil
	}

	ack, disconnect := ackForFrame(frame)
	if disconnect {
		return errStreamDisconnect
	}
	if ack != nil {
		if err := s.writeJSON(ack); err != nil {
			return fmt.Errorf("stream ack: %w", err)
		}
	}
	return nil
}

func decodeFrame(raw []byte) (*streamFrame, error) {
	var frame streamFrame
	if err := json.Unmarshal(raw, &frame); err != nil {
		return nil, err
	}
	return &frame, nil
}

func ackForFrame(frame *streamFrame) (ack *streamAck, disconnect bool) {
	if frame == nil {
		return nil, false
	}
	messageID := frame.Headers.get("messageId")
	topic := frame.Headers.get("topic")
	switch {
	case frame.Type == frameTypeSystem && topic == topicDisconnect:
		return nil, true
	case frame.Type == frameTypeSystem && topic == topicPing:
		a := newStreamAck(200, messageID, "OK", frame.dataString())
		return &a, false
	case frame.Type == frameTypeCallback && topic == topicBotMessages:
		a := newStreamAck(200, messageID, "OK", `{"response":null}`)
		return &a, false
	default:
		a := newStreamAck(404, messageID, "topic not implemented", "")
		return &a, false
	}
}
