// Command sitegen renders the shared pages in content/ to the static website in web/.
// Run it from the repo root after any change to content/, then commit web/:
//
//	go run ./cmd/sitegen
//
// The output is plain HTML, one CSS file and one small script. Every page works with JavaScript off.
package main

import (
	"embed"
	"flag"
	"fmt"
	"html"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/dbabi/agentbox-web/content"
	"github.com/dbabi/agentbox-web/internal/snib"
)

//go:embed assets/style.css assets/keys.js
var assets embed.FS

// copied as is from mascot/ into the site root
var images = []string{"favicon-16.png", "favicon-32.png", "social-preview.png"}

// baseURL is the absolute URL the site is served from, with a trailing slash. og:image needs it absolute, and
// 404.html, which is served at any depth, uses its path for every link.
const baseURL = "https://dbabi.github.io/agentbox-web/"

const csp = "default-src 'self'; img-src 'self'; script-src 'self'; style-src 'self'; object-src 'none'; base-uri 'none'"

func main() {
	out := flag.String("out", "web", "output directory")
	mascot := flag.String("mascot", "mascot", "the mascot directory, for the favicons and og:image")
	flag.Parse()
	if err := build(*out, *mascot); err != nil {
		fmt.Fprintln(os.Stderr, "sitegen:", err)
		os.Exit(1)
	}
}

func build(out, mascot string) error {
	pages, err := content.Pages()
	if err != nil {
		return err
	}
	files := map[string]string{}
	for _, p := range pages {
		name, root := p.Slug+"/index.html", "../"
		if p.Key == 1 {
			name, root = "index.html", ""
		}
		s, err := render(pages, p, root)
		if err != nil {
			return fmt.Errorf("%s: %w", p.Slug, err)
		}
		files[name] = s
	}
	// 404.html is served at any depth, so its links are absolute, under the base path
	u, err := url.Parse(baseURL)
	if err != nil || !strings.HasSuffix(u.Path, "/") {
		return fmt.Errorf("baseURL %q: want an absolute URL ending in /", baseURL)
	}
	nf := content.Page{Slug: "404", Title: "no such page", Pose: "sleeping", Lines: []content.Line{
		{Kind: content.Comment, Spans: []content.Span{{Text: "no such page. try help, or go home:"}}},
		{Kind: content.Bullet, Spans: []content.Span{{Text: "cd ~", URL: u.Path}}},
	}}
	if files["404.html"], err = render(pages, nf, u.Path); err != nil {
		return err
	}
	for _, a := range []string{"style.css", "keys.js"} {
		b, err := assets.ReadFile("assets/" + a)
		if err != nil {
			return err
		}
		files[a] = string(b)
	}
	for _, name := range images {
		b, err := os.ReadFile(filepath.Join(mascot, name))
		if err != nil {
			return err
		}
		files[name] = string(b)
	}
	for name, s := range files {
		p := filepath.Join(out, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
			return err
		}
	}
	return nil
}

var esc = html.EscapeString

// tagClass maps a status tag to its color class. The tag text is always shown too.
var tagClass = map[string]string{
	"built": "ok", "passed": "ok", "ok": "ok",
	"next": "next", "partly built": "next", "in progress": "next",
	"designed": "dim", "planned": "dim", "--": "dim",
	"not run": "warn",
}

