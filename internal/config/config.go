package config

import (
	"fmt"
	"os"
	"strconv"
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
	Chat       ChatConfig       `yaml:"chat"`
	OCR        OCRConfig        `yaml:"ocr"`
	Extract    ExtractConfig    `yaml:"extract"`
	Storage    StorageConfig    `yaml:"storage"`
	Log        LogConfig        `yaml:"log"`
	Metrics    MetricsConfig    `yaml:"metrics"`
	RequestLog RequestLogConfig `yaml:"request_log"`
	DingTalk   DingTalkConfig   `yaml:"dingtalk"`
	DBConn     DBConnConfig     `yaml:"dbconn"`
}

// DingTalkConfig 钉钉 Stream 机器人（群内 @ 后预检索 + 流式卡片）。
// 发卡片/群消息时的 robotCode 与 Client ID 相同，无需单独配置。
type DingTalkConfig struct {
	Enabled        bool   `yaml:"enabled"`
	ClientID       string `yaml:"client_id"`
	ClientSecret   string `yaml:"client_secret"`
	CardTemplateID string `yaml:"card_template_id"`
	// ReplyMode chat=现有 CompleteStream（默认）；agent=钉钉预检索后 Agent.Run；
	// web=薄包装，与控制台 Agent 页同一套 Agent.Run（不预注入 hits、不拦语料未命中）。
	ReplyMode string `yaml:"reply_mode"`
	// ReplyTag 非空时追加到每条出站末尾，用于确认是哪套进程在回群（排查串号）。
	ReplyTag string `yaml:"reply_tag"`
}

const (
	DingTalkReplyChat  = "chat"
	DingTalkReplyAgent = "agent"
	DingTalkReplyWeb   = "web"
)

// UseAgent 是否走钉钉预检索 + Agent 工具循环。未配置或非法值视为 chat。
func (c DingTalkConfig) UseAgent() bool {
	return strings.EqualFold(strings.TrimSpace(c.ReplyMode), DingTalkReplyAgent)
}

// UseWebAgent 是否走与控制台 Agent 页一致的薄包装（agent.Run，不预注入 RAGHits）。
func (c DingTalkConfig) UseWebAgent() bool {
	return strings.EqualFold(strings.TrimSpace(c.ReplyMode), DingTalkReplyWeb)
}

type ServerConfig struct {
	Addr string `yaml:"addr"`
	Mode string `yaml:"mode"`
	// CORSOrigins 浏览器 Origin 白名单。空或含 * 时允许所有来源；填写具体 Origin 则仅放行这些源。
	CORSOrigins []string `yaml:"cors_origins"`
}

func (s ServerConfig) corsOriginList() []string {
	out := make([]string, 0, len(s.CORSOrigins))
	for _, o := range s.CORSOrigins {
		o = strings.TrimSpace(o)
		if o == "" {
			continue
		}
		out = append(out, o)
	}
	return out
}

// CORSAllowAll 未配置 cors_origins，或列表中含 * 时允许任意 Origin。
func (s ServerConfig) CORSAllowAll() bool {
	list := s.corsOriginList()
	if len(list) == 0 {
		return true
	}
	for _, o := range list {
		if o == "*" {
			return true
		}
	}
	return false
}

// CORSAllowOrigins 在未放开全部来源时返回白名单。
func (s ServerConfig) CORSAllowOrigins() []string {
	if s.CORSAllowAll() {
		return nil
	}
	return s.corsOriginList()
}

type AuthConfig struct {
	APIKeys      []string `yaml:"api_keys"`
	AdminAPIKeys []string `yaml:"admin_api_keys"`
}

type DatabaseConfig struct {
	// Enabled 为 false 时不连接数据库（最小化部署）；nil 时若 DSN 非空则启用。
	Enabled      *bool  `yaml:"enabled"`
	DSN          string `yaml:"dsn"`
	MaxOpenConns int    `yaml:"max_open_conns"`
	MaxIdleConns int    `yaml:"max_idle_conns"`
	AutoMigrate  bool   `yaml:"auto_migrate"`
}

