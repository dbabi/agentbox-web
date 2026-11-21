package main

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const web = "../../web" // cmd/sitegen -> web

// TestWebUpToDate fails if site/web differs from a fresh build, so the web and the TUI cannot drift.
// Fix: go run ./cmd/sitegen (from the repo root), then commit web/.
func TestWebUpToDate(t *testing.T) {
	tmp := t.TempDir()
	if err := build(tmp, "../../mascot"); err != nil {
		t.Fatal(err)
	}
	want, got := tree(t, tmp), tree(t, web)
	for name, b := range want {
		if !bytes.Equal(got[name], b) {
			t.Errorf("web/%s is stale or missing; run go run ./cmd/sitegen", name)
		}
	}
	for name := range got {
		if _, ok := want[name]; !ok {
			t.Errorf("web/%s is not generated; delete it", name)
		}
	}
}

// TestNoExternalRequests: every src, stylesheet and script is local. Only <a href> and og:image, which a
// link preview reads and the browser never loads, may point off-site.
func TestNoExternalRequests(t *testing.T) {
	attr := regexp.MustCompile(`(?:src|href|content)="([^"]*)"`)
	for name, b := range tree(t, web) {
		if !strings.HasSuffix(name, ".html") {
			continue
		}
		s := string(b)
		for _, tag := range regexp.MustCompile(`<(script|link|img|iframe|meta)\b[^>]*>`).FindAllString(s, -1) {
			if strings.Contains(tag, `property="og:image"`) {
				continue
			}
			for _, m := range attr.FindAllStringSubmatch(tag, -1) {
				if strings.Contains(m[1], "//") {
					t.Errorf("%s: external reference in %s", name, tag)
				}
			}
		}
		if len(b) > 30<<10 && !strings.HasPrefix(name, "snib/") {
			t.Errorf("%s: %d bytes, budget is 30 KB", name, len(b))
		}
		for _, want := range []string{`lang="en"`, `class="skip" href="#main"`, `<h1`, `aria-label="pages"`, `<label for="prompt"`, "Content-Security-Policy"} {
			if !strings.Contains(s, want) {
				t.Errorf("%s lacks %s", name, want)
			}
		}
		if n := strings.Count(s, `aria-current="page"`); n != 1 && name != "404.html" {
			t.Errorf("%s: %d aria-current tabs, want 1", name, n)
		}
	}
}

func tree(t *testing.T, root string) map[string][]byte {
	t.Helper()
	m := map[string][]byte{}
	err := filepath.WalkDir(root, func(p string, e fs.DirEntry, err error) error {
		if err != nil || e.IsDir() {
			return err
		}
		b, err := os.ReadFile(p)
		rel, _ := filepath.Rel(root, p)
		m[filepath.ToSlash(rel)] = b
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return m
}
