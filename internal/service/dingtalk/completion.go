package dingtalk

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/webapp/go-app/ai-agent/internal/model"
	"github.com/webapp/go-app/ai-agent/internal/service/agent"
	"github.com/webapp/go-app/ai-agent/internal/service/chat"
	"github.com/webapp/go-app/ai-agent/internal/service/llm"
	"github.com/webapp/go-app/ai-agent/internal/service/rag"
	"go.uber.org/zap"
)

const streamMinInterval = 300 * time.Millisecond

func (b *Bot) completeViaChat(ctx context.Context, data *botCallback, tr *msgTrace, conv *model.Conversation, uid, ragQuery string, corpusID *uuid.UUID, corpusIDs []uuid.UUID, hits []rag.Hit, enableSearch, skipRAG bool) {
	prefix, skipQuery := b.chatPrefix(data.ConversationID)
	historyLimit := b.chatHistoryLimit(data.ConversationID)
	streamer := b.newStreamer(ctx, data, tr)
	var acc strings.Builder
	lastFlush := time.Now()
	tr.outcome = "llm"
	res, err := b.chat.CompleteStream(ctx, chat.CompleteInput{
		ConversationID:   conv.ID,
		UID:              uid,
		Message:          ragQuery,
		RAGEnabled:       len(hits) > 0,
		RAGExplicit:      true,
		CorpusID:         corpusID,
		CorpusIDs:        corpusIDs,
		RAGHits:          hits,
		TopK:             b.ragTop,
		EnableSearch:     enableSearch,
		PrefixPrompt:     prefix,
		PrefixSkipsQuery: skipQuery,
		HistoryLimit:     historyLimit,
		Dictionary:       b.chatDictionary(data.ConversationID),
		Tools:            b.chatTools(data.ConversationID),
		ToolsOnly:        true,
		SkipRAG:          skipRAG,
		RequestID:        tr.requestID,
		LogLLMRequest: func(provider, model string, messages []llm.Message) {
			turns := llmTurnLogs(messages, b.previewMax)
			b.recordStep(tr, stepLLMRequest, eventLLMRequest, map[string]any{
				"provider":        provider,
				"model":           model,
				"enable_search":   enableSearch,
				"rag_enabled":     len(hits) > 0,
				"conversation_id": conv.ID.String(),
				"messages":        len(messages),
				"conversation":    turns,
			},
				zap.String("provider", provider),
				zap.String("model", model),
				zap.Bool("enable_search", enableSearch),
				zap.Bool("rag_enabled", len(hits) > 0),
				zap.String("conversation_id", conv.ID.String()),
				zap.Int("messages", len(messages)),
				zap.Any("conversation", turns),
			)
		},
	}, func(delta string) error {
		acc.WriteString(delta)
		now := time.Now()
		if now.Sub(lastFlush) < streamMinInterval && acc.Len() < 80 {
			return nil
		}
		lastFlush = now
		return streamer.update(acc.String(), false)
	})
	if err != nil {
		b.log.Error("complete stream", zap.Error(err), zap.String("request_id", tr.requestID))
		final := acc.String()
		if final == "" {
			final = llm.PublicMessage(err)
		}
		tr.status = 500
		tr.errMsg = err.Error()
		tr.reply = final
		_ = streamer.finish(final, true)
		return
	}
	text := res.Content
	if strings.TrimSpace(text) == "" {
		text = acc.String()
	}
	if strings.TrimSpace(text) == "" {
		text = "没有生成内容。"
	}
	if res.MessageID != uuid.Nil {
		id := res.MessageID
		tr.assistantMsgID = &id
	}
	tr.reply = text
	_ = streamer.finish(text, false)
}

