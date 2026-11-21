package snib

import (
	"flag"
	"fmt"
	"os"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/dbabi/agentbox-web/content"
)

var update = flag.Bool("update", false, "rewrite the golden files in testdata")

// The ring rule must match the one-liner in mascot/README.md in dbabi/agentbox, which made refine/r2/grid32-ring.json.
func TestRingMatchesMascotRingFile(t *testing.T) {
	g, err := content.LoadGrid("grid32.json")
	if err != nil {
		t.Fatal(err)
	}
	want, err := content.LoadGrid("grid32-ring.json")
	if err != nil {
		t.Fatal(err)
	}
	if got := AddRing(g.Frames[0]); !reflect.DeepEqual(got, want.Frames[0]) {
		t.Fatalf("ring(grid32) differs from grid32-ring.json:\n%s", strings.Join(got, "\n"))
	}
	if want.Palette[string(Ring)] != RingHex {
		t.Fatalf("ring color %q, want %q", want.Palette[string(Ring)], RingHex)
	}
}

// Every pose and idle frame keeps the 1 px empty border, so the ring fits inside the grid.
func TestPosesLoadAndFitTheRing(t *testing.T) {
	for pose := range Poses {
		g, err := content.LoadGrid(Poses[pose])
		if err != nil {
			t.Fatal(err)
		}
		for i, f := range g.Frames {
			if len(f) != 32 || len(f[0]) != 32 {
				t.Fatalf("%s frame %d: %dx%d, want 32x32", pose, i, len(f[0]), len(f))
			}
			for y, r := range f {
				if r[0] != '.' || r[31] != '.' || (y == 0 || y == 31) && r != strings.Repeat(".", 32) {
					t.Fatalf("%s frame %d: no empty border at row %d", pose, i, y)
				}
			}
		}
	}
	if g, _ := Load("idle"); len(g.Frames) != 4 || g.MS != 500 {
		t.Fatalf("idle: %d frames of %d ms, want 4 of 500", len(g.Frames), g.MS)
	}
}

func golden(t *testing.T, name, got string) {
	t.Helper()
	p := "testdata/" + name
	if *update {
		if err := os.WriteFile(p, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if got != string(want) {
		t.Fatalf("%s differs; run go test -update and review the diff\n%s", p, got)
	}
}

func TestHalfBlockGoldenAndPalette(t *testing.T) {
	g, err := Load("happy")
	if err != nil {
		t.Fatal(err)
	}
	out := HalfBlock(g.Palette, g.Frames[0])
	golden(t, "happy.ansi", out)

	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	if len(lines) != 16 {
		t.Fatalf("%d rows, want 16", len(lines))
	}
	sgr := regexp.MustCompile(`\x1b\[[0-9;]*m`)
	for i, l := range lines {
		if n := len([]rune(sgr.ReplaceAllString(l, ""))); n != 32 {
			t.Fatalf("row %d: %d cols, want 32", i, n)
		}
	}
	inPalette := map[string]bool{}
	for _, hex := range g.Palette {
		if hex != "transparent" {
			r, gg, b := rgb(hex)
			inPalette[fmt.Sprintf("%d;%d;%d", r, gg, b)] = true
		}
	}
	for _, m := range regexp.MustCompile(`[34]8;2;(\d+;\d+;\d+)`).FindAllStringSubmatch(out, -1) {
		if !inPalette[m[1]] {
			t.Fatalf("color %s is not in the palette", m[1])
		}
	}
}

func TestASCIIGolden(t *testing.T) {
	g, err := Load("happy")
	if err != nil {
		t.Fatal(err)
	}
	out := ASCII(g.Frames[0])
	golden(t, "happy.txt", out)
	for _, want := range []string{"$", "*", "o", "#"} { // padlock, LED, cheeks, outline
		if !strings.Contains(out, want) {
			t.Fatalf("ASCII art lost %q:\n%s", want, out)
		}
	}
}

func TestSVG(t *testing.T) {
	g, err := Load("guarding")
	if err != nil {
		t.Fatal(err)
	}
	s := SVG(g.Palette, g.Frames[0], 128, Alt["guarding"])
	for _, want := range []string{`width="128" height="128"`, `role="img"`, `<title>Snib, the agentbox mascot, guarding`, `crispEdges`, `fill="#f6f2ff"`} {
		if !strings.Contains(s, want) {
			t.Fatalf("SVG lacks %q", want)
		}
	}
	if d := SVG(g.Palette, g.Frames[0], 64, ""); !strings.Contains(d, `aria-hidden="true"`) || strings.Contains(d, "<title>") {
		t.Fatal("decorative SVG must be aria-hidden with no title")
	}
}
