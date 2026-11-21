package content

import (
	"encoding/json"
	"fmt"
	"unicode/utf8"
)

// Grid is a mascot grid in the format of mascot/README.md in dbabi/agentbox: one character per pixel, "." is transparent.
// A still image has one frame. An animation has several, each shown for MS milliseconds.
type Grid struct {
	Palette map[string]string `json:"palette"`
	Rows    []string          `json:"rows,omitempty"`
	Frames  [][]string        `json:"frames,omitempty"`
	MS      int               `json:"ms,omitempty"`
}

// LoadGrid reads content/snib/<name> and checks its shape.
func LoadGrid(name string) (Grid, error) {
	var g Grid
	b, err := FS.ReadFile("snib/" + name)
	if err != nil {
		return g, err
	}
	if err := json.Unmarshal(b, &g); err != nil {
		return g, fmt.Errorf("%s: %w", name, err)
	}
	if g.Rows != nil {
		g.Frames = [][]string{g.Rows}
		g.Rows = nil
	}
	if len(g.Frames) == 0 {
		return g, fmt.Errorf("%s: no rows", name)
	}
	for f, rows := range g.Frames {
		if err := Check(g.Palette, rows); err != nil {
			return g, fmt.Errorf("%s: frame %d: %w", name, f, err)
		}
	}
	return g, nil
}

// Check returns an error like px.py's if rows are ragged or use a key not in the palette.
func Check(palette map[string]string, rows []string) error {
	if len(rows) == 0 {
		return fmt.Errorf("no rows")
	}
	w := len(rows[0])
	for y, r := range rows {
		if len(r) != w || utf8.RuneCountInString(r) != w {
			return fmt.Errorf("row %d has width %d, want %d", y, len(r), w)
		}
		for _, c := range r {
			if _, ok := palette[string(c)]; !ok {
				return fmt.Errorf("row %d: char %q not in palette", y, c)
			}
		}
	}
	return nil
}
