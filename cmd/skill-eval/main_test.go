package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/neko233-com/skill233-video-script/internal/story"
)

func TestGraderRejectsStaleHandoff(t *testing.T) {
	dir := t.TempDir()
	fixture := filepath.Join("..", "..", "examples", "serial")
	if err := story.Init(dir, "七号怀表", "short-drama"); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"seed.json", "unit-001.json"} {
		data, err := os.ReadFile(filepath.Join(fixture, name))
		if err != nil {
			t.Fatal(err)
		}
		var b story.Batch
		if err = story.Decode(data, &b); err != nil {
			t.Fatal(err)
		}
		var draft []byte
		if b.Unit > 0 {
			draft, _ = os.ReadFile(filepath.Join(fixture, "001.md"))
		}
		if _, err = story.Accept(dir, b, draft); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(dir, "drafts"), 0755); err != nil {
		t.Fatal(err)
	}
	draftText, err := os.ReadFile(filepath.Join(fixture, "001.md"))
	if err != nil {
		t.Fatal(err)
	}
	for name, text := range map[string]string{"drafts/001.md": string(draftText), "active-strategy.md": "genre.suspense", "handoff.md": "- accepted revision: 1\n- next unit: 1\n"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := grade(dir, 1); err == nil || !strings.Contains(err.Error(), "stale handoff") {
		t.Fatal("stale handoff accepted", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "handoff.md"), []byte("- accepted revision: 2\n- next unit: 2\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := grade(dir, 1); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "drafts", "001.md"), []byte("接受后又改了正文"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := grade(dir, 1); err == nil || !strings.Contains(err.Error(), "differs from accepted") {
		t.Fatal("unbound working draft accepted", err)
	}
}
