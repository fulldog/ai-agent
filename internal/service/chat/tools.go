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

// toolSpecs 普通对话要发给模型的工具。only 为 false 时始终包含 agent.default_tools，
// 并在 chat.tools_enabled 时并入 chat.tools，再追加 extra。only 为 true 时只用 extra。
func (s *Service) toolSpecs(extra []string, only bool) []llm.ToolSpec {
	if s == nil || s.registry == nil {
		return nil
	}
	if only {
		return s.registry.Specs(extra)
	}
	if s.cfg == nil {
		return nil
	}
	var configured []string
	if s.cfg.Chat.IsToolsEnabled() {
		configured = s.cfg.Chat.Tools
	}
	names := tools.MergeNames(s.cfg.Agent.DefaultTools, configured, extra)
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
	if !in.SkipRAG && corpusID == nil && conv != nil {
		corpusID = conv.CorpusID
	}
	var corpusIDs []uuid.UUID
	if !in.SkipRAG {
		corpusIDs = in.CorpusIDs
	}
	env := &tools.Env{
		CorpusID: corpusID, CorpusIDs: corpusIDs, TopK: in.TopK,
		RAGQuery: in.Message, RAGDisabled: in.SkipRAG, RAG: s.rag,
		Dictionary: in.Dictionary,
	}
	if s.cfg != nil {
		env.DefaultTopK = s.cfg.RAG.TopK
	}
	if conv != nil && conv.ID != uuid.Nil {
		conversationID := conv.ID
		env.SaveSQL = func(sqlText string) {
			if err := agent.SaveExecutedSQL(s.db, conversationID, sqlText); err != nil && s.llmLog != nil {
				s.llmLog.Error("save executed sql", zap.Error(err), zap.String("request_id", in.RequestID))
			}
		}
	}
	return env
}

// toolLoop 执行「LLM → 工具 → LLM」循环，直到模型不再请求工具或用完工具轮数。
// onDelta 非 nil 时走流式。返回最后一轮响应，token 为各轮累计值。
func (s *Service) toolLoop(ctx context.Context, in CompleteInput, call callContext, msgs []llm.Message, onDelta func(string) error) (*llm.ChatResponse, error) {
	specs := s.toolSpecs(in.Tools, in.ToolsOnly)
	rounds := 1
	if len(specs) > 0 {
		rounds = s.maxToolSteps() + 1 // 最后一轮不带工具，强制模型给出回答
	}
	env := s.toolEnv(in, call.conv)
	promptTokens, completionTokens := 0, 0
	var last *llm.ChatResponse
	usedTool := false
	allowClarify := in.PrefixSkipsQuery || in.SkipRAG

	for round := 0; round < rounds; round++ {
		req := chatRequest(in, call.provider, call.model, msgs)
		if round < rounds-1 {
			req.Tools = specs
			if len(specs) > 0 && !usedTool && !allowClarify {
				req.ToolChoice = "required"
			}
		}
		start := time.Now()
		var resp *llm.ChatResponse
		var err error
		suppressDelta := len(req.Tools) > 0 && !usedTool && !allowClarify
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
	hasDB := false
	hasFile := false
	for _, spec := range specs {
		switch spec.Function.Name {
		case "dbconn":
			hasDB = true
		case "generate_file":
			hasFile = true
		}
	}
	if hasDB {
		prompt += "\n" + dbconnToolPrompt
	}
	if hasFile {
		prompt += "\n" + fileToolPrompt
	}
	return prompt
}

const (
	baseToolPrompt   = "需要准确信息时调用工具，并依据工具结果作答；不需要时直接回答，不要为了用工具而用工具。本次请求若已附带 tools，说明工具已启用，禁止回答「未启用工具/请到工具函数管理开启」。"
	dbconnToolPrompt = "若问题涉及供应商、付款、订单、业务数据或表结构：要核实数据必须调用 dbconn，不要沿用历史查询结果，也不要凭猜测作答。系统提示里的【数据字典】是表名和列名的唯一依据，写 SQL 前先对照该全文，禁止编造表名或列名。缺标签时先按名称查库，根据本次查询结果再决定是否追问。知识摘录未覆盖时再用 knowledge_search 查口径。不要只把 SQL 写在回复里而不调用工具。"
	fileToolPrompt   = "用户要求导出、下载或生成 Excel/表格文件时：数据来自本次 dbconn query 的，调用 generate_file 只传 filename 和 from_query=true，不要把查询结果再写进参数；中文列名写在 SELECT 别名里。不是查询结果的小表才传 columns 和 rows。回复中必须附上工具返回的下载链接。"
)
