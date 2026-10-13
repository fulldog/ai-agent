package dbconn

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/webapp/go-app/ai-agent/internal/config"
	"go.uber.org/zap"
)

const maxSchemaRows = 100

var dialSeq atomic.Uint64

const defaultDictionaryPath = "docs/srm_dictionary.json"

// Client 独立 MySQL 业务库只读访问。schema 读本地数据字典。
type Client struct {
	db       *sql.DB
	tunnel   *sshTunnel
	mu       sync.RWMutex
	dict     *Dictionary
	dictPath string
	maxRows  int
	timeout  time.Duration
	log      *zap.Logger
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

	dictPath := strings.TrimSpace(cfg.Dictionary)
	if dictPath == "" {
		dictPath = defaultDictionaryPath
	}
	dict, err := LoadDictionary(dictPath)
	if err != nil {
		return nil, err
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

	c := &Client{
		db: db, tunnel: tunnel, dict: dict, dictPath: dictPath,
		maxRows: cfg.MaxRows, timeout: time.Duration(cfg.TimeoutSeconds) * time.Second,
		log: log.With(zap.String("component", "dbconn")),
	}
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

// Table 一次 SELECT 读到的表。Rows 与 Columns 顺序一致，单元格已转成文本。
type Table struct {
	SQL       string
	Columns   []string
	Rows      [][]string
	Truncated bool
}

func (c *Client) Query(ctx context.Context, sqlText string) (string, error) {
	text, _, err := c.QueryTable(ctx, sqlText)
	return text, err
}

// QueryTable 执行只读 SELECT，同时返回给模型看的 JSON 和完整表。
// JSON 与表是同一次读取的同一批行，表不会比 JSON 少行。
func (c *Client) QueryTable(ctx context.Context, sqlText string) (string, Table, error) {
	if c == nil || c.db == nil {
		return "", Table{}, fmt.Errorf("业务库未配置")
	}
	clean, err := ValidateSelect(sqlText)
	if err != nil {
		return "", Table{}, err
	}
	var table Table
	start := time.Now()
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
		text, next, err := encodeRows(rows, c.maxRows)
		if err != nil {
			return "", err
		}
		table = next
		return text, nil
	}
	text, err := c.do(ctx, run)
	c.logQuery(clean, len(table.Rows), table.Truncated, time.Since(start), err)
	if err != nil {
		return "", Table{SQL: clean}, err
	}
	table.SQL = clean
	return text, table, nil
}

func (c *Client) logQuery(sqlText string, rows int, truncated bool, elapsed time.Duration, err error) {
	if c == nil || c.log == nil {
		return
	}
	fields := []zap.Field{
		zap.String("sql", sqlText),
		zap.Int("rows", rows),
		zap.Bool("truncated", truncated),
		zap.Duration("elapsed", elapsed),
	}
	if err != nil {
		c.log.Error("dbconn query", append(fields, zap.Error(err))...)
		return
	}
	c.log.Info("dbconn query", fields...)
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

func encodeRows(rows *sql.Rows, maxRows int) (string, Table, error) {
	cols, err := rows.Columns()
	if err != nil {
		return "", Table{}, err
	}
	type result struct {
		Columns   []string         `json:"columns"`
		Rows      []map[string]any `json:"rows"`
		Truncated bool             `json:"truncated"`
	}
	out := result{Columns: cols, Rows: make([]map[string]any, 0)}
	table := Table{Columns: append([]string(nil), cols...), Rows: make([][]string, 0)}
	for rows.Next() {
		if maxRows > 0 && len(out.Rows) >= maxRows {
			out.Truncated = true
			table.Truncated = true
			break
		}
		raw := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range raw {
			ptrs[i] = &raw[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return "", Table{}, err
		}
		rec := make(map[string]any, len(cols))
		line := make([]string, len(cols))
		for i, name := range cols {
			val := normalizeValue(raw[i])
			rec[name] = val
			line[i] = cellText(val)
		}
		out.Rows = append(out.Rows, rec)
		table.Rows = append(table.Rows, line)
	}
	if err := rows.Err(); err != nil {
		return "", Table{}, err
	}
	b, err := json.Marshal(out)
	if err != nil {
		return "", Table{}, err
	}
	return string(b), table, nil
}

func cellText(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case bool:
		if x {
			return "是"
		}
		return "否"
	case int:
		return strconv.Itoa(x)
	case int64:
		return strconv.FormatInt(x, 10)
	case uint64:
		return strconv.FormatUint(x, 10)
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(x), 'f', -1, 32)
	default:
		return fmt.Sprint(x)
	}
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
