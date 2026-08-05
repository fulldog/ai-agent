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
