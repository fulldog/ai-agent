package agent

import (
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/webapp/go-app/ai-agent/internal/model"
	"github.com/webapp/go-app/ai-agent/internal/service/llm"
	"github.com/webapp/go-app/ai-agent/internal/service/rag"
)

const (
	baseAgentPrompt = "你是有用的 AI 助手。需要准确信息时请调用工具，并根据工具结果作答。回答简洁直接，先给结论；不要套话、不要重复问题或工具原文。"
	// toolsReadyPrompt 有工具 Spec 时追加：防止模型跟语料说「去工具函数管理开启」。
	toolsReadyPrompt  = "下方「当前可用工具」已由服务端挂载到本次请求，可直接 function call，无需用户去任何「工具函数管理」或后台开关。禁止回答「未启用工具/请先开启工具」之类推诿；知识摘录仅供参考，与工具可用性无关。"
	dbconnAgentPrompt = "涉及供应商、付款、订单、业务表或系统数据时：先按知识摘录中的流程与前置条件执行（缺信息先追问，不要直接查库）。需要核实数据时必须调用工具，不要凭猜测下结论。摘录未覆盖时再用 knowledge_search 查口径；再用 dbconn 的 schema 对照表与列注释；最后组织只读 SELECT 并调用 dbconn 的 query 执行。不要只把 SQL 写在回复里。语料未命中或无法对应到表时，说明缺什么，不要编造表名或数据。"
	fileAgentPrompt   = "用户要求导出、下载或生成 Excel/表格文件时：先查询并整理成用户能看懂的列和行，再调用 generate_file。拿到下载链接后，回复里必须带上该链接，不要只说文件已生成。"
	ragOnlyHint       = "【说明】下列知识摘录是本次必须遵循的流程与口径。若与「当前可用工具」冲突，以工具调用为准，不要根据摘录要求用户去开启工具。"
)

func requireFirstTool(hits []rag.Hit) bool {
	return len(sanitizeRAGHits(hits)) == 0
}

func agentSystemPrompt(specs []llm.ToolSpec) string {
	if len(specs) == 0 {
		return baseAgentPrompt
	}
	names := make([]string, 0, len(specs))
	hasDB := false
	hasFile := false
	for _, spec := range specs {
		names = append(names, spec.Function.Name)
		switch spec.Function.Name {
		case "dbconn":
			hasDB = true
		case "generate_file":
			hasFile = true
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
	if hasFile {
		b.WriteString("\n\n")
		b.WriteString(fileAgentPrompt)
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

// sanitizeRAGHits 去掉会诱导模型推诿「去开工具」的摘录片段。
func sanitizeRAGHits(hits []rag.Hit) []rag.Hit {
	if len(hits) == 0 {
		return hits
	}
	out := make([]rag.Hit, 0, len(hits))
	for _, h := range hits {
		c := h.Content
		if strings.Contains(c, "工具函数管理") || strings.Contains(c, "未启用任何工具") || strings.Contains(c, "开启对应工具") {
			continue
		}
		out = append(out, h)
	}
	return out
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
