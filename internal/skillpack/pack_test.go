package skillpack

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

func TestInstalledSkillIsSelfContained(t *testing.T) {
	root := filepath.Join("..", "..")
	dir := filepath.Join(t.TempDir(), "安装 space", "skill233-video-script")
	if err := Copy(root, dir); err != nil {
		t.Fatal(err)
	}
	if err := Copy(root, dir); err == nil {
		t.Fatal("existing skill overwritten")
	}
	links := regexp.MustCompile(`\]\(([^)]+)\)`)
	if err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || filepath.Ext(path) != ".md" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, m := range links.FindAllStringSubmatch(string(data), -1) {
			target := strings.Split(m[1], "#")[0]
			if target == "" || strings.Contains(target, "://") {
				continue
			}
			if _, err := os.Stat(filepath.Join(filepath.Dir(path), target)); err != nil {
				t.Errorf("broken installed reference %s -> %s", path, target)
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(t.TempDir(), "story")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	cmd := exec.Command("go", "build", "-o", bin, "./cmd/story")
	cmd.Dir = dir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("installed helper build failed: %s: %v", output, err)
	}
	cmd = exec.Command(bin, "compose", "--dir", filepath.Join(dir, "strategies"), "--dir", filepath.Join(dir, "examples", "strategies"), "--ids", "genre.cyber-mystery,structure.novel-serial,style.sparse-poetic")
	if output, err := cmd.CombinedOutput(); err != nil || !strings.Contains(string(output), "记忆所有权") {
		t.Fatalf("installed extension flow failed: %s: %v", output, err)
	}
}
