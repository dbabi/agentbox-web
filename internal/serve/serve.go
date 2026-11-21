// Package serve runs the read-only TUI over SSH with Charm Wish.
//
// Anonymous readers get the TUI and nothing else: no shell, no exec beyond the page names, no port
// forwarding, no subsystems (so no SFTP or SCP) and no agent forwarding. The default address is loopback.
package serve

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/ssh"
	"charm.land/wish/v2"
	"charm.land/wish/v2/accesscontrol"
	"charm.land/wish/v2/activeterm"
	"charm.land/wish/v2/bubbletea"
	"charm.land/wish/v2/ratelimiter"
	"charm.land/wish/v2/recover"
	gossh "golang.org/x/crypto/ssh"
	"golang.org/x/time/rate"

	"github.com/dbabi/agentbox-web/content"
	"github.com/dbabi/agentbox-web/internal/tui"
)

// DefaultAddr is loopback. Exposing the server is a deliberate act: pass -listen or AGENTBOX_SITE_LISTEN.
const DefaultAddr = "127.0.0.1:23234"

// The client sets the PTY size. The model and the renderer allocate per cell, so the server caps it.
const maxW, maxH = 500, 300

// Redact is a slog ReplaceAttr that drops the raw remote address, which the SSH library logs on a panic.
func Redact(_ []string, a slog.Attr) slog.Attr {
	if a.Key == "remote" || a.Key == "remote_addr" {
		return slog.String(a.Key, "redacted")
	}
	return a
}

// Config holds the server limits. Zero values take the defaults below.
type Config struct {
	Addr        string
	HostKeyPath string // optional; refused inside a git work tree. "" is an in-memory key per start
	MaxSessions int    // open connections. Each connection gets one session, so this also caps sessions
	IdleTimeout time.Duration
	MaxTimeout  time.Duration
	Log         *slog.Logger
}

func (c *Config) defaults() {
	if c.Addr == "" {
		c.Addr = DefaultAddr
	}
	if c.MaxSessions == 0 {
		c.MaxSessions = 32
	}
	if c.IdleTimeout == 0 {
		c.IdleTimeout = 120 * time.Second
	}
	if c.MaxTimeout == 0 {
		c.MaxTimeout = 30 * time.Minute
	}
	if c.Log == nil {
		c.Log = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{ReplaceAttr: Redact}))
	}
}

