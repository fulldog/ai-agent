package dbconn

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// dictionaryFullRunes 不超过这个长度时，系统提示带上字典全文。
// 更长则只带规则和表目录，字段用 schema 按表读取。
const dictionaryFullRunes = 6000

// schemaExcerptRunes 按关键字筛选时，返回正文的上限。
const schemaExcerptRunes = 8000

type dictSection struct {
	name string
	text string
}

// PromptDictionary 选择放进系统提示的字典正文。
// full 为 true 时正文已含字段，模型应直接写 SQL。
// full 为 false 时正文是规则和表目录，字段要用 schema 指定 table 读取。
func PromptDictionary(text string) (body string, full bool) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", true
	}
	if utf8.RuneCountInString(text) <= dictionaryFullRunes {
		return text, true
	}
	if strings.HasPrefix(text, "{") {
		if d, err := parseDictionary([]byte(text)); err == nil {
			return strings.TrimSpace(d.lookupCatalog()), false
		}
	}
	pre, sections := splitDict(text)
	if len(sections) == 0 {
		return text, true
	}
	return schemaOverview(pre, sections), false
}

// lookupCorpus 按数据字典返回结构说明。
// table 指定时返回该表；keyword 指定时筛选；都不指定时返回规则和表目录，不返回全部字段。
// 解析不出表时原样返回全文。
func lookupCorpus(text, keyword, table string) (string, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", fmt.Errorf("语料为空")
	}
	if strings.HasPrefix(text, "{") {
		if d, err := parseDictionary([]byte(text)); err == nil {
			return d.lookup(keyword, table)
		}
	}
	pre, sections := splitDict(text)
	if len(sections) == 0 {
		return text, nil
	}
	table = strings.TrimSpace(table)
	keyword = strings.TrimSpace(keyword)
	if table != "" {
		return sectionByName(sections, table)
	}
	if keyword != "" {
		return sectionsByKeyword(sections, keyword), nil
	}
	return schemaOverview(pre, sections), nil
}

func splitDict(text string) (preamble string, sections []dictSection) {
	var pre strings.Builder
	var gap strings.Builder
	var curName string
	var cur strings.Builder
	started := false
	flush := func() {
		if curName == "" {
			return
		}
		sections = append(sections, dictSection{
			name: curName,
			text: strings.TrimRight(cur.String(), "\n"),
		})
		cur.Reset()
		curName = ""
	}
	for _, line := range strings.Split(text, "\n") {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "##") {
			if name, ok := headingTable(trim); ok {
				flush()
				started = true
				curName = name
				if g := strings.TrimSpace(gap.String()); g != "" {
					cur.WriteString(g)
					cur.WriteString("\n\n")
					gap.Reset()
				}
				cur.WriteString(line)
				cur.WriteByte('\n')
				continue
			}
			if curName != "" {
				flush()
			}
			if started {
				gap.WriteString(line)
				gap.WriteByte('\n')
			} else {
				pre.WriteString(line)
				pre.WriteByte('\n')
			}
			continue
		}
		switch {
		case curName != "":
			cur.WriteString(line)
			cur.WriteByte('\n')
		case !started:
			pre.WriteString(line)
			pre.WriteByte('\n')
		default:
			gap.WriteString(line)
			gap.WriteByte('\n')
		}
	}
	flush()
	return strings.TrimSpace(pre.String()), sections
}

func schemaOverview(pre string, sections []dictSection) string {
	if pre != "" && preambleListsTables(pre, sections) {
		return pre
	}
	catalog := catalogFromSections(sections)
	if pre == "" {
		return catalog
	}
	return pre + "\n\n" + catalog
}

func preambleListsTables(pre string, sections []dictSection) bool {
	for _, s := range sections {
		if !strings.Contains(pre, s.name) {
			return false
		}
	}
	return true
}

func catalogFromSections(sections []dictSection) string {
	var b strings.Builder
	b.WriteString("表目录:\n")
	for _, s := range sections {
		comment := sectionComment(s.text)
		if comment == "" {
			fmt.Fprintf(&b, "- %s\n", s.name)
			continue
		}
		fmt.Fprintf(&b, "- %s | %s\n", s.name, comment)
	}
	return strings.TrimRight(b.String(), "\n")
}

func sectionComment(section string) string {
	lines := strings.Split(section, "\n")
	for i, line := range lines {
		if i == 0 {
			continue
		}
		trim := strings.TrimSpace(line)
		if trim == "" || strings.HasPrefix(trim, "#") || strings.HasPrefix(trim, "-") || strings.HasPrefix(trim, "|") || strings.HasPrefix(trim, "```") {
			continue
		}
		return trim
	}
	return ""
}

func sectionByName(sections []dictSection, table string) (string, error) {
	names := make([]string, len(sections))
	var folded []dictSection
	for i, s := range sections {
		names[i] = s.name
		if s.name == table {
			return s.text, nil
		}
		if strings.EqualFold(s.name, table) {
			folded = append(folded, s)
		}
	}
	if len(folded) == 1 {
		return folded[0].text, nil
	}
	if canon, ok := fuzzyName(names, table); ok {
		for _, s := range sections {
			if s.name == canon {
				return s.text, nil
			}
		}
	}
	msg := fmt.Sprintf("表 %s 不在数据字典中", table)
	if sug := closestNames(names, table, 6); len(sug) > 0 {
		msg += "，相近的表：" + strings.Join(sug, "、")
	}
	return "", fmt.Errorf("%s", msg)
}

func sectionsByKeyword(sections []dictSection, keyword string) string {
	kw := strings.ToLower(keyword)
	var b strings.Builder
	n := 0
	for _, s := range sections {
		blob := strings.ToLower(s.name + "\n" + s.text)
		if !strings.Contains(blob, kw) {
			continue
		}
		if n > 0 && utf8.RuneCountInString(b.String())+utf8.RuneCountInString(s.text) > schemaExcerptRunes {
			b.WriteString("\n结果已截断，请用 table 指定表名。\n")
			return b.String()
		}
		if n > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString(s.text)
		n++
	}
	if n == 0 {
		return "没有匹配的表\n"
	}
	return b.String()
}
