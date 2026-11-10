package story

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
)

const Schema = 1

type Project struct {
	Schema int    `json:"schema"`
	Title  string `json:"title"`
	Format string `json:"format"`
}

type Entity struct {
	ID          string   `json:"id"`
	Kind        string   `json:"kind"`
	Name        string   `json:"name"`
	Aliases     []string `json:"aliases,omitempty"`
	Description string   `json:"description"`
}

type Fact struct {
	ID         string `json:"id"`
	Subject    string `json:"subject"`
	Key        string `json:"key"`
	Value      string `json:"value"`
	From       int    `json:"from"`
	Until      *int   `json:"until,omitempty"`
	Locked     bool   `json:"locked,omitempty"`
	Supersedes string `json:"supersedes,omitempty"`
	Evidence   string `json:"evidence"`
}

type Relation struct {
	ID         string `json:"id"`
	From       string `json:"from"`
	To         string `json:"to"`
	Kind       string `json:"kind"`
	Since      int    `json:"since"`
	Until      *int   `json:"until,omitempty"`
	Supersedes string `json:"supersedes,omitempty"`
	Evidence   string `json:"evidence"`
}

type Knowledge struct {
	ID        string `json:"id"`
	Character string `json:"character"`
	FactID    string `json:"fact_id"`
	At        int    `json:"at"`
	Evidence  string `json:"evidence"`
}

type Event struct {
	ID           string   `json:"id"`
	At           int      `json:"at"`
	Participants []string `json:"participants"`
	Summary      string   `json:"summary"`
	Causes       []string `json:"causes,omitempty"`
	Requires     []string `json:"requires,omitempty"`
	Evidence     string   `json:"evidence"`
}

type Hook struct {
	ID        string   `json:"id"`
	Question  string   `json:"question"`
	DueUnit   int      `json:"due_unit"`
	Entities  []string `json:"entities"`
	Evidence  string   `json:"evidence"`
	PlantUnit int      `json:"plant_unit"`
	Status    string   `json:"status"`
}

type HookUpdate struct {
	ID       string `json:"id"`
	Status   string `json:"status"`
	Evidence string `json:"evidence"`
	Reason   string `json:"reason,omitempty"`
}

// Claim is an assertion extracted by the writing agent, not a prose parser.
type Claim struct {
	Subject  string `json:"subject"`
	Key      string `json:"key"`
	Value    string `json:"value"`
	At       int    `json:"at"`
	Knower   string `json:"knower,omitempty"`
	Evidence string `json:"evidence"`
}

type Batch struct {
	BaseRevision int          `json:"base_revision"`
	Unit         int          `json:"unit"`
	Summary      string       `json:"summary"`
	Entities     []Entity     `json:"entities,omitempty"`
	Facts        []Fact       `json:"facts,omitempty"`
	Relations    []Relation   `json:"relations,omitempty"`
	Knowledge    []Knowledge  `json:"knowledge,omitempty"`
	Events       []Event      `json:"events,omitempty"`
	Hooks        []Hook       `json:"hooks,omitempty"`
	HookUpdates  []HookUpdate `json:"hook_updates,omitempty"`
	Claims       []Claim      `json:"claims,omitempty"`
}

type Draft struct {
	SHA256 string `json:"sha256"`
}

type Record struct {
	Schema   int    `json:"schema"`
	Revision int    `json:"revision"`
	PrevHash string `json:"prev_hash"`
	Hash     string `json:"hash"`
	Batch    Batch  `json:"batch"`
	Draft    *Draft `json:"draft,omitempty"`
}

type Origin struct {
	Revision int `json:"revision"`
	Unit     int `json:"unit"`
}

type State struct {
	Project    Project
	Revision   int
	LastUnit   int
	Head       string
	Entities   map[string]Entity
	Facts      map[string]Fact
	Relations  map[string]Relation
	Knowledge  map[string]Knowledge
	Events     map[string]Event
	Hooks      map[string]Hook
	Origins    map[string]Origin
	Records    []Record
	factIndex  map[string][]string
	knownIndex map[string][]string
	assertUses map[string][]Claim
	eventUses  map[string][]string
}

