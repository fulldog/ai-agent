package dbconn

import (
	"fmt"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	tokIdent  byte = 'i'
	tokPunct  byte = 'p'
	tokString byte = 's'
	tokNumber byte = 'n'
)

// CheckSQL 用绑定的数据字典预检只读 SQL，并把能唯一对应上的表名、列名改成字典里的写法。
// 字典为空或解析不出表时原样返回，由调用方继续做只读校验。预检不通过时不改 SQL。
func CheckSQL(sql, dictionary string) (string, error) {
	if strings.TrimSpace(dictionary) == "" {
		return sql, nil
	}
	cat := parseSQLCatalog(dictionary)
	if len(cat.byName) == 0 {
		return sql, nil
	}
	clean, err := ValidateSelect(sql)
	if err != nil {
		return "", err
	}
	toks, err := scanSQL(clean)
	if err != nil {
		return "", err
	}
	w := newSQLWalker(toks, cat)
	w.run()
	if len(w.problems) > 0 {
		return "", fmt.Errorf("SQL 未通过数据字典预检: %s", strings.Join(w.problems, "；"))
	}
	out := applySQLEdits(clean, w.edits)
	if _, err := ValidateSelect(out); err != nil {
		return "", fmt.Errorf("SQL 未通过数据字典预检: %w", err)
	}
	return out, nil
}

type sqlTable struct {
	name string
	cols map[string][]string // 小写列名 -> 字典中的写法
}

func (t *sqlTable) addCol(name string) {
	name = strings.TrimSpace(name)
	if name == "" || t == nil {
		return
	}
	key := strings.ToLower(name)
	for _, c := range t.cols[key] {
		if c == name {
			return
		}
	}
	t.cols[key] = append(t.cols[key], name)
}

func (t *sqlTable) colNames() []string {
	if t == nil {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(t.cols))
	for _, list := range t.cols {
		for _, n := range list {
			if _, ok := seen[n]; ok {
				continue
			}
			seen[n] = struct{}{}
			out = append(out, n)
		}
	}
	sort.Strings(out)
	return out
}

type sqlCatalog struct {
	byName map[string]*sqlTable
	byFold map[string][]*sqlTable
}

func newSQLCatalog() *sqlCatalog {
	return &sqlCatalog{
		byName: map[string]*sqlTable{},
		byFold: map[string][]*sqlTable{},
	}
}

func (c *sqlCatalog) addTable(name string) *sqlTable {
	if t, ok := c.byName[name]; ok {
		return t
	}
	t := &sqlTable{name: name, cols: map[string][]string{}}
	c.byName[name] = t
	key := strings.ToLower(name)
	c.byFold[key] = append(c.byFold[key], t)
	return t
}

