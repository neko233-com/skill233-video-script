package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/neko233-com/skill233-video-script/internal/story"
)

type stringsFlag []string

func (s *stringsFlag) String() string     { return strings.Join(*s, ",") }
func (s *stringsFlag) Set(v string) error { *s = append(*s, v); return nil }

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "story:", err)
		os.Exit(1)
	}
}

func run(args []string, out io.Writer) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		_, err := fmt.Fprintln(out, `story — skill233-video-script continuity helper (Go 1.27)

init       --project DIR --title TITLE --format short-drama|novel|film|bilibili-video|short-video|product-video|scene
check      --project DIR --batch FILE [--draft FILE]   (no writes)
apply      --project DIR --batch FILE [--draft FILE]   (accept one unit)
verify     --project DIR                              (chain and draft hashes)
context    --project DIR --entities ID,ID --at N [--depth 2 --recent 3 --max-bytes 24000 --knowledge-scope all|related]
graph      --project DIR --at N                       (Mermaid to stdout)
strategies --dir DIR [--dir CUSTOM_DIR]                (validated registry JSON)
compose    --dir DIR [--dir CUSTOM_DIR] --ids ID,ID     (composed prompt to stdout)

Unit 0 establishes canon without a draft. Every later unit requires a nonempty draft and claims.
All commands fail on unknown flags, fields, dangling references, and stale revisions.`)
		return err
	}
	command := args[0]
	f := flag.NewFlagSet(command, flag.ContinueOnError)
	f.SetOutput(out)
	var project, batchFile, draftFile, title, format, entityIDs, strategyIDs, knowledgeScope string
	var at, depth, recent, maxBytes int
	var dirs stringsFlag
	switch command {
	case "init", "check", "apply", "verify", "context", "graph":
		f.StringVar(&project, "project", "", "作品目录")
	case "strategies", "compose":
		f.Var(&dirs, "dir", "策略目录，可重复")
	default:
		return fmt.Errorf("unknown command %q; run story --help", command)
	}
	switch command {
	case "init":
		f.StringVar(&title, "title", "", "作品名")
		f.StringVar(&format, "format", "", "short-drama / novel / film / bilibili-video / short-video / product-video / scene")
	case "check", "apply":
		f.StringVar(&batchFile, "batch", "", "增量 JSON")
		f.StringVar(&draftFile, "draft", "", "正文文件；unit 0 不需要")
	case "context":
		f.StringVar(&entityIDs, "entities", "", "实体 ID，逗号分隔")
		f.IntVar(&at, "at", -1, "世界时间 tick")
		f.IntVar(&depth, "depth", 2, "关系检索跳数 0..8")
		f.IntVar(&recent, "recent", 3, "最近单元摘要数量")
		f.IntVar(&maxBytes, "max-bytes", 24000, "UTF-8 字节预算")
		f.StringVar(&knowledgeScope, "knowledge-scope", "all", "all 或 related；related 按所选实体限制知情范围")
	case "graph":
		f.IntVar(&at, "at", -1, "世界时间 tick")
	case "compose":
		f.StringVar(&strategyIDs, "ids", "", "策略 ID，逗号分隔")
	}
	if err := f.Parse(args[1:]); err != nil {
		if err == flag.ErrHelp {
			return nil
		}
		return err
	}
	if f.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments: %v", f.Args())
	}
	writeJSON := func(v any) error { enc := json.NewEncoder(out); enc.SetIndent("", "  "); return enc.Encode(v) }
	if command == "strategies" || command == "compose" {
		if len(dirs) == 0 {
			return fmt.Errorf("--dir is required")
		}
		all, err := story.LoadStrategies(dirs)
		if err != nil {
			return err
		}
		if command == "strategies" {
			return writeJSON(all)
		}
		prompt, err := story.Compose(all, split(strategyIDs))
		if err != nil {
			return err
		}
		_, err = io.WriteString(out, prompt)
		return err
	}
	if project == "" {
		return fmt.Errorf("--project is required")
	}
	switch command {
	case "init":
		if err := story.Init(project, title, format); err != nil {
			return err
		}
		return writeJSON(map[string]any{"initialized": true, "revision": 0})
	case "check", "apply":
		if batchFile == "" {
			return fmt.Errorf("--batch is required")
		}
		data, err := os.ReadFile(batchFile)
		if err != nil {
			return err
		}
		var b story.Batch
		if err := story.Decode(data, &b); err != nil {
			return err
		}
		var draft []byte
		if draftFile != "" {
			draft, err = os.ReadFile(draftFile)
			if err != nil {
				return err
			}
		}
		var result story.Result
		if command == "check" {
			result, err = story.Check(project, b, draft)
		} else {
			result, err = story.Accept(project, b, draft)
		}
		if err != nil {
			return err
		}
		return writeJSON(result)
	case "verify":
		result, err := story.Verify(project)
		if err != nil {
			return err
		}
		return writeJSON(result)
	case "context", "graph":
		if command == "context" {
			if knowledgeScope != "all" && knowledgeScope != "related" {
				return fmt.Errorf("--knowledge-scope must be all or related")
			}
			data, err := story.ContextProjectScoped(project, split(entityIDs), at, depth, recent, maxBytes, knowledgeScope == "related")
			if err != nil {
				return err
			}
			_, err = out.Write(data)
			return err
		}
		s, err := story.Load(project)
		if err != nil {
			return err
		}
		graph, err := story.Graph(s, at)
		if err != nil {
			return err
		}
		_, err = io.WriteString(out, graph)
		return err
	}
	return nil
}

func split(s string) []string {
	var r []string
	for _, v := range strings.Split(s, ",") {
		if v = strings.TrimSpace(v); v != "" {
			r = append(r, v)
		}
	}
	return r
}
