package content

import (
	"reflect"
	"testing"
)

func TestPages(t *testing.T) {
	pages, err := Pages()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"home", "install", "how", "security", "roadmap", "docs", "snib"}
	if len(pages) != len(want) {
		t.Fatalf("%d pages, want %d", len(pages), len(want))
	}
	for i, p := range pages {
		if p.Key != i+1 || p.Slug != want[i] {
			t.Fatalf("page %d: key %d slug %q, want %d %q", i, p.Key, p.Slug, i+1, want[i])
		}
		// each page lists its source docs in dbabi/agentbox, so drift can be reviewed
		if len(p.Sources) == 0 {
			t.Fatalf("%s: no sources", p.Slug)
		}
	}
}

func TestParse(t *testing.T) {
	p, err := Parse("---\nkey: 9\nslug: t\ntitle: t\npose: happy\nsources: a, b\n---\n" +
		"# see [docs](https://x/y) now [built]\n$ echo [built] [x](y)\n- a [b] c\n12. step\n> CAUTION: hot\n\n```\n  raw [ok]\n```\nplain [ok] text\n")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(p.Sources, []string{"a", "b"}) {
		t.Fatalf("sources %q", p.Sources)
	}
	want := []Line{
		{Kind: Comment, Spans: []Span{{Text: "see "}, {Text: "docs", URL: "https://x/y"}, {Text: " now "}, {Text: "[built]", Tag: "built"}}},
		{Kind: Command, Spans: []Span{{Text: "echo [built] [x](y)"}}},
		{Kind: Bullet, Spans: []Span{{Text: "a [b] c"}}},
		{Kind: Step, N: 12, Spans: []Span{{Text: "step"}}},
		{Kind: Caution, Spans: []Span{{Text: "hot"}}},
		{Kind: Blank},
		{Kind: Pre, Raw: []string{"  raw [ok]"}},
		{Kind: Text, Spans: []Span{{Text: "plain "}, {Text: "[ok]", Tag: "ok"}, {Text: " text"}}},
	}
	if !reflect.DeepEqual(p.Lines, want) {
		t.Fatalf("got\n%#v\nwant\n%#v", p.Lines, want)
	}
	if got := p.Lines[0].Plain(); got != "see docs <https://x/y> now [built]" {
		t.Fatalf("Plain() = %q", got)
	}
	for _, bad := range []string{"no front matter", "---\nkey: 1\n", "---\nkey: 1\nslug: a\ntitle: a\npose: a\n---\n```\nopen"} {
		if _, err := Parse(bad); err == nil {
			t.Errorf("Parse(%q): want error", bad)
		}
	}
}

func TestStatus(t *testing.T) {
	for _, c := range []struct{ src, tag, phases, text string }{
		{"[built] 1-2 seed, install", "[built]", "1-2", "seed, install"},
		{"[next] 4 registry", "[next]", "4", "registry"},
		{"[ok] firewall", "[ok]", "", "firewall"},
	} {
		p, err := Parse("---\nkey: 1\nslug: a\ntitle: a\npose: a\n---\n" + c.src + "\n")
		if err != nil {
			t.Fatal(err)
		}
		tag, ph, text, ok := p.Lines[0].Status()
		if !ok || tag != c.tag || ph != c.phases || text != c.text {
			t.Errorf("%q: %q %q %q %v", c.src, tag, ph, text, ok)
		}
	}
	if _, _, _, ok := (Line{Kind: Comment, Spans: []Span{{Text: "[ok]", Tag: "ok"}, {Text: " x"}}}).Status(); ok {
		t.Error("a comment is not a status row")
	}
}

func TestPaletteSwatches(t *testing.T) {
	pages, _ := Pages()
	var n int
	for _, l := range pages[6].Lines {
		if l.Kind == Swatch {
			n++
		}
	}
	if n != 10 {
		t.Fatalf("%d swatches, want 10", n)
	}
}
