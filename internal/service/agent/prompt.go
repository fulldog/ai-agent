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
	dbconnAgentPrompt = "若问题涉及供应商、付款、订单、业务数据或表结构：要核实数据必须调用 dbconn，不要沿用历史查询结果，也不要凭猜测作答。会话绑定了数据字典时，query 会在执行前按该字典预检并修正表名和列名；预检失败就不会查库，按返回的表名和列名改写后再调用。需要字典全文用 action=schema。禁止编造表名或列名。缺标签时先按名称查库，根据本次查询结果再决定是否追问。知识摘录未覆盖时再用 knowledge_search 查口径。不要只把 SQL 写在回复里而不调用工具。"
	fileAgentPrompt   = ""
	//fileAgentPrompt = "用户要求导出、下载或生成 Excel/表格文件时：数据来自本次 dbconn query 的，调用 generate_file 只传 filename 和 from_query=true，不要把查询结果再写进参数；中文列名写在 SELECT 别名里。不是查询结果的小表才传 columns 和 rows。拿到下载链接后，回复里必须带上该链接，不要只说文件已生成。"
	ragOnlyHint = "【说明】下列知识摘录是本次必须遵循的流程与口径。若与「当前可用工具」冲突，以工具调用为准，不要根据摘录要求用户去开启工具。"
)

func requireFirstTool(hits []rag.Hit) bool {
	return len(sanitizeRAGHits(hits)) == 0
}

// PrefixSkipsQuery 报告群前置提示词原文是否要求本轮不要查库。
// 为 true 时首轮不强制 function call。必须用库里的原文，包装后的系统提示不能拿来判断。
func PrefixSkipsQuery(raw string) bool {
	return strings.Contains(raw, "不要查库")
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

const dictionaryLead = "【数据字典】下面是本会话的表结构全文。写 SQL 时表名、列名、关联只能来自这里，禁止编造。已附带本字典时直接写 query，不要先调用 schema。"

// AppendDictionary 把会话配置的数据字典原文接到系统提示末尾，供写 SQL 前阅读。
func AppendDictionary(system, dictionary string) string {
	return system
	dictionary = strings.TrimSpace(dictionary)
	if dictionary == "" {
		return system
	}
	block := dictionaryLead + "\n\n" + dictionary
	system = strings.TrimSpace(system)
	if system == "" {
		return block
	}
	return system + "\n\n" + block
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
		if m.Role == "system" || m.Role == "tool" {
			continue
		}
		msgs = append(msgs, llm.Message{Role: m.Role, Content: m.Content})
	}
	msgs = append(msgs, llm.Message{Role: "user", Content: user})
	return msgs
}

// HistoryWindow 决定本轮带入几条历史。explicit 非 nil 时以它为准，0 表示不带；nil 时用 fallback。
func HistoryWindow(explicit *int, fallback int) (limit int, include bool) {
	if explicit != nil {
		if *explicit <= 0 {
			return 0, false
		}
		return *explicit, true
	}
	if fallback <= 0 {
		fallback = 10
	}
	return fallback, true
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
	err := s.db.Where("conversation_id = ? AND role <> ?", conversationID, "tool").
		Order("created_at desc").
		Limit(limit).
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	slices.Reverse(rows)
	return rows, nil
}