func NewState(p Project) *State {
	return &State{Project: p, LastUnit: -1, Entities: map[string]Entity{}, Facts: map[string]Fact{},
		Relations: map[string]Relation{}, Knowledge: map[string]Knowledge{}, Events: map[string]Event{},
		Hooks: map[string]Hook{}, Origins: map[string]Origin{}, factIndex: map[string][]string{},
		knownIndex: map[string][]string{}, assertUses: map[string][]Claim{}, eventUses: map[string][]string{}}
}

var idPattern = regexp.MustCompile(`^[a-z][a-z0-9._-]{0,95}$`)

func required(values ...string) bool {
	for _, s := range values {
		if strings.TrimSpace(s) == "" {
			return false
		}
	}
	return true
}

func active(from int, until *int, at int) bool {
	return at >= from && (until == nil || at < *until)
}

func overlap(a int, aEnd *int, b int, bEnd *int) bool {
	return (aEnd == nil || b < *aEnd) && (bEnd == nil || a < *bEnd)
}

func validInterval(from int, until *int) bool {
	return from >= 0 && (until == nil || *until > from)
}

// Apply mutates an isolated replay state. Callers must discard it on failure.
func (s *State) Apply(b Batch) ([]string, error) {
	if b.BaseRevision != s.Revision {
		return nil, fmt.Errorf("stale revision: got %d, current %d", b.BaseRevision, s.Revision)
	}
	if b.Unit != s.LastUnit+1 {
		return nil, fmt.Errorf("unit must be %d, got %d", s.LastUnit+1, b.Unit)
	}
	if !required(b.Summary) {
		return nil, fmt.Errorf("summary is required")
	}
	if b.Unit > 0 && len(b.Claims) == 0 {
		return nil, fmt.Errorf("正文单元必须提供关键事实 claims；无状态变化也需核对")
	}
	addID := func(id string) error {
		if !idPattern.MatchString(id) {
			return fmt.Errorf("invalid id %q", id)
		}
		if _, ok := s.Origins[id]; ok {
			return fmt.Errorf("duplicate id %s", id)
		}
		s.Origins[id] = Origin{s.Revision + 1, b.Unit}
		return nil
	}
	entityExists := func(id string) bool { _, ok := s.Entities[id]; return ok }
	aliasOwners := map[string]string{}
	for _, e := range s.Entities {
		for _, name := range append([]string{e.Name}, e.Aliases...) {
			aliasOwners[strings.ToLower(strings.TrimSpace(name))] = e.ID
		}
	}
	for _, e := range b.Entities {
		if err := addID(e.ID); err != nil {
			return nil, err
		}
		if !slices.Contains([]string{"character", "location", "organization", "item", "world"}, e.Kind) || !required(e.Name, e.Description) {
			return nil, fmt.Errorf("entity %s: invalid kind or missing name/description", e.ID)
		}
		for _, name := range append([]string{e.Name}, e.Aliases...) {
			key := strings.ToLower(strings.TrimSpace(name))
			if key == "" {
				return nil, fmt.Errorf("empty alias: %s", e.ID)
			}
			if owner, ok := aliasOwners[key]; ok && owner != e.ID {
				return nil, fmt.Errorf("alias %q belongs to both %s and %s", name, owner, e.ID)
			}
			aliasOwners[key] = e.ID
		}
		s.Entities[e.ID] = e
	}
	for _, f := range b.Facts {
		if err := addID(f.ID); err != nil {
			return nil, err
		}
		if !entityExists(f.Subject) || !required(f.Key, f.Value, f.Evidence) || !validInterval(f.From, f.Until) {
			return nil, fmt.Errorf("invalid fact %s", f.ID)
		}
		if f.Supersedes != "" {
			old, ok := s.Facts[f.Supersedes]
			if !ok || old.Subject != f.Subject || old.Key != f.Key || old.Locked || old.Until != nil || f.From <= old.From {
				return nil, fmt.Errorf("fact %s cannot supersede %s (locked, closed, different key or invalid time)", f.ID, f.Supersedes)
			}
			for _, c := range s.assertUses[old.ID] {
				if c.At >= f.From {
					return nil, fmt.Errorf("invalidates accepted claim at %s: fact version %s ends before assertion", c.Evidence, old.ID)
				}
			}
			for _, id := range s.eventUses[old.ID] {
				if s.Events[id].At >= f.From {
					return nil, fmt.Errorf("event %s: prerequisite %s invalidated", id, old.ID)
				}
			}
			end := f.From
			old.Until = &end
			s.Facts[old.ID] = old
		}
		for _, id := range s.factIndex[f.Subject+"\x00"+f.Key] {
			old := s.Facts[id]
			if old.Subject == f.Subject && old.Key == f.Key && overlap(old.From, old.Until, f.From, f.Until) {
				return nil, fmt.Errorf("conflicting facts %s and %s: %s.%s", old.ID, f.ID, f.Subject, f.Key)
			}
		}
		s.Facts[f.ID] = f
		s.factIndex[f.Subject+"\x00"+f.Key] = append(s.factIndex[f.Subject+"\x00"+f.Key], f.ID)
	}
	for _, r := range b.Relations {
		if err := addID(r.ID); err != nil {
			return nil, err
		}
		if !entityExists(r.From) || !entityExists(r.To) || r.From == r.To || !required(r.Kind, r.Evidence) || !validInterval(r.Since, r.Until) {
			return nil, fmt.Errorf("invalid relation %s", r.ID)
		}
		if r.Supersedes != "" {
			old, ok := s.Relations[r.Supersedes]
			if !ok || old.From != r.From || old.To != r.To || old.Kind != r.Kind || old.Until != nil || r.Since <= old.Since {
				return nil, fmt.Errorf("relation %s cannot supersede %s", r.ID, r.Supersedes)
			}
			end := r.Since
			old.Until = &end
			s.Relations[old.ID] = old
		}
		for _, old := range s.Relations {
			if old.From == r.From && old.To == r.To && old.Kind == r.Kind && overlap(old.Since, old.Until, r.Since, r.Until) {
				return nil, fmt.Errorf("overlapping relations %s and %s", old.ID, r.ID)
			}
		}
		s.Relations[r.ID] = r
	}
	for _, k := range b.Knowledge {
		if err := addID(k.ID); err != nil {
			return nil, err
		}
		f, ok := s.Facts[k.FactID]
		if !ok || s.Entities[k.Character].Kind != "character" || k.At < f.From || !required(k.Evidence) {
			return nil, fmt.Errorf("invalid knowledge %s: unknown fact/character, future fact or missing evidence", k.ID)
		}
		s.Knowledge[k.ID] = k
		s.knownIndex[k.Character+"\x00"+k.FactID] = append(s.knownIndex[k.Character+"\x00"+k.FactID], k.ID)
	}
	for _, e := range b.Events {
		if err := addID(e.ID); err != nil {
			return nil, err
		}
		if e.At < 0 || !required(e.Summary, e.Evidence) || len(e.Participants) == 0 {
			return nil, fmt.Errorf("invalid event %s", e.ID)
		}
		for _, id := range e.Participants {
			if !entityExists(id) {
				return nil, fmt.Errorf("event %s: unknown entity %s", e.ID, id)
			}
		}
		for _, id := range e.Requires {
			f, ok := s.Facts[id]
			if !ok || !active(f.From, f.Until, e.At) {
				return nil, fmt.Errorf("event %s: missing/inactive prerequisite %s", e.ID, id)
			}
		}
		s.Events[e.ID] = e
		for _, id := range e.Requires {
			s.eventUses[id] = append(s.eventUses[id], e.ID)
		}
	}
	// Old nodes cannot gain outgoing edges. Visit new nodes; old prerequisites
	// are guarded by eventUses when a fact version is superseded.
	colors := map[string]int{}
	var visit func(string) error
	visit = func(id string) error {
		if s.Origins[id].Revision <= s.Revision {
			return nil
		}
		if colors[id] == 1 {
			return fmt.Errorf("causal cycle at %s", id)
		}
		if colors[id] == 2 {
			return nil
		}
		colors[id] = 1
		e := s.Events[id]
		for _, dep := range e.Causes {
			cause, ok := s.Events[dep]
			if !ok || cause.At > e.At {
				return fmt.Errorf("event %s: unknown/future cause %s", id, dep)
			}
			if err := visit(dep); err != nil {
				return err
			}
		}
		for _, dep := range e.Requires {
			f := s.Facts[dep]
			if !active(f.From, f.Until, e.At) {
				return fmt.Errorf("event %s: prerequisite %s invalidated", id, dep)
			}
		}
		colors[id] = 2
		return nil
	}
	for _, event := range b.Events {
		if err := visit(event.ID); err != nil {
			return nil, err
		}
	}
	for _, h := range b.Hooks {
		if err := addID(h.ID); err != nil {
			return nil, err
		}
		if h.PlantUnit != b.Unit || h.DueUnit < b.Unit || h.Status != "open" || !required(h.Question, h.Evidence) || len(h.Entities) == 0 {
			return nil, fmt.Errorf("invalid hook %s", h.ID)
		}
		for _, id := range h.Entities {
			if !entityExists(id) {
				return nil, fmt.Errorf("hook %s: unknown entity %s", h.ID, id)
			}
		}
		s.Hooks[h.ID] = h
	}
	for _, u := range b.HookUpdates {
		h, ok := s.Hooks[u.ID]
		if !ok || h.Status != "open" || !slices.Contains([]string{"paid", "retired"}, u.Status) || !required(u.Evidence) || (u.Status == "retired" && !required(u.Reason)) {
			return nil, fmt.Errorf("invalid hook transition %s", u.ID)
		}
		h.Status = u.Status
		h.Evidence = u.Evidence
		s.Hooks[h.ID] = h
	}
	for _, c := range b.Claims {
		if err := s.checkClaim(c); err != nil {
			return nil, err
		}
		for _, id := range s.factIndex[c.Subject+"\x00"+c.Key] {
			f := s.Facts[id]
			if active(f.From, f.Until, c.At) {
				s.assertUses[id] = append(s.assertUses[id], c)
				break
			}
		}
	}
	s.Revision++
	s.LastUnit = b.Unit
	var warnings []string
	for _, id := range sortedKeys(s.Hooks) {
		h := s.Hooks[id]
		if h.Status == "open" && h.DueUnit <= b.Unit {
			warnings = append(warnings, fmt.Sprintf("hook %s due at unit %d: %s", id, h.DueUnit, h.Question))
		}
	}
	return warnings, nil
}