// IsEnabled 是否启用 PostgreSQL。最小化部署可设 enabled: false 或清空 dsn。
func (c DatabaseConfig) IsEnabled() bool {
	if c.Enabled != nil {
		return *c.Enabled
	}
	return strings.TrimSpace(c.DSN) != ""
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
	// MaxHistory 每次对话请求带入的历史消息条数上限（不含当前 user / system）。<=0 时归一为 10。
	MaxHistory int `yaml:"max_history"`
	// SystemPrompt 对话默认风格（钉钉/控制台）。空则使用代码内置的精简回答提示。
	SystemPrompt string `yaml:"system_prompt"`
	// EnableThinking 通义 Qwen3 等模型的思考链。nil/false 时请求里显式 enable_thinking=false，避免网关默认开启。
	EnableThinking *bool `yaml:"enable_thinking"`
	// Providers 多厂商；key 为 deepseek / qwen / kimi / doubao / openai_compat 等。
	Providers map[string]LLMProviderConfig `yaml:"providers"`
}

// ThinkingEnabled 是否允许模型思考。未配置时为 false。
func (c LLMConfig) ThinkingEnabled() bool {
	return c.EnableThinking != nil && *c.EnableThinking
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
	// MaxDistance 余弦距离上限（越小越相似）。对话 / Agent / RAG 调试 / 钉钉检索超过该值视为未命中；<=0 表示不过滤。
	MaxDistance float64 `yaml:"max_distance"`
}

type AgentConfig struct {
	MaxSteps     int      `yaml:"max_steps"`
	DefaultTools []string `yaml:"default_tools"`
}

// ChatConfig 普通对话（/chat/completions、钉钉）的工具与 RAG 默认设置，与 Agent 共用工具注册表。
type ChatConfig struct {
	// ToolsEnabled nil 视为 true：在 default_tools 之外再并入 chat.tools。
	// 关闭时仍会挂载 agent.default_tools（仅跳过 chat.tools 追加项）。
	ToolsEnabled *bool `yaml:"tools_enabled"`
	// Tools 对话额外工具名；与 agent.default_tools 合并（去重），不能替换默认集。
	Tools []string `yaml:"tools"`
	// MaxToolSteps 一次对话内最多允许的工具轮数；<=0 归一为 4。超出后强制模型直接作答。
	MaxToolSteps int `yaml:"max_tool_steps"`
	// RAGEnabled nil 视为 true：请求未显式传 rag 时，默认做向量检索（无 corpus_id 则搜全部语料）。
	RAGEnabled *bool `yaml:"rag_enabled"`
}

// IsToolsEnabled 普通对话是否携带工具。未配置时默认开启。
func (c ChatConfig) IsToolsEnabled() bool {
	if c.ToolsEnabled != nil {
		return *c.ToolsEnabled
	}
	return true
}

// IsRAGEnabled 普通对话是否默认做 RAG。未配置时默认开启。
func (c ChatConfig) IsRAGEnabled() bool {
	if c.RAGEnabled != nil {
		return *c.RAGEnabled
	}
	return true
}

// DefaultAgentTools 全局默认工具集（含 RAG 检索与业务库查询）。
func DefaultAgentTools() []string {
	return []string{"knowledge_search", "current_time", "calculator", "dbconn"}
}

// DBConnConfig 独立 MySQL 业务库（Agent dbconn 工具）。勿填 ai-agent 自身的 PostgreSQL DSN。
type DBConnConfig struct {
	Enabled        *bool           `yaml:"enabled"`
	Driver         string          `yaml:"driver"` // 仅 mysql
	DSN            string          `yaml:"dsn"`
	MaxOpenConns   int             `yaml:"max_open_conns"`
	MaxIdleConns   int             `yaml:"max_idle_conns"`
	MaxRows        int             `yaml:"max_rows"`
	TimeoutSeconds int             `yaml:"timeout_seconds"`
	SSH            DBConnSSHConfig `yaml:"ssh"`
}