func (c *sqlCatalog) tableNames() []string {
	out := make([]string, 0, len(c.byName))
	for name := range c.byName {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func (c *sqlCatalog) resolveTable(name string) (string, error) {
	if c == nil || strings.TrimSpace(name) == "" {
		return "", fmt.Errorf("表名不能为空")
	}
	if t, ok := c.byName[name]; ok {
		return t.name, nil
	}
	folded := c.byFold[strings.ToLower(name)]
	switch len(folded) {
	case 1:
		return folded[0].name, nil
	case 0:
		if canon, ok := fuzzyName(c.tableNames(), name); ok {
			return canon, nil
		}
		msg := fmt.Sprintf("表 %s 不在数据字典中", name)
		if sug := closestNames(c.tableNames(), name, 6); len(sug) > 0 {
			msg += "，相近的表：" + strings.Join(sug, "、")
		}
		return "", fmt.Errorf("%s", msg)
	default:
		names := make([]string, len(folded))
		for i, t := range folded {
			names[i] = t.name
		}
		return "", fmt.Errorf("表名 %s 在数据字典中有多种写法：%s", name, strings.Join(names, "、"))
	}
}

func parseSQLCatalog(text string) *sqlCatalog {
	text = strings.TrimSpace(text)
	if text == "" {
		return newSQLCatalog()
	}
	if strings.HasPrefix(text, "{") {
		if d, err := parseDictionary([]byte(text)); err == nil {
			return catalogFromDictionary(d)
		}
	}
	return parseMarkdownCatalog(text)
}

func catalogFromDictionary(d *Dictionary) *sqlCatalog {
	c := newSQLCatalog()
	if d == nil {
		return c
	}
	for i := range d.Tables {
		tb := c.addTable(d.Tables[i].Name)
		for _, col := range d.Tables[i].Columns {
			tb.addCol(col.Name)
		}
	}
	return c
}

func parseMarkdownCatalog(text string) *sqlCatalog {
	c := newSQLCatalog()
	var cur *sqlTable
	for _, line := range strings.Split(text, "\n") {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "#") {
			cur = nil
			if name, ok := headingTable(trim); ok {
				cur = c.addTable(name)
			}
			continue
		}
		if cur == nil {
			continue
		}
		if name, ok := parsePipeColumn(trim); ok {
			cur.addCol(name)
			continue
		}
		if name, ok := parseColumnBullet(trim); ok {
			cur.addCol(name)
		}
	}
	return c
}

func headingTable(line string) (string, bool) {
	n := 0
	for n < len(line) && line[n] == '#' {
		n++
	}
	if n < 2 || n > 3 || n >= len(line) || (line[n] != ' ' && line[n] != '\t') {
		return "", false
	}
	rest := strings.TrimSpace(line[n:])
	if len(rest) >= 2 && rest[0] == '`' && rest[len(rest)-1] == '`' {
		rest = rest[1 : len(rest)-1]
	}
	if !identRe.MatchString(rest) {
		return "", false
	}
	return rest, true
}

func parsePipeColumn(line string) (string, bool) {
	if !strings.HasPrefix(line, "|") {
		return "", false
	}
	parts := strings.Split(line, "|")
	if len(parts) < 4 {
		return "", false
	}
	num := strings.TrimSpace(parts[1])
	if num == "" || num[0] < '0' || num[0] > '9' {
		return "", false
	}
	name := strings.Trim(strings.TrimSpace(parts[2]), "` ")
	if !identRe.MatchString(name) {
		return "", false
	}
	return name, true
}

func parseColumnBullet(line string) (string, bool) {
	if !strings.HasPrefix(line, "-") {
		return "", false
	}
	rest := strings.TrimSpace(line[1:])
	rest = strings.TrimPrefix(rest, "`")
	i := 0
	for i < len(rest) {
		r, size := utf8.DecodeRuneInString(rest[i:])
		if i == 0 && !isIdentStart(r) {
			return "", false
		}
		if i > 0 && !isIdentCont(r) {
			break
		}
		i += size
	}
	if i == 0 {
		return "", false
	}
	name := rest[:i]
	tail := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(rest[i:]), "`"))
	if tail == "" || tail[0] > 127 || strings.HasPrefix(tail, "|") {
		return name, true
	}
	word := tail
	if sp := strings.IndexAny(tail, " \t|("); sp >= 0 {
		word = tail[:sp]
	}
	if _, ok := sqlTypes[strings.ToLower(strings.Trim(word, "`"))]; !ok {
		return "", false
	}
	return name, true
}

var sqlTypes = map[string]struct{}{
	"int": {}, "integer": {}, "bigint": {}, "smallint": {}, "tinyint": {}, "mediumint": {},
	"varchar": {}, "char": {}, "nvarchar": {}, "nchar": {},
	"text": {}, "tinytext": {}, "mediumtext": {}, "longtext": {},
	"datetime": {}, "timestamp": {}, "date": {}, "time": {}, "year": {},
	"decimal": {}, "numeric": {}, "float": {}, "double": {}, "real": {},
	"bit": {}, "bool": {}, "boolean": {}, "json": {},
	"blob": {}, "tinyblob": {}, "mediumblob": {}, "longblob": {},
	"enum": {}, "set": {}, "binary": {}, "varbinary": {},
}

type sqlTok struct {
	kind   byte
	lit    string
	kw     string
	start  int
	end    int
	quoted bool
}

type sqlEdit struct {
	start int
	end   int
	text  string
}

type colRef struct {
	parts []sqlTok
	star  bool
}

type sqlSource struct {
	key    string
	table  *sqlTable
	opaque bool
}

type sqlScope struct {
	paren   int
	parent  *sqlScope
	from    bool
	expect  bool
	sources []sqlSource
	labels  map[string]struct{}
	cols    []colRef
}

func newSQLScope(paren int, parent *sqlScope) *sqlScope {
	return &sqlScope{paren: paren, parent: parent, labels: map[string]struct{}{}}
}

func (s *sqlScope) addReal(key string, tb *sqlTable) {
	key = strings.ToLower(key)
	s.sources = append(s.sources, sqlSource{key: key, table: tb})
	if key != "" {
		s.labels[key] = struct{}{}
	}
}

func (s *sqlScope) addOpaque(key string) {
	key = strings.ToLower(key)
	s.sources = append(s.sources, sqlSource{key: key, opaque: true})
	if key != "" {
		s.labels[key] = struct{}{}
	}
}

func (s *sqlScope) addLabel(name string) {
	if name == "" {
		return
	}
	s.labels[strings.ToLower(name)] = struct{}{}
}

func (s *sqlScope) hasLabel(name string) bool {
	if s == nil || name == "" {
		return false
	}
	_, ok := s.labels[strings.ToLower(name)]
	return ok
}

