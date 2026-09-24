package chat

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/webapp/go-app/ai-agent/internal/metrics"
	"github.com/webapp/go-app/ai-agent/internal/model"
	"github.com/webapp/go-app/ai-agent/internal/service/agent"
	"github.com/webapp/go-app/ai-agent/internal/service/agent/tools"
	"github.com/webapp/go-app/ai-agent/internal/service/llm"
	"github.com/webapp/go-app/ai-agent/internal/service/llmog"
	"go.uber.org/zap"
)

// callContext 一次对话内多轮 LLM 调用共享的上下文。
type callContext struct {
	conv     *model.Conversation
	client   *llm.Client
	provider string
	model    string
}

// toolSpecs 普通对话要发给模型的工具。始终包含 agent.default_tools；
// chat.tools_enabled 为 true 时再并入 chat.tools；无可用注册工具时返回 nil。
func (s *Service) toolSpecs() []llm.ToolSpec {
	if s == nil || s.registry == nil || s.cfg == nil {
		return nil
	}
	var extra []string
	if s.cfg.Chat.IsToolsEnabled() {
		extra = s.cfg.Chat.Tools
	}
	names := tools.MergeNames(s.cfg.Agent.DefaultTools, extra)
	return s.registry.Specs(names)
}

func (s *Service) maxToolSteps() int {
	if s != nil && s.cfg != nil && s.cfg.Chat.MaxToolSteps > 0 {
		return s.cfg.Chat.MaxToolSteps
	}
	return 4
}

func (s *Service) toolEnv(in CompleteInput, conv *model.Conversation) *tools.Env {
	corpusID := in.CorpusID
	if corpusID == nil && conv != nil {
		corpusID = conv.CorpusID
	}
	env := &tools.Env{CorpusID: corpusID, CorpusIDs: in.CorpusIDs, TopK: in.TopK, RAG: s.rag}
	if s.cfg != nil {
		env.DefaultTopK = s.cfg.RAG.TopK
	}
	return env
}

// toolLoop 执行「LLM → 工具 → LLM」循环，直到模型不再请求工具或用完工具轮数。
// onDelta 非 nil 时走流式。返回最后一轮响应，token 为各轮累计值。
func (s *Service) toolLoop(ctx context.Context, in CompleteInput, call callContext, msgs []llm.Message, onDelta func(string) error) (*llm.ChatResponse, error) {
	specs := s.toolSpecs()
	rounds := 1
	if len(specs) > 0 {
		rounds = s.maxToolSteps() + 1 // 最后一轮不带工具，强制模型给出回答
	}
	env := s.toolEnv(in, call.conv)
	promptTokens, completionTokens := 0, 0
	var last *llm.ChatResponse
	usedTool := false

	for round := 0; round < rounds; round++ {
		req := chatRequest(in, call.provider, call.model, msgs)
		if round < rounds-1 {
			req.Tools = specs
			if len(specs) > 0 && !usedTool {
				req.ToolChoice = "required"
			}
		}
		start := time.Now()
		var resp *llm.ChatResponse
		var err error
		suppressDelta := len(req.Tools) > 0 && !usedTool
		streamCB := func(ev llm.StreamEvent) error {
			if suppressDelta || ev.Content == "" || onDelta == nil {
				return nil
			}
			return onDelta(ev.Content)
		}
		if onDelta != nil {
			resp, err = call.client.ChatStream(ctx, req, streamCB)
		} else {
			resp, err = call.client.Chat(ctx, req)
		}
		if err != nil && req.ToolChoice == "required" {
			s.llmLog.Warn("retry chat tool round without tool_choice=required", zap.Error(err))
			req.ToolChoice = ""
			if onDelta != nil {
				resp, err = call.client.ChatStream(ctx, req, streamCB)
			} else {
				resp, err = call.client.Chat(ctx, req)
			}
		}
		if resp != nil {
			promptTokens += resp.PromptTokens
			completionTokens += resp.CompletionTokens
		}
		s.logRound(in, call, msgs, resp, err, onDelta != nil, round, len(req.Tools), start)
		if err != nil {
			return nil, err
		}
		last = resp
		if len(resp.ToolCalls) == 0 {
			break
		}
		usedTool = true
		msgs = append(msgs, llm.Message{Role: "assistant", Content: resp.Content, ToolCalls: resp.ToolCalls})
		for _, tc := range resp.ToolCalls {
			result, toolErr := s.registry.Exec(ctx, tc.Function.Name, tc.Function.Arguments, env)
			status := "ok"
			if toolErr != nil {
				status = "error"
				result = toolErr.Error()
			}
			metrics.ToolCalls.WithLabelValues(tc.Function.Name, status).Inc()
			msgs = append(msgs, llm.Message{
				Role: "tool", ToolCallID: tc.ID, Name: tc.Function.Name, Content: result,
			})
		}
	}

	if last == nil {
		return nil, fmt.Errorf("模型未返回内容")
	}
	out := *last
	out.PromptTokens = promptTokens
	out.CompletionTokens = completionTokens
	if len(specs) > 0 && agent.IsToolEvasionReply(out.Content) {
		s.llmLog.Warn("chat evasion reply replaced",
			zap.String("request_id", in.RequestID),
			zap.String("preview", out.Content),
		)
		out.Content = agent.ToolEvasionFallback
	}
	return &out, nil
}

