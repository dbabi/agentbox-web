// Package content holds the site's pages and mascot grids, and parses them once.
// The web build (cmd/sitegen) and the terminal app both read it, so a page lives in one place.
// It sits in the module, not in internal/, because go:embed cannot reach outside its own directory tree.
package content

import (
	"bufio"
	"embed"
	"fmt"
	"io/fs"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

//go:embed *.md snib/*.json
var FS embed.FS

// Kind is the type of one line in a page.
type Kind int

const (
	Blank   Kind = iota
	Comment      // "# text"
	Command      // "$ cmd": a line to copy
	Text         // a plain line
	Bullet       // "- text"
	Step         // "1. text"; Line.N holds the number
	Caution      // "> CAUTION: text"
	Pre          // a ``` block; Line.Raw holds its lines, shown as is
	Palette      // "{{palette}}": expands to one Swatch line per palette color
	Swatch       // a palette color; Line.Hex and Line.Key
)

// Span is a run of inline text. A link has URL set. A status tag such as [built] has Tag set.
type Span struct {
	Text string
	URL  string
	Tag  string
}

// Line is one line of a page.
type Line struct {
	Kind  Kind
	N     int
	Spans []Span
	Raw   []string // Pre only
	Hex   string   // Swatch only
	Key   string   // Swatch only
}

// Plain returns the line text with links as "text <url>".
func (l Line) Plain() string {
	var b strings.Builder
	for _, s := range l.Spans {
		switch {
		case s.Tag != "":
			b.WriteString("[" + s.Tag + "]")
		case s.URL != "" && s.URL != s.Text:
			b.WriteString(s.Text + " <" + s.URL + ">")
		default:
			b.WriteString(s.Text)
		}
	}
	return b.String()
}

var statusRe = regexp.MustCompile(`^ (\d+(?:-\d+)?) (.*)$`)

// Status splits a status row such as "[built] 1-2 seed, ..." into its tag, its phase numbers and its text.
// Phases is "" for a row with no numbers, such as "[ok] firewall". ok is false for any other line.
func (l Line) Status() (tag, phases, text string, ok bool) {
	if l.Kind != Text || len(l.Spans) != 2 || l.Spans[0].Tag == "" || l.Spans[1].URL != "" || l.Spans[1].Tag != "" {
		return "", "", "", false
	}
	tag, text = l.Spans[0].Text, l.Spans[1].Text
	if m := statusRe.FindStringSubmatch(text); m != nil {
		return tag, m[1], m[2], true
	}
	return tag, "", strings.TrimPrefix(text, " "), true
}

// Page is one tab.
type Page struct {
	Key     int      // 1-7, the number key that opens it
	Slug    string   // URL path and prompt name
	Title   string   // tab label
	Pose    string   // Snib pose: happy, thinking, guarding, sleeping or all
	Sources []string // docs in dbabi/agentbox the text is condensed from, as paths from its root
	Lines   []Line
}

// Tags are the status words that renderers color. Each is also shown as text, so color is never the only signal.
var Tags = map[string]bool{
	"built": true, "partly built": true, "designed": true, "in progress": true,
	"ok": true, "--": true, "passed": true, "next": true, "planned": true, "not run": true,
}

// PaletteNames names each grid key, as in the table in mascot/README.md in dbabi/agentbox.
var PaletteNames = map[string]string{
	"o": "Ink", "h": "Highlight", "t": "Light", "f": "Body", "s": "Shade", "d": "Deep shade",
	"p": "Blush", "g": "Gold", "G": "Gold shade", "m": "Mint", "W": "Highlight ring",
}

var (
	stepRe = regexp.MustCompile(`^(\d+)\. (.*)$`)
	spanRe = regexp.MustCompile(`\[([^\]]+)\](\(([^)\s]+)\))?`)
)

// Pages parses every page in FS, sorted by key.
func Pages() ([]Page, error) {
	names, err := fs.Glob(FS, "*.md")
	if err != nil {
		return nil, err
	}
	var pages []Page
	for _, n := range names {
		b, err := FS.ReadFile(n)
		if err != nil {
			return nil, err
		}
		p, err := Parse(string(b))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", n, err)
		}
		pages = append(pages, p)
	}
	sort.Slice(pages, func(i, j int) bool { return pages[i].Key < pages[j].Key })
	return pages, nil
}