func (s *sqlScope) realTables() []*sqlTable {
	var out []*sqlTable
	seen := map[*sqlTable]struct{}{}
	for _, src := range s.sources {
		if src.opaque || src.table == nil || len(src.table.cols) == 0 {
			continue
		}
		if _, ok := seen[src.table]; ok {
			continue
		}
		seen[src.table] = struct{}{}
		out = append(out, src.table)
	}
	return out
}

type sqlWalker struct {
	toks     []sqlTok
	i        int
	depth    int
	after    bool
	stack    []*sqlScope
	all      []*sqlScope
	derived  []int
	ctes     map[string]struct{}
	cat      *sqlCatalog
	edits    []sqlEdit
	problems []string
}

func newSQLWalker(toks []sqlTok, cat *sqlCatalog) *sqlWalker {
	root := newSQLScope(-1, nil)
	return &sqlWalker{
		toks:  toks,
		stack: []*sqlScope{root},
		ctes:  map[string]struct{}{},
		cat:   cat,
	}
}

func (w *sqlWalker) cur() *sqlScope { return w.stack[len(w.stack)-1] }

func (w *sqlWalker) run() {
	for w.i < len(w.toks) {
		w.advance()
	}
	w.finish()
}

func (w *sqlWalker) advance() {
	before := w.i
	next := w.dispatch()
	if next <= before {
		next = before + 1
	}
	w.i = next
}

func (w *sqlWalker) dispatch() int {
	if w.i >= len(w.toks) {
		return w.i
	}
	t := w.toks[w.i]
	if t.kw == "WITH" {
		w.parseWith()
		return w.i
	}
	if t.kw != "" {
		return w.onKeyword()
	}
	switch t.kind {
	case tokPunct:
		return w.onPunct()
	case tokString, tokNumber:
		w.after = true
		return w.i + 1
	case tokIdent:
		return w.onIdent()
	default:
		return w.i + 1
	}
}

func (w *sqlWalker) onKeyword() int {
	switch w.toks[w.i].kw {
	case "SELECT":
		sc := newSQLScope(w.depth, w.cur())
		w.stack = append(w.stack, sc)
		w.all = append(w.all, sc)
		w.after = false
		return w.i + 1
	case "FROM", "JOIN", "STRAIGHT_JOIN":
		if w.depth == w.cur().paren {
			w.cur().from = true
			w.cur().expect = true
		}
		w.after = false
		return w.i + 1
	case "WHERE", "GROUP", "ORDER", "HAVING", "LIMIT", "UNION", "EXCEPT", "INTERSECT", "WINDOW", "FOR":
		if w.depth == w.cur().paren {
			w.cur().from = false
			w.cur().expect = false
		}
		w.after = false
		return w.i + 1
	case "END":
		w.after = true
		return w.i + 1
	case "AS":
		if w.i+1 < len(w.toks) && w.toks[w.i+1].kind == tokIdent && w.toks[w.i+1].kw == "" {
			w.cur().addLabel(w.toks[w.i+1].lit)
			w.after = true
			return w.i + 2
		}
		w.after = false
		return w.i + 1
	default:
		w.after = false
		return w.i + 1
	}
}

func (w *sqlWalker) onPunct() int {
	switch w.toks[w.i].lit {
	case "(":
		if w.cur().expect && w.depth == w.cur().paren {
			w.cur().expect = false
			w.depth++
			w.derived = append(w.derived, w.depth)
			w.after = false
			return w.i + 1
		}
		w.depth++
		w.after = false
		return w.i + 1
	case ")":
		if w.depth > 0 {
			w.depth--
		}
		for len(w.stack) > 1 && w.depth < w.stack[len(w.stack)-1].paren {
			w.stack = w.stack[:len(w.stack)-1]
		}
		if n := len(w.derived); n > 0 && w.derived[n-1] == w.depth+1 {
			w.derived = w.derived[:n-1]
			w.after = false
			return w.takeDerivedAlias(w.i + 1)
		}
		w.after = true
		return w.i + 1
	case ",":
		if w.cur().from && w.depth == w.cur().paren {
			w.cur().expect = true
		}
		w.after = false
		return w.i + 1
	case "*":
		w.after = !w.after
		return w.i + 1
	default:
		w.after = false
		return w.i + 1
	}
}

