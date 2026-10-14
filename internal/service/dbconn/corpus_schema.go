package dbconn

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

const (
	maxCorpusRunes    = 6000
	maxCorpusHeadings = 80
	maxSectionRunes   = 2000
)

type corpusSection struct {
	lines []string
}

func lookupCorpus(text, keyword, table string) (string, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", fmt.Errorf("语料为空")
	}
	table = strings.TrimSpace(table)
	keyword = strings.TrimSpace(keyword)
	if table != "" {
		name, err := requireSrmTable(table)
		if err != nil {
			return "", err
		}
		out, ok := excerptCorpus(text, name, true)
		if !ok {
			return "", fmt.Errorf("未找到表 %s", name)
		}
		return out, nil
	}
	if keyword != "" {
		out, ok := excerptCorpus(text, keyword, false)
		if !ok {
			return "没有匹配的内容\n", nil
		}
		return out, nil
	}
	return corpusOutline(text), nil
}

func corpusOutline(text string) string {
	var b strings.Builder
	b.WriteString("语料目录:\n")
	n := 0
	for _, line := range strings.Split(text, "\n") {
		if !isHeading(line) {
			continue
		}
		b.WriteString(strings.TrimSpace(line))
		b.WriteByte('\n')
		n++
		if n >= maxCorpusHeadings {
			b.WriteString("结果已截断\n")
			break
		}
	}
	if n == 0 {
		return clipRunes(text, maxCorpusRunes)
	}
	return b.String()
}

func excerptCorpus(text, needle string, caseSensitive bool) (string, bool) {
	var parts []string
	matched := false
	for _, sec := range splitSections(text) {
		if !sectionHit(sec, needle, caseSensitive) {
			continue
		}
		matched = true
		parts = append(parts, clipSection(sec, needle, caseSensitive))
	}
	if !matched {
		return "", false
	}
	return clipRunes(strings.Join(parts, "\n"), maxCorpusRunes), true
}

func splitSections(text string) []corpusSection {
	lines := strings.Split(text, "\n")
	var sections []corpusSection
	var cur corpusSection
	started := false
	flush := func() {
		if !started {
			return
		}
		sections = append(sections, cur)
	}
	for _, line := range lines {
		if isHeading(line) {
			flush()
			cur = corpusSection{lines: []string{line}}
			started = true
			continue
		}
		if !started {
			cur = corpusSection{}
			started = true
		}
		cur.lines = append(cur.lines, line)
	}
	flush()
	return sections
}

func isHeading(line string) bool {
	s := strings.TrimSpace(line)
	i := 0
	for i < len(s) && s[i] == '#' {
		i++
	}
	return i > 0 && i <= 6 && i < len(s) && s[i] == ' '
}

func sectionHit(sec corpusSection, needle string, caseSensitive bool) bool {
	for _, line := range sec.lines {
		if caseSensitive {
			if strings.Contains(line, needle) {
				return true
			}
			continue
		}
		if containsFold(line, needle) {
			return true
		}
	}
	return false
}

func clipSection(sec corpusSection, needle string, caseSensitive bool) string {
	body := strings.Join(sec.lines, "\n")
	if utf8.RuneCountInString(body) <= maxSectionRunes {
		return body
	}
	hits := matchLineIndexes(sec.lines, needle, caseSensitive)
	if len(hits) == 0 {
		return clipRunes(body, maxSectionRunes)
	}
	var b strings.Builder
	start := 0
	if isHeading(sec.lines[0]) {
		b.WriteString(sec.lines[0])
		b.WriteByte('\n')
		start = 1
	}
	ranges := lineWindows(hits, start, len(sec.lines), 2, 12)
	for i, span := range ranges {
		if i > 0 {
			b.WriteString("...\n")
		}
		for _, line := range sec.lines[span[0]:span[1]] {
			b.WriteString(line)
			b.WriteByte('\n')
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func matchLineIndexes(lines []string, needle string, caseSensitive bool) []int {
	var hits []int
	for i, line := range lines {
		ok := strings.Contains(line, needle)
		if !caseSensitive {
			ok = containsFold(line, needle)
		}
		if ok {
			hits = append(hits, i)
		}
	}
	return hits
}

func lineWindows(hits []int, start, n, before, after int) [][2]int {
	var out [][2]int
	for _, hit := range hits {
		lo := hit - before
		if lo < start {
			lo = start
		}
		hi := hit + after + 1
		if hi > n {
			hi = n
		}
		if len(out) > 0 && lo <= out[len(out)-1][1] {
			out[len(out)-1][1] = hi
			continue
		}
		out = append(out, [2]int{lo, hi})
	}
	return out
}

func clipRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return strings.TrimRight(string(r[:n]), "\n") + "\n结果已截断\n"
}