// Parse reads one page: front matter between "---" lines, then the line subset.
func Parse(src string) (Page, error) {
	var p Page
	sc := bufio.NewScanner(strings.NewReader(src))
	ln := 0
	next := func() (string, bool) {
		if !sc.Scan() {
			return "", false
		}
		ln++
		return strings.TrimRight(sc.Text(), " \t\r"), true
	}
	if s, ok := next(); !ok || s != "---" {
		return p, fmt.Errorf("line 1: want front matter (---)")
	}
	for {
		s, ok := next()
		if !ok {
			return p, fmt.Errorf("front matter not closed")
		}
		if s == "---" {
			break
		}
		k, v, ok := strings.Cut(s, ":")
		if !ok {
			return p, fmt.Errorf("line %d: want key: value", ln)
		}
		v = strings.TrimSpace(v)
		switch strings.TrimSpace(k) {
		case "key":
			n, err := strconv.Atoi(v)
			if err != nil {
				return p, fmt.Errorf("line %d: key: %w", ln, err)
			}
			p.Key = n
		case "slug":
			p.Slug = v
		case "title":
			p.Title = v
		case "pose":
			p.Pose = v
		case "sources":
			for _, s := range strings.Split(v, ",") {
				p.Sources = append(p.Sources, strings.TrimSpace(s))
			}
		default:
			return p, fmt.Errorf("line %d: unknown key %q", ln, k)
		}
	}
	if p.Key == 0 || p.Slug == "" || p.Title == "" || p.Pose == "" {
		return p, fmt.Errorf("front matter needs key, slug, title and pose")
	}
	for {
		s, ok := next()
		if !ok {
			break
		}
		switch {
		case s == "":
			p.Lines = append(p.Lines, Line{Kind: Blank})
		case strings.HasPrefix(s, "```"):
			var raw []string
			for {
				r, ok := next()
				if !ok {
					return p, fmt.Errorf("line %d: ``` block not closed", ln)
				}
				if strings.HasPrefix(r, "```") {
					break
				}
				raw = append(raw, r)
			}
			p.Lines = append(p.Lines, Line{Kind: Pre, Raw: raw})
		case s == "{{palette}}":
			sw, err := swatches()
			if err != nil {
				return p, err
			}
			p.Lines = append(p.Lines, sw...)
		case s == "#" || strings.HasPrefix(s, "# "):
			p.Lines = append(p.Lines, Line{Kind: Comment, Spans: spans(strings.TrimPrefix(strings.TrimPrefix(s, "#"), " "))})
		case strings.HasPrefix(s, "$ "):
			// commands are literal: no link or tag parsing
			p.Lines = append(p.Lines, Line{Kind: Command, Spans: []Span{{Text: s[2:]}}})
		case strings.HasPrefix(s, "- "):
			p.Lines = append(p.Lines, Line{Kind: Bullet, Spans: spans(s[2:])})
		case strings.HasPrefix(s, "> CAUTION: "):
			p.Lines = append(p.Lines, Line{Kind: Caution, Spans: spans(strings.TrimPrefix(s, "> CAUTION: "))})
		case stepRe.MatchString(s):
			m := stepRe.FindStringSubmatch(s)
			n, _ := strconv.Atoi(m[1])
			p.Lines = append(p.Lines, Line{Kind: Step, N: n, Spans: spans(m[2])})
		default:
			p.Lines = append(p.Lines, Line{Kind: Text, Spans: spans(s)})
		}
	}
	return p, sc.Err()
}

// spans splits text into plain runs, [text](url) links and [tag] status tags.
// Brackets that are neither stay plain text.
func spans(s string) []Span {
	var out []Span
	plain := func(t string) {
		if t == "" {
			return
		}
		if n := len(out); n > 0 && out[n-1].URL == "" && out[n-1].Tag == "" {
			out[n-1].Text += t
			return
		}
		out = append(out, Span{Text: t})
	}
	last := 0
	for _, m := range spanRe.FindAllStringSubmatchIndex(s, -1) {
		text := s[m[2]:m[3]]
		switch {
		case m[6] >= 0:
			plain(s[last:m[0]])
			out = append(out, Span{Text: text, URL: s[m[6]:m[7]]})
		case Tags[text]:
			plain(s[last:m[0]])
			out = append(out, Span{Text: "[" + text + "]", Tag: text})
		default:
			continue
		}
		last = m[1]
	}
	plain(s[last:])
	return out
}

// PaletteOrder is the order of the palette table in mascot/README.md in dbabi/agentbox.
var PaletteOrder = []string{"o", "h", "t", "f", "s", "d", "p", "g", "G", "m"}

func swatches() ([]Line, error) {
	g, err := LoadGrid("grid32.json")
	if err != nil {
		return nil, err
	}
	var out []Line
	for _, k := range PaletteOrder {
		out = append(out, Line{Kind: Swatch, Key: k, Hex: g.Palette[k],
			Spans: []Span{{Text: g.Palette[k] + " " + strings.ToLower(PaletteNames[k])}}})
	}
	return out, nil
}
