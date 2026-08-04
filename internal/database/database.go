package database

import (
	"fmt"

	"github.com/webapp/go-app/ai-agent/internal/config"
	"github.com/webapp/go-app/ai-agent/internal/model"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func Open(cfg config.DatabaseConfig) (*gorm.DB, error) {
	db, err := gorm.Open(postgres.Open(cfg.DSN), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
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
	); err != nil {
		return fmt.Errorf("automigrate: %w", err)
	}
	// Ensure embedding column exists as vector type (GORM cannot manage it well).
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
