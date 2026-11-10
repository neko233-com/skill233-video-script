package story

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

func Decode(data []byte, dst any) error {
	d := json.NewDecoder(bytes.NewReader(bytes.TrimPrefix(data, []byte{0xef, 0xbb, 0xbf})))
	d.DisallowUnknownFields()
	if err := d.Decode(dst); err != nil {
		return err
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return fmt.Errorf("expected exactly one JSON value")
	}
	return nil
}

func digest(data []byte) string { h := sha256.Sum256(data); return hex.EncodeToString(h[:]) }

func recordHash(r Record) string { r.Hash = ""; data, _ := json.Marshal(r); return digest(data) }

func projectHash(p Project) string { data, _ := json.Marshal(p); return digest(data) }

func validProject(p Project) bool {
	return p.Schema == Schema && required(p.Title) && slices.Contains([]string{"short-drama", "novel", "film", "bilibili-video", "short-video", "product-video", "scene"}, p.Format)
}

func Init(dir, title, format string) error {
	p := Project{Schema: Schema, Title: title, Format: format}
	if !validProject(p) {
		return fmt.Errorf("title required; format must be short-drama, novel, film, bilibili-video, short-video, product-video, or scene")
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	for _, sub := range []string{"records", "blobs"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0755); err != nil {
			return err
		}
	}
	data, _ := json.MarshalIndent(p, "", "  ")
	return atomicNew(filepath.Join(dir, "project.json"), append(data, '\n'))
}

// atomicNew publishes a synced complete file without replacing an existing path.
// The project must be on a filesystem supporting hard links (NTFS/APFS/ext4).
func atomicNew(path string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".pending-")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = os.Link(tmp, path); err != nil {
		return fmt.Errorf("publish %s (no replacement; filesystem must support hard links): %w", filepath.Base(path), err)
	}
	return nil
}

// loadVerified replays the authoritative records. It is used for a full audit
// and for rebuilding an absent or stale disposable index.
func loadVerified(dir string) (*State, error) {
	data, err := os.ReadFile(filepath.Join(dir, "project.json"))
	if err != nil {
		return nil, err
	}
	var p Project
	if err := Decode(data, &p); err != nil {
		return nil, err
	}
	if !validProject(p) {
		return nil, fmt.Errorf("invalid project schema/title/format")
	}
	s := NewState(p)
	s.Head = projectHash(p)
	entries, err := os.ReadDir(filepath.Join(dir, "records"))
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		expected := fmt.Sprintf("%08d.json", s.Revision+1)
		if entry.IsDir() || entry.Name() != expected {
			return nil, fmt.Errorf("noncontiguous or unexpected record: %s, expected %s", entry.Name(), expected)
		}
		data, err := os.ReadFile(filepath.Join(dir, "records", entry.Name()))
		if err != nil {
			return nil, err
		}
		var r Record
		if err := Decode(data, &r); err != nil {
			return nil, fmt.Errorf("record %s: %w", entry.Name(), err)
		}
		if r.Schema != Schema || r.Revision != s.Revision+1 || r.PrevHash != s.Head || r.Hash != recordHash(r) {
			return nil, fmt.Errorf("broken hash/revision chain: %s", entry.Name())
		}
		if (r.Batch.Unit == 0) != (r.Draft == nil) {
			return nil, fmt.Errorf("record %s: unit 0 has no draft; every later unit requires a draft", entry.Name())
		}
		if r.Draft != nil && !validHash(r.Draft.SHA256) {
			return nil, fmt.Errorf("invalid draft hash: %s", entry.Name())
		}
		if _, err := s.Apply(r.Batch); err != nil {
			return nil, fmt.Errorf("record %s: %w", entry.Name(), err)
		}
		s.Head = r.Hash
		s.Records = append(s.Records, r)
	}
	return s, nil
}

// Load uses a project-local checkpoint when its project and current head match.
// Historical edits are detected by Verify, which always replays every record.
func Load(dir string) (*State, error) {
	if s, err := loadCheckpoint(dir); err == nil {
		return s, nil
	}
	return loadVerified(dir)
}

