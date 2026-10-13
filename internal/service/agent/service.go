package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/webapp/go-app/ai-agent/internal/config"
	"github.com/webapp/go-app/ai-agent/internal/metrics"
	"github.com/webapp/go-app/ai-agent/internal/model"
	"github.com/webapp/go-app/ai-agent/internal/service/agent/tools"
	"github.com/webapp/go-app/ai-agent/internal/service/dbconn"
	"github.com/webapp/go-app/ai-agent/internal/service/llm"
	"github.com/webapp/go-app/ai-agent/internal/service/llmog"
	"github.com/webapp/go-app/ai-agent/internal/service/rag"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type Event struct {
	Type    string         `json:"type"` // tool_call|tool_result|delta|done|error
	Payload map[string]any `json:"payload"`
}

type Service struct {
	db       *gorm.DB
	cfg      *config.Config
	pool     *llm.Pool
	rag      *rag.Service
	registry *tools.Registry
	llmLog   *zap.Logger // 完整 prompt/回复 → logs/llm-*.log
}

func New(db *gorm.DB, cfg *config.Config, pool *llm.Pool, ragSvc *rag.Service, llmLog *zap.Logger, bizDB *dbconn.Client) *Service {
	if llmLog == nil {
		llmLog = zap.NewNop()
	}
	reg := tools.Default()
	if bizDB != nil {
		reg = tools.WithDBConn(bizDB)
	}
	return &Service{
		db:       db,
		cfg:      cfg,
		pool:     pool,
		rag:      ragSvc,
		registry: reg,
		llmLog:   llmLog,
	}
}

type RunInput struct {
	ConversationID *uuid.UUID
	UID            string
	Admin          bool
	Input          string
	Provider       string
	Model          string
	MaxSteps       int
	Tools          []string
	CorpusID       *uuid.UUID
	CorpusIDs      []uuid.UUID
	TopK           int
	RAGHits        []rag.Hit
	EnableSearch   bool
	PrefixPrompt   string
	// SkipRAG 为 true 时不预检索，knowledge_search 也不查库。
	SkipRAG bool
	// ToolsOnly 为 true 时只用 Tools，不并入 agent.default_tools。空列表表示本轮不挂工具。
	ToolsOnly bool
	RequestID string
	Stream    bool
}

type RunResult struct {
	RunID            uuid.UUID
	MessageID        uuid.UUID
	Output           string
	StepCount        int
	PromptTokens     int
	CompletionTokens int
	Status           string
}

