package agent

import (
	"regexp"
	"strings"
)

var (
	srmIdent    = regexp.MustCompile("`?(?:[A-Za-z0-9]+\\.)?Srm_[A-Za-z0-9_]+`?")
	colIdent    = `[A-Z][a-z0-9]+(?:[A-Z][a-z0-9]*[a-z][a-z0-9]*)+(?:_[A-Za-z0-9]+)*`
	colAssign   = regexp.MustCompile(`\b` + colIdent + `\s*=\s*-?\d+(?:\.\d+)?`)
	colName     = regexp.MustCompile(`\b` + colIdent + `\b`)
	spacedAnd   = regexp.MustCompile(`\s+且\s+`)
	colonComma  = regexp.MustCompile(`[：:]\s*[，,]`)
	lineColon   = regexp.MustCompile(`(?m)^[ \t]*[：:][ \t]*`)
	lineArrow   = regexp.MustCompile(`(?m)^[ \t]*→[ \t]*`)
	multiSpace  = regexp.MustCompile(`[ \t]{2,}`)
	multiBlank  = regexp.MustCompile(`\n{3,}`)
	spaceParen  = regexp.MustCompile(`\s+（`)
	tiaoDe      = regexp.MustCompile(`条\s*的`)
	repeatComma = regexp.MustCompile(`[，,]{2,}`)
)

// MaskSchemaLeak 去掉回复里的业务库表名和字段名，保留中文业务说法和枚举含义。
func MaskSchemaLeak(s string) string {
	if strings.TrimSpace(s) == "" {
		return s
	}
	s = srmIdent.ReplaceAllString(s, "")
	s = colAssign.ReplaceAllString(s, "")
	s = colName.ReplaceAllString(s, "")
	s = colonComma.ReplaceAllString(s, "：")
	s = spacedAnd.ReplaceAllString(s, "")
	s = tiaoDe.ReplaceAllString(s, "条")
	s = lineColon.ReplaceAllString(s, "")
	s = lineArrow.ReplaceAllString(s, "")
	s = repeatComma.ReplaceAllString(s, "，")
	s = spaceParen.ReplaceAllString(s, "（")
	s = multiSpace.ReplaceAllString(s, " ")
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimSpace(line)
	}
	s = strings.TrimSpace(strings.Join(lines, "\n"))
	return multiBlank.ReplaceAllString(s, "\n\n")
}
