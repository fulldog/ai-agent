package metrics

import (
	"strconv"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	HTTPRequests = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "ai_agent_http_requests_total",
		Help: "Total HTTP requests",
	}, []string{"method", "path_template", "status"})

	HTTPDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "ai_agent_http_request_duration_seconds",
		Help:    "HTTP request latency",
		Buckets: prometheus.DefBuckets,
	}, []string{"method", "path_template", "status"})

	LLMRequests = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "ai_agent_llm_requests_total",
		Help: "Total LLM requests",
	}, []string{"model", "stream", "status"})

	LLMDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "ai_agent_llm_request_duration_seconds",
		Help:    "LLM request latency",
		Buckets: []float64{0.1, 0.25, 0.5, 1, 2, 5, 10, 30, 60, 120},
	}, []string{"model", "stream", "status"})

	LLMTokens = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "ai_agent_llm_tokens_total",
		Help: "LLM tokens consumed",
	}, []string{"model", "type"})

	AgentRuns = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "ai_agent_agent_runs_total",
		Help: "Agent runs",
	}, []string{"status"})

	AgentSteps = promauto.NewCounter(prometheus.CounterOpts{
		Name: "ai_agent_agent_steps_total",
		Help: "Agent steps",
	})

	ToolCalls = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "ai_agent_tool_calls_total",
		Help: "Tool calls",
	}, []string{"tool", "status"})

	RAGSearch = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "ai_agent_rag_search_total",
		Help: "RAG searches",
	}, []string{"status"})

	RAGDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "ai_agent_rag_search_duration_seconds",
		Help:    "RAG search latency",
		Buckets: prometheus.DefBuckets,
	}, []string{"status"})
)

func ObserveHTTP(method, pathTemplate string, status int, seconds float64) {
	st := strconv.Itoa(status)
	HTTPRequests.WithLabelValues(method, pathTemplate, st).Inc()
	HTTPDuration.WithLabelValues(method, pathTemplate, st).Observe(seconds)
}

func ObserveLLM(model string, stream bool, status string, seconds float64, promptTokens, completionTokens int) {
	streamLabel := "false"
	if stream {
		streamLabel = "true"
	}
	LLMRequests.WithLabelValues(model, streamLabel, status).Inc()
	LLMDuration.WithLabelValues(model, streamLabel, status).Observe(seconds)
	if promptTokens > 0 {
		LLMTokens.WithLabelValues(model, "prompt").Add(float64(promptTokens))
	}
	if completionTokens > 0 {
		LLMTokens.WithLabelValues(model, "completion").Add(float64(completionTokens))
	}
}
