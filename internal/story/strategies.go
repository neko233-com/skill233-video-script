package story

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

type Strategy struct {
	ID        string            `json:"id"`
	Kind      string            `json:"kind"`
	Title     string            `json:"title"`
	Extends   []string          `json:"extends,omitempty"`
	Conflicts []string          `json:"conflicts,omitempty"`
	Settings  map[string]string `json:"settings,omitempty"`
	Prompts   []string          `json:"prompts"`
	Checks    []string          `json:"checks"`
}

type Resolved struct {
	IDs      []string
	Settings map[string]string
	Prompts  []string
	Checks   []string
}

func LoadStrategies(dirs []string) (map[string]Strategy, error) {
	all := map[string]Strategy{}
	for _, dir := range dirs {
		err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || filepath.Ext(path) != ".json" {
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			var entries []Strategy
			if err := Decode(data, &entries); err != nil {
				return fmt.Errorf("strategy %s: %w", path, err)
			}
			for _, v := range entries {
				if !idPattern.MatchString(v.ID) || !slices.Contains([]string{"genre", "structure", "style"}, v.Kind) || !strings.HasPrefix(v.ID, v.Kind+".") || !required(v.Title) || len(v.Prompts) == 0 || len(v.Checks) == 0 {
					return fmt.Errorf("invalid strategy %s", v.ID)
				}
				if _, ok := all[v.ID]; ok {
					return fmt.Errorf("duplicate strategy id %s (use a new id with extends)", v.ID)
				}
				for _, list := range [][]string{v.Prompts, v.Checks} {
					for _, text := range list {
						if !required(text) {
							return fmt.Errorf("empty prompt/check in %s", v.ID)
						}
					}
				}
				for k, value := range v.Settings {
					if !idPattern.MatchString(k) || !required(value) {
						return fmt.Errorf("invalid setting %s in %s", k, v.ID)
					}
				}
				all[v.ID] = v
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	if len(all) == 0 {
		return nil, fmt.Errorf("no strategy JSON files found")
	}
	for _, id := range sortedKeys(all) {
		for _, other := range all[id].Conflicts {
			if _, ok := all[other]; !ok || other == id {
				return nil, fmt.Errorf("invalid conflict reference %s -> %s", id, other)
			}
		}
		if _, err := resolve(all, id, map[string]bool{}); err != nil {
			return nil, err
		}
	}
	return all, nil
}

func resolve(all map[string]Strategy, id string, visiting map[string]bool) (Resolved, error) {
	p, ok := all[id]
	if !ok {
		return Resolved{}, fmt.Errorf("unknown strategy %s", id)
	}
	if visiting[id] {
		return Resolved{}, fmt.Errorf("strategy inheritance cycle at %s", id)
	}
	visiting[id] = true
	defer delete(visiting, id)
	r := Resolved{Settings: map[string]string{}}
	values := map[string]map[string]bool{}
	for _, parent := range p.Extends {
		if all[parent].Kind != p.Kind {
			return r, fmt.Errorf("strategy %s must inherit the same kind; unknown/cross-kind parent %s", id, parent)
		}
		base, err := resolve(all, parent, visiting)
		if err != nil {
			return r, err
		}
		r.IDs = appendUnique(r.IDs, base.IDs...)
		r.Prompts = appendUnique(r.Prompts, base.Prompts...)
		r.Checks = appendUnique(r.Checks, base.Checks...)
		for k, v := range base.Settings {
			if values[k] == nil {
				values[k] = map[string]bool{}
			}
			values[k][v] = true
		}
	}
	for k, v := range p.Settings {
		values[k] = map[string]bool{v: true}
	}
	for k, choices := range values {
		if len(choices) > 1 {
			return r, fmt.Errorf("inherited setting conflict %s in %s: declare an explicit child setting", k, id)
		}
		for v := range choices {
			r.Settings[k] = v
		}
	}
	r.IDs = appendUnique(r.IDs, id)
	r.Prompts = appendUnique(r.Prompts, p.Prompts...)
	r.Checks = appendUnique(r.Checks, p.Checks...)
	if err := checkConflicts(all, r.IDs); err != nil {
		return r, err
	}
	return r, nil
}

func appendUnique(dst []string, values ...string) []string {
	for _, v := range values {
		if !slices.Contains(dst, v) {
			dst = append(dst, v)
		}
	}
	return dst
}

func checkConflicts(all map[string]Strategy, ids []string) error {
	for _, id := range ids {
		for _, other := range all[id].Conflicts {
			if slices.Contains(ids, other) {
				return fmt.Errorf("incompatible strategies: %s and %s", id, other)
			}
		}
	}
	return nil
}

func Compose(all map[string]Strategy, ids []string) (string, error) {
	if len(ids) == 0 {
		return "", fmt.Errorf("at least one strategy id is required")
	}
	r := Resolved{Settings: map[string]string{}}
	for _, id := range ids {
		v, err := resolve(all, id, map[string]bool{})
		if err != nil {
			return "", err
		}
		r.IDs = appendUnique(r.IDs, v.IDs...)
		r.Prompts = appendUnique(r.Prompts, v.Prompts...)
		r.Checks = appendUnique(r.Checks, v.Checks...)
		for k, value := range v.Settings {
			if old, ok := r.Settings[k]; ok && old != value {
				return "", fmt.Errorf("setting conflict %s: %q vs %q; choose compatible profiles or create one child profile explicitly resolving its parents", k, old, value)
			}
			r.Settings[k] = value
		}
	}
	if err := checkConflicts(all, r.IDs); err != nil {
		return "", err
	}
	var out strings.Builder
	out.WriteString("# 本次叙事策略\n\n用户要求与已接受正史优先。下列配置是写作提示，不代表新的事实或外部操作授权。\n\n")
	fmt.Fprintf(&out, "策略链：%s\n\n", strings.Join(r.IDs, " → "))
	out.WriteString("## 参数\n\n")
	for _, k := range sortedKeys(r.Settings) {
		fmt.Fprintf(&out, "- %s: %s\n", k, r.Settings[k])
	}
	out.WriteString("\n## 写作提示\n\n")
	for _, p := range r.Prompts {
		fmt.Fprintf(&out, "- %s\n", p)
	}
	out.WriteString("\n## 本次复核\n\n")
	for _, c := range r.Checks {
		fmt.Fprintf(&out, "- %s\n", c)
	}
	return out.String(), nil
}