func (w *sqlWalker) onIdent() int {
	t := w.toks[w.i]
	if t.lit == "" {
		return w.i + 1
	}
	if w.cur().expect && w.depth == w.cur().paren {
		parts, next := w.readQual(w.i)
		return w.takeTable(parts, next)
	}
	if w.i+1 < len(w.toks) && w.toks[w.i+1].lit == "(" {
		w.after = false
		return w.i + 1
	}
	if w.i+1 < len(w.toks) && w.toks[w.i+1].kind == tokString {
		w.after = true
		return w.i + 2
	}
	parts, next := w.readQual(w.i)
	if next+1 < len(w.toks) && w.toks[next].lit == "." && w.toks[next+1].lit == "*" {
		w.cur().cols = append(w.cur().cols, colRef{parts: parts, star: true})
		w.after = true
		return next + 2
	}
	if len(parts) >= 2 {
		w.cur().cols = append(w.cur().cols, colRef{parts: parts})
		w.after = true
		return next
	}
	if w.after {
		w.cur().addLabel(t.lit)
		return w.i + 1
	}
	w.cur().cols = append(w.cur().cols, colRef{parts: parts})
	w.after = true
	return next
}

func (w *sqlWalker) readQual(i int) ([]sqlTok, int) {
	parts := []sqlTok{w.toks[i]}
	for i+2 < len(w.toks) && w.toks[i+1].lit == "." && w.toks[i+2].kind == tokIdent && w.toks[i+2].kw == "" {
		parts = append(parts, w.toks[i+2])
		i += 2
	}
	return parts, i + 1
}

func (w *sqlWalker) takeTable(parts []sqlTok, next int) int {
	tok, canon, opaque, msg := w.classifyTable(parts)
	if msg != "" {
		w.problem(msg)
		opaque = true
	}
	alias := ""
	if next < len(w.toks) && w.toks[next].kw == "AS" {
		next++
		if next < len(w.toks) && w.toks[next].kind == tokIdent && w.toks[next].kw == "" {
			alias = w.toks[next].lit
			next++
		}
	} else if next < len(w.toks) && w.toks[next].kind == tokIdent && w.toks[next].kw == "" {
		alias = w.toks[next].lit
		next++
	}
	if opaque {
		key := alias
		if key == "" {
			key = tok.lit
		}
		w.cur().addOpaque(key)
	} else if tb := w.cat.byName[canon]; tb != nil {
		key := alias
		if key == "" {
			key = canon
		}
		w.cur().addReal(key, tb)
		if tok.lit != canon {
			w.edit(tok, canon)
		}
	}
	w.cur().expect = false
	w.after = false
	return next
}

func (w *sqlWalker) classifyTable(parts []sqlTok) (sqlTok, string, bool, string) {
	if len(parts) == 0 {
		return sqlTok{}, "", true, "表名不能为空"
	}
	if len(parts) > 2 {
		return parts[len(parts)-1], "", true, "非法表名"
	}
	tok := parts[len(parts)-1]
	if _, ok := w.ctes[strings.ToLower(tok.lit)]; ok || strings.EqualFold(tok.lit, "dual") {
		return tok, "", true, ""
	}
	canon, err := w.cat.resolveTable(tok.lit)
	if err != nil {
		return tok, "", true, err.Error()
	}
	return tok, canon, false, ""
}

func (w *sqlWalker) takeDerivedAlias(i int) int {
	if i < len(w.toks) && w.toks[i].kw == "AS" {
		i++
	}
	if i < len(w.toks) && w.toks[i].kind == tokIdent && w.toks[i].kw == "" {
		w.cur().addOpaque(w.toks[i].lit)
		return i + 1
	}
	return i
}

func (w *sqlWalker) parseWith() {
	w.i++
	if w.i < len(w.toks) && w.toks[w.i].kw == "RECURSIVE" {
		w.i++
	}
	for {
		if w.i >= len(w.toks) || w.toks[w.i].kind != tokIdent || w.toks[w.i].kw != "" {
			w.problem("WITH 缺少公用表名称")
			return
		}
		w.ctes[strings.ToLower(w.toks[w.i].lit)] = struct{}{}
		w.i++
		if w.i < len(w.toks) && w.toks[w.i].lit == "(" {
			w.skipBalanced()
		}
		if w.i >= len(w.toks) || w.toks[w.i].kw != "AS" {
			w.problem("WITH 缺少 AS")
			return
		}
		w.i++
		if w.i >= len(w.toks) || w.toks[w.i].lit != "(" {
			w.problem("WITH 缺少子查询")
			return
		}
		w.depth++
		closeAt := w.depth
		w.i++
		for w.i < len(w.toks) && w.depth >= closeAt {
			w.advance()
		}
		if w.i < len(w.toks) && w.toks[w.i].lit == "," {
			w.i++
			continue
		}
		return
	}
}

func (w *sqlWalker) skipBalanced() {
	depth := 0
	for w.i < len(w.toks) {
		switch w.toks[w.i].lit {
		case "(":
			depth++
			w.i++
		case ")":
			depth--
			w.i++
			if depth == 0 {
				return
			}
		default:
			w.i++
		}
	}
}

