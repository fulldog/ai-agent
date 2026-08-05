package database

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/webapp/go-app/ai-agent/internal/config"
	"github.com/webapp/go-app/ai-agent/internal/model"
	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	glogger "gorm.io/gorm/logger"
)

func Open(cfg config.DatabaseConfig, log *zap.Logger) (*gorm.DB, error) {
	if log == nil {
		log = zap.NewNop()
	}
	db, err := gorm.Open(postgres.Open(cfg.DSN), &gorm.Config{
		Logger: newZapGormLogger(log, glogger.Info, 200*time.Millisecond),
	})
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	if cfg.MaxOpenConns > 0 {
		sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	}
	if cfg.MaxIdleConns > 0 {
		sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	}
	if err := db.Exec(`CREATE EXTENSION IF NOT EXISTS vector`).Error; err != nil {
		return nil, fmt.Errorf("enable pgvector: %w", err)
	}
	if cfg.AutoMigrate {
		if err := migrate(db); err != nil {
			return nil, err
		}
		if err := EnsureSchemaComments(db); err != nil {
			return nil, err
		}
	}
	return db, nil
}

func migrate(db *gorm.DB) error {
	if err := db.AutoMigrate(
		&model.Conversation{},
		&model.Message{},
		&model.Corpus{},
		&model.Document{},
		&model.Chunk{},
		&model.AgentRun{},
		&model.AgentStep{},
		&model.RequestLog{},
		&model.LLMCallLog{},
		&model.FileExtraction{},
	); err != nil {
		return fmt.Errorf("automigrate: %w", err)
	}
	var dim int
	_ = db.Raw(`SELECT 1`).Scan(&dim)
	return nil
}

// EnsureChunkEmbeddingColumn adds pgvector column if missing.
func EnsureChunkEmbeddingColumn(db *gorm.DB, dimensions int) error {
	if dimensions <= 0 {
		dimensions = 768
	}
	sql := fmt.Sprintf(`
DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM information_schema.columns
    WHERE table_name = 'chunks' AND column_name = 'embedding'
  ) THEN
    ALTER TABLE chunks ADD COLUMN embedding vector(%d);
  END IF;
END $$;`, dimensions)
	if err := db.Exec(sql).Error; err != nil {
		return fmt.Errorf("add embedding column: %w", err)
	}
	_ = db.Exec(`COMMENT ON COLUMN chunks.embedding IS '分块向量(pgvector)，维度与 embed.dimensions 一致'`).Error
	return nil
}

func EnsureVectorIndex(db *gorm.DB, kind string) error {
	switch kind {
	case "hnsw":
		return db.Exec(`
CREATE INDEX IF NOT EXISTS idx_chunks_embedding_hnsw
ON chunks USING hnsw (embedding vector_cosine_ops)
WITH (m = 16, ef_construction = 64)`).Error
	case "ivfflat":
		return db.Exec(`
CREATE INDEX IF NOT EXISTS idx_chunks_embedding_ivfflat
ON chunks USING ivfflat (embedding vector_cosine_ops)
WITH (lists = 100)`).Error
	default:
		return nil
	}
}

// zapGormLogger 将 GORM SQL 写入 zap（走 info/error 分类文件）。
type zapGormLogger struct {
	zap           *zap.Logger
	level         glogger.LogLevel
	slowThreshold time.Duration
}

func newZapGormLogger(log *zap.Logger, level glogger.LogLevel, slow time.Duration) glogger.Interface {
	return &zapGormLogger{zap: log.With(zap.String("component", "gorm")), level: level, slowThreshold: slow}
}

func (l *zapGormLogger) LogMode(level glogger.LogLevel) glogger.Interface {
	n := *l
	n.level = level
	return &n
}

func (l *zapGormLogger) Info(ctx context.Context, msg string, data ...interface{}) {
	if l.level < glogger.Info {
		return
	}
	l.zap.Sugar().Infof(msg, data...)
}

func (l *zapGormLogger) Warn(ctx context.Context, msg string, data ...interface{}) {
	if l.level < glogger.Warn {
		return
	}
	l.zap.Sugar().Warnf(msg, data...)
}

func (l *zapGormLogger) Error(ctx context.Context, msg string, data ...interface{}) {
	if l.level < glogger.Error {
		return
	}
	l.zap.Sugar().Errorf(msg, data...)
}

func (l *zapGormLogger) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	if l.level <= glogger.Silent {
		return
	}
	elapsed := time.Since(begin)
	sql, rows := fc()
	fields := []zap.Field{
		zap.String("sql", sql),
		zap.Int64("rows", rows),
		zap.Duration("elapsed", elapsed),
	}
	switch {
	case err != nil && !errors.Is(err, gorm.ErrRecordNotFound):
		l.zap.Error("sql", append(fields, zap.Error(err))...)
	case l.slowThreshold > 0 && elapsed > l.slowThreshold && l.level >= glogger.Warn:
		l.zap.Warn("sql_slow", fields...)
	case l.level >= glogger.Info:
		l.zap.Info("sql", fields...)
	}
}
