package corpus

import (
	"strings"

	"github.com/webapp/go-app/ai-agent/pkg/extract"
)

// ResolveKind 归一化来源类型。已写入的 file/text 优先；旧数据按来源扩展名推断。
func ResolveKind(kind, source string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "file", "text":
		return strings.ToLower(strings.TrimSpace(kind))
	}
	if extract.IsSupportedExtension(source) {
		return "file"
	}
	return "text"
}

// joinChunks 按分块时的重叠长度拼回原文。重叠对不上时用换行拼接，避免误删正文。
func joinChunks(parts []string, overlap int) string {
	if len(parts) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(parts[0])
	prev := []rune(parts[0])
	for _, next := range parts[1:] {
		nr := []rune(next)
		if overlap > 0 && len(prev) >= overlap && len(nr) >= overlap && string(prev[len(prev)-overlap:]) == string(nr[:overlap]) {
			b.WriteString(string(nr[overlap:]))
		} else if next != "" {
			if b.Len() > 0 {
				b.WriteByte('\n')
			}
			b.WriteString(next)
		}
		prev = nr
	}
	return b.String()
}
