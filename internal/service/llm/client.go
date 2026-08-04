package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/webapp/go-app/ai-agent/internal/metrics"
)

type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	Name       string     `json:"name,omitempty"`
}

type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function ToolFunction `json:"function"`
}

type ToolFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type ToolSpec struct {
	Type     string       `json:"type"`
	Function ToolSpecFunc `json:"function"`
}

type ToolSpecFunc struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

type ChatRequest struct {
	Model       string
	Messages    []Message
	Temperature float64
	MaxTokens   int
	Tools       []ToolSpec
	ToolChoice  string
}

type ChatResponse struct {
	Content          string
	ToolCalls        []ToolCall
	FinishReason     string
	PromptTokens     int
	CompletionTokens int
	LatencyMs        int64
}

type StreamEvent struct {
	Content   string
	ToolCalls []ToolCall
	Done      bool
	Usage     *ChatResponse
	Err       error
}

type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

func NewClient(baseURL, apiKey string, timeoutSec int) *Client {
	if timeoutSec <= 0 {
		timeoutSec = 120
	}
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: time.Duration(timeoutSec) * time.Second,
		},
	}
}

func (c *Client) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	start := time.Now()
	status := "ok"
	var out *ChatResponse
	var err error
	defer func() {
		pt, ct := 0, 0
		if out != nil {
			pt, ct = out.PromptTokens, out.CompletionTokens
		}
		if err != nil {
			status = "error"
		}
		metrics.ObserveLLM(req.Model, false, status, time.Since(start).Seconds(), pt, ct)
	}()

	body := c.buildBody(req, false)
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	c.setHeaders(httpReq)
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		err = fmt.Errorf("llm error %d: %s", resp.StatusCode, string(raw))
		return nil, err
	}
	var parsed completionResponse
	if err = json.Unmarshal(raw, &parsed); err != nil {
		return nil, err
	}
	out = &ChatResponse{
		PromptTokens:     parsed.Usage.PromptTokens,
		CompletionTokens: parsed.Usage.CompletionTokens,
		LatencyMs:        time.Since(start).Milliseconds(),
	}
	if len(parsed.Choices) > 0 {
		out.Content = parsed.Choices[0].Message.Content
		out.ToolCalls = parsed.Choices[0].Message.ToolCalls
		out.FinishReason = parsed.Choices[0].FinishReason
	}
	return out, nil
}

func (c *Client) ChatStream(ctx context.Context, req ChatRequest, onChunk func(StreamEvent) error) (*ChatResponse, error) {
	start := time.Now()
	status := "ok"
	usage := &ChatResponse{}
	var err error
	defer func() {
		if err != nil {
			status = "error"
		}
		metrics.ObserveLLM(req.Model, true, status, time.Since(start).Seconds(), usage.PromptTokens, usage.CompletionTokens)
	}()

	body := c.buildBody(req, true)
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	c.setHeaders(httpReq)
	// streaming needs no overall client timeout blocking; use context
	client := &http.Client{Timeout: 0}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(resp.Body)
		err = fmt.Errorf("llm stream error %d: %s", resp.StatusCode, string(raw))
		return nil, err
	}

	reader := bufio.NewReader(resp.Body)
	var content strings.Builder
	toolAcc := map[int]*ToolCall{}

	for {
		line, readErr := reader.ReadString('\n')
		if readErr != nil {
			if readErr == io.EOF {
				break
			}
			err = readErr
			return nil, err
		}
		line = strings.TrimSpace(line)
		if line == "" || !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			break
		}
		var chunk streamResponse
		if uerr := json.Unmarshal([]byte(data), &chunk); uerr != nil {
			continue
		}
		if chunk.Usage != nil {
			usage.PromptTokens = chunk.Usage.PromptTokens
			usage.CompletionTokens = chunk.Usage.CompletionTokens
		}
		if len(chunk.Choices) == 0 {
			continue
		}
		delta := chunk.Choices[0].Delta
		if delta.Content != "" {
			content.WriteString(delta.Content)
			if onChunk != nil {
				if err = onChunk(StreamEvent{Content: delta.Content}); err != nil {
					return nil, err
				}
			}
		}
		for _, tc := range delta.ToolCalls {
			idx := tc.Index
			acc, ok := toolAcc[idx]
			if !ok {
				acc = &ToolCall{ID: tc.ID, Type: tc.Type, Function: ToolFunction{Name: tc.Function.Name}}
				toolAcc[idx] = acc
			}
			if tc.ID != "" {
				acc.ID = tc.ID
			}
			if tc.Type != "" {
				acc.Type = tc.Type
			}
			if tc.Function.Name != "" {
				acc.Function.Name = tc.Function.Name
			}
			acc.Function.Arguments += tc.Function.Arguments
		}
	}

	var tools []ToolCall
	for i := 0; i < len(toolAcc); i++ {
		if tc, ok := toolAcc[i]; ok {
			tools = append(tools, *tc)
		}
	}
	usage.Content = content.String()
	usage.ToolCalls = tools
	usage.LatencyMs = time.Since(start).Milliseconds()
	if onChunk != nil {
		_ = onChunk(StreamEvent{Done: true, Usage: usage})
	}
	return usage, nil
}

func (c *Client) buildBody(req ChatRequest, stream bool) map[string]any {
	body := map[string]any{
		"model":    req.Model,
		"messages": req.Messages,
		"stream":   stream,
	}
	if stream {
		body["stream_options"] = map[string]any{"include_usage": true}
	}
	if req.Temperature > 0 {
		body["temperature"] = req.Temperature
	}
	if req.MaxTokens > 0 {
		body["max_tokens"] = req.MaxTokens
	}
	if len(req.Tools) > 0 {
		body["tools"] = req.Tools
		if req.ToolChoice != "" {
			body["tool_choice"] = req.ToolChoice
		}
	}
	return body
}

func (c *Client) setHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
}

type completionResponse struct {
	Choices []struct {
		FinishReason string  `json:"finish_reason"`
		Message      Message `json:"message"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
}

type streamResponse struct {
	Choices []struct {
		Delta struct {
			Content   string `json:"content"`
			ToolCalls []struct {
				Index    int    `json:"index"`
				ID       string `json:"id"`
				Type     string `json:"type"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"delta"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
}