func (w *sqlWalker) finish() {
	for _, sc := range w.all {
		for _, ref := range sc.cols {
			if ref.star {
				w.resolveStar(sc, ref.parts)
				continue
			}
			switch len(ref.parts) {
			case 1:
				w.resolveBare(sc, ref.parts[0])
			case 2:
				w.resolveQual(sc, ref.parts[0], ref.parts[1])
			default:
				if len(ref.parts) >= 2 {
					w.resolveSchemaCol(ref.parts)
				}
			}
		}
	}
}

func (w *sqlWalker) resolveStar(sc *sqlScope, parts []sqlTok) {
	if len(parts) == 0 {
		return
	}
	qual := parts[len(parts)-1]
	src, ok := findSQLSource(sc, qual.lit)
	if !ok {
		w.problem(fmt.Sprintf("未知表或别名 %s", qual.lit))
		return
	}
	w.fixQualifier(qual, src)
}

func (w *sqlWalker) resolveQual(sc *sqlScope, qual, col sqlTok) {
	src, ok := findSQLSource(sc, qual.lit)
	if !ok {
		w.problem(fmt.Sprintf("未知表或别名 %s", qual.lit))
		return
	}
	w.fixQualifier(qual, src)
	if src.opaque || src.table == nil || len(src.table.cols) == 0 {
		return
	}
	w.fixColumn(src.table, col, qual.lit)
}

func (w *sqlWalker) resolveSchemaCol(parts []sqlTok) {
	tableTok := parts[len(parts)-2]
	colTok := parts[len(parts)-1]
	canon, err := w.cat.resolveTable(tableTok.lit)
	if err != nil {
		w.problem(err.Error())
		return
	}
	if tableTok.lit != canon {
		w.edit(tableTok, canon)
	}
	tb := w.cat.byName[canon]
	if tb == nil || len(tb.cols) == 0 {
		return
	}
	w.fixColumn(tb, colTok, canon)
}

func (w *sqlWalker) resolveBare(sc *sqlScope, tok sqlTok) {
	if sc == nil {
		return
	}
	if sc.hasLabel(tok.lit) {
		return
	}
	tables := sc.realTables()
	if len(tables) == 0 {
		w.resolveBare(sc.parent, tok)
		return
	}
	got := foldCanons(tables, tok.lit)
	switch len(got) {
	case 1:
		w.edit(tok, got[0])
	case 0:
		if w.exactInParent(sc.parent, tok) {
			return
		}
		if canon, ok := fuzzyColumn(tables, tok.lit); ok {
			w.edit(tok, canon)
			return
		}
		if ignorableBare(tok.lit) || !asciiIdent(tok.lit) {
			return
		}
		w.problem(missingBare(tables, tok.lit))
	default:
		for _, c := range got {
			if c == tok.lit {
				return
			}
		}
		w.problem(fmt.Sprintf("列 %s 大小写有歧义：%s", tok.lit, strings.Join(got, "、")))
	}
}

func (w *sqlWalker) exactInParent(sc *sqlScope, tok sqlTok) bool {
	if sc == nil {
		return false
	}
	tables := sc.realTables()
	got := foldCanons(tables, tok.lit)
	if len(got) == 1 {
		w.edit(tok, got[0])
		return true
	}
	for _, c := range got {
		if c == tok.lit {
			return true
		}
	}
	if len(got) == 0 {
		return w.exactInParent(sc.parent, tok)
	}
	return false
}

func (w *sqlWalker) fixQualifier(qual sqlTok, src sqlSource) {
	if src.table == nil {
		return
	}
	if strings.EqualFold(qual.lit, src.table.name) && qual.lit != src.table.name {
		w.edit(qual, src.table.name)
	}
}

func (w *sqlWalker) fixColumn(tb *sqlTable, tok sqlTok, qual string) {
	got := tb.cols[strings.ToLower(tok.lit)]
	switch len(got) {
	case 1:
		w.edit(tok, got[0])
	case 0:
		if canon, ok := fuzzyColumn([]*sqlTable{tb}, tok.lit); ok {
			w.edit(tok, canon)
			return
		}
		w.problem(missingCol(tb, qual, tok.lit))
	default:
		for _, c := range got {
			if c == tok.lit {
				return
			}
		}
		w.problem(fmt.Sprintf("列 %s 大小写有歧义：%s", tok.lit, strings.Join(got, "、")))
	}
}

func (w *sqlWalker) edit(tok sqlTok, canon string) {
	if tok.lit == canon || canon == "" {
		return
	}
	for _, e := range w.edits {
		if e.start == tok.start {
			return
		}
	}
	text := canon
	if tok.quoted {
		text = "`" + strings.ReplaceAll(canon, "`", "``") + "`"
	}
	w.edits = append(w.edits, sqlEdit{start: tok.start, end: tok.end, text: text})
}

