package chat

import (
	"fmt"
	"strings"

	"github.com/webapp/go-app/ai-agent/internal/service/rag"
)

const defaultReplyStyle = "回答简洁直接：先给结论，必要时用短列表补要点。不要重复用户问题，不要开场白或结尾客套，不要复述已给出的材料全文。"

const ragContextHeader = "【知识摘录】只采用与问题直接相关的句子作答；材料不足就明确说不知道。不要整段照抄。"

func (s *Service) replyStyle() string {
	if s != nil && s.cfg != nil {
		if p := strings.TrimSpace(s.cfg.LLM.SystemPrompt); p != "" {
			return p
		}
	}
	return defaultReplyStyle
}

func mergeSystem(style, convPrompt, ragBlock string) string {
	parts := make([]string, 0, 3)
	for _, p := range []string{convPrompt, style, ragBlock} {
		p = strings.TrimSpace(p)
		if p != "" {
			parts = append(parts, p)
		}
	}
	return strings.Join(parts, "\n\n")
}

func ragContext(hits []rag.Hit) string {
	if len(hits) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(ragContextHeader)
	b.WriteByte('\n')
	for i, h := range hits {
		b.WriteString(fmt.Sprintf("[%d] %s\n", i+1, h.Content))
	}
	return strings.TrimSpace(b.String())
}
