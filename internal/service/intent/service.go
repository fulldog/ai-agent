package intent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/webapp/go-app/ai-agent/internal/config"
	"github.com/webapp/go-app/ai-agent/internal/model"
	"github.com/webapp/go-app/ai-agent/internal/service/llm"
	"github.com/webapp/go-app/ai-agent/internal/service/llmog"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// Service 用户意图分析（不依赖业务 DB；可选写 llm 日志）。
type Service struct {
	cfg    *config.Config
	pool   *llm.Pool
	db     *gorm.DB // 可为 nil
	llmLog *zap.Logger
}

func New(cfg *config.Config, pool *llm.Pool, db *gorm.DB, llmLog *zap.Logger) *Service {
	if llmLog == nil {
		llmLog = zap.NewNop()
	}
	return &Service{cfg: cfg, pool: pool, db: db, llmLog: llmLog}
}

type AnalyzeInput struct {
	Text      string
	Provider  string // 默认 qwen
	Model     string
	MaxTokens int
	RequestID string
}

type AnalyzeOutput struct {
	Result
	Provider         string `json:"provider"`
	Model            string `json:"model"`
	RawContent       string `json:"raw_content,omitempty"`
	PromptTokens     int    `json:"prompt_tokens"`
	CompletionTokens int    `json:"completion_tokens"`
}

// Analyze 将自然语言丢给大模型，解析为固定 JSON 意图结构。不使用业务库表。
func (s *Service) Analyze(ctx context.Context, in AnalyzeInput) (*AnalyzeOutput, error) {
	text := strings.TrimSpace(in.Text)
	if text == "" {
		return &AnalyzeOutput{Result: Result{Code: 1, Msg: "请输入要分析的内容", Data: []Item{}}}, nil
	}

	provider := strings.TrimSpace(in.Provider)
	if provider == "" {
		provider = "qwen"
	}
	client, providerName, modelName, err := s.pool.Resolve(provider, in.Model)
	if err != nil {
		return nil, err
	}

	maxTokens := in.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 2048
	}

	msgs := []llm.Message{
		{Role: "system", Content: SystemPrompt},
		{Role: "user", Content: text},
	}
	req := llm.ChatRequest{
		Model:          modelName,
		Messages:       msgs,
		Temperature:    0,
		MaxTokens:      maxTokens,
		ResponseFormat: "json_object",
	}

	start := time.Now()
	resp, err := client.Chat(ctx, req)
	if err != nil && looksLikeResponseFormatError(err) {
		req.ResponseFormat = ""
		resp, err = client.Chat(ctx, req)
	}

	status := "ok"
	errMsg := ""
	content := ""
	pt, ct := 0, 0
	if err != nil {
		status = "error"
		errMsg = err.Error()
	} else if resp != nil {
		content = resp.Content
		pt, ct = resp.PromptTokens, resp.CompletionTokens
	}

	finishReason := ""
	if resp != nil {
		finishReason = resp.FinishReason
	}
	llmog.Save(s.db, s.llmLog, &model.LLMCallLog{
		RequestID: in.RequestID,
		Provider:  providerName, Model: modelName, Stream: false, Status: status,
		PromptTokens: pt, CompletionTokens: ct, LatencyMs: time.Since(start).Milliseconds(),
		RequestSummary: fmt.Sprintf("intent_analyze chars=%d", len([]rune(text))),
		ErrorMessage:   errMsg,
	}, &llmog.Payload{Messages: msgs, Response: content, FinishReason: finishReason})

	if err != nil {
		return nil, err
	}

	parsed, perr := ParseResultJSON(content)
	out := &AnalyzeOutput{
		Provider:         providerName,
		Model:            modelName,
		RawContent:       content,
		PromptTokens:     pt,
		CompletionTokens: ct,
	}
	if perr != nil {
		// 模型未按 JSON 返回时：当作普通回答，code=1，msg 用原文
		out.Code = 1
		out.Msg = strings.TrimSpace(stripCodeFence(content))
		if out.Msg == "" {
			out.Msg = "暂时无法理解您的问题，请换种说法试试"
		}
		out.Data = []Item{}
		return out, nil
	}
	out.Result = *parsed
	if out.Data == nil {
		out.Data = []Item{}
	}
	ensureFailureCode(&out.Result)
	expandNonBanAccountLists(&out.Result)
	normalizeRefundAmounts(&out.Result)
	return out, nil
}

// ensureFailureCode 非 0 一律规范为 code=1，保留模型生成的 msg（普通问答内容）。
func ensureFailureCode(r *Result) {
	if r == nil {
		return
	}
	if r.Code != 0 {
		r.Code = 1
		if r.Data == nil {
			r.Data = []Item{}
		}
		if strings.TrimSpace(r.Msg) == "" {
			r.Msg = "暂时无法按账户操作模版识别，请换种说法或直接提问"
		}
	}
}

// expandNonBanAccountLists 非封停场景若误用 media_account_ids，拆成多条 data（每账户一条）。
func expandNonBanAccountLists(r *Result) {
	if r == nil || r.Code != 0 || len(r.Data) == 0 {
		return
	}
	out := make([]Item, 0, len(r.Data))
	for _, it := range r.Data {
		kt := KeyWordType(it.KeyWordType)
		if isForbiddenKeyWord(kt) {
			if it.MediaAccountIDs == nil {
				it.MediaAccountIDs = []string{}
			}
			out = append(out, it)
			continue
		}
		ids := trimAccountIDs(it.MediaAccountIDs)
		if len(ids) == 0 {
			it.MediaAccountIDs = []string{}
			out = append(out, it)
			continue
		}
		for _, id := range ids {
			cp := it
			cp.MediaAccountID = id
			cp.MediaAccountIDs = []string{}
			out = append(out, cp)
		}
	}
	r.Data = out
}

func isForbiddenKeyWord(kt KeyWordType) bool {
	switch kt {
	case WxForbiddenPermanent, WxForbiddenConfirm, WxForbiddenCancel:
		return true
	default:
		return false
	}
}

func trimAccountIDs(ids []string) []string {
	if len(ids) == 0 {
		return nil
	}
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id != "" {
			out = append(out, id)
		}
	}
	return out
}

// normalizeRefundAmounts 退款类意图的 icon_amount 强制为负（已为负或 0 则不动）。
func normalizeRefundAmounts(r *Result) {
	if r == nil || r.Code != 0 {
		return
	}
	for i := range r.Data {
		switch KeyWordType(r.Data[i].KeyWordType) {
		case Return, ReturnAll, BatchReturn, TransferTryBestType:
			if r.Data[i].IconAmount.IsPositive() {
				r.Data[i].IconAmount = r.Data[i].IconAmount.Neg()
			}
		}
	}
}

func stripCodeFence(content string) string {
	s := strings.TrimSpace(content)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```JSON")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	return strings.TrimSpace(s)
}

// ParseResultJSON 解析模型输出（可剥 ```json 围栏）。
func ParseResultJSON(content string) (*Result, error) {
	s := stripCodeFence(content)

	var r Result
	if err := json.Unmarshal([]byte(s), &r); err != nil {
		return nil, err
	}
	expandNonBanAccountLists(&r)
	normalizeRefundAmounts(&r)
	return &r, nil
}

func looksLikeResponseFormatError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "response_format") ||
		strings.Contains(msg, "json_object") ||
		strings.Contains(msg, "json mode")
}
