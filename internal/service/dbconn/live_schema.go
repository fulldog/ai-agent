package dbconn

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

type rowQueryer interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

type liveTable struct {
	name    string
	comment string
	rows    int
}

type liveColumn struct {
	table    string
	name     string
	typ      string
	nullable bool
	key      string
	def      string
	extra    string
	comment  string
}

type liveIndex struct {
	table  string
	name   string
	unique bool
	seq    int
	column string
}

func (c *Client) liveSchema(ctx context.Context, keyword, table string) (string, error) {
	only := ""
	if strings.TrimSpace(table) != "" {
		name, err := requireSrmTable(table)
		if err != nil {
			return "", err
		}
		only = name
	}
	var d *Dictionary
	_, err := c.do(ctx, func(ctx context.Context) (string, error) {
		loaded, err := c.readLiveDictionary(ctx, only)
		if err != nil {
			return "", err
		}
		d = loaded
		return "", nil
	})
	if err != nil {
		return "", err
	}
	if d == nil || len(d.Tables) == 0 {
		if only != "" {
			return "", fmt.Errorf("未找到表 %s", only)
		}
		return "没有匹配的表\n", nil
	}
	if err := d.index(); err != nil {
		return "", err
	}
	if only != "" {
		return d.lookupTable(only)
	}
	return d.lookup(keyword, "")
}

