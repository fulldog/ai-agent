package logger

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Options controls zap output and file rotation.
type Options struct {
	Level      string
	Encoding   string
	Dir        string // relative to process working directory; default "logs"
	Filename   string // legacy base name; unused when CategoryFiles=true
	MaxSizeMB  int    // rotate when file exceeds this size; default 100
	MaxBackups int
	MaxAgeDays int
	AlsoStdout bool // also write to stdout; pro/release should be false
}

// Bundle 按类别拆分日志文件：access / info / error / llm。
type Bundle struct {
	Access *zap.Logger // HTTP 访问日志 → logs/access-YYYY-MM-DD.log
	Info   *zap.Logger // 业务 info/warn/debug、SQL → logs/info-*.log
	Error  *zap.Logger // error 及以上 → logs/error-*.log
	LLM    *zap.Logger // 大模型完整 prompt/回复 → logs/llm-*.log（与 llm_call_logs.id 关联）
	App    *zap.Logger // 应用默认：info 级进 info 文件，error 级进 error 文件
}

// NewBundle 创建分类日志；pro/release 请设 AlsoStdout=false。
func NewBundle(opt Options) (*Bundle, error) {
	opt = normalizeOptions(opt)
	if err := os.MkdirAll(opt.Dir, 0o755); err != nil {
		return nil, fmt.Errorf("create log dir: %w", err)
	}

	lvl := parseLevel(opt.Level)
	encoder := newEncoder(opt.Encoding)

	accessWS := zapcore.AddSync(&dailySizeWriter{dir: opt.Dir, prefix: "access", maxBytes: int64(opt.MaxSizeMB) * 1024 * 1024})
	infoWS := zapcore.AddSync(&dailySizeWriter{dir: opt.Dir, prefix: "info", maxBytes: int64(opt.MaxSizeMB) * 1024 * 1024})
	errorWS := zapcore.AddSync(&dailySizeWriter{dir: opt.Dir, prefix: "error", maxBytes: int64(opt.MaxSizeMB) * 1024 * 1024})
	llmWS := zapcore.AddSync(&dailySizeWriter{dir: opt.Dir, prefix: "llm", maxBytes: int64(opt.MaxSizeMB) * 1024 * 1024})

	accessCores := []zapcore.Core{zapcore.NewCore(encoder, accessWS, lvl)}
	infoFileCore := zapcore.NewCore(encoder, infoWS, zap.LevelEnablerFunc(func(l zapcore.Level) bool {
		return l >= lvl && l < zapcore.ErrorLevel
	}))
	errorFileCore := zapcore.NewCore(encoder, errorWS, zap.LevelEnablerFunc(func(l zapcore.Level) bool {
		return l >= zapcore.ErrorLevel
	}))
	// error 同时落一份到 info，便于按时间线排查
	errorToInfoCore := zapcore.NewCore(encoder, infoWS, zap.LevelEnablerFunc(func(l zapcore.Level) bool {
		return l >= zapcore.ErrorLevel
	}))
	// 完整 prompt/回复体积大，仅写 llm 文件，不 tee 到 stdout
	llmCores := []zapcore.Core{zapcore.NewCore(encoder, llmWS, lvl)}

	infoCores := []zapcore.Core{infoFileCore}
	errorCores := []zapcore.Core{errorFileCore}
	appCores := []zapcore.Core{infoFileCore, errorFileCore, errorToInfoCore}

	if opt.AlsoStdout {
		stdout := zapcore.AddSync(os.Stdout)
		accessCores = append(accessCores, zapcore.NewCore(encoder, stdout, lvl))
		infoCores = append(infoCores, zapcore.NewCore(encoder, stdout, zap.LevelEnablerFunc(func(l zapcore.Level) bool {
			return l >= lvl && l < zapcore.ErrorLevel
		})))
		errorCores = append(errorCores, zapcore.NewCore(encoder, stdout, zap.LevelEnablerFunc(func(l zapcore.Level) bool {
			return l >= zapcore.ErrorLevel
		})))
		appCores = append(appCores, zapcore.NewCore(encoder, stdout, lvl))
	}

	opts := []zap.Option{zap.AddCaller(), zap.AddStacktrace(zapcore.ErrorLevel)}
	b := &Bundle{
		Access: zap.New(zapcore.NewTee(accessCores...), opts...).With(zap.String("category", "access")),
		Info:   zap.New(zapcore.NewTee(infoCores...), opts...).With(zap.String("category", "info")),
		Error:  zap.New(zapcore.NewTee(errorCores...), opts...).With(zap.String("category", "error")),
		LLM:    zap.New(zapcore.NewTee(llmCores...), opts...).With(zap.String("category", "llm")),
		App:    zap.New(zapcore.NewTee(appCores...), opts...),
	}
	return b, nil
}

