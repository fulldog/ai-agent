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
	"github.com/webapp/go-app/ai-agent/internal/service/llm"
	"github.com/webapp/go-app/ai-agent/internal/service/llmog"
	"github.com/webapp/go-app/ai-agent/internal/service/rag"
	"gorm.io/gorm"
)

type Event struct {
	Type    string         `json:"type"` // tool_call|tool_result|delta|done|error
	Payload map[string]any `json:"payload"`
}

type Service struct {
	db  *gorm.DB
	cfg *config.Config
	llm *llm.Client
	rag *rag.Service
}

func New(db *gorm.DB, cfg *config.Config, llmClient *llm.Client, ragSvc *rag.Service) *Service {
	return &Service{db: db, cfg: cfg, llm: llmClient, rag: ragSvc}
}

type RunInput struct {
	ConversationID *uuid.UUID
	Input          string
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
	modelName := in.Model
	if modelName == "" {
		modelName = s.cfg.LLM.DefaultModel
	}
	tools := in.Tools
	if len(tools) == 0 {
		tools = s.cfg.Agent.DefaultTools
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
		{Role: "system", Content: "You are a helpful AI agent. Use tools when needed to answer accurately."},
		{Role: "user", Content: in.Input},
	}
	toolSpecs := s.toolSpecs(tools)
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
			resp, err = s.llm.ChatStream(ctx, llm.ChatRequest{
				Model: modelName, Messages: msgs, Tools: toolSpecs,
			}, func(ev llm.StreamEvent) error {
				if ev.Content != "" && emit != nil {
					return emit(Event{Type: "delta", Payload: map[string]any{"content": ev.Content}})
				}
				return nil
			})
		} else {
			resp, err = s.llm.Chat(ctx, llm.ChatRequest{
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
		llmog.Save(s.db, &model.LLMCallLog{
			RequestID: in.RequestID, ConversationID: in.ConversationID, AgentRunID: &run.ID,
			Provider: s.cfg.LLM.Provider, Model: modelName, Stream: in.Stream, Status: status,
			PromptTokens: pt, CompletionTokens: ct, LatencyMs: time.Since(start).Milliseconds(),
			RequestSummary: fmt.Sprintf("agent_step=%d tools=%d", step, len(toolSpecs)), ErrorMessage: errMsg,
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
			result, toolErr := s.execTool(ctx, tc.Function.Name, tc.Function.Arguments, in.CorpusID, in.TopK)
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
		// last assistant content fallback
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

func (s *Service) toolSpecs(names []string) []llm.ToolSpec {
	all := map[string]llm.ToolSpec{
		"knowledge_search": {
			Type: "function",
			Function: llm.ToolSpecFunc{
				Name:        "knowledge_search",
				Description: "Search the knowledge base (RAG) for relevant passages",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"query": map[string]any{"type": "string", "description": "search query"},
					},
					"required": []string{"query"},
				},
			},
		},
		"current_time": {
			Type: "function",
			Function: llm.ToolSpecFunc{
				Name:        "current_time",
				Description: "Get the current server time in RFC3339",
				Parameters: map[string]any{
					"type":       "object",
					"properties": map[string]any{},
				},
			},
		},
	}
	var out []llm.ToolSpec
	for _, n := range names {
		if spec, ok := all[n]; ok {
			out = append(out, spec)
		}
	}
	return out
}

func (s *Service) execTool(ctx context.Context, name, args string, corpusID *uuid.UUID, topK int) (string, error) {
	switch name {
	case "current_time":
		return time.Now().Format(time.RFC3339), nil
	case "knowledge_search":
		var p struct {
			Query string `json:"query"`
		}
		_ = json.Unmarshal([]byte(args), &p)
		if strings.TrimSpace(p.Query) == "" {
			return "", fmt.Errorf("query is required")
		}
		if corpusID == nil {
			return "", fmt.Errorf("corpus_id is required for knowledge_search")
		}
		if topK <= 0 {
			topK = s.cfg.RAG.TopK
		}
		hits, err := s.rag.Search(ctx, *corpusID, p.Query, topK)
		if err != nil {
			return "", err
		}
		if len(hits) == 0 {
			return "no results", nil
		}
		var b strings.Builder
		for i, h := range hits {
			b.WriteString(fmt.Sprintf("[%d] (distance=%.4f) %s\n", i+1, h.Score, h.Content))
		}
		return b.String(), nil
	default:
		return "", fmt.Errorf("unknown tool: %s", name)
	}
}