// IsLoopback reports whether addr (host:port) binds only to the local machine.
func IsLoopback(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// New builds the hardened server. It does not listen; call ListenAndServe or Serve.
func New(c Config) (*ssh.Server, error) {
	c.defaults()
	if !IsLoopback(c.Addr) {
		c.Log.Warn("listening on a non-loopback address: anyone who can reach it can read the site", "addr", c.Addr)
	}
	keyOpt, err := hostKey(c.HostKeyPath)
	if err != nil {
		return nil, err
	}
	pages, err := content.Pages()
	if err != nil {
		return nil, err
	}
	var routes []string
	for _, p := range pages {
		routes = append(routes, p.Slug)
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}
	var open atomic.Int64
	var started, conns sync.Map // sessions that reached the TUI; connections with a session
	srv, err := wish.NewServer(
		wish.WithAddress(c.Addr),
		keyOpt, // always set: without a key option wish.NewServer writes id_ed25519 into the CWD
		wish.WithIdleTimeout(c.IdleTimeout),
		wish.WithMaxTimeout(c.MaxTimeout),
		wish.WithMiddleware( // the last one listed runs first
			bubbletea.MiddlewareWithProgramHandler(program(&started)),
			activeterm.Middleware(),
			accesscontrol.Middleware(routes...),
			oneSession(&conns),
			logMiddleware(c.Log, salt, &started),
			recover.Middleware(),
			ratelimiter.Middleware(ratelimiter.NewRateLimiter(rate.Every(time.Second), 5, 1000)),
		),
	)
	if err != nil {
		return nil, err
	}
	srv.HandshakeTimeout = 10 * time.Second
	// Only "session" channels. No direct-tcpip, so no local forwarding; no tcpip-forward handler, so no
	// remote forwarding; no subsystems, so no SFTP. Agent forwarding is never set up (see the README).
	srv.ChannelHandlers = map[string]ssh.ChannelHandler{"session": ssh.DefaultSessionHandler}
	srv.RequestHandlers = map[string]ssh.RequestHandler{}
	srv.SubsystemHandlers = map[string]ssh.SubsystemHandler{}
	srv.LocalPortForwardingCallback = nil
	srv.ReversePortForwardingCallback = nil
	srv.ServerConfigCallback = func(ssh.Context) *gossh.ServerConfig { return &gossh.ServerConfig{MaxAuthTries: 3} }
	max := int64(c.MaxSessions)
	srv.ConnCallback = func(_ ssh.Context, conn net.Conn) net.Conn {
		if open.Add(1) > max {
			open.Add(-1)
			return nil // the server closes it
		}
		return conn
	}
	srv.ConnectionCloseCallback = func(net.Conn) { open.Add(-1) }
	return srv, nil
}

// program builds the TUI for one session. It caps the window size the client sends, at the start and on
// every resize, before the model or the renderer sees it.
func program(started *sync.Map) bubbletea.ProgramHandler {
	return func(s ssh.Session) *tea.Program {
		page := ""
		if cmd := s.Command(); len(cmd) > 0 {
			page = cmd[0] // accesscontrol already allowed only page names
		}
		pty, _, _ := s.Pty()
		m, err := tui.New(tui.Options{Page: page, SSH: true})
		if err != nil {
			wish.Fatalln(s, "agentbox-site: "+err.Error())
			return nil
		}
		started.Store(s, true)
		w, h := capSize(pty.Window.Width, pty.Window.Height)
		// these come after MakeOptions on purpose: it sets the raw client size and a filter, and the last option wins.
		// The filter keeps the suspend-to-resume rule of the one it replaces
		opts := append(bubbletea.MakeOptions(s), tea.WithWindowSize(w, h))
		return tea.NewProgram(m, append(opts, tea.WithFilter(func(_ tea.Model, msg tea.Msg) tea.Msg {
			switch msg := msg.(type) {
			case tea.SuspendMsg:
				return tea.ResumeMsg{}
			case tea.WindowSizeMsg:
				msg.Width, msg.Height = capSize(msg.Width, msg.Height)
				return msg
			}
			return msg
		}))...)
	}
}

func capSize(w, h int) (int, int) { return max(0, min(w, maxW)), max(0, min(h, maxH)) }

// oneSession refuses a second session channel on a connection, so one client cannot run many TUIs
// over a single connection.
func oneSession(conns *sync.Map) wish.Middleware {
	return func(next ssh.Handler) ssh.Handler {
		return func(s ssh.Session) {
			id := s.Context().SessionID()
			if _, busy := conns.LoadOrStore(id, true); busy {
				wish.Fatalln(s, "agentbox-site: one session per connection")
				return
			}
			defer conns.Delete(id)
			next(s)
		}
	}
}

// logMiddleware logs session start and end, the duration and a salted hash of the remote IP.
// A session that ends before the TUI starts (an exec, no PTY, a second session) is logged as refused.
// It never logs the command, env, keys, terminal type or typed input.
func logMiddleware(log *slog.Logger, salt []byte, started *sync.Map) wish.Middleware {
	return func(next ssh.Handler) ssh.Handler {
		return func(s ssh.Session) {
			host, _, _ := net.SplitHostPort(s.RemoteAddr().String())
			sum := sha256.Sum256(append(append([]byte{}, salt...), host...))
			id := hex.EncodeToString(sum[:4])
			start := time.Now()
			log.Info("session start", "ip", id)
			next(s)
			_, ran := started.LoadAndDelete(s)
			reason := "app exit"
			switch {
			case !ran:
				reason = "refused"
			case errors.Is(s.Context().Err(), context.Canceled):
				reason = "closed"
			}
			log.Info("session end", "ip", id, "duration", time.Since(start).Round(time.Second).String(), "reason", reason)
		}
	}
}

// hostKey returns an in-memory ed25519 key, or the key file at path if it is outside any git work tree.
func hostKey(path string) (ssh.Option, error) {
	if path == "" {
		_, priv, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return nil, err
		}
		b, err := gossh.MarshalPrivateKey(priv, "")
		if err != nil {
			return nil, err
		}
		return wish.WithHostKeyPEM(pem.EncodeToMemory(b)), nil
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	dir := filepath.Dir(abs)
	if d, err := filepath.EvalSymlinks(dir); err == nil {
		dir = d
	}
	for d := dir; ; d = filepath.Dir(d) {
		if _, err := os.Stat(filepath.Join(d, ".git")); err == nil {
			return nil, fmt.Errorf("host key %s is inside the git work tree %s; keep it outside so it is never committed", path, d)
		}
		if filepath.Dir(d) == d {
			break
		}
	}
	return wish.WithHostKeyPath(abs), nil
}
