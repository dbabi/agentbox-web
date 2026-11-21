package main

import (
	"strings"
	"testing"

	"github.com/dbabi/agentbox-web/content"
)

func TestPlainEveryPage(t *testing.T) {
	pages, err := content.Pages()
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range pages {
		out := Plain(p, 76)
		if strings.Contains(out, "\x1b") {
			t.Fatalf("%s: plain output has escape codes", p.Slug)
		}
		for _, l := range strings.Split(out, "\n") {
			if len(l) > 76 && !strings.HasPrefix(l, "$ ") && !strings.Contains(l, "<http") && !strings.ContainsAny(l, "|+[") {
				t.Errorf("%s: line over 76 cols: %q", p.Slug, l)
			}
		}
	}
	home, _ := find("home")
	if out := Plain(home, 76); !strings.Contains(out, "no AI agent can run on it yet") || !strings.Contains(out, "[ok] firewall") {
		t.Fatalf("home lost its status lines:\n%s", out)
	}
}

func TestFindUnknown(t *testing.T) {
	if _, err := find("nope"); err == nil {
		t.Fatal("want error for an unknown page")
	}
	if p, err := find("4"); err != nil || p.Slug != "security" {
		t.Fatalf("find(4) = %q, %v", p.Slug, err)
	}
}
