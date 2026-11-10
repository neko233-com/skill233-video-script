package story

import (
	"bytes"
	"encoding/hex"
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

// The cache is disposable and never replaces project.json or records/.
// Snapshots accelerate validation; the secondary B+tree indexes serve context
// without loading the snapshot or scanning historical records.
const cacheVersion = "3"

var buckets = []string{"meta", "entity", "fact", "fact-keys", "fact-timeline", "fact-locked", "world-fact", "relation", "relation-entity", "knowledge", "knowledge-character", "knowledge-subject-character", "event", "event-entity-time", "hook", "hook-entity", "hook-due", "origin", "recent"}

type checkpoint struct {
	State      *State              `json:"state"`
	FactIndex  map[string][]string `json:"fact_index"`
	KnownIndex map[string][]string `json:"known_index"`
	AssertUses map[string][]Claim  `json:"assert_uses"`
	EventUses  map[string][]string `json:"event_uses"`
}

func cachePath(dir string) string { return filepath.Join(dir, ".story-cache", "index-v3.db") }

func openCache(dir string, create bool) (*bolt.DB, error) {
	path := cachePath(dir)
	if !create {
		if _, err := os.Stat(path); err != nil {
			return nil, err
		}
	} else if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	return bolt.Open(path, 0600, &bolt.Options{Timeout: 2 * time.Second})
}

func getMeta(tx *bolt.Tx, key string) string {
	b := tx.Bucket([]byte("meta"))
	if b == nil {
		return ""
	}
	return string(b.Get([]byte(key)))
}

func putMeta(tx *bolt.Tx, key, value string) error {
	return tx.Bucket([]byte("meta")).Put([]byte(key), []byte(value))
}

func validateCache(dir string, tx *bolt.Tx) error {
	if getMeta(tx, "version") != cacheVersion {
		return errors.New("missing or incompatible project index")
	}
	data, err := os.ReadFile(filepath.Join(dir, "project.json"))
	if err != nil {
		return err
	}
	var p Project
	if err := Decode(data, &p); err != nil || !validProject(p) {
		return errors.New("invalid project metadata")
	}
	if getMeta(tx, "project") != projectHash(p) {
		return errors.New("project metadata changed")
	}
	rev, err := strconv.Atoi(getMeta(tx, "revision"))
	if err != nil || rev < 0 {
		return errors.New("invalid indexed revision")
	}
	lastUnit, err := strconv.Atoi(getMeta(tx, "last-unit"))
	if err != nil || (rev == 0 && lastUnit != -1) {
		return errors.New("invalid indexed last unit")
	}
	head := projectHash(p)
	if rev > 0 {
		data, err = os.ReadFile(filepath.Join(dir, "records", fmt.Sprintf("%08d.json", rev)))
		if err != nil {
			return err
		}
		var r Record
		if err := Decode(data, &r); err != nil || r.Schema != Schema || r.Revision != rev || r.Batch.Unit != lastUnit || r.Hash != recordHash(r) {
			return errors.New("indexed head record is invalid")
		}
		head = r.Hash
	}
	if head != getMeta(tx, "head") {
		return errors.New("indexed head changed")
	}
	_, err = os.Stat(filepath.Join(dir, "records", fmt.Sprintf("%08d.json", rev+1)))
	if err == nil {
		return errors.New("index is behind the record chain")
	}
	if !os.IsNotExist(err) {
		return err
	}
	return nil
}

func loadCheckpoint(dir string) (*State, error) {
	db, err := openCache(dir, false)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	var cp checkpoint
	err = db.View(func(tx *bolt.Tx) error {
		if err := validateCache(dir, tx); err != nil {
			return err
		}
		m := tx.Bucket([]byte("meta"))
		raw := m.Get([]byte("snapshot"))
		if len(raw) == 0 || digest(raw) != getMeta(tx, "snapshot-hash") {
			return errors.New("invalid index checkpoint")
		}
		if err := json.Unmarshal(raw, &cp); err != nil {
			return err
		}
		if cp.State == nil || strconv.Itoa(cp.State.Revision) != getMeta(tx, "revision") || cp.State.Head != getMeta(tx, "head") || projectHash(cp.State.Project) != getMeta(tx, "project") || cp.FactIndex == nil || cp.KnownIndex == nil || cp.AssertUses == nil || cp.EventUses == nil {
			return errors.New("inconsistent index checkpoint")
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	cp.State.factIndex = cp.FactIndex
	cp.State.knownIndex = cp.KnownIndex
	cp.State.assertUses = cp.AssertUses
	cp.State.eventUses = cp.EventUses
	return cp.State, nil
}

func putJSON(tx *bolt.Tx, bucket, key string, value any) error {
	b := tx.Bucket([]byte(bucket))
	if b == nil {
		return fmt.Errorf("missing index bucket %s", bucket)
	}
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return b.Put([]byte(key), data)
}

func link(tx *bolt.Tx, bucket, key string) error {
	return tx.Bucket([]byte(bucket)).Put([]byte(key), []byte{1})
}
func unlink(tx *bolt.Tx, bucket, key string) error {
	return tx.Bucket([]byte(bucket)).Delete([]byte(key))
}
func pair(a, b string) string { return a + "\x00" + b }
func eventKey(entity string, e Event) string {
	return pair(entity, fmt.Sprintf("%020d\x00%s", e.At, e.ID))
}
func dueKey(h Hook) string { return fmt.Sprintf("%020d\x00%s", h.DueUnit, h.ID) }

func indexEntity(tx *bolt.Tx, e Entity) error { return putJSON(tx, "entity", e.ID, e) }
func indexFact(tx *bolt.Tx, s *State, f Fact) error {
	if err := putJSON(tx, "fact", f.ID, f); err != nil {
		return err
	}
	key := hex.EncodeToString([]byte(f.Key))
	if err := link(tx, "fact-keys", pair(f.Subject, key)); err != nil {
		return err
	}
	if err := link(tx, "fact-timeline", pair(f.Subject, pair(key, fmt.Sprintf("%020d\x00%s", f.From, f.ID)))); err != nil {
		return err
	}
	if f.Locked {
		if err := link(tx, "fact-locked", pair(f.Subject, f.ID)); err != nil {
			return err
		}
	}
	if f.Locked && s.Entities[f.Subject].Kind == "world" {
		return link(tx, "world-fact", f.ID)
	}
	return nil
}
func indexRelation(tx *bolt.Tx, r Relation) error {
	if err := putJSON(tx, "relation", r.ID, r); err != nil {
		return err
	}
	if err := link(tx, "relation-entity", pair(r.From, r.ID)); err != nil {
		return err
	}
	return link(tx, "relation-entity", pair(r.To, r.ID))
}
func indexKnowledge(tx *bolt.Tx, s *State, k Knowledge) error {
	if err := putJSON(tx, "knowledge", k.ID, k); err != nil {
		return err
	}
	if err := link(tx, "knowledge-character", pair(k.Character, k.ID)); err != nil {
		return err
	}
	f, ok := s.Facts[k.FactID]
	if !ok {
		return fmt.Errorf("knowledge %s references unknown fact", k.ID)
	}
	return link(tx, "knowledge-subject-character", pair(f.Subject, pair(k.Character, fmt.Sprintf("%020d\x00%s", k.At, k.ID))))
}
func indexEvent(tx *bolt.Tx, e Event) error {
	if err := putJSON(tx, "event", e.ID, e); err != nil {
		return err
	}
	for _, id := range e.Participants {
		if err := link(tx, "event-entity-time", eventKey(id, e)); err != nil {
			return err
		}
	}
	return nil
}
func indexHook(tx *bolt.Tx, h Hook) error {
	if err := putJSON(tx, "hook", h.ID, h); err != nil {
		return err
	}
	if h.Status == "open" {
		if err := link(tx, "hook-due", dueKey(h)); err != nil {
			return err
		}
		for _, id := range h.Entities {
			if err := link(tx, "hook-entity", pair(id, h.ID)); err != nil {
				return err
			}
		}
	} else {
		if err := unlink(tx, "hook-due", dueKey(h)); err != nil {
			return err
		}
		for _, id := range h.Entities {
			if err := unlink(tx, "hook-entity", pair(id, h.ID)); err != nil {
				return err
			}
		}
	}
	return nil
}
func indexRecord(tx *bolt.Tx, r Record) error {
	return putJSON(tx, "recent", fmt.Sprintf("%020d", r.Revision), Summary{r.Revision, r.Batch.Unit, r.Batch.Summary, r.Draft})
}

func saveSnapshot(tx *bolt.Tx, s *State) error {
	cp := checkpoint{s, s.factIndex, s.knownIndex, s.assertUses, s.eventUses}
	data, err := json.Marshal(cp)
	if err != nil {
		return err
	}
	if err = tx.Bucket([]byte("meta")).Put([]byte("snapshot"), data); err != nil {
		return err
	}
	for k, v := range map[string]string{"version": cacheVersion, "project": projectHash(s.Project), "revision": strconv.Itoa(s.Revision), "last-unit": strconv.Itoa(s.LastUnit), "head": s.Head, "entities": strconv.Itoa(len(s.Entities)), "snapshot-hash": digest(data)} {
		if err = putMeta(tx, k, v); err != nil {
			return err
		}
	}
	return nil
}

func rebuildTransaction(tx *bolt.Tx, s *State) error {
	for _, name := range buckets {
		if err := tx.DeleteBucket([]byte(name)); err != nil && !errors.Is(err, bolt.ErrBucketNotFound) {
			return err
		}
		if _, err := tx.CreateBucket([]byte(name)); err != nil {
			return err
		}
	}
	for _, id := range sortedKeys(s.Entities) {
		if err := indexEntity(tx, s.Entities[id]); err != nil {
			return err
		}
	}
	for _, id := range sortedKeys(s.Facts) {
		if err := indexFact(tx, s, s.Facts[id]); err != nil {
			return err
		}
	}
	for _, id := range sortedKeys(s.Relations) {
		if err := indexRelation(tx, s.Relations[id]); err != nil {
			return err
		}
	}
	for _, id := range sortedKeys(s.Knowledge) {
		if err := indexKnowledge(tx, s, s.Knowledge[id]); err != nil {
			return err
		}
	}
	for _, id := range sortedKeys(s.Events) {
		if err := indexEvent(tx, s.Events[id]); err != nil {
			return err
		}
	}
	for _, id := range sortedKeys(s.Hooks) {
		if err := indexHook(tx, s.Hooks[id]); err != nil {
			return err
		}
	}
	for _, id := range sortedKeys(s.Origins) {
		if err := putJSON(tx, "origin", id, s.Origins[id]); err != nil {
			return err
		}
	}
	for _, r := range s.Records {
		if err := indexRecord(tx, r); err != nil {
			return err
		}
	}
	return saveSnapshot(tx, s)
}

func rebuildCheckpoint(dir string, s *State) error {
	db, err := openCache(dir, true)
	if err != nil {
		return err
	}
	defer db.Close()
	return db.Update(func(tx *bolt.Tx) error { return rebuildTransaction(tx, s) })
}

func updateCheckpoint(dir string, s *State, r Record) error {
	db, err := openCache(dir, true)
	if err != nil {
		return err
	}
	defer db.Close()
	return db.Update(func(tx *bolt.Tx) error {
		if getMeta(tx, "version") != cacheVersion || getMeta(tx, "head") != r.PrevHash || getMeta(tx, "revision") != strconv.Itoa(r.Revision-1) || getMeta(tx, "project") != projectHash(s.Project) {
			return rebuildTransaction(tx, s)
		}
		b := r.Batch
		for _, v := range b.Entities {
			if err := indexEntity(tx, s.Entities[v.ID]); err != nil {
				return err
			}
		}
		for _, v := range b.Facts {
			if v.Supersedes != "" {
				if err := indexFact(tx, s, s.Facts[v.Supersedes]); err != nil {
					return err
				}
			}
			if err := indexFact(tx, s, s.Facts[v.ID]); err != nil {
				return err
			}
		}
		for _, v := range b.Relations {
			if v.Supersedes != "" {
				if err := indexRelation(tx, s.Relations[v.Supersedes]); err != nil {
					return err
				}
			}
			if err := indexRelation(tx, s.Relations[v.ID]); err != nil {
				return err
			}
		}
		for _, v := range b.Knowledge {
			if err := indexKnowledge(tx, s, s.Knowledge[v.ID]); err != nil {
				return err
			}
		}
		for _, v := range b.Events {
			if err := indexEvent(tx, s.Events[v.ID]); err != nil {
				return err
			}
		}
		for _, v := range b.Hooks {
			if err := indexHook(tx, s.Hooks[v.ID]); err != nil {
				return err
			}
		}
		for _, v := range b.HookUpdates {
			if err := indexHook(tx, s.Hooks[v.ID]); err != nil {
				return err
			}
		}
		ids := []string{}
		for _, v := range b.Entities {
			ids = append(ids, v.ID)
		}
		for _, v := range b.Facts {
			ids = append(ids, v.ID)
		}
		for _, v := range b.Relations {
			ids = append(ids, v.ID)
		}
		for _, v := range b.Knowledge {
			ids = append(ids, v.ID)
		}
		for _, v := range b.Events {
			ids = append(ids, v.ID)
		}
		for _, v := range b.Hooks {
			ids = append(ids, v.ID)
		}
		for _, id := range ids {
			if err := putJSON(tx, "origin", id, s.Origins[id]); err != nil {
				return err
			}
		}
		if err := indexRecord(tx, r); err != nil {
			return err
		}
		return saveSnapshot(tx, s)
	})
}

func getJSON(tx *bolt.Tx, bucket, key string, out any) error {
	b := tx.Bucket([]byte(bucket))
	if b == nil {
		return fmt.Errorf("missing index bucket %s", bucket)
	}
	data := b.Get([]byte(key))
	if data == nil {
		return fmt.Errorf("missing indexed %s %s", bucket, key)
	}
	return Decode(data, out)
}
func scanPrefix(tx *bolt.Tx, bucket, prefix string, fn func(string) error) error {
	b := tx.Bucket([]byte(bucket))
	if b == nil {
		return fmt.Errorf("missing index bucket %s", bucket)
	}
	p := []byte(prefix)
	c := b.Cursor()
	for k, _ := c.Seek(p); k != nil && bytes.HasPrefix(k, p); k, _ = c.Next() {
		if err := fn(strings.TrimPrefix(string(k), prefix)); err != nil {
			return err
		}
	}
	return nil
}
