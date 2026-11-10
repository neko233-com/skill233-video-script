package story

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"testing"
)

// Both benchmark paths return the same packet over the same persisted history.
func BenchmarkContextHistory1000(b *testing.B) {
	dir := b.TempDir()
	if err := Init(dir, "benchmark", "novel"); err != nil {
		b.Fatal(err)
	}
	if _, err := Accept(dir, seed(), nil); err != nil {
		b.Fatal(err)
	}
	s, err := Load(dir)
	if err != nil {
		b.Fatal(err)
	}
	for n := 1; n <= 1000; n++ {
		id := fmt.Sprintf("c.extra%d", n)
		batch := Batch{BaseRevision: s.Revision, Unit: n, Summary: fmt.Sprintf("第%d章", n),
			Claims:   []Claim{{Subject: "c.lin", Key: "identity", Value: "修复师", At: n, Evidence: "正文"}},
			Entities: []Entity{{ID: id, Kind: "character", Name: id, Description: "历史人物"}},
			Facts:    []Fact{{ID: fmt.Sprintf("f.extra%d", n), Subject: id, Key: "identity", Value: "历史人物", From: n, Locked: true, Evidence: "正文"}}}
		prev := s.Head
		if _, err := s.Apply(batch); err != nil {
			b.Fatal(err)
		}
		draft := []byte(fmt.Sprintf("第%d章：合成正文。", n))
		hash := digest(draft)
		if err := atomicNew(filepath.Join(dir, "blobs", hash+".md"), draft); err != nil {
			b.Fatal(err)
		}
		r := Record{Schema: Schema, Revision: s.Revision, PrevHash: prev, Batch: batch, Draft: &Draft{SHA256: hash}}
		r.Hash = recordHash(r)
		data, err := json.Marshal(r)
		if err != nil {
			b.Fatal(err)
		}
		if err := atomicNew(filepath.Join(dir, "records", fmt.Sprintf("%08d.json", r.Revision)), data); err != nil {
			b.Fatal(err)
		}
		s.Head = r.Hash
	}
	ids := []string{"c.lin"}
	indexed, err := ContextProject(dir, ids, 1000, 2, 3, 6000)
	if err != nil {
		b.Fatal(err)
	}
	verified, err := loadVerified(dir)
	if err != nil {
		b.Fatal(err)
	}
	replayed, err := Context(verified, ids, 1000, 2, 3, 6000)
	if err != nil {
		b.Fatal(err)
	}
	var a, c Packet
	if json.Unmarshal(indexed, &a) != nil || json.Unmarshal(replayed, &c) != nil || !reflect.DeepEqual(a, c) {
		b.Fatalf("packets differ; benchmark would not compare equivalent work\n%s\n%s", indexed, replayed)
	}
	b.Run("indexed", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			if _, err := ContextProject(dir, ids, 1000, 2, 3, 6000); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("verified-replay", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			s, err := loadVerified(dir)
			if err == nil {
				_, err = Context(s, ids, 1000, 2, 3, 6000)
			}
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}