func (c *Client) readLiveDictionary(ctx context.Context, table string) (*Dictionary, error) {
	if c == nil || c.db == nil {
		return nil, fmt.Errorf("业务库未配置")
	}
	tx, err := c.beginRead(ctx)
	if err != nil {
		return nil, fmt.Errorf("开启只读事务: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	tables, err := queryLiveTables(ctx, tx, table)
	if err != nil {
		return nil, err
	}
	cols, err := queryLiveColumns(ctx, tx, table)
	if err != nil {
		return nil, err
	}
	indexes, err := queryLiveIndexes(ctx, tx, table)
	if err != nil {
		return nil, err
	}
	return assembleLiveDictionary(tables, cols, indexes), nil
}

func queryLiveTables(ctx context.Context, q rowQueryer, table string) ([]liveTable, error) {
	query := `
SELECT TABLE_NAME, TABLE_COMMENT, IFNULL(TABLE_ROWS, 0)
FROM information_schema.TABLES
WHERE TABLE_SCHEMA = DATABASE()
  AND TABLE_NAME LIKE BINARY ?
ORDER BY TABLE_NAME`
	args := []any{tableNamePrefix + "%"}
	if table != "" {
		query = `
SELECT TABLE_NAME, TABLE_COMMENT, IFNULL(TABLE_ROWS, 0)
FROM information_schema.TABLES
WHERE TABLE_SCHEMA = DATABASE()
  AND TABLE_NAME = ?`
		args = []any{table}
	}
	var out []liveTable
	err := queryRows(ctx, q, query, args, func(rows *sql.Rows) error {
		var item liveTable
		var nrows int64
		if err := rows.Scan(&item.name, &item.comment, &nrows); err != nil {
			return err
		}
		if !hasTablePrefix(item.name) {
			return nil
		}
		item.rows = int(nrows)
		out = append(out, item)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("读取表: %w", err)
	}
	return out, nil
}

func queryLiveColumns(ctx context.Context, q rowQueryer, table string) ([]liveColumn, error) {
	query := `
SELECT TABLE_NAME, COLUMN_NAME, COLUMN_TYPE, IS_NULLABLE, COLUMN_KEY,
       COLUMN_DEFAULT, EXTRA, COLUMN_COMMENT
FROM information_schema.COLUMNS
WHERE TABLE_SCHEMA = DATABASE()
  AND TABLE_NAME LIKE BINARY ?
ORDER BY TABLE_NAME, ORDINAL_POSITION`
	args := []any{tableNamePrefix + "%"}
	if table != "" {
		query = `
SELECT TABLE_NAME, COLUMN_NAME, COLUMN_TYPE, IS_NULLABLE, COLUMN_KEY,
       COLUMN_DEFAULT, EXTRA, COLUMN_COMMENT
FROM information_schema.COLUMNS
WHERE TABLE_SCHEMA = DATABASE()
  AND TABLE_NAME = ?
ORDER BY ORDINAL_POSITION`
		args = []any{table}
	}
	var out []liveColumn
	err := queryRows(ctx, q, query, args, func(rows *sql.Rows) error {
		var item liveColumn
		var nullable string
		var def sql.NullString
		if err := rows.Scan(&item.table, &item.name, &item.typ, &nullable, &item.key, &def, &item.extra, &item.comment); err != nil {
			return err
		}
		if !hasTablePrefix(item.table) {
			return nil
		}
		item.nullable = nullable == "YES"
		if def.Valid {
			item.def = def.String
		}
		out = append(out, item)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("读取字段: %w", err)
	}
	return out, nil
}

func queryLiveIndexes(ctx context.Context, q rowQueryer, table string) ([]liveIndex, error) {
	query := `
SELECT TABLE_NAME, INDEX_NAME, NON_UNIQUE, SEQ_IN_INDEX, COLUMN_NAME
FROM information_schema.STATISTICS
WHERE TABLE_SCHEMA = DATABASE()
  AND TABLE_NAME LIKE BINARY ?
ORDER BY TABLE_NAME, INDEX_NAME, SEQ_IN_INDEX`
	args := []any{tableNamePrefix + "%"}
	if table != "" {
		query = `
SELECT TABLE_NAME, INDEX_NAME, NON_UNIQUE, SEQ_IN_INDEX, COLUMN_NAME
FROM information_schema.STATISTICS
WHERE TABLE_SCHEMA = DATABASE()
  AND TABLE_NAME = ?
ORDER BY INDEX_NAME, SEQ_IN_INDEX`
		args = []any{table}
	}
	var out []liveIndex
	err := queryRows(ctx, q, query, args, func(rows *sql.Rows) error {
		var item liveIndex
		var nonUnique int
		if err := rows.Scan(&item.table, &item.name, &nonUnique, &item.seq, &item.column); err != nil {
			return err
		}
		if !hasTablePrefix(item.table) {
			return nil
		}
		item.unique = nonUnique == 0
		out = append(out, item)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("读取索引: %w", err)
	}
	return out, nil
}

func queryRows(ctx context.Context, q rowQueryer, query string, args []any, scan func(*sql.Rows) error) error {
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		if err := scan(rows); err != nil {
			return err
		}
	}
	return rows.Err()
}

func assembleLiveDictionary(tables []liveTable, cols []liveColumn, indexes []liveIndex) *Dictionary {
	colsByTable := map[string][]DictColumn{}
	for _, col := range cols {
		colsByTable[col.table] = append(colsByTable[col.table], DictColumn{
			Name: col.name, Type: col.typ, Nullable: col.nullable,
			Key: col.key, Default: col.def, Extra: col.extra, Comment: col.comment,
		})
	}
	type idxBuild struct {
		unique  bool
		columns []string
	}
	idxByTable := map[string]map[string]*idxBuild{}
	idxOrder := map[string][]string{}
	pkByTable := map[string][]string{}
	for _, idx := range indexes {
		if idx.name == "PRIMARY" {
			pkByTable[idx.table] = append(pkByTable[idx.table], idx.column)
		}
		byName := idxByTable[idx.table]
		if byName == nil {
			byName = map[string]*idxBuild{}
			idxByTable[idx.table] = byName
		}
		item := byName[idx.name]
		if item == nil {
			item = &idxBuild{unique: idx.unique}
			byName[idx.name] = item
			idxOrder[idx.table] = append(idxOrder[idx.table], idx.name)
		}
		item.columns = append(item.columns, idx.column)
	}
	out := &Dictionary{Tables: make([]DictTable, 0, len(tables))}
	for _, table := range tables {
		item := DictTable{
			Name: table.name, Comment: table.comment, ApproxRows: table.rows,
			PrimaryKey: append([]string(nil), pkByTable[table.name]...),
			Columns:    colsByTable[table.name],
		}
		for _, name := range idxOrder[table.name] {
			idx := idxByTable[table.name][name]
			item.Indexes = append(item.Indexes, DictIndex{Name: name, Unique: idx.unique, Columns: idx.columns})
		}
		out.Tables = append(out.Tables, item)
	}
	return out
}
