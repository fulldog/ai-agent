package dbconn

import (
	"context"
	"fmt"
	"strings"
)

const tableNamePrefix = "Srm"

const listTablesSQL = `
SELECT TABLE_NAME, TABLE_TYPE, TABLE_COMMENT
FROM information_schema.TABLES
WHERE TABLE_SCHEMA = DATABASE()
  AND TABLE_NAME LIKE BINARY 'Srm%'
  AND (? = '' OR TABLE_NAME LIKE ? ESCAPE '\\' OR TABLE_COMMENT LIKE ? ESCAPE '\\')
ORDER BY TABLE_NAME
LIMIT ?`

const listColumnsSQL = `
SELECT COLUMN_NAME, DATA_TYPE, COLUMN_TYPE, IS_NULLABLE, COLUMN_KEY, COLUMN_DEFAULT, COLUMN_COMMENT
FROM information_schema.COLUMNS
WHERE TABLE_SCHEMA = DATABASE()
  AND BINARY TABLE_NAME = ?
ORDER BY ORDINAL_POSITION`

const listColumnsInSchemaSQL = `
SELECT COLUMN_NAME, DATA_TYPE, COLUMN_TYPE, IS_NULLABLE, COLUMN_KEY, COLUMN_DEFAULT, COLUMN_COMMENT
FROM information_schema.COLUMNS
WHERE TABLE_SCHEMA = ?
  AND BINARY TABLE_NAME = ?
ORDER BY ORDINAL_POSITION`

// Schema 列出当前库表或指定表的列（含注释，作为数据字典）。
func (c *Client) Schema(ctx context.Context, keyword, table string) (string, error) {
	if c == nil || c.db == nil {
		return "", fmt.Errorf("业务库未配置")
	}
	table = strings.TrimSpace(table)
	kw := strings.TrimSpace(keyword)
	pat := ""
	if kw != "" {
		pat = likeContains(kw)
	}
	return c.do(ctx, func(ctx context.Context) (string, error) {
		tx, err := c.beginRead(ctx)
		if err != nil {
			return "", fmt.Errorf("开启只读事务: %w", err)
		}
		defer func() { _ = tx.Rollback() }()

		if table != "" {
			schemaName, tableName, err := splitTableIdent(table)
			if err != nil {
				return "", err
			}
			if !hasTablePrefix(tableName) {
				return "", fmt.Errorf("仅允许扫描表名前缀为 %s 的表（区分大小写）", tableNamePrefix)
			}
			query := listColumnsSQL
			args := []any{tableName}
			if schemaName != "" {
				query = listColumnsInSchemaSQL
				args = []any{schemaName, tableName}
			}
			rows, err := tx.QueryContext(ctx, query, args...)
			if err != nil {
				return "", fmt.Errorf("查询列信息失败: %w", err)
			}
			defer rows.Close()
			return encodeRows(rows, maxSchemaRows)
		}

		rows, err := tx.QueryContext(ctx, listTablesSQL, kw, pat, pat, maxSchemaRows)
		if err != nil {
			return "", fmt.Errorf("查询表信息失败: %w", err)
		}
		defer rows.Close()
		return encodeRows(rows, maxSchemaRows)
	})
}
