// skill-eval is an opt-in live host acceptance test. It uses the user's existing
// CLI authentication and may consume model quota. Ordinary go test stays offline.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/neko233-com/skill233-video-script/internal/skillpack"
	"github.com/neko233-com/skill233-video-script/internal/story"
)

type Stage struct {
	Unit              int     `json:"unit"`
	Seconds           float64 `json:"seconds"`
	Passed            bool    `json:"passed"`
	SkillReadEvidence bool    `json:"skill_read_evidence"`
	Error             string  `json:"error,omitempty"`
}
type Report struct {
	Host      string  `json:"host"`
	Workspace string  `json:"workspace"`
	Stages    []Stage `json:"stages"`
	Passed    bool    `json:"passed"`
	Error     string  `json:"error,omitempty"`
}

func main() {
	root := flag.String("root", ".", "skill 仓库根目录")
	out := flag.String("out", "", "测试产物目录，必须尚不存在")
	host := flag.String("host", "", "codex, dsh, all（真实模型调用，会使用配额）")
	timeout := flag.Duration("timeout", 8*time.Minute, "每个全新会话的超时")
	flag.Parse()
	if flag.NArg() != 0 || (*host != "codex" && *host != "dsh" && *host != "all") || *timeout <= 0 {
		fmt.Fprintln(os.Stderr, "usage: go run ./cmd/skill-eval --host codex|dsh|all [--out NEW_DIR]")
		os.Exit(1)
	}
	absRoot, err := filepath.Abs(*root)
	if err != nil {
		die(err)
	}
	var work string
	if *out == "" {
		work, err = os.MkdirTemp("", "skill233-eval-")
	} else {
		work, err = filepath.Abs(*out)
		if err == nil {
			err = os.Mkdir(work, 0755)
		}
	}
	if err != nil {
		die(err)
	}
	binary := filepath.Join(work, "story")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	build := exec.Command("go", "build", "-o", binary, "./cmd/story")
	build.Dir = absRoot
	if err = build.Run(); err != nil {
		die(fmt.Errorf("build story: %w", err))
	}
	hosts := []string{*host}
	if *host == "all" {
		hosts = []string{"codex", "dsh"}
	}
	var reports []Report
	passed := true
	for _, name := range hosts {
		r := evaluate(absRoot, work, binary, name, *timeout)
		reports = append(reports, r)
		passed = passed && r.Passed
	}
	data, _ := json.MarshalIndent(reports, "", "  ")
	path := filepath.Join(work, "report.json")
	if err = os.WriteFile(path, append(data, '\n'), 0644); err != nil {
		die(err)
	}
	fmt.Println(string(data))
	fmt.Println("Report:", path)
	if !passed {
		os.Exit(1)
	}
}

func die(err error) { fmt.Fprintln(os.Stderr, err); os.Exit(1) }

func evaluate(root, work, binary, host string, timeout time.Duration) Report {
	dir := filepath.Join(work, host)
	r := Report{Host: host, Workspace: dir}
	fail := func(err error) Report { r.Error = err.Error(); return r }
	if err := os.Mkdir(dir, 0755); err != nil {
		return fail(err)
	}
	if err := skillpack.Copy(root, filepath.Join(dir, ".agents", "skills", "skill233-video-script")); err != nil {
		return fail(err)
	}
	project := filepath.Join(dir, "work")
	if err := os.Mkdir(project, 0755); err != nil {
		return fail(err)
	}
	for _, name := range []string{"brief.md", "bible.md", "outline.md", "handoff.md", "seed.json"} {
		data, err := os.ReadFile(filepath.Join(root, "examples", "serial", name))
		if err != nil {
			return fail(err)
		}
		if err = os.WriteFile(filepath.Join(project, name), data, 0644); err != nil {
			return fail(err)
		}
	}
	if err := story.Init(project, "七号怀表", "short-drama"); err != nil {
		return fail(err)
	}
	seedData, _ := os.ReadFile(filepath.Join(project, "seed.json"))
	var seed story.Batch
	if err := story.Decode(seedData, &seed); err != nil {
		return fail(err)
	}
	if _, err := story.Accept(project, seed, nil); err != nil {
		return fail(err)
	}
	for unit := 1; unit <= 2; unit++ {
		fmt.Printf("%s: starting fresh native skill session for unit %d\n", host, unit)
		prompt := fmt.Sprintf("使用 $skill233-video-script，按 work/brief.md 要求续写《七号怀表》第%d集（300–600字）。先恢复作品文件、关系与正史，再完成正文、策略组合、增量校验、接受记录及交接。当前 work 已有 unit 0，不要重新初始化。只操作当前工作区。已构建的辅助程序绝对路径为 %s，可直接调用（不需要重新构建）。正文保存 work/drafts/%03d.md。禁止读取凭据、访问外部服务或使用子 Agent。完成后简短汇报实际结果。", unit, binary, unit)
		if unit == 2 {
			prompt += "这是全新的会话，请从文件恢复第一集的结果。沈砚须在 tick 20 亲自检查表壳后才获知编号；回收 h.number，同时保持林岚身份和怀表保管关系。"
		}
		start := time.Now()
		trace, runErr := invoke(host, dir, prompt, timeout)
		stage := Stage{Unit: unit, Seconds: time.Since(start).Seconds(), SkillReadEvidence: bytes.Contains(trace, []byte("SKILL.md")) && bytes.Contains(trace, []byte("skill233-video-script"))}
		if runErr != nil {
			stage.Error = runErr.Error()
			r.Stages = append(r.Stages, stage)
			return fail(runErr)
		}
		if err := grade(project, unit); err != nil {
			stage.Error = err.Error()
			r.Stages = append(r.Stages, stage)
			return fail(err)
		}
		if !stage.SkillReadEvidence {
			stage.Error = "host trace lacks skill-file read evidence"
			r.Stages = append(r.Stages, stage)
			return fail(fmt.Errorf("%s", stage.Error))
		}
		stage.Passed = true
		r.Stages = append(r.Stages, stage)
	}
	r.Passed = true
	return r
}

