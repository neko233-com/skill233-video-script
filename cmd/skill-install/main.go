package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/neko233-com/skill233-video-script/internal/skillpack"
)

func main() {
	source := flag.String("source", ".", "仓库根目录")
	target := flag.String("target", "", "安装目录，例如 ~/.agents/skills/skill233-video-script（请展开 ~）")
	flag.Parse()
	if *target == "" || flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "usage: skill-install --source ROOT --target ABSOLUTE_SKILL_DIRECTORY")
		os.Exit(1)
	}
	absSource, err := filepath.Abs(*source)
	if err == nil {
		if !filepath.IsAbs(*target) {
			err = fmt.Errorf("--target must be absolute")
		} else {
			err = skillpack.Copy(absSource, filepath.Clean(*target))
		}
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("Installed native skill:", filepath.Join(*target, "SKILL.md"))
}
