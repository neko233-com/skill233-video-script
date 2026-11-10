package main

import (
	"archive/zip"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestNativeBundleWithoutGoOnPath(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	target := runtime.GOOS + "-" + runtime.GOARCH
	if err = makeBundles(root, out, []string{target}); err != nil {
		t.Fatal(err)
	}
	archivePath := filepath.Join(out, "skill233-video-script-"+target+".zip")
	z, err := zip.OpenReader(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	defer z.Close()
	install := t.TempDir()
	for _, entry := range z.File {
		clean := filepath.Clean(filepath.FromSlash(entry.Name))
		if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			t.Fatal("unsafe archive path", entry.Name)
		}
		dst := filepath.Join(install, clean)
		if err = os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
			t.Fatal(err)
		}
		in, err := entry.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(in)
		in.Close()
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(dst, data, entry.Mode().Perm()); err != nil {
			t.Fatal(err)
		}
	}
	skill := filepath.Join(install, "skill233-video-script")
	binary := filepath.Join(skill, "bin", "story")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	project := filepath.Join(install, "作品")
	run := func(args ...string) string {
		t.Helper()
		cmd := exec.Command(binary, args...)
		cmd.Env = targetEnv(os.Environ(), runtime.GOOS, runtime.GOARCH)
		var env []string
		for _, v := range cmd.Env {
			if !strings.HasPrefix(strings.ToUpper(v), "PATH=") {
				env = append(env, v)
			}
		}
		cmd.Env = append(env, "PATH=")
		data, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("packaged tool without Go: %s: %v", data, err)
		}
		return string(data)
	}
	run("init", "--project", project, "--title", "无 Go 验收", "--format", "film")
	run("apply", "--project", project, "--batch", filepath.Join(skill, "examples", "serial", "seed.json"))
	data := run("context", "--project", project, "--entities", "c.lin,i.watch", "--at", "0", "--knowledge-scope", "related")
	if !strings.Contains(data, "林岚") || !strings.Contains(data, "knowledge_subjects") {
		t.Fatal("packaged indexed context missing", data)
	}
	if _, err = os.Stat(filepath.Join(skill, "third_party", "bbolt-LICENSE")); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(out, "SHA256SUMS")); err != nil {
		t.Fatal(err)
	}
}
