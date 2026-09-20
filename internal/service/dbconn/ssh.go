package dbconn

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/webapp/go-app/ai-agent/internal/config"
	"go.uber.org/zap"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

const (
	sshDialTimeout   = 15 * time.Second
	sshTCPKeepalive  = 30 * time.Second
	maxReconnectWait = 30 * time.Second
)

type sshTunnel struct {
	cfg    config.DBConnSSHConfig
	log    *zap.Logger
	mu     sync.RWMutex
	reconn sync.Mutex
	client *ssh.Client
	stop   context.CancelFunc
	wg     sync.WaitGroup
	closed atomic.Bool
}

func newSSHTunnel(cfg config.DBConnSSHConfig, log *zap.Logger) *sshTunnel {
	if log == nil {
		log = zap.NewNop()
	}
	return &sshTunnel{cfg: cfg, log: log.With(zap.String("component", "dbconn_ssh"))}
}

func (t *sshTunnel) start(ctx context.Context) error {
	if err := validateSSHConfig(t.cfg); err != nil {
		return err
	}
	if err := t.connect(ctx); err != nil {
		return err
	}
	loopCtx, cancel := context.WithCancel(context.Background())
	t.stop = cancel
	t.wg.Go(func() { t.maintain(loopCtx) })
	return nil
}

func (t *sshTunnel) Close() {
	if t == nil || !t.closed.CompareAndSwap(false, true) {
		return
	}
	if t.stop != nil {
		t.stop()
	}
	t.wg.Wait()
	t.mu.Lock()
	cli := t.client
	t.client = nil
	t.mu.Unlock()
	if cli != nil {
		_ = cli.Close()
	}
}

func (t *sshTunnel) current() *ssh.Client {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.client
}

func (t *sshTunnel) Dial(ctx context.Context, addr string) (net.Conn, error) {
	cli := t.current()
	if cli == nil {
		if err := t.connect(ctx); err != nil {
			return nil, err
		}
		cli = t.current()
	}
	if cli == nil {
		return nil, fmt.Errorf("ssh 未连接")
	}
	type dialed struct {
		conn net.Conn
		err  error
	}
	ch := make(chan dialed, 1)
	go func() {
		conn, err := cli.Dial("tcp", addr)
		ch <- dialed{conn, err}
	}()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case d := <-ch:
		return d.conn, d.err
	}
}

func (t *sshTunnel) connect(ctx context.Context) error {
	t.reconn.Lock()
	defer t.reconn.Unlock()
	if t.closed.Load() {
		return fmt.Errorf("ssh 已关闭")
	}
	cli, err := dialSSH(ctx, t.cfg)
	if err != nil {
		return err
	}
	t.mu.Lock()
	old := t.client
	t.client = cli
	t.mu.Unlock()
	if old != nil {
		_ = old.Close()
	}
	t.log.Info("ssh 已连接", zap.String("addr", sshAddr(t.cfg)))
	return nil
}

func (t *sshTunnel) maintain(ctx context.Context) {
	ka := time.Duration(t.cfg.KeepaliveSeconds) * time.Second
	if ka <= 0 {
		ka = 30 * time.Second
	}
	wait := time.Duration(t.cfg.ReconnectWaitSeconds) * time.Second
	if wait <= 0 {
		wait = 2 * time.Second
	}

	for {
		cli := t.current()
		if cli == nil {
			if !t.reconnectLoop(ctx, &wait) {
				return
			}
			continue
		}
		dead := t.watch(ctx, cli, ka)
		if ctx.Err() != nil || t.closed.Load() {
			return
		}
		if dead {
			t.log.Warn("ssh 断开，准备重连")
			if !t.reconnectLoop(ctx, &wait) {
				return
			}
		}
	}
}

func (t *sshTunnel) watch(ctx context.Context, cli *ssh.Client, ka time.Duration) bool {
	waitCh := make(chan struct{})
	go func() {
		_ = cli.Wait()
		close(waitCh)
	}()
	ticker := time.NewTicker(ka)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return false
		case <-waitCh:
			return true
		case <-ticker.C:
			if _, _, err := cli.SendRequest("keepalive@openssh.com", true, nil); err != nil {
				t.log.Warn("ssh 保活失败", zap.Error(err))
				_ = cli.Close()
				return true
			}
		}
	}
}

