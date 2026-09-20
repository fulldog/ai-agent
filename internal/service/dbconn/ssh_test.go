package dbconn

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"strings"
	"testing"
	"time"

	"github.com/webapp/go-app/ai-agent/internal/config"
)

func TestRewriteNet(t *testing.T) {
	t.Parallel()
	got, err := rewriteNet("user:p@tcp(127.0.0.1:3306)/biz?parseTime=true", "dbconnssh1")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "@dbconnssh1(127.0.0.1:3306)/biz") {
		t.Fatalf("got %q", got)
	}
	if _, err := rewriteNet("::::", "x"); err == nil {
		t.Fatal("expected parse error")
	}
}

func TestValidateSSHConfig(t *testing.T) {
	t.Parallel()
	if err := validateSSHConfig(config.DBConnSSHConfig{}); err == nil {
		t.Fatal("empty host")
	}
	if err := validateSSHConfig(config.DBConnSSHConfig{Host: "h"}); err == nil {
		t.Fatal("empty user")
	}
	if err := validateSSHConfig(config.DBConnSSHConfig{Host: "h", User: "u"}); err == nil {
		t.Fatal("no password")
	}
	if err := validateSSHConfig(config.DBConnSSHConfig{Host: "h", User: "u", PrivateKeyPath: "k"}); err == nil {
		t.Fatal("key only should fail")
	}
	if err := validateSSHConfig(config.DBConnSSHConfig{Host: "h", User: "u", Password: "p"}); err != nil {
		t.Fatal(err)
	}
	methods, err := sshAuth(config.DBConnSSHConfig{User: "u", Password: "secret"})
	if err != nil || len(methods) != 2 {
		t.Fatalf("password auth methods: n=%d err=%v", len(methods), err)
	}
}

func TestParseSigner(t *testing.T) {
	t.Parallel()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	block := &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}
	raw := pem.EncodeToMemory(block)
	if _, err := parseSigner(raw, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := parseSigner([]byte("not-a-key"), ""); err == nil {
		t.Fatal("expected parse error")
	}
}

func TestNextBackoff(t *testing.T) {
	t.Parallel()
	if got := nextBackoff(2*time.Second, 30*time.Second); got != 4*time.Second {
		t.Fatalf("got %s", got)
	}
	if got := nextBackoff(20*time.Second, 30*time.Second); got != 30*time.Second {
		t.Fatalf("cap got %s", got)
	}
}

func TestIsTransient(t *testing.T) {
	t.Parallel()
	if isTransient(nil) {
		t.Fatal("nil")
	}
	if !isTransient(errSSHText("ssh: connection lost")) {
		t.Fatal("ssh text")
	}
	if isTransient(errSSHText("sql 不能为空")) {
		t.Fatal("app error")
	}
}

type errSSHText string

func (e errSSHText) Error() string { return string(e) }

func TestOpenRequiresSSHHostWhenForced(t *testing.T) {
	t.Parallel()
	_, err := Open(config.DBConnConfig{
		DSN: "user:p@tcp(127.0.0.1:3306)/biz",
		SSH: config.DBConnSSHConfig{Enabled: true},
	}, nil)
	if err == nil || !strings.Contains(err.Error(), "ssh.host") {
		t.Fatalf("want ssh.host error, got %v", err)
	}
}

func TestSSHAddr(t *testing.T) {
	t.Parallel()
	if got := sshAddr(config.DBConnSSHConfig{Host: "jump", Port: 2222}); got != "jump:2222" {
		t.Fatalf("got %q", got)
	}
	if got := sshAddr(config.DBConnSSHConfig{Host: "jump"}); got != "jump:22" {
		t.Fatalf("default port %q", got)
	}
}
