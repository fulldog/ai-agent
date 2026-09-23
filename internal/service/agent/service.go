package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
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

var errUIDRequired = errors.New("uid required")

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
	TopK           int
	RAGHits        []rag.Hit
	EnableSearch   bool
	RequestID      string
	Stream         bool
}

type RunResult struct {
	RunID            uuid.UUID
	Output           string
	StepCount        int
	PromptTokens     int
	CompletionTokens int
	Status           string
}

type ToolInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Default     bool   `json:"default"`
}

func (s *Service) ListTools() []ToolInfo {
	if s == nil {
		return nil
	}
	listed := s.registry.List()
	var defaults []string
	if s.cfg != nil {
		defaults = s.cfg.Agent.DefaultTools
	}
	want := make(map[string]struct{}, len(defaults))
	for _, n := range defaults {
		want[n] = struct{}{}
	}
	out := make([]ToolInfo, 0, len(listed))
	for _, t := range listed {
		_, def := want[t.Name]
		if len(want) == 0 {
			def = true
		}
		out = append(out, ToolInfo{Name: t.Name, Description: t.Description, Default: def})
	}
	return out
}

type ListRunsInput struct {
	UID            string
	All            bool
	ConversationID *uuid.UUID
	Status         string
	Limit          int
	Offset         int
}

