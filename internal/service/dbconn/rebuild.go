package dbconn

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// ErrDBNotConnected 业务库连接不可用，无法重建字典。
var ErrDBNotConnected = errors.New("业务库未连接")

var defaultConventions = []string{
	"只读查询，表名必须原样使用（`Srm_` 开头，大小写敏感）",
	"多数表用 `IsDelete`（`bit(1)`，`0` 正常、`1` 删除）做软删除。查当前有效数据时加 `IsDelete = 0`",
	"`IsDelete_Mark` 常与 `IsDelete` 一起进入主键或唯一键：未删除时为 `0001-01-01`，删除后写入删除时间，用来允许同一业务键多次软删",
	"`SysNo` 是各表自己的自增号，不能用来跨表关联",
	"人员字段（`OwnerId`、`CreateById`、`ModifyById` 等）是 ERP 人员 Id，不在 `Srm` 表内",
	"`VendorLabel` 在多张表里既是业务标签，也是联合主键的一部分。过滤供应商时要同时带上标签，避免串数据",
	"注释里的枚举（如 `0/1/2/4`）以字段 `COLUMN_COMMENT` 为准，不要自行发明取值",
	"逗号包裹的列表字段（如 `,1,2,`）按字符串存储，匹配时用 `LIKE '%,值,%'`",
}

var relationSkipColumns = map[string]struct{}{
	"sysno":         {},
	"isdelete":      {},
	"isdelete_mark": {},
	"createtime":    {},
	"createbyid":    {},
	"modifytime":    {},
	"modifybyid":    {},
}

type scannedTable struct {
	name    string
	comment string
	rows    int
}

type scannedColumn struct {
	table    string
	name     string
	typ      string
	nullable bool
	key      string
	def      string
	extra    string
	comment  string
}

type scannedIndex struct {
	table  string
	name   string
	unique bool
	seq    int
	column string
}

