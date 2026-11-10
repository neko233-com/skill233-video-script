package story

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	bolt "go.etcd.io/bbolt"
)

const indexedEventLimit = 32

var errContextQuery = errors.New("invalid context query")

// ContextProject reads only the selected graph neighborhood and the latest
// bounded event summaries. An absent/stale cache is rebuilt from verified logs.
func ContextProject(dir string, ids []string, at, depth, recent, maxBytes int) ([]byte, error) {
	return ContextProjectScoped(dir, ids, at, depth, recent, maxBytes, false)
}

// Related knowledge is explicitly scoped to selected entities and world rules.
// An omitted subject must not be interpreted as unknown to the character.
func ContextProjectScoped(dir string, ids []string, at, depth, recent, maxBytes int, related bool) ([]byte, error) {
	if at < 0 || depth < 0 || depth > 8 || recent < 0 || maxBytes < 512 || len(ids) == 0 {
		return nil, fmt.Errorf("require entities, at>=0, depth 0..8, recent>=0, max-bytes>=512")
	}
	data, err := indexedContext(dir, ids, at, depth, recent, maxBytes, related)
	if err == nil {
		return data, nil
	}
	if errors.Is(err, errContextQuery) {
		return nil, err
	}
	// Reject malformed authority rather than rebuilding an index from bad history.
	s, verifyErr := loadVerified(dir)
	if verifyErr != nil {
		return nil, verifyErr
	}
	if rebuildErr := rebuildCheckpoint(dir, s); rebuildErr != nil {
		// Preserve a corrupted cache, but never move a cache merely locked by another process.
		if !errors.Is(rebuildErr, bolt.ErrTimeout) && !os.IsNotExist(rebuildErr) {
			path := cachePath(dir)
			backup := fmt.Sprintf("%s.corrupt-%d", path, time.Now().UnixNano())
			if _, statErr := os.Lstat(backup); os.IsNotExist(statErr) {
				if moveErr := os.Rename(path, backup); moveErr == nil {
					rebuildErr = rebuildCheckpoint(dir, s)
				}
			}
		}
		if rebuildErr != nil {
			return contextFallback(s, ids, at, depth, recent, maxBytes, related)
		}
	}
	data, err = indexedContext(dir, ids, at, depth, recent, maxBytes, related)
	if errors.Is(err, errContextQuery) {
		return nil, err
	}
	if err != nil {
		return contextFallback(s, ids, at, depth, recent, maxBytes, related)
	}
	return data, nil
}