func render(pages []content.Page, p content.Page, root string) (string, error) {
	var b strings.Builder
	title := p.Title + " · agentbox"
	if p.Key == 1 {
		title = "agentbox: early alpha"
	}
	desc := "agentbox: a headless OS for AI coding agents. early alpha."
	for _, l := range p.Lines {
		if l.Kind == content.Comment {
			desc = l.Plain()
			break
		}
	}
	cwd := "~/" + p.Slug
	if p.Key == 1 {
		cwd = "~"
	}
	fmt.Fprintf(&b, `<!doctype html>
<html lang="en" class="no-js">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta http-equiv="Content-Security-Policy" content="%s">
<meta name="color-scheme" content="dark light">
<meta name="referrer" content="no-referrer">
<title>%s</title>
<meta name="description" content="%s">
<meta property="og:title" content="%s">
<meta property="og:description" content="%s">
%s<link rel="icon" type="image/png" sizes="32x32" href="%sfavicon-32.png">
<link rel="icon" type="image/png" sizes="16x16" href="%sfavicon-16.png">
<link rel="stylesheet" href="%sstyle.css">
<script src="%skeys.js" defer></script>
</head>
<body data-cmd="%s">
<a class="skip" href="#main">skip to content</a>
<header class="top">
<nav aria-label="pages"><ul>
<li class="brand"><span aria-hidden="true">▣</span> agentbox</li>
`, csp, esc(title), esc(desc), esc(title), esc(desc), ogImage(), root, root, root, root, esc(content.MainCommand))
	for _, t := range pages {
		href := root + t.Slug + "/"
		if t.Key == 1 {
			href = root
			if root == "" {
				href = "./"
			}
		}
		cur := ""
		if t.Slug == p.Slug {
			cur = ` aria-current="page"`
		}
		fmt.Fprintf(&b, `<li><a href="%s" data-slug="%s"%s><span class="k" aria-hidden="true">%d</span>%s</a></li>
`, esc(href), esc(t.Slug), cur, t.Key, esc(t.Title))
	}
	b.WriteString(`<li class="ver">v0 alpha</li>
</ul></nav>
</header>
<main id="main" tabindex="-1">
`)
	if err := hero(&b, p); err != nil {
		return "", err
	}
	fmt.Fprintf(&b, "<h1 class=\"ln c\" tabindex=\"-1\">%s</h1>\n", esc(cwd))
	if err := lines(&b, p.Lines); err != nil {
		return "", err
	}
	if p.Pose == "all" {
		if err := gallery(&b); err != nil {
			return "", err
		}
	}
	fmt.Fprintf(&b, `</main>
<footer class="bot">
<p id="out" class="out" aria-live="polite"></p>
<div class="row">
<label for="prompt" class="ps"><span class="u">snib@agentbox</span><span class="at">:</span><span class="w">%s</span>$</label>
<span class="field"><input id="prompt" maxlength="64" autocomplete="off" autocapitalize="off" spellcheck="false" aria-describedby="out"><span class="caret" aria-hidden="true"></span></span>
<span class="chip">%s</span>
<span class="hints" aria-hidden="true">j/k move  enter open  / prompt</span>
<button type="button" class="js hb" id="helpbtn" hidden>? help</button>
</div>
</footer>
<dialog id="help" aria-labelledby="help-h">
<h2 id="help-h"># keys</h2>
<dl>
`, esc(cwd), esc(content.Status))
	for _, k := range content.Keys {
		if k.Web {
			fmt.Fprintf(&b, "<dt>%s</dt><dd>%s</dd>\n", esc(k.Keys), esc(k.Help))
		}
	}
	b.WriteString(`</dl>
<p># single-key shortcuts never fire in the prompt or with ctrl, alt or cmd held.</p>
<p><button type="button" id="keys">keys: on</button> <span class="dim">turn them off if they get in your way. saved in this browser.</span></p>
<form method="dialog"><button>close (esc)</button></form>
</dialog>
</body>
</html>
`)
	return b.String(), nil
}

func ogImage() string {
	return fmt.Sprintf("<meta property=\"og:image\" content=\"%ssocial-preview.png\">\n", esc(baseURL))
}

// hero is Snib in this page's pose. The snib page gets the idle loop with a pause button.
func hero(b *strings.Builder, p content.Page) error {
	if p.Pose != "all" {
		g, err := snib.Load(p.Pose)
		if err != nil {
			return err
		}
		fmt.Fprintf(b, "<figure class=\"snib\">%s</figure>\n", snib.SVG(g.Palette, g.Frames[0], 128, snib.Alt[p.Pose]))
		return nil
	}
	g, err := snib.Load("idle")
	if err != nil {
		return err
	}
	w := len(g.Frames[0][0])
	fmt.Fprintf(b, `<figure class="snib idle"><svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" width="128" height="128" shape-rendering="crispEdges" role="img"><title>%s</title>`,
		w, len(g.Frames[0]), esc(snib.Alt["idle"]))
	for i, f := range g.Frames {
		fmt.Fprintf(b, `<g class="f f%d">%s</g>`, i, snib.Rects(g.Palette, f))
	}
	b.WriteString("</svg>\n<figcaption><button type=\"button\" class=\"js\" id=\"motion\" hidden>pause</button></figcaption></figure>\n")
	return nil
}

func gallery(b *strings.Builder) error {
	b.WriteString("<ul class=\"poses\">\n")
	for _, pose := range []string{"happy", "thinking", "guarding", "sleeping"} {
		g, err := snib.Load(pose)
		if err != nil {
			return err
		}
		fmt.Fprintf(b, "<li><figure>%s<figcaption>%s</figcaption></figure></li>\n", snib.SVG(g.Palette, g.Frames[0], 64, snib.Alt[pose]), pose)
	}
	b.WriteString("</ul>\n")
	return nil
}