func (s *Service) logRound(in CompleteInput, call callContext, msgs []llm.Message, resp *llm.ChatResponse, callErr error, stream bool, round, toolCount int, start time.Time) {
	status := "ok"
	errMsg := ""
	if callErr != nil {
		status = "error"
		errMsg = callErr.Error()
	}
	content, finishReason := "", ""
	pt, ct := 0, 0
	var toolCalls any
	if resp != nil {
		content, finishReason = resp.Content, resp.FinishReason
		pt, ct = resp.PromptTokens, resp.CompletionTokens
		if len(resp.ToolCalls) > 0 {
			toolCalls = resp.ToolCalls
		}
	}
	var convID *uuid.UUID
	if call.conv != nil {
		convID = &call.conv.ID
	}
	llmog.Save(s.db, s.llmLog, &model.LLMCallLog{
		RequestID:        in.RequestID,
		ConversationID:   convID,
		Provider:         call.provider,
		Model:            call.model,
		Stream:           stream,
		Status:           status,
		PromptTokens:     pt,
		CompletionTokens: ct,
		LatencyMs:        time.Since(start).Milliseconds(),
		RequestSummary:   fmt.Sprintf("messages=%d stream=%v tools=%d round=%d", len(msgs), stream, toolCount, round),
		ErrorMessage:     errMsg,
	}, &llmog.Payload{
		Messages: msgs, Response: content, ToolCalls: toolCalls, FinishReason: finishReason,
	})
}

// toolSystemPrompt 工具可用时追加的调用约定，与 Agent 保持一致。
func toolSystemPrompt(specs []llm.ToolSpec) string {
	if len(specs) == 0 {
		return ""
	}
	prompt := baseToolPrompt
	for _, spec := range specs {
		if spec.Function.Name == "dbconn" {
			prompt += "\n" + dbconnToolPrompt
			break
		}
	}
	return prompt
}

const (
	baseToolPrompt   = "需要准确信息时调用工具，并依据工具结果作答；不需要时直接回答，不要为了用工具而用工具。本次请求若已附带 tools，说明工具已启用，禁止回答「未启用工具/请到工具函数管理开启」。"
	dbconnToolPrompt = "若问题涉及供应商、付款、订单、业务数据或表结构：先按已注入的知识摘录执行（缺前置信息先追问，不要直接查库）。摘录未覆盖时再用 knowledge_search 查口径；再用 dbconn 的 schema 对照表与列注释选定表和列，最后组织只读 SELECT 并调用 dbconn 的 query 执行。不要只把 SQL 写在回复里而不调用工具；无法对应到表时说明缺什么，不要编造表名或数据。"
)
