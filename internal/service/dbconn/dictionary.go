package dbconn

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Dictionary 本地 Srm 数据字典。schema 只读这份结构，不查 information_schema。
type Dictionary struct {
	Conventions []string    `json:"conventions"`
	Relations   []Relation  `json:"relations"`
	Tables      []DictTable `json:"tables"`

	byName   map[string]*DictTable
	relByCol map[string][]RelationTable
}

// Relation 跨表关联键。
type Relation struct {
	Column string          `json:"column"`
	Tables []RelationTable `json:"tables"`
}

// RelationTable 关联键出现的表及键类型（PRI/UNI/MUL，可空）。
type RelationTable struct {
	Name string `json:"name"`
	Key  string `json:"key,omitempty"`
}

// DictTable 一张表的结构。
type DictTable struct {
	Name       string       `json:"name"`
	Comment    string       `json:"comment"`
	ApproxRows int          `json:"approx_rows"`
	PrimaryKey []string     `json:"primary_key"`
	Indexes    []DictIndex  `json:"indexes"`
	Columns    []DictColumn `json:"columns"`
}

// DictIndex 索引。
type DictIndex struct {
	Name    string   `json:"name"`
	Unique  bool     `json:"unique"`
	Columns []string `json:"columns"`
}

// DictColumn 字段。
type DictColumn struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Nullable bool   `json:"nullable"`
	Key      string `json:"key,omitempty"`
	Default  string `json:"default,omitempty"`
	Extra    string `json:"extra,omitempty"`
	Comment  string `json:"comment,omitempty"`
}

// LoadDictionary 读取 JSON 数据字典并建立表名索引。
func LoadDictionary(path string) (*Dictionary, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取数据字典 %s: %w", path, err)
	}
	d, err := parseDictionary(raw)
	if err != nil {
		return nil, fmt.Errorf("数据字典 %s: %w", path, err)
	}
	return d, nil
}

func parseDictionary(raw []byte) (*Dictionary, error) {
	var d Dictionary
	if err := json.Unmarshal(raw, &d); err != nil {
		return nil, fmt.Errorf("解析数据字典: %w", err)
	}
	if err := d.index(); err != nil {
		return nil, fmt.Errorf("数据字典: %w", err)
	}
	return &d, nil
}

// NewDictionaryStore 只绑定字典文件路径，不连接 MySQL。
func NewDictionaryStore(path string) *Client {
	return &Client{dictPath: strings.TrimSpace(path)}
}

// UpdateDictionary 校验请求体，写入 dictPath，并替换内存中的字典。
func (c *Client) UpdateDictionary(raw []byte) (tables, columns int, err error) {
	if c == nil || strings.TrimSpace(c.dictPath) == "" {
		return 0, 0, fmt.Errorf("业务库未配置")
	}
	d, err := parseDictionary(raw)
	if err != nil {
		return 0, 0, err
	}
	pretty, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return 0, 0, fmt.Errorf("序列化数据字典: %w", err)
	}
	pretty = append(pretty, '\n')
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := writeDictionaryFile(c.dictPath, pretty); err != nil {
		return 0, 0, err
	}
	c.dict = d
	cols := 0
	for i := range d.Tables {
		cols += len(d.Tables[i].Columns)
	}
	return len(d.Tables), cols, nil
}

func writeDictionaryFile(path string, raw []byte) error {
	dir := filepath.Dir(path)
	f, err := os.CreateTemp(dir, ".srm_dictionary-*.json")
	if err != nil {
		return fmt.Errorf("写入数据字典: %w", err)
	}
	tmp := f.Name()
	_, werr := f.Write(raw)
	cerr := f.Close()
	if werr != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("写入数据字典: %w", werr)
	}
	if cerr != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("写入数据字典: %w", cerr)
	}
	backup := path + ".bak"
	_ = os.Remove(backup)
	if err := os.Rename(path, backup); err != nil && !os.IsNotExist(err) {
		_ = os.Remove(tmp)
		return fmt.Errorf("写入数据字典: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Rename(backup, path)
		_ = os.Remove(tmp)
		return fmt.Errorf("写入数据字典: %w", err)
	}
	_ = os.Remove(backup)
	return nil
}

func (d *Dictionary) index() error {
	if d == nil || len(d.Tables) == 0 {
		return fmt.Errorf("没有表")
	}
	d.byName = make(map[string]*DictTable, len(d.Tables))
	for i := range d.Tables {
		t := &d.Tables[i]
		name := strings.TrimSpace(t.Name)
		if name == "" {
			return fmt.Errorf("存在空表名")
		}
		if _, ok := d.byName[name]; ok {
			return fmt.Errorf("表名重复: %s", name)
		}
		t.Name = name
		d.byName[name] = t
	}
	d.relByCol = make(map[string][]RelationTable, len(d.Relations))
	for _, rel := range d.Relations {
		col := strings.TrimSpace(rel.Column)
		if col == "" {
			continue
		}
		d.relByCol[col] = rel.Tables
	}
	return nil
}

