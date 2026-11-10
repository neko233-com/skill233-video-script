// skill-dist runs on the maintainer's Go 1.27 toolchain. Consumers receive
// ready-to-run native binaries and never need the Go compiler.
package main

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

var supported = []string{"windows-amd64", "windows-arm64", "darwin-amd64", "darwin-arm64", "linux-amd64", "linux-arm64"}

func main() {
	root := flag.String("root", ".", "skill 仓库根目录")
	out := flag.String("out", "", "输出目录；不会覆盖现有文件")
	targets := flag.String("targets", strings.Join(supported, ","), "逗号分隔的系统-架构")
	flag.Parse()
	if *out == "" || flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "usage: go run ./cmd/skill-dist --out DIR [--targets windows-amd64,...]")
		os.Exit(1)
	}
	absRoot, err := filepath.Abs(*root)
	if err != nil {
		fail(err)
	}
	absOut, err := filepath.Abs(*out)
	if err != nil {
		fail(err)
	}
	if err = makeBundles(absRoot, absOut, strings.Split(*targets, ",")); err != nil {
		fail(err)
	}
}
func fail(err error) { fmt.Fprintln(os.Stderr, err); os.Exit(1) }

func makeBundles(root, out string, targets []string) error {
	if err := os.MkdirAll(out, 0755); err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, target := range targets {
		if !contains(supported, target) || seen[target] {
			return fmt.Errorf("invalid or duplicate target %q", target)
		}
		seen[target] = true
	}
	if len(targets) == 0 {
		return fmt.Errorf("at least one target is required")
	}
	licenses := map[string]string{}
	for _, m := range []struct{ module, name string }{{"go.etcd.io/bbolt", "bbolt-LICENSE"}, {"golang.org/x/sys", "x-sys-LICENSE"}} {
		cmd := exec.Command("go", "list", "-m", "-json", m.module)
		cmd.Dir = root
		data, err := cmd.Output()
		if err != nil {
			return fmt.Errorf("locate %s license: %w", m.module, err)
		}
		var mod struct {
			Dir string `json:"Dir"`
		}
		if err = json.Unmarshal(data, &mod); err != nil {
			return err
		}
		licenses[m.name] = filepath.Join(mod.Dir, "LICENSE")
	}
	// Scratch path is uniquely created by this process inside the OS temp directory.
	tmp, err := os.MkdirTemp("", "skill233-dist-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	var checks []string
	for _, target := range targets {
		parts := strings.Split(target, "-")
		exe := "story"
		if parts[0] == "windows" {
			exe += ".exe"
		}
		binary := filepath.Join(tmp, target, exe)
		if err = os.MkdirAll(filepath.Dir(binary), 0755); err != nil {
			return err
		}
		cmd := exec.Command("go", "build", "-trimpath", "-ldflags=-s -w", "-o", binary, "./cmd/story")
		cmd.Dir = root
		cmd.Env = targetEnv(os.Environ(), parts[0], parts[1])
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("build %s: %s: %w", target, output, err)
		}
		name := "skill233-video-script-" + target + ".zip"
		path := filepath.Join(out, name)
		if err = archive(root, path, binary, exe, licenses); err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		hash := sha256.Sum256(data)
		checks = append(checks, fmt.Sprintf("%x  %s", hash, name))
		fmt.Println("built", name)
	}
	sort.Strings(checks)
	return writeExclusive(filepath.Join(out, "SHA256SUMS"), []byte(strings.Join(checks, "\n")+"\n"))
}

func contains(values []string, v string) bool {
	for _, x := range values {
		if x == v {
			return true
		}
	}
	return false
}
func targetEnv(env []string, goos, arch string) []string {
	var out []string
	for _, v := range env {
		p := strings.SplitN(v, "=", 2)
		if len(p) > 0 && (strings.EqualFold(p[0], "GOOS") || strings.EqualFold(p[0], "GOARCH") || strings.EqualFold(p[0], "CGO_ENABLED")) {
			continue
		}
		out = append(out, v)
	}
	return append(out, "GOOS="+goos, "GOARCH="+arch, "CGO_ENABLED=0")
}
func writeExclusive(path string, data []byte) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func archive(root, path, binary, exe string, licenses map[string]string) (err error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	z := zip.NewWriter(f)
	defer func() {
		if e := z.Close(); err == nil {
			err = e
		}
		if e := f.Close(); err == nil {
			err = e
		}
	}()
	add := func(source, name string, mode fs.FileMode) error {
		info, err := os.Stat(source)
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("not a regular file: %s", source)
		}
		data, err := os.ReadFile(source)
		if err != nil {
			return err
		}
		h := &zip.FileHeader{Name: filepath.ToSlash(filepath.Join("skill233-video-script", name)), Method: zip.Deflate}
		h.SetMode(mode)
		h.SetModTime(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
		w, err := z.CreateHeader(h)
		if err != nil {
			return err
		}
		_, err = w.Write(data)
		return err
	}
	for _, name := range []string{"SKILL.md", "LICENSE", "README.md", "VALIDATION.md"} {
		if err = add(filepath.Join(root, name), name, 0644); err != nil {
			return err
		}
	}
	for _, dir := range []string{"agents", "references", "strategies", "examples"} {
		base := filepath.Join(root, dir)
		err = filepath.WalkDir(base, func(path string, d fs.DirEntry, e error) error {
			if e != nil {
				return e
			}
			if d.IsDir() {
				return nil
			}
			if d.Type()&os.ModeSymlink != 0 {
				return fmt.Errorf("symlink not allowed in distribution: %s", path)
			}
			rel, e := filepath.Rel(root, path)
			if e != nil {
				return e
			}
			return add(path, rel, 0644)
		})
		if err != nil {
			return err
		}
	}
	for _, name := range []string{"bbolt-LICENSE", "x-sys-LICENSE"} {
		if err = add(licenses[name], filepath.Join("third_party", name), 0644); err != nil {
			return err
		}
	}
	if err = add(binary, filepath.Join("bin", exe), 0755); err != nil {
		return err
	}
	return nil
}
