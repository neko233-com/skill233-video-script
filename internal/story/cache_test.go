package story

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func readPacket(t *testing.T, data []byte) Packet {
	t.Helper()
	var p Packet
	if err := json.Unmarshal(data, &p); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestIndexedContextMatchesVerifiedReplay(t *testing.T) {
	dir := newProject(t)
	accept(t, dir, seed())
	accept(t, dir, chapter1())
	accept(t, dir, chapter2())
	s, err := loadVerified(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []struct {
		ids       []string
		at, depth int
	}{{[]string{"c.shen"}, 0, 0}, {[]string{"c.shen"}, 10, 2}, {[]string{"c.lin", "i.watch"}, 20, 1}} {
		want, err := Context(s, q.ids, q.at, q.depth, 3, 24000)
		if err != nil {
			t.Fatal(err)
		}
		got, err := ContextProject(dir, q.ids, q.at, q.depth, 3, 24000)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(readPacket(t, want), readPacket(t, got)) {
			t.Fatalf("indexed result differs from authoritative replay\nwant %s\ngot %s", want, got)
		}
	}
}

func TestStaleAndDamagedIndexRecovery(t *testing.T) {
	for _, mode := range []string{"stale", "corrupt", "missing"} {
		t.Run(mode, func(t *testing.T) {
			dir := newProject(t)
			accept(t, dir, seed())
			old, err := os.ReadFile(cachePath(dir))
			if err != nil {
				t.Fatal(err)
			}
			accept(t, dir, chapter1())
			switch mode {
			case "stale":
				err = os.WriteFile(cachePath(dir), old, 0600)
			case "corrupt":
				err = os.WriteFile(cachePath(dir), []byte("broken cache"), 0600)
			case "missing":
				err = os.Remove(cachePath(dir))
			}
			if err != nil {
				t.Fatal(err)
			}
			data, err := ContextProject(dir, []string{"c.shen", "i.watch"}, 10, 1, 3, 24000)
			if err != nil {
				t.Fatal(err)
			}
			p := readPacket(t, data)
			if p.Revision != 2 || p.NextUnit != 2 {
				t.Fatalf("not recovered: %+v", p)
			}
			owner := false
			for _, f := range p.Facts {
				if f.ID == "f.owner2" && f.Value == "c.shen" {
					owner = true
				}
			}
			if !owner {
				t.Fatal("updated owner lost")
			}
			if _, err := loadCheckpoint(dir); err != nil {
				t.Fatalf("cache not rebuilt: %v", err)
			}
			scoped, err := ContextProjectScoped(dir, []string{"c.shen", "i.watch"}, 10, 1, 3, 24000, true)
			if err != nil {
				t.Fatal(err)
			}
			if got := readPacket(t, scoped); got.Revision != p.Revision || got.NextUnit != p.NextUnit || !strings.Contains(got.Notice, "不能推断") {
				t.Fatalf("scoped recovery is inconsistent: %+v", got)
			}
			if _, err := Verify(dir); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestRelatedFallbackDisclosesIndexFailure(t *testing.T) {
	dir := newProject(t)
	accept(t, dir, seed())
	s, err := loadVerified(dir)
	if err != nil {
		t.Fatal(err)
	}
	data, err := contextFallback(s, []string{"c.lin"}, 0, 1, 1, 24000, true)
	if err != nil {
		t.Fatal(err)
	}
	notice := readPacket(t, data).Notice
	if !strings.Contains(notice, "不能推断") || !strings.Contains(notice, "索引不可用") {
		t.Fatalf("fallback notice omitted: %q", notice)
	}
}

func TestIndexedHistoryBoundaryAndFullAudit(t *testing.T) {
	dir := newProject(t)
	accept(t, dir, seed())
	accept(t, dir, chapter1())
	accept(t, dir, chapter2())
	if _, err := ContextProject(dir, []string{"c.shen"}, 20, 1, 2, 24000); err != nil {
		t.Fatal(err)
	}
	// A warm indexed lookup reads the metadata and current head, not every old record.
	oldPath := filepath.Join(dir, "records", "00000001.json")
	backup := oldPath + ".fixture"
	if err := os.Rename(oldPath, backup); err != nil {
		t.Fatal(err)
	}
	if _, err := ContextProject(dir, []string{"c.shen"}, 20, 1, 2, 24000); err != nil {
		t.Fatalf("indexed lookup unexpectedly depended on old record: %v", err)
	}
	if _, err := Verify(dir); err == nil {
		t.Fatal("full audit failed to detect missing historical record")
	}
	if err := os.Rename(backup, oldPath); err != nil {
		t.Fatal(err)
	}
}

func TestIndexedContinuityRejectionAndProjectIsolation(t *testing.T) {
	a, b := newProject(t), newProject(t)
	accept(t, a, seed())
	accept(t, b, seed())
	accept(t, a, chapter1())
	state, err := Load(a)
	if err != nil {
		t.Fatal(err)
	}
	if state.Revision != 2 {
		t.Fatal("wrong cached revision")
	}
	broken := chapter2()
	broken.Claims[0].At = 19
	if _, err := Check(a, broken, []byte("bad")); err == nil || !strings.Contains(err.Error(), "knowledge leak") {
		t.Fatal("cached validation missed leak", err)
	}
	data, err := ContextProject(b, []string{"c.lin", "i.watch"}, 0, 0, 2, 24000)
	if err != nil {
		t.Fatal(err)
	}
	if p := readPacket(t, data); p.Revision != 1 {
		t.Fatalf("projects shared state: %+v", p)
	}
}

func TestIndexedEventWindowIsExplicit(t *testing.T) {
	dir := newProject(t)
	b := seed()
	for i := 0; i < 40; i++ {
		b.Events = append(b.Events, Event{ID: fmt.Sprintf("e.test%03d", i), At: i, Participants: []string{"c.lin"}, Summary: "事件", Evidence: "示例"})
	}
	accept(t, dir, b)
	data, err := ContextProject(dir, []string{"c.lin"}, 50, 0, 0, 24000)
	if err != nil {
		t.Fatal(err)
	}
	p := readPacket(t, data)
	if !p.MoreEvents || len(p.Events) != 32 {
		t.Fatalf("event window not disclosed: %d %+v", len(p.Events), p)
	}
}

func TestRelatedKnowledgeScopeAndFullValidation(t *testing.T) {
	dir := newProject(t)
	b := seed()
	b.Entities = append(b.Entities, Entity{ID: "i.ring", Kind: "item", Name: "旧戒指", Description: "另一条故事线"})
	b.Facts = append(b.Facts, Fact{ID: "f.ring", Subject: "i.ring", Key: "secret", Value: "刻有名字", From: 0, Locked: true, Evidence: "用户设定"})
	b.Knowledge = append(b.Knowledge, Knowledge{ID: "k.shen.ring", Character: "c.shen", FactID: "f.ring", At: 1, Evidence: "前史检查戒指"}, Knowledge{ID: "k.shen.watch", Character: "c.shen", FactID: "f.secret", At: 2, Evidence: "前史检查怀表"})
	accept(t, dir, b)
	data, err := ContextProjectScoped(dir, []string{"c.shen", "i.watch"}, 10, 0, 1, 24000, true)
	if err != nil {
		t.Fatal(err)
	}
	p := readPacket(t, data)
	foundWatch, foundRing := false, false
	for _, k := range p.Knowledge {
		foundWatch = foundWatch || k.ID == "k.shen.watch"
		foundRing = foundRing || k.ID == "k.shen.ring"
	}
	if !foundWatch || foundRing || !strings.Contains(p.Notice, "不能推断") {
		t.Fatalf("incorrect scoped knowledge: %+v", p)
	}
	data, err = ContextProjectScoped(dir, []string{"c.shen", "i.watch", "i.ring"}, 10, 0, 1, 24000, true)
	if err != nil {
		t.Fatal(err)
	}
	p = readPacket(t, data)
	foundRing = false
	for _, k := range p.Knowledge {
		foundRing = foundRing || k.ID == "k.shen.ring"
	}
	if !foundRing {
		t.Fatal("targeted fact did not restore knowledge")
	}
	// A scoped query must not narrow the authoritative continuity validation.
	chapter := chapter1()
	chapter.Claims = append(chapter.Claims, Claim{Subject: "i.ring", Key: "secret", Value: "刻有名字", At: 10, Knower: "c.shen", Evidence: "1-2"})
	if _, err = Check(dir, chapter, []byte("正文")); err != nil {
		t.Fatal(err)
	}
}

func TestFactTimelineLookupWithManyVersions(t *testing.T) {
	dir := newProject(t)
	b := seed()
	previous := "f.owner"
	for i := 1; i <= 500; i++ {
		id := fmt.Sprintf("f.owner%04d", i)
		value := "c.lin"
		if i%2 == 1 {
			value = "c.shen"
		}
		b.Facts = append(b.Facts, Fact{ID: id, Subject: "i.watch", Key: "owner", Value: value, From: i, Supersedes: previous, Evidence: "连续状态变化"})
		previous = id
	}
	accept(t, dir, b)
	for _, at := range []int{0, 1, 99, 250, 500} {
		data, err := ContextProjectScoped(dir, []string{"i.watch"}, at, 0, 0, 12000, true)
		if err != nil {
			t.Fatal(err)
		}
		p := readPacket(t, data)
		count := 0
		for _, f := range p.Facts {
			if f.Subject == "i.watch" && f.Key == "owner" {
				count++
				if !active(f.From, f.Until, at) {
					t.Fatalf("wrong version at %d: %+v", at, f)
				}
			}
		}
		if count != 1 {
			t.Fatalf("expected one owner version at %d, got %d", at, count)
		}
	}
}