func (w *sqlWalker) problem(msg string) {
	for _, p := range w.problems {
		if p == msg {
			return
		}
	}
	if len(w.problems) >= 8 {
		return
	}
	w.problems = append(w.problems, msg)
}

func findSQLSource(sc *sqlScope, name string) (sqlSource, bool) {
	key := strings.ToLower(name)
	for sc != nil {
		for _, src := range sc.sources {
			if src.key == key {
				return src, true
			}
		}
		sc = sc.parent
	}
	return sqlSource{}, false
}

func foldCanons(tables []*sqlTable, name string) []string {
	key := strings.ToLower(name)
	var out []string
	seen := map[string]struct{}{}
	for _, tb := range tables {
		for _, canon := range tb.cols[key] {
			if _, ok := seen[canon]; ok {
				continue
			}
			seen[canon] = struct{}{}
			out = append(out, canon)
		}
	}
	return out
}

func missingCol(tb *sqlTable, qual, name string) string {
	where := tb.name
	if qual != "" && !strings.EqualFold(qual, tb.name) {
		where = qual + "（" + tb.name + "）"
	}
	msg := fmt.Sprintf("%s 没有列 %s", where, name)
	if sug := closestNames(tb.colNames(), name, 6); len(sug) > 0 {
		msg += "，相近的列：" + strings.Join(sug, "、")
	}
	return msg
}

func missingBare(tables []*sqlTable, name string) string {
	msg := fmt.Sprintf("当前查询没有列 %s", name)
	type item struct {
		label string
		score int
	}
	low := strings.ToLower(name)
	var items []item
	for _, tb := range tables {
		for _, col := range tb.colNames() {
			dist := damerau(low, strings.ToLower(col))
			if dist > 8 {
				continue
			}
			items = append(items, item{tb.name + "." + col, dist})
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].score != items[j].score {
			return items[i].score < items[j].score
		}
		return items[i].label < items[j].label
	})
	if len(items) > 6 {
		items = items[:6]
	}
	if len(items) == 0 {
		return msg
	}
	sug := make([]string, len(items))
	for i, it := range items {
		sug[i] = it.label
	}
	return msg + "，相近的列：" + strings.Join(sug, "、")
}

func ignorableBare(name string) bool {
	switch strings.ToUpper(name) {
	case "CURRENT_DATE", "CURRENT_TIME", "CURRENT_TIMESTAMP", "CURRENT_USER",
		"LOCALTIME", "LOCALTIMESTAMP", "UTC_DATE", "UTC_TIME", "UTC_TIMESTAMP", "SYSDATE":
		return true
	default:
		return false
	}
}

func asciiIdent(s string) bool {
	for _, r := range s {
		if r > 127 {
			return false
		}
	}
	return true
}

func fuzzyColumn(tables []*sqlTable, name string) (string, bool) {
	seen := map[string]struct{}{}
	var names []string
	for _, tb := range tables {
		if tb == nil {
			continue
		}
		for _, col := range tb.colNames() {
			key := strings.ToLower(col)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			names = append(names, col)
		}
	}
	return fuzzyName(names, name)
}

func fuzzyName(cands []string, name string) (string, bool) {
	if len([]rune(name)) < 4 {
		return "", false
	}
	low := strings.ToLower(name)
	best, bestD, second := "", 99, 99
	for _, cand := range cands {
		d := damerau(low, strings.ToLower(cand))
		if d < bestD {
			second = bestD
			bestD = d
			best = cand
		} else if d < second {
			second = d
		}
	}
	if best != "" && bestD >= 1 && bestD <= 2 && bestD < second {
		return best, true
	}
	return "", false
}

func closestNames(cands []string, name string, limit int) []string {
	low := strings.ToLower(name)
	type item struct {
		name  string
		score int
	}
	items := make([]item, 0, len(cands))
	for _, cand := range cands {
		cl := strings.ToLower(cand)
		if cl == low {
			continue
		}
		dist := damerau(low, cl)
		score := dist
		switch {
		case strings.HasPrefix(cl, low) || strings.HasPrefix(low, cl):
			score = dist - 3
		case strings.Contains(cl, low) || strings.Contains(low, cl):
			score = dist - 1
		}
		if dist > 8 && score > 4 {
			continue
		}
		items = append(items, item{name: cand, score: score})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].score != items[j].score {
			return items[i].score < items[j].score
		}
		return items[i].name < items[j].name
	})
	if len(items) > limit {
		items = items[:limit]
	}
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.name
	}
	return out
}

