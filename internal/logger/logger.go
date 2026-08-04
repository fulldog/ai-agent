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
	Filename   string // base name without date; default "ai-agent"
	MaxSizeMB  int    // rotate when file exceeds this size; default 100
	MaxBackups int    // reserved for future cleanup; currently unused
	MaxAgeDays int    // reserved for future cleanup; currently unused
	AlsoStdout bool   // also write to stdout; default true
}

// New builds a zap logger that writes under Dir with daily + size rotation.
// Active file: logs/ai-agent-2006-01-02.log
// When size exceeds MaxSizeMB: logs/ai-agent-2006-01-02.001.log, .002.log, ...
func New(opt Options) (*zap.Logger, error) {
	opt = normalizeOptions(opt)
	if err := os.MkdirAll(opt.Dir, 0o755); err != nil {
		return nil, fmt.Errorf("create log dir: %w", err)
	}

	lvl := zapcore.InfoLevel
	switch strings.ToLower(opt.Level) {
	case "debug":
		lvl = zapcore.DebugLevel
	case "warn":
		lvl = zapcore.WarnLevel
	case "error":
		lvl = zapcore.ErrorLevel
	}

	encCfg := zap.NewProductionEncoderConfig()
	encCfg.TimeKey = "ts"
	encCfg.EncodeTime = zapcore.ISO8601TimeEncoder
	var encoder zapcore.Encoder
	if strings.EqualFold(opt.Encoding, "console") {
		encCfg = zap.NewDevelopmentEncoderConfig()
		encCfg.EncodeTime = zapcore.ISO8601TimeEncoder
		encoder = zapcore.NewConsoleEncoder(encCfg)
	} else {
		encoder = zapcore.NewJSONEncoder(encCfg)
	}

	fileWS := zapcore.AddSync(&dailySizeWriter{
		dir:      opt.Dir,
		prefix:   opt.Filename,
		maxBytes: int64(opt.MaxSizeMB) * 1024 * 1024,
	})

	cores := []zapcore.Core{
		zapcore.NewCore(encoder, fileWS, lvl),
	}
	if opt.AlsoStdout {
		cores = append(cores, zapcore.NewCore(encoder, zapcore.AddSync(os.Stdout), lvl))
	}
	log := zap.New(zapcore.NewTee(cores...), zap.AddCaller(), zap.AddStacktrace(zapcore.ErrorLevel))
	return log, nil
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
	// Rotate before write when current file already at/over limit (keep single huge lines intact).
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
	// If reopening an already oversized file (restart), advance index.
	if w.size >= w.maxBytes {
		_ = w.file.Close()
		w.file = nil
		return w.open(day, index+1)
	}
	return nil
}
