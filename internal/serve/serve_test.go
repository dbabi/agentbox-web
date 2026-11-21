package serve

import (
	"bytes"
	"errors"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	gossh "golang.org/x/crypto/ssh"
)

// start runs a server on an ephemeral loopback port and returns its address.
func start(t *testing.T, c Config) string {
	t.Helper()
	if c.Log == nil {
		c.Log = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	srv, err := New(c)
	if err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })
	return ln.Addr().String()
}

func dial(t *testing.T, addr string) *gossh.Client {
	t.Helper()
	c, err := gossh.Dial("tcp", addr, &gossh.ClientConfig{
		User:            "anyone",
		HostKeyCallback: gossh.InsecureIgnoreHostKey(), // the test server has a fresh key each run
		Timeout:         5 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func exitStatus(err error) int {
	var e *gossh.ExitError
	if errors.As(err, &e) {
		return e.ExitStatus()
	}
	return -1
}

func TestDefaultsAreLoopback(t *testing.T) {
	var c Config
	c.defaults()
	if c.Addr != "127.0.0.1:23234" || !IsLoopback(c.Addr) {
		t.Errorf("default addr %s", c.Addr)
	}
	for addr, want := range map[string]bool{
		"127.0.0.1:1": true, "localhost:1": true, "[::1]:1": true,
		"0.0.0.0:1": false, "[::]:1": false, ":1": false, "192.168.1.2:1": false, "bad": false,
	} {
		if IsLoopback(addr) != want {
			t.Errorf("IsLoopback(%s) = %v", addr, !want)
		}
	}
}

func TestHostKeyInsideRepoRefused(t *testing.T) {
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	key := filepath.Join(repo, "deep", "id_ed25519")
	if _, err := New(Config{HostKeyPath: key}); err == nil || !strings.Contains(err.Error(), "git work tree") {
		t.Fatalf("a host key inside a git work tree was accepted: %v", err)
	}
	if _, err := os.Stat(key); !os.IsNotExist(err) {
		t.Error("a key file was written")
	}
}

func TestExecRefused(t *testing.T) {
	addr := start(t, Config{})
	c := dial(t, addr)
	for _, cmd := range []string{"whoami", "sh -c id", "security"} { // security has no PTY, so activeterm refuses it
		s, err := c.NewSession()
		if err != nil {
			t.Fatal(err)
		}
		out, err := s.CombinedOutput(cmd)
		if exitStatus(err) != 1 {
			t.Errorf("exec %q: err %v, out %q; want exit 1", cmd, err, out)
		}
	}
	s, _ := c.NewSession()
	if err := s.Shell(); err != nil {
		t.Fatal(err)
	}
	if err := s.Wait(); exitStatus(err) != 1 {
		t.Errorf("shell without a PTY: %v, want exit 1", err)
	}
}

func TestSubsystemsRefused(t *testing.T) {
	c := dial(t, start(t, Config{}))
	for _, name := range []string{"sftp", "scp", "default"} {
		s, _ := c.NewSession()
		if err := s.RequestSubsystem(name); err == nil {
			t.Errorf("subsystem %s accepted", name)
		}
	}
}

func TestForwardingRefused(t *testing.T) {
	c := dial(t, start(t, Config{}))
	if conn, err := c.Dial("tcp", "127.0.0.1:22"); err == nil {
		_ = conn.Close()
		t.Error("local port forwarding (direct-tcpip) accepted")
	}
	if ln, err := c.Listen("tcp", "127.0.0.1:0"); err == nil {
		_ = ln.Close()
		t.Error("remote port forwarding (tcpip-forward) accepted")
	}
	if ok, _, _ := c.SendRequest("streamlocal-forward@openssh.com", true, nil); ok {
		t.Error("streamlocal-forward accepted")
	}
}

func TestPTYSessionAndNoAgentChannel(t *testing.T) {
	c := dial(t, start(t, Config{}))
	agentChans := c.HandleChannelOpen("auth-agent@openssh.com")
	s, err := c.NewSession()
	if err != nil {
		t.Fatal(err)
	}
	// ssh -A: the library replies to auth-agent-req, but the server never opens an agent channel
	_, _ = s.SendRequest("auth-agent-req@openssh.com", true, nil)
	if err := s.RequestPty("xterm-256color", 24, 80, gossh.TerminalModes{}); err != nil {
		t.Fatal(err)
	}
	out, _ := s.StdoutPipe()
	in, _ := s.StdinPipe()
	if err := s.Start("security"); err != nil {
		t.Fatal(err)
	}
	got := make(chan string, 1)
	go func() {
		var b strings.Builder
		buf := make([]byte, 4096)
		for !strings.Contains(b.String(), "security") {
			n, err := out.Read(buf)
			b.Write(buf[:n])
			if err != nil {
				break
			}
		}
		got <- b.String()
	}()
	select {
	case s := <-got:
		if !strings.Contains(s, "security") {
			t.Errorf("no TUI output: %q", s)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the TUI did not draw within 5 s")
	}
	select {
	case ch := <-agentChans:
		_ = ch.Reject(gossh.Prohibited, "")
		t.Error("the server opened an agent channel")
	case <-time.After(500 * time.Millisecond):
	}
	_, _ = in.Write([]byte("q"))
	done := make(chan error, 1)
	go func() { done <- s.Wait() }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Error("q did not end the session")
	}
}

func TestSessionCap(t *testing.T) {
	addr := start(t, Config{MaxSessions: 2})
	dial(t, addr)
	dial(t, addr)
	c, err := gossh.Dial("tcp", addr, &gossh.ClientConfig{
		User: "anyone", HostKeyCallback: gossh.InsecureIgnoreHostKey(), Timeout: 5 * time.Second,
	})
	if err == nil {
		_ = c.Close()
		t.Fatal("connection 3 was accepted with a cap of 2")
	}
}

// syncBuf is a log sink the server goroutines and the test can share.
type syncBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuf) Write(p []byte) (int, error) { s.mu.Lock(); defer s.mu.Unlock(); return s.b.Write(p) }
func (s *syncBuf) String() string              { s.mu.Lock(); defer s.mu.Unlock(); return s.b.String() }

// ptySession starts the TUI on a new session and waits for its first frame.
func ptySession(t *testing.T, c *gossh.Client, rows, cols int) (*gossh.Session, io.Writer) {
	t.Helper()
	s, err := c.NewSession()
	if err != nil {
		t.Fatal(err)
	}
	if err := s.RequestPty("xterm-256color", rows, cols, gossh.TerminalModes{}); err != nil {
		t.Fatal(err)
	}
	out, _ := s.StdoutPipe()
	in, _ := s.StdinPipe()
	if err := s.Start("home"); err != nil {
		t.Fatal(err)
	}
	got := make(chan bool, 1)
	go func() {
		var b strings.Builder
		buf := make([]byte, 4096)
		for !strings.Contains(b.String(), "home") {
			n, err := out.Read(buf)
			b.Write(buf[:n])
			if err != nil {
				got <- false
				return
			}
		}
		got <- true
		_, _ = io.Copy(io.Discard, out)
	}()
	select {
	case ok := <-got:
		if !ok {
			t.Fatal("the TUI ended before it drew")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the TUI did not draw within 5 s")
	}
	return s, in
}

func TestOneSessionPerConnection(t *testing.T) {
	var logs syncBuf
	addr := start(t, Config{Log: slog.New(slog.NewTextHandler(&logs, nil))})
	c := dial(t, addr)
	first, _ := ptySession(t, c, 24, 80)
	defer first.Close()
	s, err := c.NewSession()
	if err != nil {
		t.Fatal(err)
	}
	if out, err := s.CombinedOutput("home"); exitStatus(err) != 1 || !strings.Contains(string(out), "one session per connection") {
		t.Errorf("second session: err %v, out %q", err, out)
	}
	// a refused exec on a fresh connection is logged as refused, not as a normal exit
	s2, _ := dial(t, addr).NewSession()
	_, _ = s2.CombinedOutput("uname")
	deadline := time.Now().Add(2 * time.Second)
	for strings.Count(logs.String(), "reason=refused") < 2 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if n := strings.Count(logs.String(), "reason=refused"); n != 2 {
		t.Errorf("%d refused sessions logged, want 2:\n%s", n, logs.String())
	}
	if strings.Contains(logs.String(), "uname") || strings.Contains(logs.String(), "127.0.0.1") {
		t.Errorf("the log holds a command or an address:\n%s", logs.String())
	}
}

// A client may claim any PTY size. The server must not allocate for it.
func TestHugeWindowCapped(t *testing.T) {
	if w, h := capSize(1<<30, -1); w != maxW || h != 0 {
		t.Fatalf("capSize: %dx%d", w, h)
	}
	c := dial(t, start(t, Config{}))
	s, in := ptySession(t, c, 1<<30, 1<<30)
	defer s.Close()
	_, _ = in.Write([]byte("q"))
	done := make(chan error, 1)
	go func() { done <- s.Wait() }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Error("the capped session did not quit on q")
	}
}

func TestRedact(t *testing.T) {
	var b bytes.Buffer
	slog.New(slog.NewTextHandler(&b, &slog.HandlerOptions{ReplaceAttr: Redact})).Error("x", "remote", "203.0.113.9:4000", "panic", "p")
	if strings.Contains(b.String(), "203.0.113.9") || !strings.Contains(b.String(), "remote=redacted") {
		t.Errorf("not redacted: %s", b.String())
	}
}