func damerau(a, b string) int {
	ar, br := []rune(a), []rune(b)
	if len(ar) == 0 {
		return len(br)
	}
	if len(br) == 0 {
		return len(ar)
	}
	if len(ar) > 64 || len(br) > 64 {
		return 99
	}
	d := make([][]int, len(ar)+1)
	for i := range d {
		d[i] = make([]int, len(br)+1)
		d[i][0] = i
	}
	for j := range d[0] {
		d[0][j] = j
	}
	for i := 1; i <= len(ar); i++ {
		for j := 1; j <= len(br); j++ {
			cost := 1
			if ar[i-1] == br[j-1] {
				cost = 0
			}
			d[i][j] = min(d[i-1][j]+1, d[i][j-1]+1, d[i-1][j-1]+cost)
			if i > 1 && j > 1 && ar[i-1] == br[j-2] && ar[i-2] == br[j-1] {
				d[i][j] = min(d[i][j], d[i-2][j-2]+1)
			}
		}
	}
	return d[len(ar)][len(br)]
}

func applySQLEdits(sql string, edits []sqlEdit) string {
	if len(edits) == 0 {
		return sql
	}
	sort.Slice(edits, func(i, j int) bool { return edits[i].start > edits[j].start })
	out := sql
	for _, e := range edits {
		if e.start < 0 || e.end > len(out) || e.start > e.end {
			continue
		}
		out = out[:e.start] + e.text + out[e.end:]
	}
	return out
}

func scanSQL(sql string) ([]sqlTok, error) {
	var out []sqlTok
	i := 0
	for i < len(sql) {
		r, size := utf8.DecodeRuneInString(sql[i:])
		if unicode.IsSpace(r) {
			i += size
			continue
		}
		if r == '\'' || r == '"' || r == '`' {
			end, err := skipQuoted(sql, i)
			if err != nil {
				return nil, err
			}
			if r == '`' {
				lit := strings.ReplaceAll(sql[i+1:end-1], "``", "`")
				out = append(out, sqlTok{kind: tokIdent, lit: lit, start: i, end: end, quoted: true})
			} else {
				out = append(out, sqlTok{kind: tokString, start: i, end: end})
			}
			i = end
			continue
		}
		if r == '#' || (r == '-' && i+1 < len(sql) && sql[i+1] == '-' && commentDash(sql, i+2)) {
			for i < len(sql) && sql[i] != '\n' {
				i++
			}
			continue
		}
		if r == '/' && i+1 < len(sql) && sql[i+1] == '*' {
			if i+2 < len(sql) && sql[i+2] == '!' {
				return nil, fmt.Errorf("禁止可执行注释")
			}
			end := strings.Index(sql[i+2:], "*/")
			if end < 0 {
				return nil, fmt.Errorf("SQL 注释未闭合")
			}
			i += end + 4
			continue
		}
		if isIdentStart(r) {
			j := i + size
			for j < len(sql) {
				r2, s2 := utf8.DecodeRuneInString(sql[j:])
				if !isIdentCont(r2) {
					break
				}
				j += s2
			}
			lit := sql[i:j]
			out = append(out, sqlTok{kind: tokIdent, lit: lit, kw: keywordOf(lit), start: i, end: j})
			i = j
			continue
		}
		if r >= '0' && r <= '9' || (r == '.' && i+1 < len(sql) && sql[i+1] >= '0' && sql[i+1] <= '9') {
			j := scanNumber(sql, i)
			out = append(out, sqlTok{kind: tokNumber, lit: sql[i:j], start: i, end: j})
			i = j
			continue
		}
		if op, n, ok := scanOp(sql, i); ok {
			out = append(out, sqlTok{kind: tokPunct, lit: op, start: i, end: n})
			i = n
			continue
		}
		out = append(out, sqlTok{kind: tokPunct, lit: sql[i : i+size], start: i, end: i + size})
		i += size
	}
	return out, nil
}

func commentDash(sql string, i int) bool {
	if i >= len(sql) {
		return true
	}
	return sql[i] == ' ' || sql[i] == '\t' || sql[i] == '\n' || sql[i] == '\r'
}

func isIdentStart(r rune) bool {
	return r == '_' || unicode.IsLetter(r)
}

func isIdentCont(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}