// New 兼容旧接口：返回 App 日志（info/error 分流文件）。
func New(opt Options) (*zap.Logger, error) {
	b, err := NewBundle(opt)
	if err != nil {
		return nil, err
	}
	return b.App, nil
}

func parseLevel(level string) zapcore.Level {
	switch strings.ToLower(level) {
	case "debug":
		return zapcore.DebugLevel
	case "warn":
		return zapcore.WarnLevel
	case "error":
		return zapcore.ErrorLevel
	default:
		return zapcore.InfoLevel
	}
}

func newEncoder(encoding string) zapcore.Encoder {
	encCfg := zap.NewProductionEncoderConfig()
	encCfg.TimeKey = "ts"
	encCfg.EncodeTime = zapcore.ISO8601TimeEncoder
	if strings.EqualFold(encoding, "console") {
		encCfg = zap.NewDevelopmentEncoderConfig()
		encCfg.EncodeTime = zapcore.ISO8601TimeEncoder
		return zapcore.NewConsoleEncoder(encCfg)
	}
	return zapcore.NewJSONEncoder(encCfg)
}

func normalizeOptions(opt Options) Options {
	if strings.TrimSpace(opt.Dir) == "" {
		opt.Dir = "logs"
	}
	if strings.TrimSpace(opt.Filename) == "" {
		opt.Filename = "ai-agent"
	}
	if opt.MaxSizeMB <= 0 {
		opt.MaxSizeMB = 100
	}
	if opt.Level == "" {
		opt.Level = "info"
	}
	if opt.Encoding == "" {
		opt.Encoding = "json"
	}
	return opt
}

// IsProdMode 生产类模式：不向控制台输出日志。
func IsProdMode(mode string) bool {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "release", "prod", "pro", "production":
		return true
	default:
		return false
	}
}

// dailySizeWriter rotates by calendar day and by size.
type dailySizeWriter struct {
	dir      string
	prefix   string
	maxBytes int64

	mu    sync.Mutex
	day   string
	index int
	size  int64
	file  *os.File
}

func (w *dailySizeWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	day := time.Now().Format("2006-01-02")
	if w.file == nil || w.day != day {
		if err := w.open(day, 0); err != nil {
			return 0, err
		}
	}
	if w.size >= w.maxBytes {
		if err := w.open(w.day, w.index+1); err != nil {
			return 0, err
		}
	}
	n, err := w.file.Write(p)
	w.size += int64(n)
	return n, err
}

func (w *dailySizeWriter) Sync() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return nil
	}
	return w.file.Sync()
}

func (w *dailySizeWriter) open(day string, index int) error {
	if w.file != nil {
		_ = w.file.Close()
		w.file = nil
	}
	name := fmt.Sprintf("%s-%s.log", w.prefix, day)
	if index > 0 {
		name = fmt.Sprintf("%s-%s.%03d.log", w.prefix, day, index)
	}
	path := filepath.Join(w.dir, name)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return err
	}
	w.file = f
	w.day = day
	w.index = index
	w.size = info.Size()
	if w.size >= w.maxBytes {
		_ = w.file.Close()
		w.file = nil
		return w.open(day, index+1)
	}
	return nil
}
