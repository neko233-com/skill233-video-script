package story

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"unicode"
)

func seed() Batch {
	return Batch{Unit: 0, Summary: "人物与秘密设定", Entities: []Entity{
		{ID: "c.lin", Kind: "character", Name: "林岚", Aliases: []string{"阿岚"}, Description: "修复师，习惯先检验证据"},
		{ID: "c.shen", Kind: "character", Name: "沈砚", Description: "调查员，谨慎守诺"},
		{ID: "i.watch", Kind: "item", Name: "旧怀表", Description: "不可联网的机械怀表"},
	}, Facts: []Fact{
		{ID: "f.name", Subject: "c.lin", Key: "identity", Value: "修复师", From: 0, Locked: true, Evidence: "用户设定"},
		{ID: "f.owner", Subject: "i.watch", Key: "owner", Value: "c.lin", From: 0, Evidence: "用户设定"},
		{ID: "f.secret", Subject: "i.watch", Key: "secret", Value: "表壳内有编号七", From: 0, Locked: true, Evidence: "作者真相"},
	}, Relations: []Relation{{ID: "r.old", From: "c.lin", To: "c.shen", Kind: "同学", Since: 0, Evidence: "用户设定"}},
		Knowledge: []Knowledge{{ID: "k.lin", Character: "c.lin", FactID: "f.secret", At: 0, Evidence: "前史：林岚检验表壳"}},
		Hooks:     []Hook{{ID: "h.number", Question: "编号七指向谁？", DueUnit: 2, Entities: []string{"i.watch"}, PlantUnit: 0, Status: "open", Evidence: "用户规划"}}}
}

func chapter1() Batch {
	return Batch{BaseRevision: 1, Unit: 1, Summary: "沈砚接收怀表，但尚未知晓编号", Facts: []Fact{{ID: "f.owner2", Subject: "i.watch", Key: "owner", Value: "c.shen", From: 10, Supersedes: "f.owner", Evidence: "1-1 林岚把怀表交给沈砚"}},
		Events: []Event{{ID: "e.transfer", At: 10, Participants: []string{"c.lin", "c.shen", "i.watch"}, Summary: "林岚交出怀表", Requires: []string{"f.owner2"}, Evidence: "1-1"}},
		Claims: []Claim{{Subject: "c.lin", Key: "identity", Value: "修复师", At: 10, Evidence: "1-1"}, {Subject: "i.watch", Key: "owner", Value: "c.shen", At: 10, Evidence: "1-1"}}}
}

func chapter2() Batch {
	return Batch{BaseRevision: 2, Unit: 2, Summary: "沈砚通过检验获知编号，回应旧伏笔", Knowledge: []Knowledge{{ID: "k.shen", Character: "c.shen", FactID: "f.secret", At: 20, Evidence: "2-1 打开表壳"}},
		Events:      []Event{{ID: "e.open", At: 20, Participants: []string{"c.shen", "i.watch"}, Summary: "检查表壳", Causes: []string{"e.transfer"}, Requires: []string{"f.owner2"}, Evidence: "2-1"}},
		HookUpdates: []HookUpdate{{ID: "h.number", Status: "paid", Evidence: "2-2 编号对上档案"}},
		Claims:      []Claim{{Subject: "i.watch", Key: "secret", Value: "表壳内有编号七", At: 20, Knower: "c.shen", Evidence: "2-1 沈砚说出编号"}}}
}

func newProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := Init(dir, "七号怀表", "short-drama"); err != nil {
		t.Fatal(err)
	}
	return dir
}
func accept(t *testing.T, dir string, b Batch) Result {
	t.Helper()
	var draft []byte
	if b.Unit > 0 {
		draft = []byte(fmt.Sprintf("第%d集：检验怀表，推进调查。", b.Unit))
	}
	r, err := Accept(dir, b, draft)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestContinuationAndRecovery(t *testing.T) {
	dir := newProject(t)
	accept(t, dir, seed())
	accept(t, dir, chapter1())
	accept(t, dir, chapter2())
	s, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if s.Revision != 3 || s.LastUnit != 2 || s.Hooks["h.number"].Status != "paid" {
		t.Fatalf("bad recovered state: %+v", s)
	}
	if active(s.Facts["f.owner"].From, s.Facts["f.owner"].Until, 10) {
		t.Fatal("old ownership still active")
	}
	if _, err = Verify(dir); err != nil {
		t.Fatal(err)
	}
	data, err := Context(s, []string{"c.shen"}, 20, 2, 3, 24000)
	if err != nil {
		t.Fatal(err)
	}
	var p Packet
	if err = Decode(data, &p); err != nil {
		t.Fatal(err)
	}
	if p.NextUnit != 3 || len(p.Knowledge) != 2 || len(p.Relations) != 1 {
		t.Fatalf("incomplete context: %s", data)
	}
	graph, err := Graph(s, 20)
	if err != nil || !strings.Contains(graph, "同学") {
		t.Fatal(graph, err)
	}
}

func TestRejectedBatchesLeaveCanonUntouched(t *testing.T) {
	tests := []struct {
		name, want string
		change     func(*Batch)
	}{
		{"locked", "cannot supersede", func(b *Batch) {
			b.Facts = append(b.Facts, Fact{ID: "f.fake", Subject: "c.lin", Key: "identity", Value: "警察", From: 1, Supersedes: "f.name", Evidence: "1-2"})
		}},
		{"overlap", "conflicting facts", func(b *Batch) {
			b.Facts = append(b.Facts, Fact{ID: "f.fake", Subject: "i.watch", Key: "owner", Value: "nobody", From: 10, Evidence: "1-2"})
		}},
		{"alias", "alias", func(b *Batch) {
			b.Entities = append(b.Entities, Entity{ID: "c.other", Kind: "character", Name: "阿岚", Description: "不同的人"})
		}},
		{"missing-relation-target", "invalid relation", func(b *Batch) {
			b.Relations = append(b.Relations, Relation{ID: "r.bad", From: "c.lin", To: "c.missing", Kind: "认识", Since: 1, Evidence: "1-2"})
		}},
		{"knowledge-leak", "knowledge leak", func(b *Batch) {
			b.Claims = append(b.Claims, Claim{Subject: "i.watch", Key: "secret", Value: "表壳内有编号七", At: 10, Knower: "c.shen", Evidence: "1-2 突然知道"})
		}},
		{"future-knowledge", "knowledge leak", func(b *Batch) {
			b.Knowledge = append(b.Knowledge, Knowledge{ID: "k.future", Character: "c.shen", FactID: "f.secret", At: 11, Evidence: "1-3"})
			b.Claims = append(b.Claims, Claim{Subject: "i.watch", Key: "secret", Value: "表壳内有编号七", At: 10, Knower: "c.shen", Evidence: "1-2"})
		}},
		{"wrong-value", "claim conflict", func(b *Batch) { b.Claims[0].Value = "警察" }},
		{"wrong-time", "unsupported claim", func(b *Batch) { b.Claims[1].At = 9; b.Claims[1].Subject = "i.missing" }},
		{"causal-cycle", "causal cycle", func(b *Batch) {
			b.Events[0].Causes = []string{"e.other"}
			b.Events = append(b.Events, Event{ID: "e.other", At: 10, Participants: []string{"c.lin"}, Summary: "循环", Causes: []string{"e.transfer"}, Evidence: "1-2"})
		}},
		{"future-cause", "future cause", func(b *Batch) {
			b.Events[0].Causes = []string{"e.other"}
			b.Events = append(b.Events, Event{ID: "e.other", At: 11, Participants: []string{"c.lin"}, Summary: "未来", Evidence: "1-3"})
		}},
		{"inactive-prerequisite", "prerequisite", func(b *Batch) { b.Events[0].Requires = []string{"f.owner"} }},
		{"stale", "stale revision", func(b *Batch) { b.BaseRevision = 0 }},
		{"skip", "unit must", func(b *Batch) { b.Unit = 3 }},
		{"no-claims", "claims", func(b *Batch) { b.Claims = nil }},
		{"retire-without-reason", "hook transition", func(b *Batch) { b.HookUpdates = []HookUpdate{{ID: "h.number", Status: "retired", Evidence: "1-1"}} }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := newProject(t)
			head := accept(t, dir, seed())
			b := chapter1()
			tt.change(&b)
			_, err := Accept(dir, b, []byte("正文"))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("want %q, got %v", tt.want, err)
			}
			s, err := Load(dir)
			if err != nil || s.Revision != 1 || s.Head != head.Hash {
				t.Fatal("rejected batch mutated canon", err)
			}
		})
	}
}