// DBConnSSHConfig 经跳板机 SSH 隧道访问业务 MySQL。必须把 enabled 设为 true 才走隧道。
type DBConnSSHConfig struct {
	Enabled               bool   `yaml:"enabled"`
	Host                  string `yaml:"host"`
	Port                  int    `yaml:"port"`
	User                  string `yaml:"user"`
	PrivateKeyPath        string `yaml:"private_key_path"`
	PrivateKey            string `yaml:"private_key"`
	Passphrase            string `yaml:"passphrase"`
	Password              string `yaml:"password"`
	KnownHostsPath        string `yaml:"known_hosts_path"`
	InsecureIgnoreHostKey bool   `yaml:"insecure_ignore_host_key"`
	KeepaliveSeconds      int    `yaml:"keepalive_seconds"`
	ReconnectWaitSeconds  int    `yaml:"reconnect_wait_seconds"`
}

// IsEnabled 是否走 SSH 隧道。仅看 ssh.enabled 开关。
func (c DBConnSSHConfig) IsEnabled() bool {
	return c.Enabled
}

// IsEnabled 是否启用业务库。未配 enabled 时以 DSN 非空为准。
func (c DBConnConfig) IsEnabled() bool {
	if c.Enabled != nil {
		return *c.Enabled && strings.TrimSpace(c.DSN) != ""
	}
	return strings.TrimSpace(c.DSN) != ""
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
	//cfg.applyEnv()
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
			BaseURL:        "https://dashscope.aliyuncs.com/compatible-mode/v1",
			DefaultModel:   "qwen-plus",
			TimeoutSeconds: 120,
			MaxHistory:     10,
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
			MaxDistance:  0.55,
		},
		Agent: AgentConfig{
			MaxSteps:     8,
			DefaultTools: DefaultAgentTools(),
		},
		Chat: ChatConfig{
			MaxToolSteps: 4,
		},
		DBConn: DBConnConfig{
			Driver:         "mysql",
			MaxOpenConns:   5,
			MaxIdleConns:   2,
			MaxRows:        50,
			TimeoutSeconds: 15,
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
	if v := os.Getenv("DATABASE_ENABLED"); v != "" {
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "0", "false", "no", "off":
			f := false
			c.Database.Enabled = &f
		case "1", "true", "yes", "on":
			t := true
			c.Database.Enabled = &t
		}
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

	if v := os.Getenv("LLM_ENABLE_THINKING"); v != "" {
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "1", "true", "yes", "on":
			on := true
			c.LLM.EnableThinking = &on
		case "0", "false", "no", "off":
			off := false
			c.LLM.EnableThinking = &off
		}
	}

	if v := os.Getenv("CHAT_TOOLS_ENABLED"); v != "" {
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "1", "true", "yes", "on":
			on := true
			c.Chat.ToolsEnabled = &on
		case "0", "false", "no", "off":
			off := false
			c.Chat.ToolsEnabled = &off
		}
	}
	if v := os.Getenv("CHAT_RAG_ENABLED"); v != "" {
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "1", "true", "yes", "on":
			on := true
			c.Chat.RAGEnabled = &on
		case "0", "false", "no", "off":
			off := false
			c.Chat.RAGEnabled = &off
		}
	}

	if v := os.Getenv("BIZ_DATABASE_URL"); v != "" {
		c.DBConn.DSN = v
	}
	if v := os.Getenv("BIZ_SSH_ENABLED"); v != "" {
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "1", "true", "yes", "on":
			c.DBConn.SSH.Enabled = true
		case "0", "false", "no", "off":
			c.DBConn.SSH.Enabled = false
		}
	}
	if v := os.Getenv("BIZ_SSH_HOST"); v != "" {
		c.DBConn.SSH.Host = v
	}
	if v := os.Getenv("BIZ_SSH_PORT"); v != "" {
		if p, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && p > 0 {
			c.DBConn.SSH.Port = p
		}
	}
	if v := os.Getenv("BIZ_SSH_USER"); v != "" {
		c.DBConn.SSH.User = v
	}
	if v := os.Getenv("BIZ_SSH_PASSWORD"); v != "" {
		c.DBConn.SSH.Password = v
	}
	if v := os.Getenv("BIZ_SSH_KNOWN_HOSTS"); v != "" {
		c.DBConn.SSH.KnownHostsPath = v
	}
	if v := os.Getenv("DINGTALK_CLIENT_ID"); v != "" {
		c.DingTalk.ClientID = v
	}
	if v := os.Getenv("DINGTALK_CLIENT_SECRET"); v != "" {
		c.DingTalk.ClientSecret = v
	}
	if v := os.Getenv("DINGTALK_CARD_TEMPLATE_ID"); v != "" {
		c.DingTalk.CardTemplateID = v
	}
	if v := os.Getenv("DINGTALK_REPLY_TAG"); v != "" {
		c.DingTalk.ReplyTag = v
	}
	if v := os.Getenv("DINGTALK_ENABLED"); v != "" {
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "1", "true", "yes", "on":
			c.DingTalk.Enabled = true
		case "0", "false", "no", "off":
			c.DingTalk.Enabled = false
		}
	}
	if v := os.Getenv("DINGTALK_REPLY_MODE"); v != "" {
		c.DingTalk.ReplyMode = v
	}
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
	if c.LLM.MaxHistory <= 0 {
		c.LLM.MaxHistory = 10
	}
	if c.Embed.Dimensions <= 0 {
		c.Embed.Dimensions = 768
	}
	if c.Agent.MaxSteps <= 0 {
		c.Agent.MaxSteps = 8
	}
	if c.Chat.MaxToolSteps <= 0 {
		c.Chat.MaxToolSteps = 4
	}
	if len(c.Agent.DefaultTools) == 0 {
		c.Agent.DefaultTools = DefaultAgentTools()
	}
	if len(c.Chat.Tools) == 0 {
		c.Chat.Tools = append([]string(nil), c.Agent.DefaultTools...)
	}
	c.DBConn.Driver = strings.ToLower(strings.TrimSpace(c.DBConn.Driver))
	if c.DBConn.Driver == "" {
		c.DBConn.Driver = "mysql"
	}
	if c.DBConn.MaxOpenConns <= 0 {
		c.DBConn.MaxOpenConns = 5
	}
	if c.DBConn.MaxIdleConns <= 0 {
		c.DBConn.MaxIdleConns = 2
	}
	if c.DBConn.MaxRows <= 0 {
		c.DBConn.MaxRows = 50
	}
	if c.DBConn.TimeoutSeconds <= 0 {
		c.DBConn.TimeoutSeconds = 15
	}
	c.DBConn.SSH.Host = strings.TrimSpace(c.DBConn.SSH.Host)
	c.DBConn.SSH.User = strings.TrimSpace(c.DBConn.SSH.User)
	c.DBConn.SSH.PrivateKeyPath = strings.TrimSpace(c.DBConn.SSH.PrivateKeyPath)
	c.DBConn.SSH.KnownHostsPath = strings.TrimSpace(c.DBConn.SSH.KnownHostsPath)
	if c.DBConn.SSH.Port <= 0 {
		c.DBConn.SSH.Port = 22
	}
	if c.DBConn.SSH.KeepaliveSeconds <= 0 {
		c.DBConn.SSH.KeepaliveSeconds = 30
	}
	if c.DBConn.SSH.ReconnectWaitSeconds <= 0 {
		c.DBConn.SSH.ReconnectWaitSeconds = 2
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
	c.DingTalk.ClientID = strings.TrimSpace(c.DingTalk.ClientID)
	c.DingTalk.ClientSecret = strings.TrimSpace(c.DingTalk.ClientSecret)
	c.DingTalk.CardTemplateID = strings.TrimSpace(c.DingTalk.CardTemplateID)
	c.DingTalk.ReplyTag = strings.TrimSpace(c.DingTalk.ReplyTag)
	switch strings.ToLower(strings.TrimSpace(c.DingTalk.ReplyMode)) {
	case DingTalkReplyAgent:
		c.DingTalk.ReplyMode = DingTalkReplyAgent
	case DingTalkReplyWeb:
		c.DingTalk.ReplyMode = DingTalkReplyWeb
	default:
		c.DingTalk.ReplyMode = DingTalkReplyChat
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
		defName = "qwen"
	}
	c.LLM.DefaultProvider = defName

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
