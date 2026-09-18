package dbconn

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/webapp/go-app/ai-agent/internal/config"
)

const maxSchemaRows = 100

// Client 独立 MySQL 业务库只读访问。
type Client struct {
	db      *sql.DB
	maxRows int
	timeout time.Duration
}

func Open(cfg config.DBConnConfig) (*Client, error) {
	if !cfg.IsEnabled() {
		return nil, nil
	}
	driver := strings.ToLower(strings.TrimSpace(cfg.Driver))
	if driver == "" {
		driver = "mysql"
	}
	if driver != "mysql" {
		return nil, fmt.Errorf("dbconn 仅支持 mysql")
	}
	db, err := sql.Open("mysql", cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("open mysql: %w", err)
	}
	if cfg.MaxOpenConns > 0 {
		db.SetMaxOpenConns(cfg.MaxOpenConns)
	}
	if cfg.MaxIdleConns > 0 {
		db.SetMaxIdleConns(cfg.MaxIdleConns)
	}
	db.SetConnMaxLifetime(30 * time.Minute)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping mysql: %w", err)
	}
	maxRows := cfg.MaxRows
	if maxRows <= 0 {
		maxRows = 50
	}
	timeout := time.Duration(cfg.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	return &Client{db: db, maxRows: maxRows, timeout: timeout}, nil
}

func (c *Client) Close() error {
	if c == nil || c.db == nil {
		return nil
	}
	return c.db.Close()
}

func (c *Client) withTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if c.timeout <= 0 {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, c.timeout)
}

func (c *Client) beginRead(ctx context.Context) (*sql.Tx, error) {
	tx, err := c.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return c.db.BeginTx(ctx, nil)
	}
	return tx, nil
}

func (c *Client) Query(ctx context.Context, sqlText string) (string, error) {
	if c == nil || c.db == nil {
		return "", fmt.Errorf("业务库未配置")
	}
	clean, err := ValidateSelect(sqlText)
	if err != nil {
		return "", err
	}
	ctx, cancel := c.withTimeout(ctx)
	defer cancel()
	tx, err := c.beginRead(ctx)
	if err != nil {
		return "", fmt.Errorf("开启只读事务: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	rows, err := tx.QueryContext(ctx, clean)
	if err != nil {
		return "", fmt.Errorf("查询失败: %w", err)
	}
	defer rows.Close()
	return encodeRows(rows, c.maxRows)
}

func encodeRows(rows *sql.Rows, maxRows int) (string, error) {
	cols, err := rows.Columns()
	if err != nil {
		return "", err
	}
	type result struct {
		Columns   []string         `json:"columns"`
		Rows      []map[string]any `json:"rows"`
		Truncated bool             `json:"truncated"`
	}
	out := result{Columns: cols, Rows: make([]map[string]any, 0)}
	for rows.Next() {
		if maxRows > 0 && len(out.Rows) >= maxRows {
			out.Truncated = true
			break
		}
		raw := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range raw {
			ptrs[i] = &raw[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return "", err
		}
		rec := make(map[string]any, len(cols))
		for i, name := range cols {
			rec[name] = normalizeValue(raw[i])
		}
		out.Rows = append(out.Rows, rec)
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	b, err := json.Marshal(out)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func normalizeValue(v any) any {
	switch x := v.(type) {
	case nil:
		return nil
	case []byte:
		return string(x)
	case time.Time:
		return x.Format(time.RFC3339)
	default:
		return x
	}
}
