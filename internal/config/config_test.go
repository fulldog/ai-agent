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
}