func (b *Bot) completeViaAgent(ctx context.Context, data *botCallback, tr *msgTrace, conv *model.Conversation, uid, ragQuery string, corpusID *uuid.UUID, corpusIDs []uuid.UUID, hits []rag.Hit, enableSearch, skipRAG bool) {
	prefix, skipQuery := b.chatPrefix(data.ConversationID)
	historyLimit := b.chatHistoryLimit(data.ConversationID)
	streamer := b.newStreamer(ctx, data, tr)
	var acc strings.Builder
	lastFlush := time.Now()
	tr.outcome = "agent"
	cid := conv.ID
	toolTrace := make([]map[string]any, 0, 8)
	res, err := b.agent.Run(ctx, agent.RunInput{
		ConversationID:   &cid,
		UID:              uid,
		Input:            ragQuery,
		CorpusID:         corpusID,
		CorpusIDs:        corpusIDs,
		TopK:             b.ragTop,
		RAGHits:          hits,
		EnableSearch:     enableSearch,
		PrefixPrompt:     prefix,
		PrefixSkipsQuery: skipQuery,
		HistoryLimit:     historyLimit,
		Dictionary:       b.chatDictionary(data.ConversationID),
		Tools:            b.chatTools(data.ConversationID),
		ToolsOnly:        true,
		SkipRAG:          skipRAG,
		RequestID:        tr.requestID,
		Stream:           true,
	}, func(ev agent.Event) error {
		switch ev.Type {
		case "delta":
			content, _ := ev.Payload["content"].(string)
			acc.WriteString(content)
			now := time.Now()
			if now.Sub(lastFlush) < streamMinInterval && acc.Len() < 80 {
				return nil
			}
			lastFlush = now
			return streamer.update(acc.String(), false)
		case "tool_call":
			name, _ := ev.Payload["name"].(string)
			args, _ := ev.Payload["arguments"].(string)
			toolTrace = append(toolTrace, map[string]any{
				"kind":      "tool_call",
				"name":      name,
				"arguments": previewText(args, b.previewMax),
			})
			hint := "正在调用工具…"
			if name == "dbconn" {
				hint = "正在查询业务库…"
			} else if name == "knowledge_search" {
				hint = "正在检索知识库…"
			}
			_ = streamer.update(hint, false)
		case "tool_result":
			name, _ := ev.Payload["name"].(string)
			content, _ := ev.Payload["content"].(string)
			toolTrace = append(toolTrace, map[string]any{
				"kind":    "tool_result",
				"name":    name,
				"content": previewText(content, b.previewMax),
			})
		}
		return nil
	})
	detail := map[string]any{
		"enable_search":   enableSearch,
		"rag_enabled":     len(hits) > 0,
		"conversation_id": conv.ID.String(),
		"tools":           toolTrace,
	}
	if res != nil {
		rid := res.RunID
		tr.agentRunID = &rid
		detail["run_id"] = res.RunID.String()
		detail["step_count"] = res.StepCount
		detail["status"] = res.Status
		if dbSteps := b.agentStepsForLog(res.RunID); len(dbSteps) > 0 {
			detail["steps"] = dbSteps
		}
	}
	b.recordStep(tr, stepLLMRequest, eventAgent, detail,
		zap.String("run_id", anyString(detail["run_id"])),
		zap.Int("tool_events", len(toolTrace)),
		zap.String("conversation_id", conv.ID.String()),
	)
	if err != nil {
		b.log.Error("agent run", zap.Error(err), zap.String("request_id", tr.requestID))
		final := acc.String()
		if final == "" {
			final = llm.PublicMessage(err)
		}
		tr.status = 500
		tr.errMsg = err.Error()
		tr.reply = final
		_ = streamer.finish(final, true)
		return
	}
	text := ""
	if res != nil {
		text = res.Output
		if res.MessageID != uuid.Nil {
			id := res.MessageID
			tr.assistantMsgID = &id
		}
	}
	if strings.TrimSpace(text) == "" {
		text = acc.String()
	}
	if strings.TrimSpace(text) == "" {
		text = "没有生成内容。"
	}
	tr.reply = text
	_ = streamer.finish(text, false)
}

