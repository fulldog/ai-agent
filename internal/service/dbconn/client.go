package dbconn

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync/atomic"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/webapp/go-app/ai-agent/internal/config"
	"go.uber.org/zap"
)

const maxSchemaRows = 100

var dialSeq atomic.Uint64

// Client 独立 MySQL 业务库只读访问。
type Client struct {
	db      *sql.DB
	tunnel  *sshTunnel
	maxRows int
	timeout time.Duration
}

func Open(cfg config.DBConnConfig, log *zap.Logger) (*Client, error) {
	if !cfg.IsEnabled() {
		return nil, nil
	}
	if log == nil {
		log = zap.NewNop()
	}
	driver := strings.ToLower(strings.TrimSpace(cfg.Driver))
	if driver == "" {
		driver = "mysql"
	}
	if driver != "mysql" {
		return nil, fmt.Errorf("dbconn 仅支持 mysql")
	}

	if cfg.SSH.Enabled && strings.TrimSpace(cfg.SSH.Host) == "" {
		return nil, fmt.Errorf("ssh.enabled 为 true 时必须配置 ssh.host")
	}

	dsn := cfg.DSN
	var tunnel *sshTunnel
	if cfg.SSH.IsEnabled() {
		t := newSSHTunnel(cfg.SSH, log)
		if err := t.start(context.Background()); err != nil {
			return nil, fmt.Errorf("ssh 隧道: %w", err)
		}
		netName := fmt.Sprintf("dbconnssh%d", dialSeq.Add(1))
		mysql.RegisterDialContext(netName, t.Dial)
		rewritten, err := rewriteNet(dsn, netName)
		if err != nil {
			t.Close()
			return nil, err
		}
		dsn = rewritten
		tunnel = t
	}

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		if tunnel != nil {
			tunnel.Close()
		}
		return nil, fmt.Errorf("open mysql: %w", err)
	}
	if cfg.MaxOpenConns > 0 {
		db.SetMaxOpenConns(cfg.MaxOpenConns)
	}
	if cfg.MaxIdleConns > 0 {
		db.SetMaxIdleConns(cfg.MaxIdleConns)
	}
	lifetime := 30 * time.Minute
	if tunnel != nil {
		lifetime = 5 * time.Minute
	}
	db.SetConnMaxLifetime(lifetime)
	db.SetConnMaxIdleTime(2 * time.Minute)

	c := &Client{db: db, tunnel: tunnel, maxRows: cfg.MaxRows, timeout: time.Duration(cfg.TimeoutSeconds) * time.Second}
	if c.maxRows <= 0 {
		c.maxRows = 50
	}
	if c.timeout <= 0 {
		c.timeout = 15 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_ = c.Close()
		return nil, fmt.Errorf("ping mysql: %w", err)
	}
	return c, nil
}

func rewriteNet(dsn, netName string) (string, error) {
	mc, err := mysql.ParseDSN(dsn)
	if err != nil {
		return "", fmt.Errorf("解析 DSN: %w", err)
	}
	mc.Net = netName
	return mc.FormatDSN(), nil
}

func (c *Client) Close() error {
	if c == nil {
		return nil
	}
	var err error
	if c.db != nil {
		err = c.db.Close()
	}
	if c.tunnel != nil {
		c.tunnel.Close()
	}
	return err
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
	run := func(ctx context.Context) (string, error) {
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
	return c.do(ctx, run)
}

func (c *Client) do(ctx context.Context, fn func(context.Context) (string, error)) (string, error) {
	ctx, cancel := c.withTimeout(ctx)
	defer cancel()
	out, err := fn(ctx)
	if err == nil || !isTransient(err) || c.tunnel == nil {
		return out, err
	}
	if rerr := c.tunnel.connect(ctx); rerr != nil {
		return "", err
	}
	return fn(ctx)
}

func isTransient(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, net.ErrClosed) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var ne net.Error
	if errors.As(err, &ne) {
		return true
	}
	msg := strings.ToLower(err.Error())
	for _, s := range []string{"broken pipe", "connection reset", "eof", "invalid connection", "bad connection", "ssh", "i/o timeout"} {
		if strings.Contains(msg, s) {
			return true
		}
	}
	return false
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
