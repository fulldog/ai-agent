package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/webapp/go-app/ai-agent/internal/config"
	"github.com/webapp/go-app/ai-agent/internal/metrics"
	"github.com/webapp/go-app/ai-agent/internal/model"
	"github.com/webapp/go-app/ai-agent/internal/service/agent/tools"
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

func New(db *gorm.DB, cfg *config.Config, pool *llm.Pool, ragSvc *rag.Service, llmLog *zap.Logger) *Service {
	if llmLog == nil {
		llmLog = zap.NewNop()
	}
	return &Service{
		db:       db,
		cfg:      cfg,
		pool:     pool,
		rag:      ragSvc,
		registry: tools.Default(),
		llmLog:   llmLog,
	}
}

type RunInput struct {
	ConversationID *uuid.UUID
	Input          string
	Provider       string
	Model          string
	MaxSteps       int
	Tools          []string
	CorpusID       *uuid.UUID
	TopK           int
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

func (s *Service) GetRun(id uuid.UUID) (*model.AgentRun, []model.AgentStep, error) {
	var run model.AgentRun
	if err := s.db.First(&run, "id = ?", id).Error; err != nil {
		return nil, nil, err
	}
	var steps []model.AgentStep
	if err := s.db.Where("run_id = ?", id).Order("step_index asc").Find(&steps).Error; err != nil {
		return nil, nil, err
	}
	return &run, steps, nil
}

func (s *Service) Run(ctx context.Context, in RunInput, emit func(Event) error) (*RunResult, error) {
	maxSteps := in.MaxSteps
	if maxSteps <= 0 {
		maxSteps = s.cfg.Agent.MaxSteps
	}
	client, providerName, modelName, err := s.pool.Resolve(in.Provider, in.Model)
	if err != nil {
		return nil, err
	}
	toolNames := in.Tools
	if len(toolNames) == 0 {
		toolNames = s.cfg.Agent.DefaultTools
	}

	run := &model.AgentRun{
		ConversationID: in.ConversationID,
		Input:          in.Input,
		Model:          modelName,
		Status:         "running",
		MaxSteps:       maxSteps,
	}
	if err := s.db.Create(run).Error; err != nil {
		return nil, err
	}

	msgs := []llm.Message{
		{Role: "system", Content: "你是有用的 AI 助手。需要准确信息时请调用工具，并根据工具结果作答。"},
		{Role: "user", Content: in.Input},
	}
	toolSpecs := s.registry.Specs(toolNames)
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
				Model: modelName, Messages: msgs, Tools: toolSpecs,
			}, func(ev llm.StreamEvent) error {
				if ev.Content != "" && emit != nil {
					return emit(Event{Type: "delta", Payload: map[string]any{"content": ev.Content}})
				}
				return nil
			})
		} else {
			resp, err = client.Chat(ctx, llm.ChatRequest{
				Model: modelName, Messages: msgs, Tools: toolSpecs,
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
	metrics.AgentRuns.WithLabelValues("succeeded").Inc()
	if emit != nil {
		_ = emit(Event{Type: "done", Payload: map[string]any{"status": "ok", "run_id": run.ID.String()}})
	}
	return &RunResult{
		RunID: run.ID, Output: final, StepCount: stepIndex,
		PromptTokens: promptTokens, CompletionTokens: completionTokens, Status: "succeeded",
	}, nil
}