func (s *Service) Run(ctx context.Context, in RunInput, emit func(Event) error) (*RunResult, error) {
	maxSteps := in.MaxSteps
	if maxSteps <= 0 {
		maxSteps = s.cfg.Agent.MaxSteps
	}
	if in.ConversationID != nil {
		var conv model.Conversation
		q := s.db.Where("id = ?", *in.ConversationID)
		if !in.Admin {
			uid := strings.TrimSpace(in.UID)
			if uid == "" {
				return nil, fmt.Errorf("conversation not found")
			}
			q = q.Where("uid = ?", uid)
		}
		if err := q.First(&conv).Error; err != nil {
			return nil, fmt.Errorf("conversation not found")
		}
		if in.CorpusID == nil {
			in.CorpusID = conv.CorpusID
		}
	}
	s.retrieveHits(ctx, &in)
	client, providerName, modelName, err := s.pool.Resolve(in.Provider, in.Model)
	if err != nil {
		return nil, err
	}
	// 控制台等入口强制带上 agent.default_tools。钉钉群 ToolsOnly 时只使用该群勾选的工具。
	var defaults []string
	if s.cfg != nil {
		defaults = s.cfg.Agent.DefaultTools
	}
	var toolNames []string
	if in.ToolsOnly {
		toolNames = append([]string(nil), in.Tools...)
	} else {
		toolNames = tools.MergeNames(defaults, in.Tools)
	}

	var history []model.Message
	if in.ConversationID != nil {
		rows, herr := s.listRecentMessages(*in.ConversationID, s.maxHistory())
		if herr != nil {
			return nil, herr
		}
		history = rows
	}

	run := &model.AgentRun{
		UID:            strings.TrimSpace(in.UID),
		ConversationID: in.ConversationID,
		Input:          in.Input,
		Model:          modelName,
		Status:         "running",
		MaxSteps:       maxSteps,
	}
	if err := s.db.Create(run).Error; err != nil {
		return nil, err
	}

	toolSpecs := s.registry.Specs(toolNames)
	if in.ConversationID != nil {
		userMsg := model.Message{ConversationID: *in.ConversationID, Role: "user", Content: in.Input}
		if err := s.db.Create(&userMsg).Error; err != nil {
			return nil, err
		}
	}
	system := mergeAgentSystem(agentSystemPrompt(toolSpecs), sanitizeRAGHits(in.RAGHits))
	if prefix := strings.TrimSpace(in.PrefixPrompt); prefix != "" {
		if system == "" {
			system = prefix
		} else {
			system = prefix + "\n\n" + system
		}
	}
	msgs := initialMessages(system, history, in.Input)
	enableSearch := in.EnableSearch && strings.EqualFold(providerName, "qwen")
	toolEnv := &tools.Env{
		CorpusID:    in.CorpusID,
		CorpusIDs:   in.CorpusIDs,
		TopK:        in.TopK,
		DefaultTopK: s.cfg.RAG.TopK,
		RAGQuery:    in.Input,
		RAGDisabled: in.SkipRAG,
		RAG:         s.rag,
	}
	promptTokens, completionTokens := 0, 0
	stepIndex := 0
	final := ""
	usedTool := false

	fail := func(err error) (*RunResult, error) {
		metrics.AgentRuns.WithLabelValues("failed").Inc()
		msg := err.Error()
		now := time.Now()
		_ = s.db.Model(run).Updates(map[string]any{
			"status": "failed", "error_message": msg, "finished_at": &now,
			"step_count": stepIndex, "prompt_tokens": promptTokens, "completion_tokens": completionTokens,
		}).Error
		if emit != nil {
			_ = emit(Event{Type: "error", Payload: map[string]any{"message": msg}})
		}
		return &RunResult{RunID: run.ID, Status: "failed", StepCount: stepIndex}, err
	}

	for step := 0; step < maxSteps; step++ {
		start := time.Now()
		var resp *llm.ChatResponse
		var err error
		chatReq := llm.ChatRequest{
			Model: modelName, Messages: msgs, Tools: toolSpecs, EnableSearch: enableSearch,
		}
		// 群前置提示要求缺信息先追问时，首轮允许直接回复，不强制 function call。
		allowClarify := strings.TrimSpace(in.PrefixPrompt) != "" || in.SkipRAG
		// 在至少成功走过一轮工具前，每步都强制 function call（防止网关丢掉 required 后空口推诿）。
		if len(toolSpecs) > 0 && !usedTool && !allowClarify {
			chatReq.ToolChoice = "required"
		}
		// 尚未跑过工具时不把正文 delta 推到钉钉：否则「未启用工具」会先刷到卡片。
		suppressDelta := len(toolSpecs) > 0 && !usedTool && !allowClarify
		streamEmit := func(ev llm.StreamEvent) error {
			if suppressDelta || ev.Content == "" || emit == nil {
				return nil
			}
			return emit(Event{Type: "delta", Payload: map[string]any{"content": ev.Content}})
		}
		if in.Stream {
			resp, err = client.ChatStream(ctx, chatReq, streamEmit)
		} else {
			resp, err = client.Chat(ctx, chatReq)
		}
		// 部分网关不支持 tool_choice=required，回退为 auto。
		if err != nil && chatReq.ToolChoice == "required" {
			s.llmLog.Warn("retry agent step without tool_choice=required", zap.Error(err))
			chatReq.ToolChoice = ""
			if in.Stream {
				resp, err = client.ChatStream(ctx, chatReq, streamEmit)
			} else {
				resp, err = client.Chat(ctx, chatReq)
			}
		}
		status := "ok"
		errMsg := ""
		if err != nil {
			status = "error"
			errMsg = err.Error()
		}
		pt, ct := 0, 0
		if resp != nil {
			pt, ct = resp.PromptTokens, resp.CompletionTokens
			promptTokens += pt
			completionTokens += ct
		}
		respContent := ""
		finishReason := ""
		var toolCalls any
		reqMsgs := append([]llm.Message(nil), msgs...)
		if resp != nil {
			respContent = resp.Content
			finishReason = resp.FinishReason
			if len(resp.ToolCalls) > 0 {
				toolCalls = resp.ToolCalls
			}
		}
		llmog.Save(s.db, s.llmLog, &model.LLMCallLog{
			RequestID: in.RequestID, ConversationID: in.ConversationID, AgentRunID: &run.ID,
			Provider: providerName, Model: modelName, Stream: in.Stream, Status: status,
			PromptTokens: pt, CompletionTokens: ct, LatencyMs: time.Since(start).Milliseconds(),
			RequestSummary: fmt.Sprintf("agent_step=%d tools=%d", step, len(toolSpecs)), ErrorMessage: errMsg,
		}, &llmog.Payload{
			Messages: reqMsgs, Response: respContent, ToolCalls: toolCalls, FinishReason: finishReason,
		})
		if err != nil {
			return fail(err)
		}
		metrics.AgentSteps.Inc()
		stepIndex++
		_ = s.db.Create(&model.AgentStep{
			RunID: run.ID, StepIndex: stepIndex, Kind: "llm",
			OutputText: resp.Content, InputJSON: "{}",
		}).Error

		if len(resp.ToolCalls) == 0 {
			final = resp.Content
			break
		}
		usedTool = true

		msgs = append(msgs, llm.Message{Role: "assistant", Content: resp.Content, ToolCalls: resp.ToolCalls})
		for _, tc := range resp.ToolCalls {
			if emit != nil {
				_ = emit(Event{Type: "tool_call", Payload: map[string]any{
					"id": tc.ID, "name": tc.Function.Name, "arguments": tc.Function.Arguments,
				}})
			}
			result, toolErr := s.registry.Exec(ctx, tc.Function.Name, tc.Function.Arguments, toolEnv)
			toolStatus := "ok"
			if toolErr != nil {
				toolStatus = "error"
				result = toolErr.Error()
			}
			metrics.ToolCalls.WithLabelValues(tc.Function.Name, toolStatus).Inc()
			stepIndex++
			inJSON, _ := json.Marshal(map[string]string{"arguments": tc.Function.Arguments})
			_ = s.db.Create(&model.AgentStep{
				RunID: run.ID, StepIndex: stepIndex, Kind: "tool_result",
				ToolName: tc.Function.Name, InputJSON: string(inJSON), OutputText: result,
			}).Error
			if emit != nil {
				_ = emit(Event{Type: "tool_result", Payload: map[string]any{
					"name": tc.Function.Name, "content": result,
				}})
			}
			msgs = append(msgs, llm.Message{
				Role: "tool", ToolCallID: tc.ID, Name: tc.Function.Name, Content: result,
			})
		}
	}

	if final == "" && len(msgs) > 0 {
		for i := len(msgs) - 1; i >= 0; i-- {
			if msgs[i].Role == "assistant" && msgs[i].Content != "" {
				final = msgs[i].Content
				break
			}
		}
	}
	if isToolEvasionReply(final) && len(toolSpecs) > 0 {
		s.llmLog.Warn("agent evasion reply, retry with tools",
			zap.String("request_id", in.RequestID),
			zap.String("preview", final),
		)
		retryFinal, retryOK := s.retryAfterEvasion(ctx, client, providerName, modelName, enableSearch, toolSpecs, msgs, in, run, &stepIndex, &promptTokens, &completionTokens, emit)
		if retryOK {
			final = retryFinal
		} else {
			final = toolEvasionFallback
		}
	}
	now := time.Now()
	_ = s.db.Model(run).Updates(map[string]any{
		"status": "succeeded", "output": final, "finished_at": &now,
		"step_count": stepIndex, "prompt_tokens": promptTokens, "completion_tokens": completionTokens,
	}).Error
	var messageID uuid.UUID
	if in.ConversationID != nil {
		pt, ct := promptTokens, completionTokens
		asst := model.Message{
			ConversationID:  *in.ConversationID,
			Role:            "assistant",
			Content:         final,
			TokenPrompt:     &pt,
			TokenCompletion: &ct,
		}
		if err := s.db.Create(&asst).Error; err != nil {
			s.llmLog.Error("persist assistant message", zap.Error(err), zap.String("request_id", in.RequestID))
		} else {
			messageID = asst.ID
		}
	}
	metrics.AgentRuns.WithLabelValues("succeeded").Inc()
	if emit != nil {
		_ = emit(Event{Type: "done", Payload: map[string]any{
			"status": "ok", "run_id": run.ID.String(), "output": final,
		}})
	}
	return &RunResult{
		RunID: run.ID, MessageID: messageID, Output: final, StepCount: stepIndex,
		PromptTokens: promptTokens, CompletionTokens: completionTokens, Status: "succeeded",
	}, nil
}
