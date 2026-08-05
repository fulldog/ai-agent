package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/webapp/go-app/ai-agent/internal/service/agent"
)

type AgentHandler struct {
	Agent *agent.Service
}

func (h *AgentHandler) parse(c *gin.Context) (agent.RunInput, bool) {
	var req struct {
		ConversationID string   `json:"conversation_id"`
		Input          string   `json:"input" binding:"required"`
		Provider       string   `json:"provider"`
		Model          string   `json:"model"`
		MaxSteps       int      `json:"max_steps"`
		Tools          []string `json:"tools"`
		RAG            *struct {
			CorpusID string `json:"corpus_id"`
			TopK     int    `json:"top_k"`
		} `json:"rag"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "bad_request", err.Error())
		return agent.RunInput{}, false
	}
	in := agent.RunInput{
		Input: req.Input, Provider: req.Provider, Model: req.Model, MaxSteps: req.MaxSteps,
		Tools: req.Tools, RequestID: requestID(c),
	}
	if req.ConversationID != "" {
		id, err := uuid.Parse(req.ConversationID)
		if err != nil {
			writeError(c, http.StatusBadRequest, "bad_request", "invalid conversation_id")
			return agent.RunInput{}, false
		}
		in.ConversationID = &id
	}
	if req.RAG != nil && req.RAG.CorpusID != "" {
		id, err := uuid.Parse(req.RAG.CorpusID)
		if err != nil {
			writeError(c, http.StatusBadRequest, "bad_request", "invalid corpus_id")
			return agent.RunInput{}, false
		}
		in.CorpusID = &id
		in.TopK = req.RAG.TopK
	}
	return in, true
}

func (h *AgentHandler) Run(c *gin.Context) {
	in, ok := h.parse(c)
	if !ok {
		return
	}
	in.Stream = false
	res, err := h.Agent.Run(c.Request.Context(), in, nil)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"run_id": res.RunID, "output": res.Output, "status": res.Status,
		"step_count": res.StepCount,
		"usage": gin.H{
			"prompt_tokens": res.PromptTokens, "completion_tokens": res.CompletionTokens,
			"total_tokens": res.PromptTokens + res.CompletionTokens,
		},
	})
}

func (h *AgentHandler) RunStream(c *gin.Context) {
	in, ok := h.parse(c)
	if !ok {
		return
	}
	in.Stream = true
	setupSSE(c)
	_, err := h.Agent.Run(c.Request.Context(), in, func(ev agent.Event) error {
		return writeSSE(c, ev.Type, ev.Payload)
	})
	if err != nil {
		_ = writeSSE(c, "error", gin.H{"message": err.Error()})
	}
}

func (h *AgentHandler) Get(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		writeError(c, http.StatusBadRequest, "bad_request", "invalid id")
		return
	}
	run, steps, err := h.Agent.GetRun(id)
	if err != nil {
		writeError(c, http.StatusNotFound, "not_found", "run not found")
		return
	}
	c.JSON(http.StatusOK, gin.H{"run": run, "steps": steps})
}
