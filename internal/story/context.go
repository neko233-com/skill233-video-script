package story

import (
	"encoding/json"
	"fmt"
	"html"
	"strings"
)

type Summary struct {
	Revision int    `json:"revision"`
	Unit     int    `json:"unit"`
	Summary  string `json:"summary"`
	Draft    *Draft `json:"draft,omitempty"`
}

type Packet struct {
	Project           Project           `json:"project"`
	Revision          int               `json:"revision"`
	NextUnit          int               `json:"next_unit"`
	At                int               `json:"at"`
	Entities          []Entity          `json:"entities"`
	Facts             []Fact            `json:"facts"`
	Relations         []Relation        `json:"relations"`
	BoundaryRelations []Relation        `json:"boundary_relations"`
	Knowledge         []Knowledge       `json:"knowledge"`
	KnowledgeScope    []string          `json:"knowledge_scope"`
	Events            []Event           `json:"events"`
	Hooks             []Hook            `json:"hooks"`
	Recent            []Summary         `json:"recent"`
	Origins           map[string]Origin `json:"origins"`
	OmittedEntities   int               `json:"omitted_entity_count"`
	OmittedOptional   int               `json:"omitted_optional_count"`
	Notice            string            `json:"notice"`
}

// Context never truncates canon, knowledge dependencies, or relevant/due hooks.
func Context(s *State, ids []string, at, depth, recent, maxBytes int) ([]byte, error) {
	if at < 0 || depth < 0 || depth > 8 || recent < 0 || maxBytes < 512 {
		return nil, fmt.Errorf("require at>=0, depth 0..8, recent>=0, max-bytes>=512")
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("provide at least one entity id")
	}
	selected := map[string]bool{}
	for _, id := range ids {
		if _, ok := s.Entities[id]; !ok {
			return nil, fmt.Errorf("unknown entity %s", id)
		}
		selected[id] = true
	}
	for i := 0; i < depth; i++ {
		next := map[string]bool{}
		for id := range selected {
			next[id] = true
		}
		for _, r := range s.Relations {
			if active(r.Since, r.Until, at) && (selected[r.From] || selected[r.To]) {
				next[r.From] = true
				next[r.To] = true
			}
		}
		selected = next
	}
	p := Packet{Project: s.Project, Revision: s.Revision, NextUnit: s.LastUnit + 1, At: at, Origins: map[string]Origin{}, Notice: "作者上下文含秘密真相；人物只能使用截至 at 获知的具体事实版本。knowledge_scope 列出本次完整查询知情的角色，其他人物需补查。boundary_relations 需要按需补查。正文语义仍需 Agent 复核。"}
	neededFacts := map[string]bool{}
	addEntity := func(id string) {
		if !selected[id] {
			selected[id] = true
		}
	}
	for _, id := range sortedKeys(s.Hooks) {
		h := s.Hooks[id]
		relevant := h.Status == "open" && h.DueUnit <= s.LastUnit+4
		for _, entity := range h.Entities {
			relevant = relevant || (h.Status == "open" && selected[entity])
		}
		if relevant {
			p.Hooks = append(p.Hooks, h)
			p.Origins[id] = s.Origins[id]
			for _, e := range h.Entities {
				addEntity(e)
			}
		}
	}
	for _, id := range sortedKeys(selected) {
		if s.Entities[id].Kind == "character" {
			p.KnowledgeScope = append(p.KnowledgeScope, id)
		}
	}
	for _, id := range sortedKeys(s.Knowledge) {
		k := s.Knowledge[id]
		if selected[k.Character] && k.At <= at {
			p.Knowledge = append(p.Knowledge, k)
			neededFacts[k.FactID] = true
			p.Origins[id] = s.Origins[id]
		}
	}
	for _, id := range sortedKeys(s.Facts) {
		f := s.Facts[id]
		if f.Locked || neededFacts[id] || (selected[f.Subject] && active(f.From, f.Until, at)) {
			p.Facts = append(p.Facts, f)
			addEntity(f.Subject)
			p.Origins[id] = s.Origins[id]
		}
	}
	for _, id := range sortedKeys(s.Entities) {
		if selected[id] {
			p.Entities = append(p.Entities, s.Entities[id])
			p.Origins[id] = s.Origins[id]
		}
	}
	for _, id := range sortedKeys(s.Relations) {
		r := s.Relations[id]
		if !active(r.Since, r.Until, at) {
			continue
		}
		if selected[r.From] && selected[r.To] {
			p.Relations = append(p.Relations, r)
			p.Origins[id] = s.Origins[id]
		} else if selected[r.From] || selected[r.To] {
			p.BoundaryRelations = append(p.BoundaryRelations, r)
			p.Origins[id] = s.Origins[id]
		}
	}
	for _, id := range sortedKeys(s.Events) {
		e := s.Events[id]
		if e.At > at {
			continue
		}
		for _, participant := range e.Participants {
			if selected[participant] {
				p.Events = append(p.Events, e)
				break
			}
		}
	}
	for i := len(s.Records) - recent; i < len(s.Records); i++ {
		if i >= 0 {
			r := s.Records[i]
			p.Recent = append(p.Recent, Summary{r.Revision, r.Batch.Unit, r.Batch.Summary, r.Draft})
		}
	}
	p.OmittedEntities = len(s.Entities) - len(p.Entities)
	for {
		data, err := json.MarshalIndent(p, "", "  ")
		if err != nil {
			return nil, err
		}
		if len(data)+1 <= maxBytes {
			return append(data, '\n'), nil
		}
		// Optional old event summaries and oldest recent summaries can be retrieved separately.
		if len(p.Events) > 0 {
			p.Events = p.Events[1:]
			p.OmittedOptional++
			continue
		}
		if len(p.Recent) > 1 {
			p.Recent = p.Recent[1:]
			p.OmittedOptional++
			continue
		}
		return nil, fmt.Errorf("essential context exceeds %d UTF-8 bytes (needs %d); narrow entities/depth or raise budget; no canon was silently dropped", maxBytes, len(data)+1)
	}
}

func Graph(s *State, at int) (string, error) {
	if at < 0 {
		return "", fmt.Errorf("at must be >=0")
	}
	var out strings.Builder
	out.WriteString("flowchart LR\n")
	nodes := map[string]string{}
	label := func(s string) string {
		return strings.ReplaceAll(strings.ReplaceAll(html.EscapeString(s), "\r", " "), "\n", " ")
	}
	for i, id := range sortedKeys(s.Entities) {
		nodes[id] = fmt.Sprintf("n%d", i)
		fmt.Fprintf(&out, "  %s[\"%s · %s\"]\n", nodes[id], label(s.Entities[id].Name), label(id))
	}
	for _, id := range sortedKeys(s.Relations) {
		r := s.Relations[id]
		if active(r.Since, r.Until, at) {
			fmt.Fprintf(&out, "  %s -->|\"%s · %s\"| %s\n", nodes[r.From], label(r.Kind), label(r.ID), nodes[r.To])
		}
	}
	return out.String(), nil
}
