package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadAndEnvOverride(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := `
server:
  addr: ":18090"
auth:
  api_keys: ["k1"]
database:
  dsn: "postgres://x"
llm:
  api_key: ""
  default_model: "deepseek-v4-flash"
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DEEPSEEK_API_KEY", "secret-key")
	t.Setenv("API_KEYS", "a,b")
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.LLM.APIKey != "secret-key" {
		t.Fatalf("api key override failed: %q", cfg.LLM.APIKey)
	}
	if cfg.LLM.Providers["deepseek"].APIKey != "secret-key" {
		t.Fatalf("provider key override failed: %#v", cfg.LLM.Providers["deepseek"])
	}
	if len(cfg.Auth.APIKeys) != 2 {
		t.Fatalf("api keys override failed: %#v", cfg.Auth.APIKeys)
	}
	if cfg.LLM.DefaultProvider != "qwen" {
		t.Fatalf("default provider: %q", cfg.LLM.DefaultProvider)
	}
	if cfg.RAG.MaxDistance != 0.55 {
		t.Fatalf("default rag.max_distance: %v", cfg.RAG.MaxDistance)
	}
	if cfg.LLM.MaxHistory != 10 {
		t.Fatalf("default llm.max_history: %d", cfg.LLM.MaxHistory)
	}
	if cfg.LLM.ThinkingEnabled() {
		t.Fatal("thinking should default off")
	}
}

func TestLLMEnableThinkingEnv(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("database:\n  dsn: postgres://x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LLM_ENABLE_THINKING", "true")
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.LLM.ThinkingEnabled() {
		t.Fatal("env should turn thinking on")
	}
}

func TestMaxHistoryFromFileAndNormalize(t *testing.T) {
	dir := t.TempDir()

	write := func(name, body string) string {
		t.Helper()
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}

	cfg, err := Load(write("set.yaml", "database:\n  dsn: postgres://x\nllm:\n  max_history: 20\n"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.LLM.MaxHistory != 20 {
		t.Fatalf("want 20, got %d", cfg.LLM.MaxHistory)
	}

	cfg, err = Load(write("zero.yaml", "database:\n  dsn: postgres://x\nllm:\n  max_history: 0\n"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.LLM.MaxHistory != 10 {
		t.Fatalf("zero should normalize to 10, got %d", cfg.LLM.MaxHistory)
	}
}

func TestDefaultToolsAndRAGEnabled(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bare.yaml")
	if err := os.WriteFile(path, []byte("database:\n  dsn: postgres://x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	want := DefaultAgentTools()
	if len(cfg.Agent.DefaultTools) != len(want) {
		t.Fatalf("default tools: %#v", cfg.Agent.DefaultTools)
	}
	for i, n := range want {
		if cfg.Agent.DefaultTools[i] != n || cfg.Chat.Tools[i] != n {
			t.Fatalf("tools[%d]=%q chat=%q want %q", i, cfg.Agent.DefaultTools[i], cfg.Chat.Tools[i], n)
		}
	}
	if !cfg.Chat.IsRAGEnabled() {
		t.Fatal("rag should default on")
	}
	if !cfg.Chat.IsToolsEnabled() {
		t.Fatal("tools should default on")
	}
	off := false
	cfg.Chat.RAGEnabled = &off
	if cfg.Chat.IsRAGEnabled() {
		t.Fatal("explicit off")
	}
}

func TestLegacyLLMProviderField(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "legacy.yaml")
	content := "database:\n  dsn: postgres://x\nllm:\n  provider: kimi\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.LLM.DefaultProvider != "kimi" {
		t.Fatalf("legacy provider should map to default_provider, got %q", cfg.LLM.DefaultProvider)
	}
}

func TestResolveLLM(t *testing.T) {
	cfg := defaultConfig()
	cfg.LLM.Providers["qwen"] = LLMProviderConfig{
		BaseURL:      "https://dashscope.aliyuncs.com/compatible-mode/v1",
		APIKey:       "qk",
		DefaultModel: "qwen-plus",
	}
	cfg.normalize()
	name, pc, err := cfg.ResolveLLM("dashscope")
	if err != nil || name != "qwen" || pc.APIKey != "qk" {
		t.Fatalf("alias resolve: name=%s err=%v", name, err)
	}
	_, _, err = cfg.ResolveLLM("kimi")
	if err == nil {
		t.Fatal("expected missing key error for kimi")
	}
}

func TestCORSAllowOrigins(t *testing.T) {
	empty := ServerConfig{}
	if !empty.CORSAllowAll() {
		t.Fatal("empty cors_origins should allow all origins")
	}
	star := ServerConfig{CORSOrigins: []string{"*"}}
	if !star.CORSAllowAll() {
		t.Fatal("* should allow all origins")
	}
	cfg := ServerConfig{CORSOrigins: []string{"  ", "http://example.test:3000"}}
	if cfg.CORSAllowAll() {
		t.Fatal("explicit origin should not allow all")
	}
	got := cfg.CORSAllowOrigins()
	if len(got) != 1 || got[0] != "http://example.test:3000" {
		t.Fatalf("whitelist, got %#v", got)
	}
}

func TestDatabaseIsEnabled(t *testing.T) {
	f, ttrue := false, true
	if (DatabaseConfig{Enabled: &f}).IsEnabled() {
		t.Fatal("enabled:false should disable")
	}
	if !(DatabaseConfig{Enabled: &ttrue}).IsEnabled() {
		t.Fatal("enabled:true should enable")
	}
	if !(DatabaseConfig{DSN: "postgres://x"}).IsEnabled() {
		t.Fatal("non-empty dsn should enable when enabled unset")
	}
	if (DatabaseConfig{}).IsEnabled() {
		t.Fatal("empty dsn should disable when enabled unset")
	}
}

func TestAlsoStdoutDefaultsByMode(t *testing.T) {
	dir := t.TempDir()

	writeAndLoad := func(mode string) *Config {
		t.Helper()
		path := filepath.Join(dir, mode+".yaml")
		content := "server:\n  mode: " + mode + "\ndatabase:\n  dsn: postgres://x\n"
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		cfg, err := Load(path)
		if err != nil {
			t.Fatal(err)
		}
		return cfg
	}

	debugCfg := writeAndLoad("debug")
	if debugCfg.Log.AlsoStdout == nil || !*debugCfg.Log.AlsoStdout {
		t.Fatalf("debug should also_stdout=true, got %#v", debugCfg.Log.AlsoStdout)
	}
	releaseCfg := writeAndLoad("release")
	if releaseCfg.Log.AlsoStdout == nil || *releaseCfg.Log.AlsoStdout {
		t.Fatalf("release should also_stdout=false, got %#v", releaseCfg.Log.AlsoStdout)
	}
	proCfg := writeAndLoad("pro")
	if proCfg.Log.AlsoStdout == nil || *proCfg.Log.AlsoStdout {
		t.Fatalf("pro should also_stdout=false, got %#v", proCfg.Log.AlsoStdout)
	}
}

func TestDBConnIsEnabledAndEnv(t *testing.T) {
	f, ttrue := false, true
	if (DBConnConfig{Enabled: &f, DSN: "user:p@tcp(127.0.0.1:3306)/biz"}).IsEnabled() {
		t.Fatal("enabled:false should disable")
	}
	if (DBConnConfig{Enabled: &ttrue}).IsEnabled() {
		t.Fatal("enabled:true with empty dsn should disable")
	}
	if !(DBConnConfig{DSN: "user:p@tcp(127.0.0.1:3306)/biz"}).IsEnabled() {
		t.Fatal("non-empty dsn should enable when enabled unset")
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "dbconn.yaml")
	if err := os.WriteFile(path, []byte("database:\n  dsn: postgres://x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BIZ_DATABASE_URL", "user:pass@tcp(127.0.0.1:3306)/biz")
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DBConn.DSN != "user:pass@tcp(127.0.0.1:3306)/biz" {
		t.Fatalf("BIZ_DATABASE_URL: %q", cfg.DBConn.DSN)
	}
	if cfg.DBConn.Driver != "mysql" || cfg.DBConn.MaxRows != 50 || cfg.DBConn.TimeoutSeconds != 15 {
		t.Fatalf("dbconn defaults: %#v", cfg.DBConn)
	}
	if !cfg.DBConn.IsEnabled() {
		t.Fatal("env dsn should enable dbconn")
	}
}

func TestDBConnSSHEnvAndDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ssh.yaml")
	if err := os.WriteFile(path, []byte("database:\n  dsn: postgres://x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BIZ_DATABASE_URL", "user:pass@tcp(127.0.0.1:3306)/biz")
	t.Setenv("BIZ_SSH_ENABLED", "true")
	t.Setenv("BIZ_SSH_HOST", "jump.example.com")
	t.Setenv("BIZ_SSH_PORT", "2222")
	t.Setenv("BIZ_SSH_USER", "ops")
	t.Setenv("BIZ_SSH_PASSWORD", "s3cret")
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.DBConn.SSH.IsEnabled() {
		t.Fatal("BIZ_SSH_ENABLED should turn on ssh")
	}
	if cfg.DBConn.SSH.Host != "jump.example.com" || cfg.DBConn.SSH.Port != 2222 || cfg.DBConn.SSH.User != "ops" {
		t.Fatalf("ssh env: %#v", cfg.DBConn.SSH)
	}
	if cfg.DBConn.SSH.Password != "s3cret" {
		t.Fatalf("password: %q", cfg.DBConn.SSH.Password)
	}
	if cfg.DBConn.SSH.KeepaliveSeconds != 30 || cfg.DBConn.SSH.ReconnectWaitSeconds != 2 {
		t.Fatalf("ssh keepalive defaults: %#v", cfg.DBConn.SSH)
	}
	if (DBConnSSHConfig{Enabled: false, Host: "x"}).IsEnabled() {
		t.Fatal("ssh enabled:false should disable even if host is set")
	}
	if !(DBConnSSHConfig{Enabled: true}).IsEnabled() {
		t.Fatal("ssh enabled:true should be on")
	}
}

func TestDingTalkEnvOverride(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dt.yaml")
	if err := os.WriteFile(path, []byte("database:\n  dsn: postgres://x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DINGTALK_ENABLED", "true")
	t.Setenv("DINGTALK_CLIENT_ID", "cid")
	t.Setenv("DINGTALK_CLIENT_SECRET", "csecret")
	t.Setenv("DINGTALK_CARD_TEMPLATE_ID", "tpl")
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.DingTalk.Enabled || cfg.DingTalk.ClientID != "cid" || cfg.DingTalk.CardTemplateID != "tpl" {
		t.Fatalf("dingtalk env: %#v", cfg.DingTalk)
	}
	if cfg.DingTalk.ReplyMode != DingTalkReplyChat {
		t.Fatalf("default reply_mode: %q", cfg.DingTalk.ReplyMode)
	}
}

func TestDingTalkReplyModeNormalize(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) string {
		t.Helper()
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}
	cfg, err := Load(write("bare.yaml", "database:\n  dsn: postgres://x\n"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DingTalk.ReplyMode != DingTalkReplyChat || cfg.DingTalk.UseAgent() {
		t.Fatalf("empty should be chat: %#v", cfg.DingTalk)
	}
	cfg, err = Load(write("bad.yaml", "database:\n  dsn: postgres://x\ndingtalk:\n  reply_mode: weird\n"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DingTalk.ReplyMode != DingTalkReplyChat {
		t.Fatalf("illegal: %q", cfg.DingTalk.ReplyMode)
	}
	cfg, err = Load(write("agent.yaml", "database:\n  dsn: postgres://x\ndingtalk:\n  reply_mode: AGENT\n"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DingTalk.ReplyMode != DingTalkReplyAgent || !cfg.DingTalk.UseAgent() {
		t.Fatalf("agent: %#v", cfg.DingTalk)
	}
	cfg, err = Load(write("web.yaml", "database:\n  dsn: postgres://x\ndingtalk:\n  reply_mode: WEB\n"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DingTalk.ReplyMode != DingTalkReplyWeb || !cfg.DingTalk.UseWebAgent() || cfg.DingTalk.UseAgent() {
		t.Fatalf("web: %#v", cfg.DingTalk)
	}
	t.Setenv("DINGTALK_REPLY_MODE", "agent")
	cfg, err = Load(write("env.yaml", "database:\n  dsn: postgres://x\n"))
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.DingTalk.UseAgent() {
		t.Fatal("DINGTALK_REPLY_MODE should select agent")
	}
}

func TestDingTalkReplyTagDefaultsToHostname(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tag.yaml")
	if err := os.WriteFile(path, []byte("database:\n  dsn: postgres://x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	want := defaultHostReplyTag()
	if cfg.DingTalk.ReplyTag != want {
		t.Fatalf("default reply_tag: got %q want %q", cfg.DingTalk.ReplyTag, want)
	}
	path2 := filepath.Join(dir, "custom.yaml")
	if err := os.WriteFile(path2, []byte("database:\n  dsn: postgres://x\ndingtalk:\n  reply_tag: 自定义\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err = Load(path2)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DingTalk.ReplyTag != "自定义" {
		t.Fatalf("custom reply_tag: %q", cfg.DingTalk.ReplyTag)
	}
}