// RebuildDictionary 重新连接业务库，读取 Srm 前缀表并重建数据字典。
func (c *Client) RebuildDictionary(ctx context.Context) (tables, columns int, err error) {
	if c == nil || c.db == nil {
		return 0, 0, ErrDBNotConnected
	}
	if strings.TrimSpace(c.dictPath) == "" {
		return 0, 0, fmt.Errorf("业务库未配置")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	timeout := 60 * time.Second
	if c.timeout > timeout {
		timeout = c.timeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if err := c.ensureConn(ctx); err != nil {
		return 0, 0, err
	}
	d, err := c.readSrmDictionary(ctx)
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
	return len(d.Tables), columnCount(d), nil
}

func (c *Client) ensureConn(ctx context.Context) error {
	if err := c.db.PingContext(ctx); err == nil {
		return nil
	} else if c.tunnel != nil {
		if rerr := c.tunnel.connect(ctx); rerr != nil {
			return fmt.Errorf("重新连接数据库: %w", rerr)
		}
		if err2 := c.db.PingContext(ctx); err2 != nil {
			return fmt.Errorf("重新连接数据库: %w", err2)
		}
		return nil
	} else {
		return fmt.Errorf("重新连接数据库: %w", err)
	}
}

func (c *Client) readSrmDictionary(ctx context.Context) (*Dictionary, error) {
	conn, err := c.db.Conn(ctx)
	if err != nil {
		return nil, fmt.Errorf("重新连接数据库: %w", err)
	}
	defer conn.Close()

	tables, err := querySrmTables(ctx, conn)
	if err != nil {
		return nil, err
	}
	if len(tables) == 0 {
		return nil, fmt.Errorf("未找到 %s 前缀的表", tableNamePrefix)
	}
	cols, err := querySrmColumns(ctx, conn)
	if err != nil {
		return nil, err
	}
	indexes, err := querySrmIndexes(ctx, conn)
	if err != nil {
		return nil, err
	}
	c.mu.RLock()
	var conventions []string
	if c.dict != nil {
		conventions = append(conventions, c.dict.Conventions...)
	}
	c.mu.RUnlock()
	d := assembleDictionary(conventions, tables, cols, indexes)
	if err := d.index(); err != nil {
		return nil, err
	}
	return d, nil
}

func querySrmTables(ctx context.Context, conn *sql.Conn) ([]scannedTable, error) {
	rows, err := conn.QueryContext(ctx, `
SELECT TABLE_NAME, TABLE_COMMENT, IFNULL(TABLE_ROWS, 0)
FROM information_schema.TABLES
WHERE TABLE_SCHEMA = DATABASE()
  AND TABLE_NAME LIKE BINARY ?
ORDER BY TABLE_NAME`, tableNamePrefix+"%")
	if err != nil {
		return nil, fmt.Errorf("读取 Srm 表: %w", err)
	}
	defer rows.Close()
	var out []scannedTable
	for rows.Next() {
		var name, comment string
		var nrows int64
		if err := rows.Scan(&name, &comment, &nrows); err != nil {
			return nil, fmt.Errorf("读取 Srm 表: %w", err)
		}
		out = append(out, scannedTable{name: name, comment: comment, rows: int(nrows)})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("读取 Srm 表: %w", err)
	}
	return out, nil
}

func querySrmColumns(ctx context.Context, conn *sql.Conn) ([]scannedColumn, error) {
	rows, err := conn.QueryContext(ctx, `
SELECT TABLE_NAME, COLUMN_NAME, COLUMN_TYPE, IS_NULLABLE, COLUMN_KEY,
       COLUMN_DEFAULT, EXTRA, COLUMN_COMMENT
FROM information_schema.COLUMNS
WHERE TABLE_SCHEMA = DATABASE()
  AND TABLE_NAME LIKE BINARY ?
ORDER BY TABLE_NAME, ORDINAL_POSITION`, tableNamePrefix+"%")
	if err != nil {
		return nil, fmt.Errorf("读取 Srm 字段: %w", err)
	}
	defer rows.Close()
	var out []scannedColumn
	for rows.Next() {
		var item scannedColumn
		var nullable, key, extra, comment string
		var def sql.NullString
		if err := rows.Scan(&item.table, &item.name, &item.typ, &nullable, &key, &def, &extra, &comment); err != nil {
			return nil, fmt.Errorf("读取 Srm 字段: %w", err)
		}
		item.nullable = nullable == "YES"
		item.key = key
		item.extra = extra
		item.comment = comment
		if def.Valid {
			item.def = def.String
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("读取 Srm 字段: %w", err)
	}
	return out, nil
}

func querySrmIndexes(ctx context.Context, conn *sql.Conn) ([]scannedIndex, error) {
	rows, err := conn.QueryContext(ctx, `
SELECT TABLE_NAME, INDEX_NAME, NON_UNIQUE, SEQ_IN_INDEX, COLUMN_NAME
FROM information_schema.STATISTICS
WHERE TABLE_SCHEMA = DATABASE()
  AND TABLE_NAME LIKE BINARY ?
ORDER BY TABLE_NAME, INDEX_NAME, SEQ_IN_INDEX`, tableNamePrefix+"%")
	if err != nil {
		return nil, fmt.Errorf("读取 Srm 索引: %w", err)
	}
	defer rows.Close()
	var out []scannedIndex
	for rows.Next() {
		var item scannedIndex
		var nonUnique int
		if err := rows.Scan(&item.table, &item.name, &nonUnique, &item.seq, &item.column); err != nil {
			return nil, fmt.Errorf("读取 Srm 索引: %w", err)
		}
		item.unique = nonUnique == 0
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("读取 Srm 索引: %w", err)
	}
	return out, nil
}

func assembleDictionary(conventions []string, tables []scannedTable, cols []scannedColumn, indexes []scannedIndex) *Dictionary {
	if len(conventions) == 0 {
		conventions = append([]string(nil), defaultConventions...)
	}
	colsByTable := map[string][]scannedColumn{}
	for _, col := range cols {
		colsByTable[col.table] = append(colsByTable[col.table], col)
	}
	type idxBuild struct {
		unique  bool
		columns []string
	}
	idxByTable := map[string]map[string]*idxBuild{}
	for _, idx := range indexes {
		byName := idxByTable[idx.table]
		if byName == nil {
			byName = map[string]*idxBuild{}
			idxByTable[idx.table] = byName
		}
		item := byName[idx.name]
		if item == nil {
			item = &idxBuild{unique: idx.unique}
			byName[idx.name] = item
		}
		item.columns = append(item.columns, idx.column)
	}

	d := &Dictionary{Conventions: conventions}
	for _, table := range tables {
		dt := DictTable{
			Name:       table.name,
			Comment:    table.comment,
			ApproxRows: table.rows,
		}
		if byName := idxByTable[table.name]; byName != nil {
			names := make([]string, 0, len(byName))
			for name := range byName {
				names = append(names, name)
			}
			sort.Strings(names)
			for _, name := range names {
				item := byName[name]
				dt.Indexes = append(dt.Indexes, DictIndex{Name: name, Unique: item.unique, Columns: item.columns})
				if name == "PRIMARY" {
					dt.PrimaryKey = append([]string(nil), item.columns...)
				}
			}
		}
		for _, col := range colsByTable[table.name] {
			dt.Columns = append(dt.Columns, DictColumn{
				Name:     col.name,
				Type:     col.typ,
				Nullable: col.nullable,
				Key:      col.key,
				Default:  col.def,
				Extra:    col.extra,
				Comment:  col.comment,
			})
		}
		d.Tables = append(d.Tables, dt)
	}
	d.Relations = buildRelations(cols)
	return d
}

func buildRelations(cols []scannedColumn) []Relation {
	type occ struct {
		table string
		key   string
	}
	grouped := map[string][]occ{}
	for _, col := range cols {
		if _, skip := relationSkipColumns[strings.ToLower(col.name)]; skip {
			continue
		}
		grouped[col.name] = append(grouped[col.name], occ{table: col.table, key: col.key})
	}
	names := make([]string, 0, len(grouped))
	for name, list := range grouped {
		if len(list) < 2 {
			continue
		}
		keyed := false
		for _, item := range list {
			if item.key == "PRI" || item.key == "UNI" || item.key == "MUL" {
				keyed = true
				break
			}
		}
		if keyed {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	rels := make([]Relation, 0, len(names))
	for _, name := range names {
		list := grouped[name]
		sort.Slice(list, func(i, j int) bool { return list[i].table < list[j].table })
		rel := Relation{Column: name}
		for _, item := range list {
			rel.Tables = append(rel.Tables, RelationTable{Name: item.table, Key: item.key})
		}
		rels = append(rels, rel)
	}
	return rels
}
