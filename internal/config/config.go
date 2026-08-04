package config

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server     ServerConfig     `yaml:"server"`
	Auth       AuthConfig       `yaml:"auth"`
	Database   DatabaseConfig   `yaml:"database"`
	LLM        LLMConfig        `yaml:"llm"`
	Embed      EmbedConfig      `yaml:"embed"`
	RAG        RAGConfig        `yaml:"rag"`
	Agent      AgentConfig      `yaml:"agent"`
	Log        LogConfig        `yaml:"log"`
	Metrics    MetricsConfig    `yaml:"metrics"`
	RequestLog RequestLogConfig `yaml:"request_log"`
}

type ServerConfig struct {
	Addr string `yaml:"addr"`
	Mode string `yaml:"mode"`
}

type AuthConfig struct {
	APIKeys      []string `yaml:"api_keys"`
	AdminAPIKeys []string `yaml:"admin_api_keys"`
}

type DatabaseConfig struct {
	DSN          string `yaml:"dsn"`
	MaxOpenConns int    `yaml:"max_open_conns"`
	MaxIdleConns int    `yaml:"max_idle_conns"`
	AutoMigrate  bool   `yaml:"auto_migrate"`
}

type LLMConfig struct {
	Provider       string `yaml:"provider"`
	BaseURL        string `yaml:"base_url"`
	APIKey         string `yaml:"api_key"`
	DefaultModel   string `yaml:"default_model"`
	TimeoutSeconds int    `yaml:"timeout_seconds"`
}

type EmbedConfig struct {
	BaseURL    string `yaml:"base_url"`
	APIKey     string `yaml:"api_key"`
	Model      string `yaml:"model"`
	Dimensions int    `yaml:"dimensions"`
}

type RAGConfig struct {
	TopK         int    `yaml:"top_k"`
	ChunkSize    int    `yaml:"chunk_size"`
	ChunkOverlap int    `yaml:"chunk_overlap"`
	VectorIndex  string `yaml:"vector_index"`
}

type AgentConfig struct {
	MaxSteps     int      `yaml:"max_steps"`
	DefaultTools []string `yaml:"default_tools"`
}

type LogConfig struct {
	Level          string `yaml:"level"`
	Encoding       string `yaml:"encoding"`
	BodyPreviewMax int    `yaml:"body_preview_max"`
	Dir            string `yaml:"dir"`          // default: logs
	Filename       string `yaml:"filename"`     // default: ai-agent
	MaxSizeMB      int    `yaml:"max_size_mb"`  // default: 100
	MaxBackups     int    `yaml:"max_backups"`  // default: 30
	MaxAgeDays     int    `yaml:"max_age_days"` // default: 30; 0 = keep forever
	AlsoStdout     *bool  `yaml:"also_stdout"`  // default: true
}

type MetricsConfig struct {
	Enabled bool   `yaml:"enabled"`
	Path    string `yaml:"path"`
	Protect bool   `yaml:"protect"`
}

type RequestLogConfig struct {
	Enabled     bool `yaml:"enabled"`
	PersistBody bool `yaml:"persist_body"`
}

func Load(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	cfg := defaultConfig()
	if err := yaml.Unmarshal(raw, cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	cfg.applyEnv()
	cfg.normalize()
	return cfg, nil
}

func defaultConfig() *Config {
	return &Config{
		Server: ServerConfig{Addr: ":18090", Mode: "debug"},
		Database: DatabaseConfig{
			DSN:          "postgres://ai_agent:password@127.0.0.1:5432/ai_agent?sslmode=disable",
			MaxOpenConns: 20,
			MaxIdleConns: 5,
			AutoMigrate:  true,
		},
		LLM: LLMConfig{
			Provider:       "deepseek",
			BaseURL:        "https://api.deepseek.com/v1",
			DefaultModel:   "deepseek-v4-flash",
			TimeoutSeconds: 120,
		},
		Embed: EmbedConfig{
			BaseURL:    "http://localhost:11434/v1",
			Model:      "nomic-embed-text",
			Dimensions: 768,
		},
		RAG: RAGConfig{
			TopK:         5,
			ChunkSize:    800,
			ChunkOverlap: 120,
			VectorIndex:  "hnsw",
		},
		Agent: AgentConfig{
			MaxSteps:     8,
			DefaultTools: []string{"knowledge_search", "current_time"},
		},
		Log: LogConfig{
			Level:          "info",
			Encoding:       "json",
			BodyPreviewMax: 4096,
			Dir:            "logs",
			Filename:       "ai-agent",
			MaxSizeMB:      100,
			MaxBackups:     30,
			MaxAgeDays:     30,
		},
		Metrics: MetricsConfig{
			Enabled: true,
			Path:    "/metrics",
		},
		RequestLog: RequestLogConfig{
			Enabled:     true,
			PersistBody: true,
		},
	}
}

func (c *Config) applyEnv() {
	if v := os.Getenv("DEEPSEEK_API_KEY"); v != "" {
		c.LLM.APIKey = v
	}
	if v := os.Getenv("DATABASE_URL"); v != "" {
		c.Database.DSN = v
	} else if v := os.Getenv("PG_DSN"); v != "" {
		c.Database.DSN = v
	}
	if v := os.Getenv("API_KEYS"); v != "" {
		parts := strings.Split(v, ",")
		keys := make([]string, 0, len(parts))
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p != "" {
				keys = append(keys, p)
			}
		}
		if len(keys) > 0 {
			c.Auth.APIKeys = keys
		}
	}
}

func (c *Config) normalize() {
	c.LLM.BaseURL = strings.TrimRight(c.LLM.BaseURL, "/")
	c.Embed.BaseURL = strings.TrimRight(c.Embed.BaseURL, "/")
	if c.Server.Addr == "" {
		c.Server.Addr = ":18090"
	}
	if c.Metrics.Path == "" {
		c.Metrics.Path = "/metrics"
	}
	if c.Log.BodyPreviewMax <= 0 {
		c.Log.BodyPreviewMax = 4096
	}
	if strings.TrimSpace(c.Log.Dir) == "" {
		c.Log.Dir = "logs"
	}
	if strings.TrimSpace(c.Log.Filename) == "" {
		c.Log.Filename = "ai-agent"
	}
	if c.Log.MaxSizeMB <= 0 {
		c.Log.MaxSizeMB = 100
	}
	if c.Log.MaxBackups <= 0 {
		c.Log.MaxBackups = 30
	}
	if c.Log.MaxAgeDays < 0 {
		c.Log.MaxAgeDays = 30
	}
	if c.Log.AlsoStdout == nil {
		// debug: stdout + file; release/production: file only
		v := !strings.EqualFold(c.Server.Mode, "release")
		c.Log.AlsoStdout = &v
	}
	if c.LLM.TimeoutSeconds <= 0 {
		c.LLM.TimeoutSeconds = 120
	}
	if c.Embed.Dimensions <= 0 {
		c.Embed.Dimensions = 768
	}
	if c.Agent.MaxSteps <= 0 {
		c.Agent.MaxSteps = 8
	}
}

func (c *Config) IsAPIKey(key string) bool {
	for _, k := range c.Auth.APIKeys {
		if k == key {
			return true
		}
	}
	return c.IsAdminAPIKey(key)
}

func (c *Config) IsAdminAPIKey(key string) bool {
	for _, k := range c.Auth.AdminAPIKeys {
		if k == key {
			return true
		}
	}
	return false
}

func (c *Config) APIKeyID(key string) string {
	if key == "" {
		return ""
	}
	if len(key) <= 8 {
		return "key:" + key
	}
	return "key:" + key[:4] + "..." + key[len(key)-4:]
}
