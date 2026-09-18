package llm

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// APIError is an upstream chat-completions HTTP error.
type APIError struct {
	StatusCode int
	Code       string
	Message    string
	Raw        string
}

func (e *APIError) Error() string {
	if e == nil {
		return "llm error"
	}
	if e.Code != "" {
		return fmt.Sprintf("llm error %d: code=%s body=%s", e.StatusCode, e.Code, e.Raw)
	}
	return fmt.Sprintf("llm error %d: %s", e.StatusCode, e.Raw)
}

func parseAPIError(status int, raw []byte) error {
	var wrap struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
			Type    string `json:"type"`
		} `json:"error"`
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	_ = json.Unmarshal(raw, &wrap)
	code := strings.TrimSpace(wrap.Error.Code)
	if code == "" {
		code = strings.TrimSpace(wrap.Code)
	}
	if code == "" {
		code = strings.TrimSpace(wrap.Error.Type)
	}
	msg := strings.TrimSpace(wrap.Error.Message)
	if msg == "" {
		msg = strings.TrimSpace(wrap.Message)
	}
	return &APIError{
		StatusCode: status,
		Code:       code,
		Message:    msg,
		Raw:        string(raw),
	}
}

// IsInspectionFailed reports whether the upstream rejected the prompt as sensitive.
func IsInspectionFailed(err error) bool {
	var api *APIError
	if errors.As(err, &api) {
		code := strings.ToLower(api.Code)
		if strings.Contains(code, "inspection") {
			return true
		}
		return strings.Contains(strings.ToLower(api.Raw), "data_inspection_failed")
	}
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "data_inspection_failed")
}

// PublicMessage is safe to show to end users.
func PublicMessage(err error) string {
	if err == nil {
		return ""
	}
	if IsInspectionFailed(err) {
		return "问题或检索到的知识触发了模型安全审核，请换种问法，或检查语料后重试。"
	}
	return err.Error()
}
