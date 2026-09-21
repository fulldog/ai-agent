package dbconn

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

var (
	stmtStart   = regexp.MustCompile(`(?i)^(WITH|SELECT)\b`)
	forbiddenKW = regexp.MustCompile(`(?i)\b(INSERT|UPDATE|DELETE|DROP|ALTER|TRUNCATE|CREATE|REPLACE|GRANT|REVOKE|LOAD|CALL|LOCK|UNLOCK|HANDLER|PREPARE|EXECUTE|DEALLOCATE|SET|USE|DO|ANALYZE|OPTIMIZE|REPAIR|FLUSH|RESET|PURGE|KILL|SHUTDOWN|BINLOG)\b`)
	intoKW      = regexp.MustCompile(`(?i)\bINTO\b`)
	fileKW      = regexp.MustCompile(`(?i)\b(OUTFILE|DUMPFILE)\b`)
	forLockKW   = regexp.MustCompile(`(?i)\bFOR\s+(UPDATE|SHARE)\b`)
	shareLockKW = regexp.MustCompile(`(?i)\bLOCK\s+IN\s+SHARE\s+MODE\b`)
	identRe     = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
)

// ValidateSelect 仅允许单条只读 SELECT / WITH ... SELECT。
func ValidateSelect(sql string) (string, error) {
	trimmed := strings.TrimSpace(sql)
	if trimmed == "" {
		return "", fmt.Errorf("sql 不能为空")
	}
	code, err := stripCommentsAndStrings(trimmed)
	if err != nil {
		return "", err
	}
	code = strings.TrimSpace(code)
	if strings.HasSuffix(code, ";") {
		code = strings.TrimSpace(code[:len(code)-1])
	}
	if code == "" {
		return "", fmt.Errorf("sql 不能为空")
	}
	if strings.Contains(code, ";") {
		return "", fmt.Errorf("禁止多条 SQL 语句")
	}
	if !stmtStart.MatchString(code) {
		return "", fmt.Errorf("仅允许 SELECT 或 WITH ... SELECT")
	}
	if forbiddenKW.MatchString(code) || intoKW.MatchString(code) || fileKW.MatchString(code) ||
		forLockKW.MatchString(code) || shareLockKW.MatchString(code) {
		return "", fmt.Errorf("仅允许只读 SELECT")
	}
	out := strings.TrimSpace(trimmed)
	if strings.HasSuffix(out, ";") {
		out = strings.TrimSpace(out[:len(out)-1])
	}
	return out, nil
}

func stripCommentsAndStrings(sql string) (string, error) {
	var b strings.Builder
	b.Grow(len(sql))
	i := 0
	for i < len(sql) {
		c := sql[i]
		if c == '\'' || c == '"' || c == '`' {
			end, err := skipQuoted(sql, i)
			if err != nil {
				return "", err
			}
			b.WriteByte(' ')
			i = end
			continue
		}
		if c == '#' || (c == '-' && i+1 < len(sql) && sql[i+1] == '-') {
			for i < len(sql) && sql[i] != '\n' {
				i++
			}
			b.WriteByte(' ')
			continue
		}
		if c == '/' && i+1 < len(sql) && sql[i+1] == '*' {
			if i+2 < len(sql) && sql[i+2] == '!' {
				return "", fmt.Errorf("禁止可执行注释")
			}
			end := strings.Index(sql[i+2:], "*/")
			if end < 0 {
				return "", fmt.Errorf("SQL 注释未闭合")
			}
			i += end + 4
			b.WriteByte(' ')
			continue
		}
		b.WriteByte(c)
		i++
	}
	return collapseSpace(b.String()), nil
}

func skipQuoted(sql string, i int) (int, error) {
	q := sql[i]
	i++
	for i < len(sql) {
		if sql[i] == '\\' {
			i += 2
			continue
		}
		if sql[i] == q {
			if i+1 < len(sql) && sql[i+1] == q {
				i += 2
				continue
			}
			return i + 1, nil
		}
		i++
	}
	return 0, fmt.Errorf("SQL 字符串未闭合")
}

func collapseSpace(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	prevSpace := false
	for _, r := range s {
		if unicode.IsSpace(r) {
			if !prevSpace {
				b.WriteByte(' ')
				prevSpace = true
			}
			continue
		}
		prevSpace = false
		b.WriteRune(r)
	}
	return b.String()
}

func hasTablePrefix(name string) bool {
	return strings.HasPrefix(strings.TrimSpace(name), tableNamePrefix)
}

func likeContains(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `%`, `\%`)
	s = strings.ReplaceAll(s, `_`, `\_`)
	return "%" + s + "%"
}

func splitTableIdent(raw string) (schema, table string, err error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", "", fmt.Errorf("table 不能为空")
	}
	var parts []string
	cur := strings.Builder{}
	inTick := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '`' {
			inTick = !inTick
			continue
		}
		if c == '.' && !inTick {
			parts = append(parts, cur.String())
			cur.Reset()
			continue
		}
		cur.WriteByte(c)
	}
	parts = append(parts, cur.String())
	if inTick || len(parts) == 0 || len(parts) > 2 {
		return "", "", fmt.Errorf("非法表名")
	}
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
		if !identRe.MatchString(parts[i]) {
			return "", "", fmt.Errorf("非法表名")
		}
	}
	if len(parts) == 1 {
		return "", parts[0], nil
	}
	return parts[0], parts[1], nil
}
