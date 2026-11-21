// Package snib renders the mascot grids: ringed, as terminal half blocks, as ASCII and as SVG.
// It draws no new art. Every pixel comes from the grids in content/snib.
package snib

import (
	"fmt"
	"html"
	"strconv"
	"strings"

	"github.com/dbabi/agentbox-web/content"
)

// Ring is the Highlight die-cut ring key and color, as in mascot/README.md in dbabi/agentbox.
const (
	Ring    = 'W'
	RingHex = "#f6f2ff"
)

// Poses maps a pose name to its grid file. "idle" is the 4-frame loop.
var Poses = map[string]string{
	"happy": "happy.json", "thinking": "thinking.json", "guarding": "guarding.json",
	"sleeping": "sleeping.json", "idle": "idle.json",
}

// Alt is the alt text for each pose. It names Snib and the pose.
var Alt = map[string]string{
	"happy":    "Snib, the agentbox mascot: a purple box with feet and a gold padlock on its antenna",
	"thinking": "Snib, the agentbox mascot, thinking, with a thought bubble",
	"guarding": "Snib, the agentbox mascot, guarding with a gold shield",
	"sleeping": "Snib, the agentbox mascot, asleep",
	"idle":     "Snib, the agentbox mascot, idle: it blinks and breathes",
}

// AddRing puts W on each empty pixel that touches the art, corners included.
// Snib's outline is Ink, the same as the page background, so the ring keeps the silhouette visible.
func AddRing(rows []string) []string {
	h := len(rows)
	at := func(x, y int) byte {
		if y < 0 || y >= h || x < 0 || x >= len(rows[y]) {
			return '.'
		}
		return rows[y][x]
	}
	out := make([]string, h)
	for y, r := range rows {
		b := []byte(r)
		for x := range b {
			if b[x] != '.' {
				continue
			}
		near:
			for j := -1; j <= 1; j++ {
				for i := -1; i <= 1; i++ {
					if c := at(x+i, y+j); c != '.' && c != Ring {
						b[x] = Ring
						break near
					}
				}
			}
		}
		out[y] = string(b)
	}
	return out
}

// Load returns the named pose with the ring added to every frame and to the palette.
func Load(pose string) (content.Grid, error) {
	name, ok := Poses[pose]
	if !ok {
		return content.Grid{}, fmt.Errorf("unknown pose %q", pose)
	}
	g, err := content.LoadGrid(name)
	if err != nil {
		return g, err
	}
	pal := map[string]string{string(Ring): RingHex}
	for k, v := range g.Palette {
		pal[k] = v
	}
	g.Palette = pal
	for i, f := range g.Frames {
		g.Frames[i] = AddRing(f)
	}
	return g, nil
}

func rgb(hex string) (r, g, b int) {
	v, _ := strconv.ParseUint(strings.TrimPrefix(hex, "#"), 16, 32)
	return int(v >> 16 & 0xff), int(v >> 8 & 0xff), int(v & 0xff)
}

// HalfBlock renders rows as 24-bit color upper and lower half blocks: one cell is two art pixels stacked,
// so a 32x32 grid is 32 cols by 16 rows. Empty pixels get no color, so the terminal background shows.
// Each line ends with a reset. shortcut: always 24-bit; the TUI downsamples with colorprofile for other terminals.
func HalfBlock(palette map[string]string, rows []string) string {
	var b strings.Builder
	color := func(k byte) string {
		if k == '.' {
			return ""
		}
		return palette[string(k)]
	}
	for y := 0; y < len(rows); y += 2 {
		top, bot := rows[y], strings.Repeat(".", len(rows[y]))
		if y+1 < len(rows) {
			bot = rows[y+1]
		}
		cur := "" // the SGR now in effect, "" after a reset
		for x := 0; x < len(top); x++ {
			t, u := color(top[x]), color(bot[x])
			var sgr, glyph string
			switch {
			case t == "" && u == "":
				sgr, glyph = "", " "
			case u == "":
				r, g, bl := rgb(t)
				sgr, glyph = fmt.Sprintf("\x1b[38;2;%d;%d;%dm", r, g, bl), "▀"
			case t == "":
				r, g, bl := rgb(u)
				sgr, glyph = fmt.Sprintf("\x1b[38;2;%d;%d;%dm", r, g, bl), "▄"
			default:
				r, g, bl := rgb(t)
				r2, g2, b2 := rgb(u)
				sgr, glyph = fmt.Sprintf("\x1b[38;2;%d;%d;%d;48;2;%d;%d;%dm", r, g, bl, r2, g2, b2), "▀"
			}
			if sgr != cur {
				if cur != "" {
					b.WriteString("\x1b[0m")
				}
				b.WriteString(sgr)
				cur = sgr
			}
			b.WriteString(glyph)
		}
		if cur != "" {
			b.WriteString("\x1b[0m")
		}
		b.WriteByte('\n')
	}
	return b.String()
}