func (s *State) checkClaim(c Claim) error {
	if c.At < 0 || !required(c.Subject, c.Key, c.Value, c.Evidence) {
		return fmt.Errorf("invalid claim at %q", c.Evidence)
	}
	for _, id := range s.factIndex[c.Subject+"\x00"+c.Key] {
		f := s.Facts[id]
		if f.Subject != c.Subject || f.Key != c.Key || !active(f.From, f.Until, c.At) {
			continue
		}
		if f.Value != c.Value {
			return fmt.Errorf("claim conflict at %s: %s.%s=%q, canon %s=%q", c.Evidence, c.Subject, c.Key, c.Value, f.ID, f.Value)
		}
		if c.Knower != "" {
			for _, id := range s.knownIndex[c.Knower+"\x00"+f.ID] {
				k := s.Knowledge[id]
				if k.Character == c.Knower && k.FactID == f.ID && k.At <= c.At {
					return nil
				}
			}
			return fmt.Errorf("knowledge leak at %s: %s does not know %s at %d", c.Evidence, c.Knower, f.ID, c.At)
		}
		return nil
	}
	return fmt.Errorf("unsupported claim at %s: no active fact %s.%s at %d", c.Evidence, c.Subject, c.Key, c.At)
}

func sortedKeys[T any](m map[string]T) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}