func TestBackdatedChangeCannotInvalidateAcceptedChapter(t *testing.T) {
	dir := newProject(t)
	accept(t, dir, seed())
	accept(t, dir, chapter1())
	b := chapter2()
	b.Events = nil
	b.Facts = []Fact{{ID: "f.owner3", Subject: "i.watch", Key: "owner", Value: "c.lin", From: 11, Supersedes: "f.owner2", Evidence: "回忆"}}
	// A later claim of ownership in an accepted chapter must survive a new flashback.
	b1 := chapter1()
	b1.Claims = append(b1.Claims, Claim{Subject: "i.watch", Key: "owner", Value: "c.shen", At: 15, Evidence: "1-3"})
	dir2 := newProject(t)
	accept(t, dir2, seed())
	accept(t, dir2, b1)
	_, err := Accept(dir2, b, []byte("倒叙正文"))
	if err == nil {
		t.Fatal("backdated contradiction accepted")
	}
}

// Synthetic drama corpus: tests continuity across 1000 episodes and a real
// post-recovery accept, not the literary quality of a million-character script.
func TestThousandEpisodeDramaRecovery(t *testing.T) {
	dir := newProject(t)
	initial := seed()
	initial.Hooks[0].DueUnit = 1000
	accept(t, dir, initial)
	s, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	hanCount := 0
	for n := 1; n <= 1000; n++ {
		b := Batch{BaseRevision: s.Revision, Unit: n, Summary: fmt.Sprintf("第%d集：连续性合成样本", n), Claims: []Claim{{Subject: "c.lin", Key: "identity", Value: "修复师", At: n * 10, Evidence: fmt.Sprintf("第%d集第一场", n)}}}
		if n == 1 {
			b = chapter1()
		}
		if n == 500 {
			b.Knowledge = []Knowledge{{ID: "k.shen", Character: "c.shen", FactID: "f.secret", At: n * 10, Evidence: "第500集打开表壳"}}
			b.Events = []Event{{ID: "e.open", At: n * 10, Participants: []string{"c.shen", "i.watch"}, Summary: "打开表壳", Causes: []string{"e.transfer"}, Requires: []string{"f.owner2"}, Evidence: "第500集"}}
			b.Claims = append(b.Claims, Claim{Subject: "i.watch", Key: "secret", Value: "表壳内有编号七", At: n * 10, Knower: "c.shen", Evidence: "第500集沈砚读出编号"})
		}
		if n == 1000 {
			b.HookUpdates = []HookUpdate{{ID: "h.number", Status: "paid", Evidence: "第1000集编号对上档案"}}
			b.Claims = append(b.Claims, Claim{Subject: "i.watch", Key: "owner", Value: "c.shen", At: n * 10, Evidence: "第1000集沈砚保管怀表"})
		}
		entityID := fmt.Sprintf("c.extra%d", n)
		b.Entities = append(b.Entities, Entity{ID: entityID, Kind: "character", Name: fmt.Sprintf("路人%d", n), Description: "不属于当前场景的历史人物"})
		b.Facts = append(b.Facts, Fact{ID: fmt.Sprintf("f.extra%d", n), Subject: entityID, Key: "identity", Value: "历史人物", From: n, Locked: true, Evidence: "合成正文设定"})
		prev := s.Head
		if _, err = s.Apply(b); err != nil {
			t.Fatal(n, err)
		}
		scene := fmt.Sprintf("第%d集\n%d-1 修复室 内 夜\n", n, n)
		switch n {
		case 1:
			scene += "林岚将怀表交给沈砚保管。\n"
		case 500:
			scene += "沈砚打开怀表，读出表壳里的编号七。\n"
		case 1000:
			scene += "沈砚拿着怀表，发现编号七对应的档案。\n"
		}
		draft := []byte(scene + strings.Repeat("微雨敲窗。", 300))
		for _, r := range string(draft) {
			if unicode.Is(unicode.Han, r) {
				hanCount++
			}
		}
		hash := digest(draft)
		if err = atomicNew(filepath.Join(dir, "blobs", hash+".md"), draft); err != nil {
			t.Fatal(err)
		}
		r := Record{Schema: Schema, Revision: s.Revision, PrevHash: prev, Batch: b, Draft: &Draft{SHA256: hash}}
		r.Hash = recordHash(r)
		data, _ := json.Marshal(r)
		if err = atomicNew(filepath.Join(dir, "records", fmt.Sprintf("%08d.json", r.Revision)), data); err != nil {
			t.Fatal(err)
		}
		s.Head = r.Hash
	}
	if hanCount <= 1_000_000 {
		t.Fatalf("stress corpus has only %d Han characters", hanCount)
	}
	result, err := Verify(dir)
	if err != nil || result.Revision != 1001 || result.Unit != 1000 {
		t.Fatal(result, err)
	}
	data, err := ContextProjectScoped(dir, []string{"c.lin", "c.shen", "i.watch"}, 10000, 2, 3, 6000, true)
	if err != nil || len(data) > 6000 {
		t.Fatal("unbounded context", len(data), err)
	}
	var p Packet
	if err = Decode(data, &p); err != nil || p.NextUnit != 1001 || p.OmittedEntities != 1000 {
		t.Fatal("wrong resume point", err)
	}
	if len(p.Relations) != 1 || len(p.Knowledge) != 2 {
		t.Fatalf("long-term relationship or knowledge missing: %+v", p)
	}
	recovered, err := Load(dir)
	if err != nil || recovered.Hooks["h.number"].Status != "paid" || recovered.Facts["f.owner2"].Value != "c.shen" {
		t.Fatal("lost long-term canon", err)
	}
	next := Batch{BaseRevision: 1001, Unit: 1001, Summary: "跨批次续写", Claims: []Claim{{Subject: "i.watch", Key: "secret", Value: "表壳内有编号七", At: 4999, Knower: "c.shen", Evidence: "错误地提前知晓"}}}
	if _, err = Check(dir, next, []byte("沈砚说出编号。")); err == nil || !strings.Contains(err.Error(), "knowledge leak") {
		t.Fatalf("early knowledge should be rejected: %v", err)
	}
	if result, err := Verify(dir); err != nil || result.Revision != 1001 {
		t.Fatalf("rejected batch changed canon: %+v, %v", result, err)
	}
	next.Claims[0].At = 10010
	next.Claims[0].Evidence = "第1001集沈砚记得此前读到的编号"
	if _, err = Accept(dir, next, []byte("第1001集：沈砚拿着怀表，回忆起表壳里的编号七。")); err != nil {
		t.Fatal("failed to resume after 1000 episodes", err)
	}
	if result, err := Verify(dir); err != nil || result.Revision != 1002 || result.Unit != 1001 {
		t.Fatalf("post-resume verification failed: %+v, %v", result, err)
	}
}