func validHash(s string) bool {
	b, err := hex.DecodeString(s)
	return err == nil && len(b) == sha256.Size && s == strings.ToLower(s)
}

type Result struct {
	Revision int      `json:"revision"`
	Unit     int      `json:"unit"`
	Hash     string   `json:"hash"`
	Warnings []string `json:"warnings"`
}

func prepare(dir string, b Batch, draft []byte) (*State, Record, []string, error) {
	s, err := Load(dir)
	if err != nil {
		return nil, Record{}, nil, err
	}
	if b.Unit == 0 && len(draft) != 0 {
		return nil, Record{}, nil, fmt.Errorf("unit 0 is canon setup and cannot contain a draft")
	}
	if b.Unit > 0 && len(bytes.TrimSpace(draft)) == 0 {
		return nil, Record{}, nil, fmt.Errorf("nonempty draft required for unit %d", b.Unit)
	}
	prev := s.Head
	warnings, err := s.Apply(b)
	if err != nil {
		return nil, Record{}, nil, err
	}
	r := Record{Schema: Schema, Revision: s.Revision, PrevHash: prev, Batch: b}
	if b.Unit > 0 {
		r.Draft = &Draft{SHA256: digest(draft)}
	}
	r.Hash = recordHash(r)
	return s, r, warnings, nil
}

func Check(dir string, b Batch, draft []byte) (Result, error) {
	_, r, warnings, err := prepare(dir, b, draft)
	if err != nil {
		return Result{}, err
	}
	return Result{r.Revision, b.Unit, r.Hash, warnings}, nil
}

func Accept(dir string, b Batch, draft []byte) (Result, error) {
	lock := filepath.Join(dir, ".story.lock")
	f, err := os.OpenFile(lock, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return Result{}, fmt.Errorf("writer lock unavailable (inspect active writer before removing a stale .story.lock): %w", err)
	}
	f.Close()
	defer os.Remove(lock)
	s, r, warnings, err := prepare(dir, b, draft)
	if err != nil {
		return Result{}, err
	}
	if r.Draft != nil {
		path := filepath.Join(dir, "blobs", r.Draft.SHA256+".md")
		if err := atomicNew(path, draft); err != nil {
			if !errors.Is(err, os.ErrExist) {
				return Result{}, err
			}
			stored, readErr := os.ReadFile(path)
			if readErr != nil || digest(stored) != r.Draft.SHA256 {
				return Result{}, fmt.Errorf("existing draft blob is corrupt")
			}
		}
	}
	data, _ := json.MarshalIndent(r, "", "  ")
	if err := atomicNew(filepath.Join(dir, "records", fmt.Sprintf("%08d.json", r.Revision)), append(data, '\n')); err != nil {
		return Result{}, err
	}
	s.Head = r.Hash
	s.Records = append(s.Records, r)
	if err := updateCheckpoint(dir, s, r); err != nil {
		warnings = append(warnings, "record accepted; project index update failed: "+err.Error())
	}
	return Result{r.Revision, b.Unit, r.Hash, warnings}, nil
}

func Verify(dir string) (Result, error) {
	s, err := loadVerified(dir)
	if err != nil {
		return Result{}, err
	}
	var warnings []string
	for _, r := range s.Records {
		if r.Draft == nil {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, "blobs", r.Draft.SHA256+".md"))
		if err != nil {
			return Result{}, fmt.Errorf("unit %d draft: %w", r.Batch.Unit, err)
		}
		if digest(data) != r.Draft.SHA256 {
			return Result{}, fmt.Errorf("unit %d draft hash mismatch", r.Batch.Unit)
		}
	}
	for _, id := range sortedKeys(s.Hooks) {
		h := s.Hooks[id]
		if h.Status == "open" && h.DueUnit <= s.LastUnit {
			warnings = append(warnings, "overdue hook: "+id)
		}
	}
	return Result{s.Revision, s.LastUnit, s.Head, warnings}, nil
}
