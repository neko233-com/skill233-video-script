package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/neko233-com/skill233-video-script/internal/story"
)

func TestCLIWorkflow(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "含 空格的作品")
	root := filepath.Join("..", "..")
	invoke := func(args ...string) []byte {
		t.Helper()
		var out bytes.Buffer
		if err := run(args, &out); err != nil {
			t.Fatal(args, err)
		}
		return out.Bytes()
	}
	invoke("init", "--project", dir, "--title", "七号怀表", "--format", "short-drama")
	invoke("apply", "--project", dir, "--batch", filepath.Join(root, "examples", "serial", "seed.json"))
	invoke("check", "--project", dir, "--batch", filepath.Join(root, "examples", "serial", "unit-001.json"), "--draft", filepath.Join(root, "examples", "serial", "001.md"))
	invoke("apply", "--project", dir, "--batch", filepath.Join(root, "examples", "serial", "unit-001.json"), "--draft", filepath.Join(root, "examples", "serial", "001.md"))
	data := invoke("verify", "--project", dir)
	var result story.Result
	if err := json.Unmarshal(data, &result); err != nil || result.Revision != 2 {
		t.Fatal(string(data), err)
	}
	invoke("context", "--project", dir, "--entities", "c.lin,c.shen,i.watch", "--at", "10")
	invoke("graph", "--project", dir, "--at", "10")
	invoke("strategies", "--dir", filepath.Join(root, "strategies"))
	invoke("compose", "--dir", filepath.Join(root, "strategies"), "--ids", "genre.suspense,structure.short-drama,style.restrained")
	if _, err := os.Stat(filepath.Join(dir, "records", "00000003.json")); !os.IsNotExist(err) {
		t.Fatal("unexpected commit")
	}
}

func TestCLIFailure(t *testing.T) {
	for _, args := range [][]string{{"unknown"}, {"init"}, {"context", "--project", "x", "--bogus"}, {"strategies"}, {"verify", "--project", "x", "extra"}} {
		var out bytes.Buffer
		if run(args, &out) == nil {
			t.Fatal("unexpected success", args)
		}
	}
}