func scanNumber(sql string, i int) int {
	j := i
	if sql[j] == '0' && j+1 < len(sql) && (sql[j+1] == 'x' || sql[j+1] == 'X') {
		j += 2
		for j < len(sql) && isHex(sql[j]) {
			j++
		}
		return j
	}
	if sql[j] == '.' {
		j++
	}
	for j < len(sql) && sql[j] >= '0' && sql[j] <= '9' {
		j++
	}
	if j < len(sql) && sql[j] == '.' && j > i && sql[i] != '.' {
		j++
		for j < len(sql) && sql[j] >= '0' && sql[j] <= '9' {
			j++
		}
	}
	if j < len(sql) && (sql[j] == 'e' || sql[j] == 'E') {
		k := j + 1
		if k < len(sql) && (sql[k] == '+' || sql[k] == '-') {
			k++
		}
		if k < len(sql) && sql[k] >= '0' && sql[k] <= '9' {
			j = k + 1
			for j < len(sql) && sql[j] >= '0' && sql[j] <= '9' {
				j++
			}
		}
	}
	return j
}

func isHex(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

func scanOp(sql string, i int) (string, int, bool) {
	if i+2 < len(sql) {
		switch sql[i : i+3] {
		case "<=>":
			return "<=>", i + 3, true
		}
	}
	if i+1 < len(sql) {
		switch sql[i : i+2] {
		case "<=", ">=", "<>", "!=", "||", "&&", ":=":
			return sql[i : i+2], i + 2, true
		}
	}
	return "", 0, false
}

func keywordOf(lit string) string {
	up := strings.ToUpper(lit)
	if _, ok := sqlKeywords[up]; ok {
		return up
	}
	return ""
}

var sqlKeywords = map[string]struct{}{
	"SELECT": {}, "FROM": {}, "WHERE": {}, "AND": {}, "OR": {}, "NOT": {}, "IN": {}, "IS": {}, "NULL": {},
	"LIKE": {}, "BETWEEN": {}, "JOIN": {}, "LEFT": {}, "RIGHT": {}, "INNER": {}, "OUTER": {}, "CROSS": {},
	"ON": {}, "AS": {}, "GROUP": {}, "BY": {}, "ORDER": {}, "HAVING": {}, "LIMIT": {}, "OFFSET": {},
	"UNION": {}, "ALL": {}, "DISTINCT": {}, "CASE": {}, "WHEN": {}, "THEN": {}, "ELSE": {}, "END": {},
	"ASC": {}, "DESC": {}, "WITH": {}, "EXISTS": {}, "TRUE": {}, "FALSE": {}, "INTERVAL": {}, "XOR": {},
	"REGEXP": {}, "RLIKE": {}, "ESCAPE": {}, "USING": {}, "NATURAL": {}, "STRAIGHT_JOIN": {}, "FORCE": {},
	"IGNORE": {}, "INDEX": {}, "KEY": {}, "FOR": {}, "PARTITION": {}, "WINDOW": {}, "OVER": {}, "ROWS": {},
	"RANGE": {}, "UNBOUNDED": {}, "PRECEDING": {}, "FOLLOWING": {}, "CURRENT": {}, "ROW": {}, "GROUPS": {},
	"FILTER": {}, "WITHIN": {}, "LATERAL": {}, "RECURSIVE": {}, "EXCEPT": {}, "INTERSECT": {}, "COLLATE": {},
	"SEPARATOR": {}, "INTO": {}, "OUTFILE": {}, "DUMPFILE": {}, "LOCK": {}, "SHARE": {}, "MODE": {},
	"UPDATE": {}, "SET": {}, "DISTINCTROW": {}, "PROCEDURE": {}, "BINARY": {}, "CHARACTER": {}, "CHARSET": {},
	"CONVERT": {}, "CAST": {}, "LEADING": {}, "TRAILING": {}, "BOTH": {}, "DAY": {}, "HOUR": {}, "MINUTE": {},
	"SECOND": {}, "MICROSECOND": {}, "WEEK": {}, "MONTH": {}, "YEAR": {}, "QUARTER": {}, "DIV": {}, "MOD": {},
	"SOUNDS": {}, "MATCH": {}, "AGAINST": {}, "DEFAULT": {}, "PRIMARY": {}, "UNIQUE": {}, "FOREIGN": {},
	"REFERENCES": {}, "CONSTRAINT": {}, "TABLE": {}, "DATABASE": {}, "SCHEMA": {}, "IF": {}, "ELSEIF": {},
	"BEGIN": {}, "DECLARE": {}, "EXPLAIN": {}, "DESCRIBE": {}, "SHOW": {}, "USE": {}, "SOME": {},
	"ANY": {}, "ADD": {}, "REPLACE": {}, "INSERT": {}, "DELETE": {}, "VALUES": {}, "BOOLEAN": {}, "UNKNOWN": {},
	"ISNULL": {}, "ROLLUP": {}, "CUBE": {}, "GROUPING": {}, "SETS": {}, "OF": {}, "SKIP": {}, "LOCKED": {},
	"NOWAIT": {}, "WAIT": {}, "TIES": {}, "NULLS": {}, "FIRST": {}, "LAST": {}, "FULL": {}, "TO": {},
}