// glyphs is the monochrome map, for NO_COLOR and dumb terminals. rank orders keys so small details
// (padlock, LED, cheeks, outline) win over the body when a cell holds two different keys.
var (
	glyphs = map[byte]byte{'o': '#', 'd': '=', 's': '=', 'f': '+', 't': '.', 'h': '.', Ring: '.',
		'g': '$', 'G': '$', 'm': '*', 'p': 'o', '.': ' '}
	rank = map[byte]int{'g': 9, 'G': 9, 'm': 8, 'p': 7, 'o': 6, 'f': 5, 'd': 4, 's': 4, 't': 3, 'h': 3, Ring: 2, '.': 0}
)

// ASCII renders rows with one character per pair of stacked pixels, the same 32x16 shape as HalfBlock.
func ASCII(rows []string) string {
	var b strings.Builder
	for y := 0; y < len(rows); y += 2 {
		line := make([]byte, len(rows[y]))
		for x := range line {
			k := rows[y][x]
			if y+1 < len(rows) && rank[rows[y+1][x]] > rank[k] {
				k = rows[y+1][x]
			}
			line[x] = glyphs[k]
		}
		b.WriteString(strings.TrimRight(string(line), " ") + "\n")
	}
	return b.String()
}

// SVG renders rows as an inline SVG at size px (a whole multiple of the grid width), with one <path> per color.
// With a title it is role="img"; without one it is a decorative, aria-hidden copy.
func SVG(palette map[string]string, rows []string, px int, title string) string {
	var b strings.Builder
	w, h := len(rows[0]), len(rows)
	a11y := `aria-hidden="true"`
	if title != "" {
		a11y = `role="img"`
	}
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" width="%d" height="%d" shape-rendering="crispEdges" %s>`,
		w, h, px, px*h/w, a11y)
	if title != "" {
		fmt.Fprintf(&b, "<title>%s</title>", html.EscapeString(title))
	}
	b.WriteString(Rects(palette, rows))
	b.WriteString("</svg>")
	return b.String()
}

// Rects draws one frame: one <path> per color, one "M x y h n v1 h-n z" run per row of same-colored pixels.
// The web stacks idle frames as <g> layers of these. Paths are about 5 times smaller than one <rect> per run.
func Rects(palette map[string]string, rows []string) string {
	runs := map[byte]*strings.Builder{}
	var order []byte
	for y, r := range rows {
		for x := 0; x < len(r); {
			k := r[x]
			n := 1
			for x+n < len(r) && r[x+n] == k {
				n++
			}
			if k != '.' {
				if runs[k] == nil {
					runs[k] = &strings.Builder{}
					order = append(order, k)
				}
				fmt.Fprintf(runs[k], "M%d %dh%dv1h-%dz", x, y, n, n)
			}
			x += n
		}
	}
	var b strings.Builder
	for _, k := range order {
		fmt.Fprintf(&b, `<path fill="%s" d="%s"/>`, palette[string(k)], runs[k].String())
	}
	return b.String()
}
