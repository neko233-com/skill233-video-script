// Package skillpack copies a self-contained native Agent Skill without host configuration edits.
package skillpack

import (
	"fmt"
	"os"
	"path/filepath"
)

func Copy(source, target string) error {
	if _, err := os.Stat(filepath.Join(source, "SKILL.md")); err != nil {
		return err
	}
	if _, err := os.Lstat(target); err == nil {
		return fmt.Errorf("target already exists; refusing to overwrite: %s", target)
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		return err
	}
	if err := os.Mkdir(target, 0755); err != nil {
		return err
	}
	// An interrupted installation may leave a partial folder. Never delete or replace
	// an existing user skill; report the path so the user can inspect it.
	for _, name := range []string{"SKILL.md", "go.mod", "LICENSE", "VALIDATION.md"} {
		data, err := os.ReadFile(filepath.Join(source, name))
		if err != nil {
			return fmt.Errorf("partial install %s: %w", target, err)
		}
		if err = os.WriteFile(filepath.Join(target, name), data, 0644); err != nil {
			return err
		}
	}
	for _, name := range []string{"agents", "references", "strategies", "examples", "cmd/story", "internal/story"} {
		if err := os.CopyFS(filepath.Join(target, name), os.DirFS(filepath.Join(source, name))); err != nil {
			return fmt.Errorf("partial install %s: %w", target, err)
		}
	}
	// The story CLI deliberately has no dependency on installation or evaluation code.
	return nil
}