// lines renders the page body. Each line is focusable (tabindex -1), so the j/k cursor is real focus.
// A numbered step owns the comment and command lines under it, up to the next step or blank line.
func lines(b *strings.Builder, ls []content.Line) error {
	for i := 0; i < len(ls); {
		l := ls[i]
		switch l.Kind {
		case content.Bullet, content.Swatch:
			cls := "b"
			if l.Kind == content.Swatch {
				cls = "sw"
			}
			fmt.Fprintf(b, "<ul class=\"%s\">\n", cls)
			for ; i < len(ls) && ls[i].Kind == l.Kind; i++ {
				s, err := spans(ls[i])
				if err != nil {
					return err
				}
				if l.Kind == content.Swatch {
					s = fmt.Sprintf(`<svg width="16" height="16" viewBox="0 0 1 1" aria-hidden="true"><rect width="1" height="1" fill="%s"/></svg> %s`, esc(ls[i].Hex), s)
				}
				fmt.Fprintf(b, "<li class=\"ln\" tabindex=\"-1\">%s</li>\n", s)
			}
			b.WriteString("</ul>\n")
		case content.Step:
			b.WriteString("<ol class=\"steps\">\n")
			for i < len(ls) && ls[i].Kind == content.Step {
				s, err := spans(ls[i])
				if err != nil {
					return err
				}
				fmt.Fprintf(b, "<li value=\"%d\"><p class=\"ln\" tabindex=\"-1\">%s</p>\n", ls[i].N, s)
				for i++; i < len(ls) && (ls[i].Kind == content.Comment || ls[i].Kind == content.Command); i++ {
					if err := one(b, ls[i]); err != nil {
						return err
					}
				}
				b.WriteString("</li>\n")
			}
			b.WriteString("</ol>\n")
		default:
			if err := one(b, l); err != nil {
				return err
			}
			i++
		}
	}
	return nil
}

func one(b *strings.Builder, l content.Line) error {
	s, err := spans(l)
	if err != nil {
		return err
	}
	switch l.Kind {
	case content.Blank:
		b.WriteString("<div class=\"gap\"></div>\n")
	case content.Comment:
		fmt.Fprintf(b, "<p class=\"ln c\" tabindex=\"-1\">%s</p>\n", s)
	case content.Command:
		// <wbr> after each / lets a long URL wrap at a path step, not inside a word. It adds no text, so copy is exact
		s = strings.ReplaceAll(s, "/", "/<wbr>")
		fmt.Fprintf(b, "<div class=\"ln cmd\" tabindex=\"-1\"><pre><code>%s</code></pre> <button type=\"button\" class=\"js copy\" aria-label=\"copy command\" hidden>copy</button></div>\n", s)
	case content.Caution:
		fmt.Fprintf(b, "<p class=\"ln caution\" tabindex=\"-1\"><strong>CAUTION:</strong> %s</p>\n", s)
	case content.Pre:
		// box drawings and the roadmap bar: each is followed by the same facts in words
		fmt.Fprintf(b, "<figure class=\"pre\" aria-hidden=\"true\"><pre>%s</pre></figure>\n", esc(strings.Join(l.Raw, "\n")))
	default:
		if tag, phases, text, ok := l.Status(); ok && phases != "" {
			// a roadmap row: tag, phases and text in grid columns, so wrapped text stays in its column
			fmt.Fprintf(b, "<p class=\"ln st\" tabindex=\"-1\"><span class=\"tag t-%s\">%s</span><span>%s</span><span>%s</span></p>\n",
				tagClass[l.Spans[0].Tag], esc(tag), esc(phases), esc(text))
			return nil
		}
		fmt.Fprintf(b, "<p class=\"ln\" tabindex=\"-1\">%s</p>\n", s)
	}
	return nil
}

// spans renders inline text. Links must be https or site-absolute: the site makes no other requests.
func spans(l content.Line) (string, error) {
	var b strings.Builder
	for _, s := range l.Spans {
		switch {
		case s.Tag != "":
			fmt.Fprintf(&b, `<span class="tag t-%s">%s</span>`, tagClass[s.Tag], esc(s.Text))
		case s.URL != "":
			if !strings.HasPrefix(s.URL, "https://") && !strings.HasPrefix(s.URL, "/") {
				return "", fmt.Errorf("link %q: want https:// or /", s.URL)
			}
			fmt.Fprintf(&b, `<a href="%s">%s</a>`, esc(s.URL), esc(s.Text))
		default:
			b.WriteString(esc(s.Text))
		}
	}
	return b.String(), nil
}