func normalizeListRuns(uid string, all bool, limit, offset int) (string, int, int, error) {
	uid = strings.TrimSpace(uid)
	if !all && uid == "" {
		return "", 0, 0, errUIDRequired
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 200 {
		limit = 200
	}
	if offset < 0 {
		offset = 0
	}
	return uid, limit, offset, nil
}

func runUIDMatch(run *model.AgentRun, uid string, admin bool) bool {
	if admin {
		return true
	}
	return run != nil && run.UID != "" && run.UID == uid
}

func (s *Service) GetRun(id uuid.UUID) (*model.AgentRun, []model.AgentStep, error) {
	return s.GetRunFor(id, "", true)
}

func (s *Service) GetRunFor(id uuid.UUID, uid string, admin bool) (*model.AgentRun, []model.AgentStep, error) {
	var run model.AgentRun
	if err := s.db.First(&run, "id = ?", id).Error; err != nil {
		return nil, nil, err
	}
	if !runUIDMatch(&run, uid, admin) && !s.runOwnedViaConversation(&run, uid) {
		return nil, nil, gorm.ErrRecordNotFound
	}
	var steps []model.AgentStep
	if err := s.db.Where("run_id = ?", id).Order("step_index asc").Find(&steps).Error; err != nil {
		return nil, nil, err
	}
	return &run, steps, nil
}

func (s *Service) runOwnedViaConversation(run *model.AgentRun, uid string) bool {
	if run == nil || run.ConversationID == nil || strings.TrimSpace(uid) == "" {
		return false
	}
	if run.UID != "" {
		return false
	}
	var n int64
	if err := s.db.Model(&model.Conversation{}).
		Where("id = ? AND uid = ?", *run.ConversationID, uid).
		Count(&n).Error; err != nil {
		return false
	}
	return n > 0
}

// ListRuns 分页列出 Agent 运行。all=true 时列出全库（可选 uid 过滤）；否则必须带 uid。
func (s *Service) ListRuns(in ListRunsInput) ([]model.AgentRun, int64, error) {
	uid, limit, offset, err := normalizeListRuns(in.UID, in.All, in.Limit, in.Offset)
	if err != nil {
		return nil, 0, err
	}
	q := s.db.Model(&model.AgentRun{})
	if uid != "" {
		if in.All {
			q = q.Where("uid = ?", uid)
		} else {
			q = q.Where(
				"uid = ? OR (uid = '' AND conversation_id IN (SELECT id FROM conversations WHERE uid = ? AND deleted_at IS NULL))",
				uid, uid,
			)
		}
	}
	if in.ConversationID != nil {
		q = q.Where("conversation_id = ?", *in.ConversationID)
	}
	if status := strings.TrimSpace(in.Status); status != "" {
		q = q.Where("status = ?", status)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []model.AgentRun
	err = q.Order("created_at desc").Limit(limit).Offset(offset).Find(&rows).Error
	return rows, total, err
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
	}
	client, providerName, modelName, err := s.pool.Resolve(in.Provider, in.Model)
	if err != nil {
		return nil, err
	}
	toolNames := in.Tools
	if len(toolNames) == 0 {
		toolNames = s.cfg.Agent.DefaultTools
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
	if len(toolSpecs) == 0 {
		s.llmLog.Warn("agent run has no tool specs",
			zap.Strings("requested", toolNames),
			zap.String("request_id", in.RequestID),
		)
	}
	if in.ConversationID != nil {
		userMsg := model.Message{ConversationID: *in.ConversationID, Role: "user", Content: in.Input}
		if err := s.db.Create(&userMsg).Error; err != nil {
			return nil, err
		}
	}
	system := mergeAgentSystem(agentSystemPrompt(toolSpecs), in.RAGHits)
	msgs := initialMessages(system, history, in.Input)
	enableSearch := in.EnableSearch && strings.EqualFold(providerName, "qwen")
	toolEnv := &tools.Env{
		CorpusID:    in.CorpusID,
		TopK:        in.TopK,
		DefaultTopK: s.cfg.RAG.TopK,
		RAG:         s.rag,
	}
	promptTokens, completionTokens := 0, 0
	stepIndex := 0
	final := ""

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
		if in.Stream {
			resp, err = client.ChatStream(ctx, llm.ChatRequest{
				Model: modelName, Messages: msgs, Tools: toolSpecs, EnableSearch: enableSearch,
			}, func(ev llm.StreamEvent) error {
				if ev.Content != "" && emit != nil {
					return emit(Event{Type: "delta", Payload: map[string]any{"content": ev.Content}})
				}
				return nil
			})
		} else {
			resp, err = client.Chat(ctx, llm.ChatRequest{
				Model: modelName, Messages: msgs, Tools: toolSpecs, EnableSearch: enableSearch,
			})
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
	now := time.Now()
	_ = s.db.Model(run).Updates(map[string]any{
		"status": "succeeded", "output": final, "finished_at": &now,
		"step_count": stepIndex, "prompt_tokens": promptTokens, "completion_tokens": completionTokens,
	}).Error
	if in.ConversationID != nil {
		pt, ct := promptTokens, completionTokens
		asst := model.Message{
			ConversationID:  *in.ConversationID,
			Role:            "assistant",
			Content:         final,
			TokenPrompt:     &pt,
			TokenCompletion: &ct,
		}
		_ = s.db.Create(&asst).Error
	}
	metrics.AgentRuns.WithLabelValues("succeeded").Inc()
	if emit != nil {
		_ = emit(Event{Type: "done", Payload: map[string]any{
			"status": "ok", "run_id": run.ID.String(), "output": final,
		}})
	}
	return &RunResult{
		RunID: run.ID, Output: final, StepCount: stepIndex,
		PromptTokens: promptTokens, CompletionTokens: completionTokens, Status: "succeeded",
	}, nil
}

const (
	baseAgentPrompt = "你是有用的 AI 助手。需要准确信息时请调用工具，并根据工具结果作答。回答简洁直接，先给结论；不要套话、不要重复问题或工具原文。"
	// toolsReadyPrompt 有工具 Spec 时追加：防止模型跟语料说「去工具函数管理开启」。
	toolsReadyPrompt  = "下方「当前可用工具」已由服务端挂载到本次请求，可直接 function call，无需用户去任何「工具函数管理」或后台开关。禁止回答「未启用工具/请先开启工具」之类推诿；知识摘录仅供参考，与工具可用性无关。"
	dbconnAgentPrompt = "涉及供应商、付款、订单、业务表或系统数据时必须调用工具核实，不要凭语料或猜测下结论。流程：先用 knowledge_search 查口径或规则（可选）；再用 dbconn 的 schema 对照表与列注释；最后组织只读 SELECT 并调用 dbconn 的 query 执行。不要只把 SQL 写在回复里。语料未命中或无法对应到表时，说明缺什么，不要编造表名或数据。"
	ragOnlyHint       = "【说明】下列知识摘录供参考；若与「当前可用工具」冲突，以工具调用为准，不要根据摘录要求用户去开启工具。"
)

func agentSystemPrompt(specs []llm.ToolSpec) string {
	if len(specs) == 0 {
		return baseAgentPrompt
	}
	names := make([]string, 0, len(specs))
	hasDB := false
	for _, spec := range specs {
		names = append(names, spec.Function.Name)
		if spec.Function.Name == "dbconn" {
			hasDB = true
		}
	}
	var b strings.Builder
	b.WriteString(baseAgentPrompt)
	b.WriteString("\n\n")
	b.WriteString(toolsReadyPrompt)
	b.WriteString("\n当前可用工具：")
	b.WriteString(strings.Join(names, "、"))
	b.WriteString("。")
	if hasDB {
		b.WriteString("\n\n")
		b.WriteString(dbconnAgentPrompt)
	}
	return b.String()
}

func mergeAgentSystem(base string, hits []rag.Hit) string {
	block := rag.HitsPrompt(hits)
	base = strings.TrimSpace(base)
	if block == "" {
		return base
	}
	block = ragOnlyHint + "\n\n" + block
	if base == "" {
		return block
	}
	return base + "\n\n" + block
}

func initialMessages(system string, history []model.Message, user string) []llm.Message {
	var msgs []llm.Message
	if strings.TrimSpace(system) != "" {
		msgs = append(msgs, llm.Message{Role: "system", Content: system})
	}
	for _, m := range history {
		if m.Role == "system" {
			continue
		}
		msgs = append(msgs, llm.Message{Role: m.Role, Content: m.Content})
	}
	msgs = append(msgs, llm.Message{Role: "user", Content: user})
	return msgs
}

func (s *Service) maxHistory() int {
	if s != nil && s.cfg != nil && s.cfg.LLM.MaxHistory > 0 {
		return s.cfg.LLM.MaxHistory
	}
	return 10
}

func (s *Service) listRecentMessages(conversationID uuid.UUID, limit int) ([]model.Message, error) {
	if limit <= 0 {
		limit = 10
	}
	var rows []model.Message
	err := s.db.Where("conversation_id = ?", conversationID).
		Order("created_at desc").
		Limit(limit).
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	slices.Reverse(rows)
	return rows, nil
}