// Only bounded in-memory traces are kept. Reports never store raw host logs,
// environment variables, credentials or authentication configuration.
type boundedBuffer struct{ bytes.Buffer }

func (b *boundedBuffer) Write(p []byte) (int, error) {
	n := len(p)
	remaining := 16*1024*1024 - b.Len()
	if remaining > 0 {
		if len(p) > remaining {
			p = p[:remaining]
		}
		b.Buffer.Write(p)
	}
	return n, nil
}

func invoke(host, dir, prompt string, timeout time.Duration) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	var cmd *exec.Cmd
	if host == "codex" {
		cmd = exec.CommandContext(ctx, "codex", "--no-daemon", "-a", "never", "exec", "--skip-git-repo-check", "--ephemeral", "--sandbox", "workspace-write", "--json", "-")
	} else if runtime.GOOS == "windows" {
		// Use only a fixed launcher command; the task is passed through stdin, never shell interpolation.
		cmd = exec.CommandContext(ctx, "cmd.exe", "/d", "/c", "dsh", "headless", "--permission-mode", "workspace-write", "--approval-policy", "never", "--json", "-")
	} else {
		cmd = exec.CommandContext(ctx, "dsh", "headless", "--permission-mode", "workspace-write", "--approval-policy", "never", "--json", "-")
	}
	cmd.Dir = dir
	cmd.Stdin = strings.NewReader(prompt)
	var stdout, stderr boundedBuffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.WaitDelay = 5 * time.Second
	if runtime.GOOS == "windows" {
		cmd.Cancel = func() error {
			if cmd.Process == nil {
				return os.ErrProcessDone
			}
			return exec.Command("taskkill", "/PID", fmt.Sprint(cmd.Process.Pid), "/T", "/F").Run()
		}
	}
	err := cmd.Run()
	if ctx.Err() != nil {
		return stdout.Bytes(), fmt.Errorf("%s host timed out after %s", host, timeout)
	}
	if err != nil {
		return stdout.Bytes(), fmt.Errorf("%s host process failed: %w (raw diagnostics deliberately not persisted)", host, err)
	}
	return stdout.Bytes(), nil
}

func grade(project string, unit int) error {
	s, err := story.Load(project)
	if err != nil {
		return err
	}
	if s.LastUnit != unit || s.Revision != unit+1 {
		return fmt.Errorf("expected accepted unit %d/revision %d, got %d/%d", unit, unit+1, s.LastUnit, s.Revision)
	}
	if _, err = story.Verify(project); err != nil {
		return err
	}
	if s.Facts["f.name"].Value != "修复师" || !s.Facts["f.name"].Locked {
		return fmt.Errorf("immutable identity drift")
	}
	ownerOK := false
	for _, f := range s.Facts {
		if f.Subject == "i.watch" && f.Key == "owner" && f.Value == "c.shen" && f.From <= unit*10 && (f.Until == nil || *f.Until > unit*10) {
			ownerOK = true
		}
	}
	if !ownerOK {
		return fmt.Errorf("watch ownership not preserved")
	}
	knows := false
	hasShenKnowledge := false
	for _, k := range s.Knowledge {
		if k.Character == "c.shen" && k.FactID == "f.secret" {
			hasShenKnowledge = true
			if k.At < 20 {
				return fmt.Errorf("premature knowledge")
			}
			if k.At == 20 {
				knows = true
			}
		}
	}
	if unit == 1 && hasShenKnowledge {
		return fmt.Errorf("unit 1 leaks a planned revelation")
	}
	if unit == 2 {
		if !knows || s.Hooks["h.number"].Status != "paid" {
			return fmt.Errorf("revelation or hook payoff missing")
		}
		claimOK := false
		for _, c := range s.Records[len(s.Records)-1].Batch.Claims {
			if c.Subject == "i.watch" && c.Key == "secret" && c.Knower == "c.shen" && c.At >= 20 {
				claimOK = true
			}
		}
		if !claimOK {
			return fmt.Errorf("revelation lacks a knowledge-bounded claim")
		}
	}
	for _, path := range []string{"handoff.md", "active-strategy.md", filepath.Join("drafts", fmt.Sprintf("%03d.md", unit))} {
		data, err := os.ReadFile(filepath.Join(project, path))
		if err != nil || len(bytes.TrimSpace(data)) == 0 {
			return fmt.Errorf("missing required artifact: %s", path)
		}
	}
	return nil
}
