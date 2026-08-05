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
	OCR        OCRConfig        `yaml:"ocr"`
	Extract    ExtractConfig    `yaml:"extract"`
	Storage    StorageConfig    `yaml:"storage"`
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

// LLMProviderConfig 单个 OpenAI 兼容上游。
type LLMProviderConfig struct {
	BaseURL      string `yaml:"base_url"`
	APIKey       string `yaml:"api_key"`
	DefaultModel string `yaml:"default_model"`
	Enabled      *bool  `yaml:"enabled"` // nil 视为 true
}

type LLMConfig struct {
	// DefaultProvider 请求未传 provider 时使用；兼容旧字段 provider。
	DefaultProvider string `yaml:"default_provider"`
	Provider        string `yaml:"provider"` // 兼容旧配置，normalize 时写入 DefaultProvider
	BaseURL         string `yaml:"base_url"` // 兼容：无 providers 时当作默认厂商
	APIKey          string `yaml:"api_key"`
	DefaultModel    string `yaml:"default_model"`
	TimeoutSeconds  int    `yaml:"timeout_seconds"`
	// Providers 多厂商；key 为 deepseek / qwen / kimi / doubao / openai_compat 等。
	Providers map[string]LLMProviderConfig `yaml:"providers"`
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

type OCRConfig struct {
	Enabled           bool   `yaml:"enabled"`
	TesseractPath     string `yaml:"tesseract_path"`
	Languages         string `yaml:"languages"`
	PDFToPPMPath      string `yaml:"pdftoppm_path"`
	PDFToTextPath     string `yaml:"pdftotext_path"`
	MinPDFTextLen     int    `yaml:"min_pdf_text_len"`
	TimeoutSeconds    int    `yaml:"timeout_seconds"`
	DPI               int    `yaml:"dpi"`                 // pdftoppm -r，默认 200
	PSM               int    `yaml:"psm"`                 // tesseract --psm，默认 3
	OEM               int    `yaml:"oem"`                 // tesseract --oem，默认 3
	PDFToPPMGray      bool   `yaml:"pdftoppm_gray"`       // pdftoppm -gray，默认 false
	CollapseCJKSpaces *bool  `yaml:"collapse_cjk_spaces"` // 去汉字间空格，默认 true
}

// StorageConfig 本地附件与抽取文本落盘。
type StorageConfig struct {
	AttachmentsDir string `yaml:"attachments_dir"` // 默认 attachments；按 YYYY/MM/DD 分子目录
}

// ExtractConfig 文档抽取后端：local 本机 OCR；kimi/qwen 云端 Files。
type ExtractConfig struct {
	Backend        string `yaml:"backend"`        // local | kimi | qwen
	FallbackLocal  bool   `yaml:"fallback_local"` // 云端失败时回退本机
	TimeoutSeconds int    `yaml:"timeout_seconds"`
}

type LogConfig struct {
	Level          string `yaml:"level"`
	Encoding       string `yaml:"encoding"`
	BodyPreviewMax int    `yaml:"body_preview_max"`
	Dir            string `yaml:"dir"`
	Filename       string `yaml:"filename"`
	MaxSizeMB      int    `yaml:"max_size_mb"`
	MaxBackups     int    `yaml:"max_backups"`
	MaxAgeDays     int    `yaml:"max_age_days"`
	AlsoStdout     *bool  `yaml:"also_stdout"`
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
			DefaultProvider: "deepseek",
			Provider:        "deepseek",
			BaseURL:         "https://api.deepseek.com/v1",
			DefaultModel:    "deepseek-v4-flash",
			TimeoutSeconds:  120,
			Providers: map[string]LLMProviderConfig{
				"deepseek": {
					BaseURL:      "https://api.deepseek.com/v1",
					DefaultModel: "deepseek-v4-flash",
				},
				"qwen": {
					BaseURL:      "https://dashscope.aliyuncs.com/compatible-mode/v1",
					DefaultModel: "qwen-plus",
				},
				"kimi": {
					BaseURL:      "https://api.moonshot.cn/v1",
					DefaultModel: "moonshot-v1-8k",
				},
				"doubao": {
					BaseURL:      "https://ark.cn-beijing.volces.com/api/v3",
					DefaultModel: "",
				},
			},
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
			DefaultTools: []string{"knowledge_search", "current_time", "calculator"},
		},
		OCR: OCRConfig{
			Enabled:           true,
			TesseractPath:     "tesseract",
			Languages:         "chi_sim+eng",
			PDFToPPMPath:      "pdftoppm",
			PDFToTextPath:     "pdftotext",
			MinPDFTextLen:     40,
			TimeoutSeconds:    180,
			DPI:               200,
			PSM:               3,
			OEM:               3,
			PDFToPPMGray:      false,
			CollapseCJKSpaces: boolPtr(true),
		},
		Storage: StorageConfig{
			AttachmentsDir: "attachments",
		},
		Extract: ExtractConfig{
			Backend:        "local",
			FallbackLocal:  false,
			TimeoutSeconds: 180,
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

	setProviderKey := func(name, envKey string) {
		v := os.Getenv(envKey)
		if v == "" {
			return
		}
		if c.LLM.Providers == nil {
			c.LLM.Providers = map[string]LLMProviderConfig{}
		}
		p := c.LLM.Providers[name]
		p.APIKey = v
		c.LLM.Providers[name] = p
		if name == "deepseek" || c.LLM.APIKey == "" {
			c.LLM.APIKey = v
		}
	}
	setProviderKey("deepseek", "DEEPSEEK_API_KEY")
	setProviderKey("qwen", "DASHSCOPE_API_KEY")
	setProviderKey("qwen", "QWEN_API_KEY")
	setProviderKey("kimi", "MOONSHOT_API_KEY")
	setProviderKey("kimi", "KIMI_API_KEY")
	setProviderKey("doubao", "ARK_API_KEY")
	setProviderKey("doubao", "DOUBAO_API_KEY")
}

func providerPreset(name string) (baseURL, model string) {
	switch strings.ToLower(name) {
	case "deepseek":
		return "https://api.deepseek.com/v1", "deepseek-v4-flash"
	case "qwen", "dashscope":
		return "https://dashscope.aliyuncs.com/compatible-mode/v1", "qwen-plus"
	case "kimi", "moonshot":
		return "https://api.moonshot.cn/v1", "moonshot-v1-8k"
	case "doubao", "ark", "volcengine":
		return "https://ark.cn-beijing.volces.com/api/v3", ""
	default:
		return "", ""
	}
}

func (c *Config) normalize() {
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
		v := !loggerIsProdMode(c.Server.Mode)
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
	if strings.TrimSpace(c.OCR.TesseractPath) == "" {
		c.OCR.TesseractPath = "tesseract"
	}
	if strings.TrimSpace(c.OCR.Languages) == "" {
		c.OCR.Languages = "chi_sim+eng"
	}
	if strings.TrimSpace(c.OCR.PDFToPPMPath) == "" {
		c.OCR.PDFToPPMPath = "pdftoppm"
	}
	if strings.TrimSpace(c.OCR.PDFToTextPath) == "" {
		c.OCR.PDFToTextPath = "pdftotext"
	}
	if c.OCR.MinPDFTextLen <= 0 {
		c.OCR.MinPDFTextLen = 40
	}
	if c.OCR.TimeoutSeconds <= 0 {
		c.OCR.TimeoutSeconds = 180
	}
	if c.OCR.DPI <= 0 {
		c.OCR.DPI = 200
	}
	if c.OCR.PSM <= 0 {
		c.OCR.PSM = 3
	}
	if c.OCR.OEM <= 0 {
		c.OCR.OEM = 3
	}
	if c.OCR.CollapseCJKSpaces == nil {
		v := true
		c.OCR.CollapseCJKSpaces = &v
	}
	if strings.TrimSpace(c.Storage.AttachmentsDir) == "" {
		c.Storage.AttachmentsDir = "attachments"
	}
	switch strings.ToLower(strings.TrimSpace(c.Extract.Backend)) {
	case "moonshot":
		c.Extract.Backend = "kimi"
	case "dashscope":
		c.Extract.Backend = "qwen"
	case "kimi", "qwen", "local":
		c.Extract.Backend = strings.ToLower(strings.TrimSpace(c.Extract.Backend))
	default:
		c.Extract.Backend = "local"
	}
	if c.Extract.TimeoutSeconds <= 0 {
		c.Extract.TimeoutSeconds = 180
	}

	c.normalizeLLMProviders()
}

func (c *Config) normalizeLLMProviders() {
	if c.LLM.Providers == nil {
		c.LLM.Providers = map[string]LLMProviderConfig{}
	}

	defName := strings.TrimSpace(c.LLM.DefaultProvider)
	if defName == "" {
		defName = strings.TrimSpace(c.LLM.Provider)
	}
	if defName == "" {
		defName = "deepseek"
	}
	c.LLM.DefaultProvider = defName
	c.LLM.Provider = defName

	// 兼容旧扁平 llm.api_key / base_url / default_model → 合并进默认厂商
	p := c.LLM.Providers[defName]
	if strings.TrimSpace(c.LLM.BaseURL) != "" {
		p.BaseURL = c.LLM.BaseURL
	}
	if strings.TrimSpace(c.LLM.APIKey) != "" {
		p.APIKey = c.LLM.APIKey
	}
	if strings.TrimSpace(c.LLM.DefaultModel) != "" {
		p.DefaultModel = c.LLM.DefaultModel
	}
	c.LLM.Providers[defName] = p

	// 确保内置厂商条目存在（可无 key）
	for _, name := range []string{"deepseek", "qwen", "kimi", "doubao"} {
		if _, ok := c.LLM.Providers[name]; ok {
			continue
		}
		base, model := providerPreset(name)
		c.LLM.Providers[name] = LLMProviderConfig{BaseURL: base, DefaultModel: model}
	}

	for name, p := range c.LLM.Providers {
		base, model := providerPreset(name)
		if strings.TrimSpace(p.BaseURL) == "" {
			p.BaseURL = base
		}
		p.BaseURL = strings.TrimRight(p.BaseURL, "/")
		if strings.TrimSpace(p.DefaultModel) == "" {
			p.DefaultModel = model
		}
		c.LLM.Providers[name] = p
	}

	// 回写扁平字段，便于旧代码读 DefaultModel / APIKey
	if p, ok := c.LLM.Providers[defName]; ok {
		c.LLM.BaseURL = p.BaseURL
		if p.APIKey != "" {
			c.LLM.APIKey = p.APIKey
		}
		if p.DefaultModel != "" {
			c.LLM.DefaultModel = p.DefaultModel
		}
	}
}

// ProviderEnabled 是否启用该厂商（未配置 enabled 时默认启用）。
func (p LLMProviderConfig) ProviderEnabled() bool {
	if p.Enabled == nil {
		return true
	}
	return *p.Enabled
}

// ResolveLLM 按 provider 名解析配置；空则用默认厂商。
func (c *Config) ResolveLLM(provider string) (name string, cfg LLMProviderConfig, err error) {
	name = strings.TrimSpace(provider)
	if name == "" {
		name = c.LLM.DefaultProvider
	}
	name = strings.ToLower(name)
	// 别名
	switch name {
	case "dashscope":
		name = "qwen"
	case "moonshot":
		name = "kimi"
	case "ark", "volcengine":
		name = "doubao"
	}
	cfg, ok := c.LLM.Providers[name]
	if !ok {
		return "", LLMProviderConfig{}, fmt.Errorf("未知模型厂商: %s", name)
	}
	if !cfg.ProviderEnabled() {
		return "", LLMProviderConfig{}, fmt.Errorf("模型厂商未启用: %s", name)
	}
	if strings.TrimSpace(cfg.APIKey) == "" {
		return "", LLMProviderConfig{}, fmt.Errorf("模型厂商 %s 未配置 api_key", name)
	}
	if strings.TrimSpace(cfg.BaseURL) == "" {
		return "", LLMProviderConfig{}, fmt.Errorf("模型厂商 %s 未配置 base_url", name)
	}
	return name, cfg, nil
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

func boolPtr(v bool) *bool { return &v }

func loggerIsProdMode(mode string) bool {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "release", "prod", "pro", "production":
		return true
	default:
		return false
	}
}