func (d *Dictionary) lookup(keyword, table string) (string, error) {
	table = strings.TrimSpace(table)
	keyword = strings.TrimSpace(keyword)
	if table != "" {
		return d.lookupTable(table)
	}
	if keyword != "" {
		return d.lookupKeyword(keyword), nil
	}
	return d.lookupCatalog(), nil
}

func (d *Dictionary) lookupTable(table string) (string, error) {
	_, tableName, err := splitTableIdent(table)
	if err != nil {
		return "", err
	}
	if !hasTablePrefix(tableName) {
		return "", fmt.Errorf("仅允许扫描表名前缀为 %s 的表（区分大小写）", tableNamePrefix)
	}
	t, ok := d.byName[tableName]
	if !ok {
		return "", fmt.Errorf("未找到表 %s", tableName)
	}
	var b strings.Builder
	writeTableHeader(&b, t)
	b.WriteString("字段:\n")
	for _, col := range t.Columns {
		b.WriteString(formatColumn(col))
		b.WriteByte('\n')
	}
	if rels := d.relationsFor(t); rels != "" {
		b.WriteString("关联:\n")
		b.WriteString(rels)
	}
	return b.String(), nil
}

func (d *Dictionary) lookupKeyword(keyword string) string {
	var b strings.Builder
	written := 0
	truncated := false
	matched := false
	for i := range d.Tables {
		t := &d.Tables[i]
		tableHit := containsFold(t.Name, keyword) || containsFold(t.Comment, keyword)
		var hits []DictColumn
		for _, col := range t.Columns {
			if containsFold(col.Name, keyword) || containsFold(col.Comment, keyword) {
				hits = append(hits, col)
			}
		}
		if !tableHit && len(hits) == 0 {
			continue
		}
		if written >= maxSchemaRows {
			truncated = true
			break
		}
		matched = true
		writeTableHeader(&b, t)
		if len(hits) == 0 {
			written++
			continue
		}
		b.WriteString("命中字段:\n")
		for _, col := range hits {
			if written >= maxSchemaRows {
				truncated = true
				break
			}
			b.WriteString(formatColumn(col))
			b.WriteByte('\n')
			written++
		}
	}
	if !matched {
		return "没有匹配的表\n"
	}
	if truncated {
		b.WriteString("结果已截断\n")
	}
	return b.String()
}

func (d *Dictionary) lookupCatalog() string {
	var b strings.Builder
	if len(d.Conventions) > 0 {
		b.WriteString("约定:\n")
		for _, item := range d.Conventions {
			b.WriteString("- ")
			b.WriteString(item)
			b.WriteByte('\n')
		}
		b.WriteByte('\n')
	}
	b.WriteString("表目录:\n")
	for i := range d.Tables {
		t := &d.Tables[i]
		fmt.Fprintf(&b, "- %s | %s | 主键: %s\n", t.Name, t.Comment, strings.Join(t.PrimaryKey, ", "))
	}
	return b.String()
}

func (d *Dictionary) relationsFor(t *DictTable) string {
	if d == nil || t == nil {
		return ""
	}
	var b strings.Builder
	seen := map[string]struct{}{}
	for _, col := range t.Columns {
		tables, ok := d.relByCol[col.Name]
		if !ok {
			continue
		}
		if _, dup := seen[col.Name]; dup {
			continue
		}
		seen[col.Name] = struct{}{}
		fmt.Fprintf(&b, "- %s: %s\n", col.Name, formatRelationTables(tables))
	}
	return b.String()
}

func writeTableHeader(b *strings.Builder, t *DictTable) {
	fmt.Fprintf(b, "表 %s\n说明: %s\n约行数: %d\n主键: %s\n", t.Name, t.Comment, t.ApproxRows, strings.Join(t.PrimaryKey, ", "))
	if len(t.Indexes) == 0 {
		return
	}
	b.WriteString("索引:\n")
	for _, idx := range t.Indexes {
		kind := "普通"
		if idx.Name == "PRIMARY" {
			kind = "主键"
		} else if idx.Unique {
			kind = "唯一"
		}
		fmt.Fprintf(b, "- %s %s %s\n", idx.Name, kind, strings.Join(idx.Columns, ", "))
	}
}

func formatColumn(col DictColumn) string {
	nullText := "非空"
	if col.Nullable {
		nullText = "可空"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "- %s %s %s", col.Name, col.Type, nullText)
	if col.Key != "" {
		fmt.Fprintf(&b, " %s", col.Key)
	}
	if col.Default != "" {
		fmt.Fprintf(&b, " 默认 %s", col.Default)
	}
	if col.Extra != "" {
		fmt.Fprintf(&b, " %s", col.Extra)
	}
	if col.Comment != "" {
		fmt.Fprintf(&b, " | %s", col.Comment)
	}
	return b.String()
}

func formatRelationTables(tables []RelationTable) string {
	parts := make([]string, 0, len(tables))
	for _, t := range tables {
		if t.Key == "" {
			parts = append(parts, t.Name)
			continue
		}
		parts = append(parts, t.Name+"("+t.Key+")")
	}
	return strings.Join(parts, ", ")
}

func containsFold(hay, needle string) bool {
	return strings.Contains(strings.ToLower(hay), strings.ToLower(needle))
}
