package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/webapp/go-app/ai-agent/internal/model"
	"github.com/webapp/go-app/ai-agent/internal/service/agent/tools"
	"github.com/webapp/go-app/ai-agent/internal/service/llm"
	"github.com/webapp/go-app/ai-agent/internal/service/llmog"
)

const toolEvasionFallback = "服务端已启用查询工具，但刚才未能完成核实。请再发一次问题，或补充供应商名称与标签后重试。"

// ToolEvasionFallback 推诿话被拦截后的对外兜底文案（钉钉出站也复用）。
const ToolEvasionFallback = toolEvasionFallback

// isToolEvasionReply 模型推诿「去开工具」——工具其实已挂载，不应原样发出。
func isToolEvasionReply(s string) bool {
	c := strings.TrimSpace(s)
	if c == "" {
		return false
	}
	return strings.Contains(c, "工具函数管理") ||
		strings.Contains(c, "未启用任何工具") ||
		strings.Contains(c, "开启对应工具") ||
		strings.Contains(c, "请先在「工具函数管理」")
}

// IsToolEvasionReply 导出给钉钉出站拦截、chat 工具循环复用。
func IsToolEvasionReply(s string) bool {
	return isToolEvasionReply(s)
}

// retryAfterEvasion 推诿后再跑一轮：强制工具，成功则返回新正文。
func (s *Service) retryAfterEvasion(
	ctx context.Context,
	client *llm.Client,
	providerName, modelName string,
	enableSearch bool,
	toolSpecs []llm.ToolSpec,
	msgs []llm.Message,
	in RunInput,
	run *model.AgentRun,
	stepIndex, promptTokens, completionTokens *int,
	emit func(Event) error,
) (string, bool) {
	if s == nil || client == nil || len(toolSpecs) == 0 {
		return "", false
	}
	retryMsgs := append([]llm.Message(nil), msgs...)
	retryMsgs = append(retryMsgs,
		llm.Message{Role: "assistant", Content: toolEvasionFallback},
		llm.Message{Role: "user", Content: "请立刻调用已挂载的工具核实，不要再说未启用工具或去工具函数管理。"},
	)
	start := time.Now()
	chatReq := llm.ChatRequest{
		Model: modelName, Messages: retryMsgs, Tools: toolSpecs,
		EnableSearch: enableSearch, ToolChoice: "required",
	}
	var resp *llm.ChatResponse
	var err error
	if in.Stream {
		resp, err = client.ChatStream(ctx, chatReq, func(ev llm.StreamEvent) error { return nil })
	} else {
		resp, err = client.Chat(ctx, chatReq)
	}
	if err != nil && chatReq.ToolChoice == "required" {
		chatReq.ToolChoice = ""
		if in.Stream {
			resp, err = client.ChatStream(ctx, chatReq, func(ev llm.StreamEvent) error { return nil })
		} else {
			resp, err = client.Chat(ctx, chatReq)
		}
	}
	pt, ct := 0, 0
	respContent := ""
	var toolCalls any
	if resp != nil {
		pt, ct = resp.PromptTokens, resp.CompletionTokens
		*promptTokens += pt
		*completionTokens += ct
		respContent = resp.Content
		if len(resp.ToolCalls) > 0 {
			toolCalls = resp.ToolCalls
		}
	}
	status := "ok"
	errMsg := ""
	if err != nil {
		status = "error"
		errMsg = err.Error()
	}
	llmog.Save(s.db, s.llmLog, &model.LLMCallLog{
		RequestID: in.RequestID, ConversationID: in.ConversationID, AgentRunID: &run.ID,
		Provider: providerName, Model: modelName, Stream: in.Stream, Status: status,
		PromptTokens: pt, CompletionTokens: ct, LatencyMs: time.Since(start).Milliseconds(),
		RequestSummary: fmt.Sprintf("agent_step=evasion_retry tools=%d", len(toolSpecs)), ErrorMessage: errMsg,
	}, &llmog.Payload{
		Messages: retryMsgs, Response: respContent, ToolCalls: toolCalls,
	})
	if err != nil || resp == nil {
		return "", false
	}
	*stepIndex++
	_ = s.db.Create(&model.AgentStep{
		RunID: run.ID, StepIndex: *stepIndex, Kind: "llm",
		OutputText: resp.Content, InputJSON: `{"evasion_retry":true}`,
	}).Error
	if len(resp.ToolCalls) == 0 {
		if isToolEvasionReply(resp.Content) {
			return "", false
		}
		return resp.Content, resp.Content != ""
	}
	retryMsgs = append(retryMsgs, llm.Message{Role: "assistant", Content: resp.Content, ToolCalls: resp.ToolCalls})
	defaultTopK := 5
	if s.cfg != nil {
		defaultTopK = s.cfg.RAG.TopK
	}
	toolEnv := &tools.Env{
		CorpusID: in.CorpusID, CorpusIDs: in.CorpusIDs, TopK: in.TopK,
		DefaultTopK: defaultTopK, RAGQuery: in.Input, RAGDisabled: in.SkipRAG, RAG: s.rag,
	}
	for _, tc := range resp.ToolCalls {
		if emit != nil {
			_ = emit(Event{Type: "tool_call", Payload: map[string]any{
				"id": tc.ID, "name": tc.Function.Name, "arguments": tc.Function.Arguments,
			}})
		}
		result, toolErr := s.registry.Exec(ctx, tc.Function.Name, tc.Function.Arguments, toolEnv)
		if toolErr != nil {
			result = toolErr.Error()
		}
		*stepIndex++
		inJSON, _ := json.Marshal(map[string]string{"arguments": tc.Function.Arguments})
		_ = s.db.Create(&model.AgentStep{
			RunID: run.ID, StepIndex: *stepIndex, Kind: "tool_result",
			ToolName: tc.Function.Name, InputJSON: string(inJSON), OutputText: result,
		}).Error
		if emit != nil {
			_ = emit(Event{Type: "tool_result", Payload: map[string]any{
				"name": tc.Function.Name, "content": result,
			}})
		}
		retryMsgs = append(retryMsgs, llm.Message{
			Role: "tool", ToolCallID: tc.ID, Name: tc.Function.Name, Content: result,
		})
	}
	start = time.Now()
	finalReq := llm.ChatRequest{Model: modelName, Messages: retryMsgs, Tools: toolSpecs, EnableSearch: enableSearch}
	var finalResp *llm.ChatResponse
	if in.Stream {
		finalResp, err = client.ChatStream(ctx, finalReq, func(ev llm.StreamEvent) error {
			if ev.Content != "" && emit != nil {
				return emit(Event{Type: "delta", Payload: map[string]any{"content": ev.Content}})
			}
			return nil
		})
	} else {
		finalResp, err = client.Chat(ctx, finalReq)
	}
	pt, ct = 0, 0
	respContent = ""
	if finalResp != nil {
		pt, ct = finalResp.PromptTokens, finalResp.CompletionTokens
		*promptTokens += pt
		*completionTokens += ct
		respContent = finalResp.Content
	}
	status = "ok"
	errMsg = ""
	if err != nil {
		status = "error"
		errMsg = err.Error()
	}
	llmog.Save(s.db, s.llmLog, &model.LLMCallLog{
		RequestID: in.RequestID, ConversationID: in.ConversationID, AgentRunID: &run.ID,
		Provider: providerName, Model: modelName, Stream: in.Stream, Status: status,
		PromptTokens: pt, CompletionTokens: ct, LatencyMs: time.Since(start).Milliseconds(),
		RequestSummary: fmt.Sprintf("agent_step=evasion_final tools=%d", len(toolSpecs)), ErrorMessage: errMsg,
	}, &llmog.Payload{Messages: retryMsgs, Response: respContent})
	if err != nil || finalResp == nil || strings.TrimSpace(finalResp.Content) == "" || isToolEvasionReply(finalResp.Content) {
		return "", false
	}
	*stepIndex++
	_ = s.db.Create(&model.AgentStep{
		RunID: run.ID, StepIndex: *stepIndex, Kind: "llm",
		OutputText: finalResp.Content, InputJSON: `{"evasion_final":true}`,
	}).Error
	return finalResp.Content, true
}
