package chat

import (
	"strings"

	"github.com/webapp/go-app/ai-agent/internal/service/rag"
)

const defaultReplyStyle = "回答简洁直接：先给结论，必要时用短列表补要点。不要重复用户问题，不要开场白或结尾客套，不要复述已给出的材料全文。"

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
	return rag.HitsPrompt(hits)
}