// completeViaWebAgent 薄包装：与控制台 Agent 共用 agent.Run，并保留钉钉会话上下文。
// - 带 ConversationID：多轮追问（如补标签）可用历史
// - RAGHits 传空切片：跳过 system 预注入（避免摘录+旧结论让模型跳过工具）；检索范围仍经 CorpusIDs 交给工具
// - 现有 completeViaAgent（钉钉预检索注入）保持不变
func (b *Bot) completeViaWebAgent(ctx context.Context, data *botCallback, tr *msgTrace, conv *model.Conversation, uid, input string, corpusID *uuid.UUID, corpusIDs []uuid.UUID, enableSearch, skipRAG bool) {
	prefix, skipQuery := b.chatPrefix(data.ConversationID)
	historyLimit := b.chatHistoryLimit(data.ConversationID)
	streamer := b.newStreamer(ctx, data, tr)
	var acc strings.Builder
	lastFlush := time.Now()
	tr.outcome = "web_agent"
	cid := conv.ID
	tr.enableSearch = enableSearch
	toolTrace := make([]map[string]any, 0, 8)
	topK := b.ragTop
	if topK <= 0 {
		topK = 5
	}
	res, err := b.agent.Run(ctx, agent.RunInput{
		ConversationID: &cid,
		UID:            uid,
		Input:          input,
		CorpusID:       corpusID,
		CorpusIDs:      corpusIDs,
		TopK:           topK,
		// 非 nil 空切片：禁止 agent.retrieveHits 写入 system；工具侧仍用 CorpusIDs。
		RAGHits:          []rag.Hit{},
		EnableSearch:     enableSearch,
		PrefixPrompt:     prefix,
		PrefixSkipsQuery: skipQuery,
		HistoryLimit:     historyLimit,
		Dictionary:       b.chatDictionary(data.ConversationID),
		Tools:            b.chatTools(data.ConversationID),
		ToolsOnly:        true,
		SkipRAG:          skipRAG,
		RequestID:        tr.requestID,
		Stream:           true,
	}, func(ev agent.Event) error {
		switch ev.Type {
		case "delta":
			content, _ := ev.Payload["content"].(string)
			acc.WriteString(content)
			now := time.Now()
			if now.Sub(lastFlush) < streamMinInterval && acc.Len() < 80 {
				return nil
			}
			lastFlush = now
			return streamer.update(acc.String(), false)
		case "tool_call":
			name, _ := ev.Payload["name"].(string)
			args, _ := ev.Payload["arguments"].(string)
			toolTrace = append(toolTrace, map[string]any{
				"kind":      "tool_call",
				"name":      name,
				"arguments": previewText(args, b.previewMax),
			})
			hint := "正在调用工具…"
			if name == "dbconn" {
				hint = "正在查询业务库…"
			} else if name == "knowledge_search" {
				hint = "正在检索知识库…"
			}
			_ = streamer.update(hint, false)
		case "tool_result":
			name, _ := ev.Payload["name"].(string)
			content, _ := ev.Payload["content"].(string)
			toolTrace = append(toolTrace, map[string]any{
				"kind":    "tool_result",
				"name":    name,
				"content": previewText(content, b.previewMax),
			})
		}
		return nil
	})
	detail := map[string]any{
		"mode":            "web",
		"enable_search":   enableSearch,
		"conversation_id": conv.ID.String(),
		"corpus_id":       uuidString(corpusID),
		"corpus_ids":      uuidStrings(corpusIDs),
		"rag_preinject":   false,
		"tools":           toolTrace,
	}
	if res != nil {
		rid := res.RunID
		tr.agentRunID = &rid
		detail["run_id"] = res.RunID.String()
		detail["step_count"] = res.StepCount
		detail["status"] = res.Status
		if dbSteps := b.agentStepsForLog(res.RunID); len(dbSteps) > 0 {
			detail["steps"] = dbSteps
		}
	}
	b.recordStep(tr, stepLLMRequest, eventWebAgent, detail,
		zap.String("run_id", anyString(detail["run_id"])),
		zap.Int("tool_events", len(toolTrace)),
		zap.String("conversation_id", conv.ID.String()),
	)
	if err != nil {
		b.log.Error("web agent run", zap.Error(err), zap.String("request_id", tr.requestID))
		final := acc.String()
		if final == "" {
			final = llm.PublicMessage(err)
		}
		tr.status = 500
		tr.errMsg = err.Error()
		tr.reply = final
		_ = streamer.finish(final, true)
		return
	}
	text := ""
	if res != nil {
		text = res.Output
		if res.MessageID != uuid.Nil {
			id := res.MessageID
			tr.assistantMsgID = &id
		}
	}
	if strings.TrimSpace(text) == "" {
		text = acc.String()
	}
	if strings.TrimSpace(text) == "" {
		text = "没有生成内容。"
	}
	tr.reply = text
	_ = streamer.finish(text, false)
}

func (b *Bot) agentStepsForLog(runID uuid.UUID) []map[string]any {
	if b == nil || b.agent == nil || runID == uuid.Nil {
		return nil
	}
	_, steps, err := b.agent.GetRun(runID)
	if err != nil || len(steps) == 0 {
		return nil
	}
	out := make([]map[string]any, 0, len(steps))
	for _, st := range steps {
		item := map[string]any{
			"step_index": st.StepIndex,
			"kind":       st.Kind,
		}
		if st.ToolName != "" {
			item["tool_name"] = st.ToolName
		}
		if st.InputJSON != "" && st.InputJSON != "{}" {
			item["input"] = previewText(st.InputJSON, b.previewMax)
		}
		if st.OutputText != "" {
			item["output"] = previewText(st.OutputText, b.previewMax)
		}
		out = append(out, item)
	}
	return out
}

func anyString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}