func contextFallback(s *State, ids []string, at, depth, recent, maxBytes int, related bool) ([]byte, error) {
	var subjects []string
	if related {
		selected := map[string]bool{}
		for _, id := range ids {
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
		for _, h := range s.Hooks {
			relevant := h.Status == "open" && h.DueUnit <= s.LastUnit+4
			for _, id := range h.Entities {
				relevant = relevant || (h.Status == "open" && selected[id])
			}
			if relevant {
				for _, id := range h.Entities {
					selected[id] = true
				}
			}
		}
		for _, f := range s.Facts {
			if f.Locked && s.Entities[f.Subject].Kind == "world" {
				selected[f.Subject] = true
			}
		}
		subjects = sortedKeys(selected)
		for id, k := range s.Knowledge {
			if !selected[s.Facts[k.FactID].Subject] {
				delete(s.Knowledge, id)
			}
		}
	}
	data, err := Context(s, ids, at, depth, recent, maxBytes)
	if err != nil {
		return nil, err
	}
	var p Packet
	if err = Decode(data, &p); err != nil {
		return nil, err
	}
	if related {
		setRelatedNotice(&p, subjects)
	}
	p.Notice += " 本次索引不可用，已完整回放记录；请检查作品 .story-cache 的文件系统权限。"
	data, err = json.MarshalIndent(p, "", "  ")
	if err != nil {
		return nil, err
	}
	if len(data)+1 > maxBytes {
		return nil, fmt.Errorf("fallback context exceeds byte budget")
	}
	return append(data, '\n'), nil
}

func indexedContext(dir string, ids []string, at, depth, recent, maxBytes int, related bool) ([]byte, error) {
	db, err := openCache(dir, false)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	var result []byte
	err = db.View(func(tx *bolt.Tx) error {
		if err := validateCache(dir, tx); err != nil {
			return err
		}
		var p Project
		raw, err := os.ReadFile(filepathJoinProject(dir))
		if err != nil {
			return err
		}
		if err = Decode(raw, &p); err != nil {
			return err
		}
		s := NewState(p)
		s.Revision, err = strconv.Atoi(getMeta(tx, "revision"))
		if err != nil {
			return err
		}
		s.LastUnit, err = strconv.Atoi(getMeta(tx, "last-unit"))
		if err != nil {
			return err
		}
		s.Head = getMeta(tx, "head")
		total, err := strconv.Atoi(getMeta(tx, "entities"))
		if err != nil {
			return err
		}
		selected := map[string]bool{}
		loadEntity := func(id string) error {
			if _, ok := s.Entities[id]; ok {
				return nil
			}
			var e Entity
			if err := getJSON(tx, "entity", id, &e); err != nil {
				return err
			}
			s.Entities[id] = e
			return getOrigin(tx, s, id)
		}
		loadRelation := func(id string) error {
			if _, ok := s.Relations[id]; ok {
				return nil
			}
			var v Relation
			if err := getJSON(tx, "relation", id, &v); err != nil {
				return err
			}
			s.Relations[id] = v
			if err := getOrigin(tx, s, id); err != nil {
				return err
			}
			if err := loadEntity(v.From); err != nil {
				return err
			}
			return loadEntity(v.To)
		}
		for _, id := range ids {
			bucket := tx.Bucket([]byte("entity"))
			if bucket == nil {
				return errors.New("missing entity index")
			}
			if bucket.Get([]byte(id)) == nil {
				return fmt.Errorf("%w: unknown entity %s", errContextQuery, id)
			}
			if err := loadEntity(id); err != nil {
				return err
			}
			selected[id] = true
		}
		for i := 0; i < depth; i++ {
			next := map[string]bool{}
			for id := range selected {
				next[id] = true
			}
			for _, id := range sortedKeys(selected) {
				if err := scanPrefix(tx, "relation-entity", id+"\x00", func(rid string) error {
					if err := loadRelation(rid); err != nil {
						return err
					}
					r := s.Relations[rid]
					if active(r.Since, r.Until, at) {
						next[r.From] = true
						next[r.To] = true
					}
					return nil
				}); err != nil {
					return err
				}
			}
			selected = next
		}
		loadHook := func(id string) error {
			if _, ok := s.Hooks[id]; ok {
				return nil
			}
			var h Hook
			if err := getJSON(tx, "hook", id, &h); err != nil {
				return err
			}
			s.Hooks[id] = h
			if err := getOrigin(tx, s, id); err != nil {
				return err
			}
			for _, e := range h.Entities {
				if err := loadEntity(e); err != nil {
					return err
				}
			}
			return nil
		}
		for _, id := range sortedKeys(selected) {
			if err := scanPrefix(tx, "hook-entity", id+"\x00", loadHook); err != nil {
				return err
			}
		}
		due := tx.Bucket([]byte("hook-due"))
		if due == nil {
			return errors.New("missing hook due index")
		}
		cursor := due.Cursor()
		for key, _ := cursor.First(); key != nil; key, _ = cursor.Next() {
			parts := strings.SplitN(string(key), "\x00", 2)
			if len(parts) != 2 {
				return errors.New("invalid hook due index")
			}
			n, err := strconv.Atoi(parts[0])
			if err != nil {
				return err
			}
			if n > s.LastUnit+4 {
				break
			}
			if err := loadHook(parts[1]); err != nil {
				return err
			}
		}
		for _, h := range s.Hooks {
			if h.Status == "open" {
				for _, id := range h.Entities {
					selected[id] = true
				}
			}
		}
		// Resolve world rules before limiting knowledge to relevant subjects.
		worldSubjects := map[string]bool{}
		if err := scanPrefix(tx, "world-fact", "", func(id string) error {
			var f Fact
			if err := getJSON(tx, "fact", id, &f); err != nil {
				return err
			}
			worldSubjects[f.Subject] = true
			return nil
		}); err != nil {
			return err
		}
		knowledgeSubjects := map[string]bool{}
		for id := range selected {
			knowledgeSubjects[id] = true
		}
		for id := range worldSubjects {
			knowledgeSubjects[id] = true
		}
		for _, id := range sortedKeys(selected) {
			if err := loadEntity(id); err != nil {
				return err
			}
			if s.Entities[id].Kind != "character" {
				continue
			}
			loadKnowledge := func(kid string) error {
				var k Knowledge
				if err := getJSON(tx, "knowledge", kid, &k); err != nil {
					return err
				}
				if k.At <= at {
					s.Knowledge[kid] = k
					return getOrigin(tx, s, kid)
				}
				return nil
			}
			if related {
				for _, subject := range sortedKeys(knowledgeSubjects) {
					if err := scanPrefix(tx, "knowledge-subject-character", pair(subject, id)+"\x00", func(suffix string) error {
						parts := strings.SplitN(suffix, "\x00", 2)
						if len(parts) != 2 {
							return errors.New("invalid knowledge index")
						}
						return loadKnowledge(parts[1])
					}); err != nil {
						return err
					}
				}
			} else if err := scanPrefix(tx, "knowledge-character", id+"\x00", loadKnowledge); err != nil {
				return err
			}
		}
		loadFact := func(id string) error {
			if _, ok := s.Facts[id]; ok {
				return nil
			}
			var f Fact
			if err := getJSON(tx, "fact", id, &f); err != nil {
				return err
			}
			s.Facts[id] = f
			if err := getOrigin(tx, s, id); err != nil {
				return err
			}
			return loadEntity(f.Subject)
		}
		for _, k := range s.Knowledge {
			if err := loadFact(k.FactID); err != nil {
				return err
			}
		}
		if err := scanPrefix(tx, "world-fact", "", loadFact); err != nil {
			return err
		}
		for _, id := range sortedKeys(selected) {
			if err := loadSubjectFacts(tx, id, at, loadFact); err != nil {
				return err
			}
		}
		// Facts required by knowledge and global world rules can add entity subjects.
		for _, f := range s.Facts {
			if f.Locked && s.Entities[f.Subject].Kind == "world" {
				selected[f.Subject] = true
			}
			for _, k := range s.Knowledge {
				if k.FactID == f.ID {
					selected[f.Subject] = true
					break
				}
			}
		}
		for _, id := range sortedKeys(selected) {
			if err := loadSubjectFacts(tx, id, at, loadFact); err != nil {
				return err
			}
			if err := scanPrefix(tx, "relation-entity", id+"\x00", loadRelation); err != nil {
				return err
			}
		}
		moreEvents := false
		for _, id := range sortedKeys(selected) {
			more, err := loadRecentEvents(tx, s, id, at)
			if err != nil {
				return err
			}
			moreEvents = moreEvents || more
		}
		for n := s.Revision - recent + 1; n <= s.Revision; n++ {
			if n <= 0 {
				continue
			}
			var v Summary
			if err := getJSON(tx, "recent", fmt.Sprintf("%020d", n), &v); err != nil {
				return err
			}
			s.Records = append(s.Records, Record{Revision: v.Revision, Batch: Batch{Unit: v.Unit, Summary: v.Summary}, Draft: v.Draft})
		}
		data, err := Context(s, ids, at, depth, recent, maxBytes)
		if err != nil {
			return fmt.Errorf("%w: %v", errContextQuery, err)
		}
		var packet Packet
		if err = Decode(data, &packet); err != nil {
			return err
		}
		packet.OmittedEntities = total - len(packet.Entities)
		packet.MoreEvents = moreEvents
		if related {
			setRelatedNotice(&packet, sortedKeys(knowledgeSubjects))
		}
		result, err = json.MarshalIndent(packet, "", "  ")
		if err != nil {
			return err
		}
		if len(result)+1 > maxBytes {
			return fmt.Errorf("%w: essential context exceeds %d UTF-8 bytes after index metadata", errContextQuery, maxBytes)
		}
		result = append(result, '\n')
		return nil
	})
	return result, err
}

func filepathJoinProject(dir string) string { return filepath.Join(dir, "project.json") }

func getOrigin(tx *bolt.Tx, s *State, id string) error {
	var origin Origin
	if err := getJSON(tx, "origin", id, &origin); err != nil {
		return err
	}
	s.Origins[id] = origin
	return nil
}

func setRelatedNotice(p *Packet, subjects []string) {
	p.KnowledgeSubjects = subjects
	p.Notice = "作者上下文含秘密真相；knowledge_scope 是本次查询的角色，knowledge_subjects 是知情记录覆盖的实体。未返回的其他实体信息不能推断为角色不知道；需要时将其加入 --entities 补查。boundary_relations 和 more_events 也需按需补查。正文语义仍需 Agent 复核。"
}

// Find the latest version per fact key at the requested tick in logarithmic
// time, and include all locked versions separately.
func loadSubjectFacts(tx *bolt.Tx, subject string, at int, load func(string) error) error {
	if err := scanPrefix(tx, "fact-locked", subject+"\x00", load); err != nil {
		return err
	}
	b := tx.Bucket([]byte("fact-timeline"))
	if b == nil {
		return errors.New("missing fact timeline index")
	}
	return scanPrefix(tx, "fact-keys", subject+"\x00", func(key string) error {
		prefix := []byte(pair(subject, key) + "\x00")
		upper := append(append([]byte{}, prefix...), []byte(fmt.Sprintf("%020d\xff", at))...)
		cursor := b.Cursor()
		k, _ := cursor.Seek(upper)
		if k == nil {
			k, _ = cursor.Last()
		} else {
			k, _ = cursor.Prev()
		}
		if k == nil || !bytes.HasPrefix(k, prefix) {
			return nil
		}
		parts := strings.SplitN(string(k), "\x00", 4)
		if len(parts) != 4 {
			return errors.New("invalid fact timeline index")
		}
		return load(parts[3])
	})
}

func loadRecentEvents(tx *bolt.Tx, s *State, entity string, at int) (bool, error) {
	b := tx.Bucket([]byte("event-entity-time"))
	if b == nil {
		return false, errors.New("missing event index")
	}
	prefix := []byte(entity + "\x00")
	upper := []byte(entity + "\x00" + fmt.Sprintf("%020d", at) + "\xff")
	cursor := b.Cursor()
	key, _ := cursor.Seek(upper)
	if key == nil {
		key, _ = cursor.Last()
	} else {
		key, _ = cursor.Prev()
	}
	count := 0
	for ; key != nil && bytes.HasPrefix(key, prefix); key, _ = cursor.Prev() {
		if count >= indexedEventLimit {
			return true, nil
		}
		parts := strings.SplitN(string(key), "\x00", 3)
		if len(parts) != 3 {
			return false, errors.New("invalid event index")
		}
		id := parts[2]
		if _, ok := s.Events[id]; !ok {
			var e Event
			if err := getJSON(tx, "event", id, &e); err != nil {
				return false, err
			}
			s.Events[id] = e
			if err := getOrigin(tx, s, id); err != nil {
				return false, err
			}
		}
		count++
	}
	return false, nil
}