func (t *sshTunnel) reconnectLoop(ctx context.Context, wait *time.Duration) bool {
	for {
		if ctx.Err() != nil || t.closed.Load() {
			return false
		}
		err := t.connect(ctx)
		if err == nil {
			*wait = time.Duration(t.cfg.ReconnectWaitSeconds) * time.Second
			if *wait <= 0 {
				*wait = 2 * time.Second
			}
			return true
		}
		t.log.Warn("ssh 重连失败", zap.Error(err), zap.Duration("retry_in", *wait))
		timer := time.NewTimer(*wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return false
		case <-timer.C:
		}
		*wait = nextBackoff(*wait, maxReconnectWait)
	}
}

func nextBackoff(cur, max time.Duration) time.Duration {
	if cur <= 0 {
		cur = time.Second
	}
	n := cur * 2
	if n > max {
		return max
	}
	return n
}

func validateSSHConfig(cfg config.DBConnSSHConfig) error {
	if strings.TrimSpace(cfg.Host) == "" {
		return fmt.Errorf("ssh.host 不能为空")
	}
	if strings.TrimSpace(cfg.User) == "" {
		return fmt.Errorf("ssh.user 不能为空")
	}
	if strings.TrimSpace(cfg.Password) == "" {
		return fmt.Errorf("ssh 需要 user 与 password")
	}
	if cfg.Enabled && strings.TrimSpace(cfg.Host) == "" {
		return fmt.Errorf("ssh.enabled 为 true 时必须配置 host")
	}
	return nil
}

func sshAddr(cfg config.DBConnSSHConfig) string {
	port := cfg.Port
	if port <= 0 {
		port = 22
	}
	return net.JoinHostPort(strings.TrimSpace(cfg.Host), fmt.Sprintf("%d", port))
}

func dialSSH(ctx context.Context, cfg config.DBConnSSHConfig) (*ssh.Client, error) {
	auth, err := sshAuth(cfg)
	if err != nil {
		return nil, err
	}
	hostKey, err := sshHostKeyCallback(cfg)
	if err != nil {
		return nil, err
	}
	sc := &ssh.ClientConfig{
		User:            cfg.User,
		Auth:            auth,
		HostKeyCallback: hostKey,
		Timeout:         sshDialTimeout,
	}
	addr := sshAddr(cfg)
	d := &net.Dialer{Timeout: sshDialTimeout, KeepAlive: sshTCPKeepalive}
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("连接 ssh %s: %w", addr, err)
	}
	if tc, ok := conn.(*net.TCPConn); ok {
		_ = tc.SetKeepAlive(true)
		_ = tc.SetKeepAlivePeriod(sshTCPKeepalive)
	}
	cc, chans, reqs, err := ssh.NewClientConn(conn, addr, sc)
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("ssh 握手 %s: %w", addr, err)
	}
	return ssh.NewClient(cc, chans, reqs), nil
}

func sshAuth(cfg config.DBConnSSHConfig) ([]ssh.AuthMethod, error) {
	password := cfg.Password
	if strings.TrimSpace(password) == "" {
		return nil, fmt.Errorf("ssh 需要 user 与 password")
	}
	// 同时提供 password 与 keyboard-interactive，兼容只开其中一种的跳板机。
	return []ssh.AuthMethod{
		ssh.Password(password),
		ssh.KeyboardInteractive(func(user, instruction string, questions []string, echos []bool) ([]string, error) {
			answers := make([]string, len(questions))
			for i := range questions {
				answers[i] = password
			}
			return answers, nil
		}),
	}, nil
}

func parseSigner(pem []byte, passphrase string) (ssh.Signer, error) {
	signer, err := ssh.ParsePrivateKey(pem)
	if err == nil {
		return signer, nil
	}
	if passphrase != "" {
		s, perr := ssh.ParsePrivateKeyWithPassphrase(pem, []byte(passphrase))
		if perr != nil {
			return nil, fmt.Errorf("解析 ssh 私钥: %w", perr)
		}
		return s, nil
	}
	return nil, fmt.Errorf("解析 ssh 私钥: %w", err)
}

func sshHostKeyCallback(cfg config.DBConnSSHConfig) (ssh.HostKeyCallback, error) {
	if cfg.InsecureIgnoreHostKey {
		return ssh.InsecureIgnoreHostKey(), nil
	}
	path := strings.TrimSpace(cfg.KnownHostsPath)
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("ssh 需要 known_hosts_path，或设置 insecure_ignore_host_key")
		}
		path = filepath.Join(home, ".ssh", "known_hosts")
	}
	cb, err := knownhosts.New(path)
	if err != nil {
		return nil, fmt.Errorf("加载 known_hosts %s: %w", path, err)
	}
	return cb, nil
}