func TestHashIntegrityAndDraftBinding(t *testing.T) {
	for _, mode := range []string{"draft", "record", "project", "gap"} {
		t.Run(mode, func(t *testing.T) {
			dir := newProject(t)
			accept(t, dir, seed())
			accept(t, dir, chapter1())
			s, err := Load(dir)
			if err != nil {
				t.Fatal(err)
			}
			switch mode {
			case "draft":
				err = os.WriteFile(filepath.Join(dir, "blobs", s.Records[1].Draft.SHA256+".md"), []byte("偷偷改正文"), 0600)
			case "record":
				path := filepath.Join(dir, "records", "00000002.json")
				data, _ := os.ReadFile(path)
				err = os.WriteFile(path, []byte(strings.Replace(string(data), "沈砚接收怀表", "沈砚毁掉怀表", 1)), 0600)
			case "project":
				path := filepath.Join(dir, "project.json")
				data, _ := os.ReadFile(path)
				err = os.WriteFile(path, []byte(strings.Replace(string(data), "七号怀表", "八号怀表", 1)), 0600)
			case "gap":
				err = os.Rename(filepath.Join(dir, "records", "00000002.json"), filepath.Join(dir, "records", "00000003.json"))
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err := Verify(dir); err == nil {
				t.Fatal("tampering not detected")
			}
		})
	}
}

func TestConcurrentWritersAndDryRun(t *testing.T) {
	dir := newProject(t)
	accept(t, dir, seed())
	b := chapter1()
	if _, err := Check(dir, b, []byte("草稿")); err != nil {
		t.Fatal(err)
	}
	s, _ := Load(dir)
	if s.Revision != 1 {
		t.Fatal("dry run wrote state")
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	success := 0
	for range 8 {
		wg.Go(func() {
			if _, err := Accept(dir, b, []byte("同一草稿")); err == nil {
				mu.Lock()
				success++
				mu.Unlock()
			}
		})
	}
	wg.Wait()
	if success != 1 {
		t.Fatalf("want one accepted writer, got %d", success)
	}
	if _, err := Verify(dir); err != nil {
		t.Fatal(err)
	}
}

func TestContextBudgetAndBoundary(t *testing.T) {
	dir := newProject(t)
	accept(t, dir, seed())
	s, _ := Load(dir)
	data, err := Context(s, []string{"c.shen"}, 0, 0, 1, 24000)
	if err != nil {
		t.Fatal(err)
	}
	var p Packet
	Decode(data, &p)
	if len(p.BoundaryRelations) != 1 || len(p.Relations) != 0 {
		t.Fatal("depth 0 must preserve the boundary without expanding unrelated locked identities")
	}
	if _, err = Context(s, []string{"c.lin"}, 0, 2, 1, 512); err == nil || !strings.Contains(err.Error(), "no canon") {
		t.Fatal("essential canon silently truncated", err)
	}
	if _, err = Context(s, []string{"missing"}, 0, 2, 1, 24000); err == nil {
		t.Fatal("unknown entity accepted")
	}
}

func TestStrictJSON(t *testing.T) {
	for _, data := range []string{`{"typo":1}`, `{} {}`, `[]`} {
		var b Batch
		if Decode([]byte(data), &b) == nil {
			t.Fatal("bad JSON accepted", data)
		}
	}
	var b Batch
	if err := Decode(append([]byte{0xef, 0xbb, 0xbf}, []byte(`{"unit":0}`)...), &b); err != nil {
		t.Fatal(err)
	}
}

func TestGlobalWorldRuleSurvivesFocusedRetrieval(t *testing.T) {
	dir := newProject(t)
	b := seed()
	b.Entities = append(b.Entities, Entity{ID: "w.rules", Kind: "world", Name: "世界法则", Description: "对所有场景生效"})
	b.Facts = append(b.Facts, Fact{ID: "f.rule", Subject: "w.rules", Key: "resurrection", Value: "不允许", From: 0, Locked: true, Evidence: "世界设定"})
	accept(t, dir, b)
	s, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	data, err := Context(s, []string{"c.shen"}, 0, 0, 0, 24000)
	if err != nil {
		t.Fatal(err)
	}
	var p Packet
	if err = Decode(data, &p); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, f := range p.Facts {
		if f.ID == "f.rule" {
			found = true
		}
	}
	if !found {
		t.Fatal("global world constraint omitted")
	}
	if len(p.KnowledgeScope) != 1 || p.KnowledgeScope[0] != "c.shen" {
		t.Fatal("ambiguous knowledge retrieval scope", p.KnowledgeScope)
	}
}

func TestStrategyCompositionAndExtensions(t *testing.T) {
	root := filepath.Join("..", "..")
	all, err := LoadStrategies([]string{filepath.Join(root, "strategies"), filepath.Join(root, "examples", "strategies")})
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 35 {
		t.Fatalf("expected 33 builtin + 2 extension strategies, got %d", len(all))
	}
	for _, id := range sortedKeys(all) {
		if _, err := Compose(all, []string{id}); err != nil {
			t.Fatal(id, err)
		}
	}
	prompt, err := Compose(all, []string{"genre.cyber-mystery", "structure.novel-serial", "style.sparse-poetic"})
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"genre.suspense", "genre.scifi", "记忆所有权", "sparse-poetic", "medium: novel"} {
		if !strings.Contains(prompt, s) {
			t.Fatalf("missing inherited prompt %s", s)
		}
	}
	for _, ids := range [][]string{{"style.lush", "style.restrained"}, {"structure.short-drama", "structure.novel-serial"}, {"style.first-person", "style.cinematic"}, {"genre.missing"}} {
		if _, err := Compose(all, ids); err == nil {
			t.Fatal("conflict accepted", ids)
		}
	}
	for _, genre := range sortedKeys(all) {
		if all[genre].Kind != "genre" {
			continue
		}
		for _, medium := range []string{"structure.short-drama", "structure.novel-serial", "structure.film", "structure.bilibili", "structure.douyin", "structure.product-story", "structure.scene"} {
			if _, err := Compose(all, []string{genre, medium, "style.restrained"}); err != nil {
				t.Fatal(genre, medium, err)
			}
		}
	}
}

func TestInvalidStrategyGraphs(t *testing.T) {
	base := Strategy{ID: "style.a", Kind: "style", Title: "a", Prompts: []string{"p"}, Checks: []string{"c"}}
	for _, mode := range []string{"cycle", "duplicate", "unknown", "conflict"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			a := base
			b := base
			b.ID = "style.b"
			switch mode {
			case "cycle":
				a.Extends = []string{b.ID}
				b.Extends = []string{a.ID}
			case "duplicate":
				b.ID = a.ID
			case "unknown":
				a.Extends = []string{"style.missing"}
			case "conflict":
				a.Conflicts = []string{b.ID}
				b.Extends = []string{a.ID}
			}
			data, _ := json.Marshal([]Strategy{a, b})
			if err := os.WriteFile(filepath.Join(dir, "custom.json"), data, 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := LoadStrategies([]string{dir}); err == nil {
				t.Fatal("invalid strategy graph accepted")
			}
		})
	}
}
