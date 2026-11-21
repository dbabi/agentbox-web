// Command tui shows the agentbox site in a terminal, or serves it over SSH.
//
//	go run github.com/dbabi/agentbox-web@latest
//	go run github.com/dbabi/agentbox-web@latest -plain security
//	go run . serve        # SSH on 127.0.0.1:23234, then: ssh -p 23234 -t 127.0.0.1 security
//	go run . -art guarding
package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/dbabi/agentbox-web/content"
	"github.com/dbabi/agentbox-web/internal/serve"
	"github.com/dbabi/agentbox-web/internal/snib"
	"github.com/dbabi/agentbox-web/internal/tui"
)

func main() {
	if len(os.Args) > 1 && (os.Args[1] == "serve" || os.Args[1] == "ssh") {
		runServe(os.Args[2:])
		return
	}
	plain := flag.Bool("plain", false, "print the page as plain text, with no color or art (for screen readers and pipes)")
	page := flag.String("page", "home", "page to show: "+strings.Join(slugs(), ", "))
	art := flag.String("art", "", "print only Snib in this pose: happy, thinking, guarding, sleeping")
	motion := flag.Bool("motion", true, "play the idle loop on the snib tab")
	flag.Parse()
	if flag.NArg() > 0 {
		*page = flag.Arg(0)
	}
	noColor := os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb"
	if *art != "" {
		out, err := drawSnib(*art, noColor)
		if err != nil {
			fail(err)
		}
		fmt.Print(out)
		return
	}
	p, err := find(*page)
	if err != nil {
		fail(err)
	}
	if *plain {
		fmt.Print(Plain(p, 76))
		return
	}
	m, err := tui.New(tui.Options{Page: p.Slug, Motion: *motion})
	if err != nil {
		fail(err)
	}
	if _, err := tea.NewProgram(m).Run(); err != nil {
		fail(err)
	}
}

func runServe(args []string) {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	def := serve.DefaultAddr
	if v := os.Getenv("AGENTBOX_SITE_LISTEN"); v != "" {
		def = v
	}
	addr := fs.String("listen", def, "host:port to listen on (env AGENTBOX_SITE_LISTEN). keep it loopback unless you mean to expose it")
	key := fs.String("host-key", "", "ed25519 host key file, outside any git work tree. default: a new in-memory key per start")
	_ = fs.Parse(args)
	// the SSH library logs the raw remote address on a panic. Redact it there too, not only in the session log
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{ReplaceAttr: serve.Redact})))
	srv, err := serve.New(serve.Config{Addr: *addr, HostKeyPath: *key, Log: slog.Default()})
	if err != nil {
		fail(err)
	}
	fmt.Fprintf(os.Stderr, "agentbox-site: ssh -p PORT -t HOST [page], on %s. ctrl-c stops it\n", *addr)
	if err := srv.ListenAndServe(); err != nil {
		fail(err)
	}
}

func drawSnib(pose string, noColor bool) (string, error) {
	g, err := snib.Load(pose)
	if err != nil {
		return "", err
	}
	if noColor {
		return snib.ASCII(g.Frames[0]), nil
	}
	return snib.HalfBlock(g.Palette, g.Frames[0]), nil
}

func slugs() []string {
	pages, _ := content.Pages()
	var s []string
	for _, p := range pages {
		s = append(s, p.Slug)
	}
	return s
}

func find(name string) (content.Page, error) {
	pages, err := content.Pages()
	if err != nil {
		return content.Page{}, err
	}
	for _, p := range pages {
		if p.Slug == name || fmt.Sprint(p.Key) == name {
			return p, nil
		}
	}
	return content.Page{}, fmt.Errorf("no such page: %s. try one of: %s", name, strings.Join(slugs(), ", "))
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "agentbox-site:", err)
	os.Exit(1)
}

// Plain renders a page as linear text wrapped at width. Commands and Pre blocks are never wrapped.
func Plain(p content.Page, width int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# ~/%s\n\n", p.Slug)
	for _, l := range p.Lines {
		switch l.Kind {
		case content.Blank:
			b.WriteString("\n")
		case content.Command:
			b.WriteString("$ " + l.Plain() + "\n")
		case content.Pre:
			b.WriteString(strings.Join(l.Raw, "\n") + "\n")
		case content.Comment:
			wrap(&b, "# ", "# ", l.Plain(), width)
		case content.Bullet:
			wrap(&b, "- ", "  ", l.Plain(), width)
		case content.Step:
			pre := fmt.Sprintf("%d. ", l.N)
			wrap(&b, pre, strings.Repeat(" ", len(pre)), l.Plain(), width)
		case content.Caution:
			wrap(&b, "# CAUTION: ", "# ", l.Plain(), width)
		case content.Swatch:
			b.WriteString("  " + l.Plain() + "\n")
		default:
			if tag, phases, text, ok := l.Status(); ok {
				pre := tag + " "
				if phases != "" {
					pre = fmt.Sprintf("%-10s%-6s", tag, phases)
				}
				wrap(&b, pre, strings.Repeat(" ", len(pre)), text, width)
				continue
			}
			wrap(&b, "", "", l.Plain(), width)
		}
	}
	return b.String()
}

func wrap(b *strings.Builder, first, rest, s string, width int) {
	line, pre := first, first
	for _, w := range strings.Fields(s) {
		if len(line) > len(pre) && len(line)+1+len(w) > width {
			b.WriteString(line + "\n")
			line, pre = rest, rest
		}
		if len(line) > len(pre) {
			line += " "
		}
		line += w
	}
	b.WriteString(line + "\n")
}
