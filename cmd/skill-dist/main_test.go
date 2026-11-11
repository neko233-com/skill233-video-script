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

	// Each run launches a separate native process, like restarting a host between batches.
	drama := filepath.Join(install, "短剧")
	run("init", "--project", drama, "--title", "七号怀表", "--format", "short-drama")
	run("apply", "--project", drama, "--batch", filepath.Join(skill, "examples", "serial", "seed.json"))
	run("check", "--project", drama, "--batch", filepath.Join(skill, "examples", "serial", "unit-001.json"), "--draft", filepath.Join(skill, "examples", "serial", "001.md"))
	run("apply", "--project", drama, "--batch", filepath.Join(skill, "examples", "serial", "unit-001.json"), "--draft", filepath.Join(skill, "examples", "serial", "001.md"))
	secondDraft := filepath.Join(install, "002.md")
	secondBatch := filepath.Join(install, "unit-002.json")
	draft := `# 第2集：七号档案

2-1 修复室 内 夜（tick 20）

沈砚紧握软布里的怀表。林岚拿来工具，指着表壳的凹槽。
林岚：现在可以打开了。你看，我记录，你保管。
沈砚依照她的指示撬开表壳，看见内壁的编号七。
沈砚：七。你早就知道？
林岚：知道编号，不知道它指谁。
沈砚把怀表收好，两人对照刚送来的档案。第七项写着林岚失踪的父亲，字迹与维修单相同。
门外的衣角不见了。档案背面露出一行新字：找到了人，就得交出表。
`
	batch := `{"base_revision":2,"unit":2,"summary":"沈砚开壳获知编号，二人从档案确认编号指向林岚的父亲",` +
		`"knowledge":[{"id":"k.shen","character":"c.shen","fact_id":"f.secret","at":20,"evidence":"2-1 沈砚打开表壳"}],` +
		`"events":[{"id":"e.open","at":20,"participants":["c.lin","c.shen","i.watch"],"summary":"沈砚打开怀表","causes":["e.transfer"],"requires":["f.owner2"],"evidence":"2-1"}],` +
		`"hook_updates":[{"id":"h.number","status":"paid","evidence":"2-1 第七项写着林岚的父亲"}],` +
		`"claims":[{"subject":"i.watch","key":"secret","value":"表壳内有编号七","at":20,"knower":"c.shen","evidence":"2-1 沈砚读出编号"},` +
		`{"subject":"i.watch","key":"owner","value":"c.shen","at":20,"evidence":"2-1 沈砚保管怀表"}]}`
	if err = os.WriteFile(secondDraft, []byte(draft), 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(secondBatch, []byte(batch), 0600); err != nil {
		t.Fatal(err)
	}
	before := run("context", "--project", drama, "--entities", "c.lin,c.shen,i.watch", "--at", "10", "--knowledge-scope", "related")
	if !strings.Contains(before, "f.owner2") || strings.Contains(before, "k.shen") {
		t.Fatalf("incorrect first-episode handoff: %s", before)
	}
	run("check", "--project", drama, "--batch", secondBatch, "--draft", secondDraft)
	run("apply", "--project", drama, "--batch", secondBatch, "--draft", secondDraft)
	after := run("context", "--project", drama, "--entities", "c.lin,c.shen,i.watch", "--at", "20", "--knowledge-scope", "related")
	if !strings.Contains(after, "k.shen") || !strings.Contains(after, `"next_unit": 3`) {
		t.Fatalf("incorrect second-episode handoff: %s", after)
	}
	verified := run("verify", "--project", drama)
	if !strings.Contains(verified, `"revision": 3`) || !strings.Contains(verified, `"unit": 2`) || strings.Contains(verified, "overdue hook") {
		t.Fatalf("short-drama full audit failed: %s", verified)
	}
}
