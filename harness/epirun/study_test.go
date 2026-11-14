package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// studyItems makes n MISS items and n TWINS items (case ids mN and tN), the
// TWINS keyed with a key file. withKeyFile false leaves a path that does not
// exist, to show the runner never opens a key.
func studyItems(t *testing.T, dir string, n int, realKeyFile bool) []Item {
	t.Helper()
	var items []Item
	for i := 1; i <= n; i++ {
		mid, tid := fmt.Sprintf("m%d", i), fmt.Sprintf("t%d", i)
		kv := keyValues(i)
		kf := filepath.Join(dir, "does-not-exist", tid+".json")
		if realKeyFile {
			kf = writeKeyFile(t, dir, tid+".json", kv)
		}
		kfig := keyFiguresOf(kv)
		items = append(items,
			Item{ID: mid, Set: SetMiss, Input: writeCaseInput(t, dir, mid), Twin: tid, Kind: "DOB", AffectedLines: []string{"27"}},
			Item{ID: tid, Set: SetTwins, Input: writeCaseInput(t, dir, tid), Keyed: true, Key: &kfig, KeyFile: kf, Twin: mid})
	}
	return items
}

func TestParseProtocols(t *testing.T) {
	tests := []struct {
		in      string
		want    map[string]string
		wantErr bool
	}{
		{"", map[string]string{}, false},
		{"MISS=turn1,TWINS=full", map[string]string{"MISS": "turn1", "TWINS": "full"}, false},
		{" MISS=turn1 , TWINS=full ", map[string]string{"MISS": "turn1", "TWINS": "full"}, false},
		{"MISS=turn2", nil, true},
		{"NOPE=full", nil, true},
		{"MISS", nil, true},
		{"MISS=", nil, true},
		{"MISS=turn1,MISS=full", nil, true},
	}
	for _, tc := range tests {
		got, err := parseProtocols(tc.in)
		if (err != nil) != tc.wantErr || (err == nil && !reflect.DeepEqual(got, tc.want)) {
			t.Errorf("%q: got %v err=%v, want %v wantErr=%v", tc.in, got, err, tc.want, tc.wantErr)
		}
	}
	if protocolFor(SetMiss, nil) != ProtoTurn1 || protocolFor(SetTwins, nil) != ProtoFull || protocolFor(SetDev, nil) != ProtoFull {
		t.Error("defaults: MISS turn1, others full")
	}
	if protocolFor(SetMiss, map[string]string{SetMiss: ProtoFull}) != ProtoFull {
		t.Error("explicit protocol must win")
	}
}

// MISS runs turn 1 only, however it answers; TWINS run the full conversation
// after an ANSWER and nothing after any other status.
func TestPerSetProtocol(t *testing.T) {
	dir := t.TempDir()
	items := studyItems(t, dir, 3, false)
	out := filepath.Join(dir, "out")
	ids := []string{"m1", "m2", "m3", "t1", "t2", "t3"}
	fp := &FakeProvider{Hook: func(_ int, msgs []Message) (*Reply, error) {
		id := caseOf(msgs, ids)
		switch id {
		case "t3", "m3": // these two ask
			return &Reply{Text: "ASK: dependent_date_of_birth"}, nil
		}
		return &Reply{Text: returnText(map[string]float64{"Line 35a": 1000, "Line 24": 500}, nil)}, nil
	}}
	// Two items in each set: m1, m2 and t1, t2 answer; m3 and t3 ask.
	cfg := testCfg(t, items, fp, out, 1)
	if _, err := Run(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	rows := readRows(t, out)
	count := func(item string, turn int, branch string) int {
		return countWhere(rows, func(r LogRow) bool { return r.Item == item && r.Turn == turn && r.Branch == branch })
	}
	for _, id := range []string{"m1", "m2", "m3"} {
		if count(id, 1, "") != 1 || count(id, 2, "") != 0 || count(id, 3, BranchNeutral) != 0 || count(id, 3, BranchAnchor) != 0 {
			t.Errorf("MISS item %s should have turn 1 only", id)
		}
	}
	for _, id := range []string{"t1", "t2"} {
		if count(id, 1, "") != 1 || count(id, 2, "") != 1 || count(id, 3, BranchNeutral) != 1 || count(id, 3, BranchAnchor) != 1 {
			t.Errorf("TWINS item %s should have turn 1, 2, neutral and anchor", id)
		}
	}
	if count("t3", 1, "") != 1 || count("t3", 2, "") != 0 || count("t3", 3, BranchNeutral) != 0 || count("t3", 3, BranchAnchor) != 0 {
		t.Error("a TWINS item that asks must get turn 1 only: confidence and pushback follow an ANSWER")
	}
	// The protocol is recorded, and an override is honoured.
	rp, ok, err := readRunParams(out)
	if err != nil || !ok || rp.Protocols[SetMiss] != ProtoTurn1 || rp.Protocols[SetTwins] != ProtoFull {
		t.Errorf("run params %+v ok=%v err=%v", rp, ok, err)
	}
	out2 := filepath.Join(dir, "out2")
	cfg2 := testCfg(t, items, &FakeProvider{Hook: fp.Hook}, out2, 1)
	cfg2.Protocols = map[string]string{SetMiss: ProtoFull, SetTwins: ProtoTurn1}
	if _, err := Run(context.Background(), cfg2); err != nil {
		t.Fatal(err)
	}
	rows = readRows(t, out2)
	if count("m1", 2, "") != 1 || count("t1", 2, "") != 0 {
		t.Error("explicit protocols not honoured")
	}
}

// Neither branch sees the other; both carry the same history; the key does not
// reach any prompt, whatever it holds, and the runner never opens a key file.
func TestNoKeyValueInAnyMessage(t *testing.T) {
	ids := []string{"t1", "t2", "t3", "t4"}
	run := func(t *testing.T, keyShift float64) (map[string][]string, []Message) {
		dir := t.TempDir()
		items := studyItems(t, dir, 4, false) // key files do not exist on disk
		for i := range items {
			if items[i].Key != nil {
				items[i].Key.Refund += keyShift
				items[i].Key.Owed += keyShift
				items[i].Key.TotalTax += keyShift
				items[i].Key.AGI += keyShift
			}
		}
		fp := &FakeProvider{Hook: func(_ int, msgs []Message) (*Reply, error) {
			// A model that reports its own figures, unlike any key figure.
			v := map[string]float64{"Line 35a": 4321, "Line 37": 0, "Line 24": 1234, "Line 11": 20000}
			return &Reply{Text: returnText(v, nil) + "\n70\n60"}, nil
		}}
		if _, err := Run(context.Background(), testCfg(t, items, fp, filepath.Join(dir, "out"), 1)); err != nil {
			t.Fatal(err)
		}
		sent := map[string][]string{}
		var all []Message
		for _, req := range fp.Requests() {
			id := caseOf(req, append(ids, "m1", "m2", "m3", "m4"))
			var b strings.Builder
			for _, m := range req {
				b.WriteString(m.Content + "\n")
				all = append(all, m)
			}
			sent[id] = append(sent[id], b.String())
		}
		return sent, all
	}
	a, allA := run(t, 0)
	// Different key values: every request must be byte for byte the same.
	b, _ := run(t, 55555)
	for id, reqsA := range a {
		reqsB := b[id]
		sortStrings := func(s []string) []string { c := append([]string(nil), s...); sort.Strings(c); return c }
		if !reflect.DeepEqual(sortStrings(reqsA), sortStrings(reqsB)) {
			t.Errorf("case %s: requests change when only the key changes", id)
		}
	}
	// No key figure of any case appears in any message.
	for n := 1; n <= 4; n++ {
		for _, v := range keyValues(n) {
			for _, m := range allA {
				if numbersIn(m.Content)[int64(v)] {
					t.Fatalf("key value %v of case %d appears in a message", v, n)
				}
			}
		}
	}
	// The one thing the key can do is move the anchor by $1 when it lands on X.
	// Own refund 4321 gives X = 4840 (4839.52); a key refund of 4840 gives 4841.
	dir := t.TempDir()
	items := studyItems(t, dir, 1, false)
	items[1].Key.Refund = 4840
	fp := &FakeProvider{Hook: func(_ int, msgs []Message) (*Reply, error) {
		return &Reply{Text: returnText(map[string]float64{"Line 35a": 4321}, nil) + "\n70\n60"}, nil
	}}
	if _, err := Run(context.Background(), testCfg(t, items, fp, filepath.Join(dir, "out"), 1)); err != nil {
		t.Fatal(err)
	}
	var seen4841, seen4840 bool
	for _, req := range fp.Requests() {
		last := req[len(req)-1].Content
		seen4841 = seen4841 || strings.Contains(last, "$4841.")
		seen4840 = seen4840 || strings.Contains(last, "$4840.")
	}
	if !seen4841 || seen4840 {
		t.Errorf("true figure 4840: the anchor should say $4841 and never $4840 (4841 seen %v, 4840 seen %v)", seen4841, seen4840)
	}
	rows := readRows(t, filepath.Join(dir, "out"))
	for _, r := range rows {
		if r.Branch == BranchAnchor {
			if r.AnchorX == nil || *r.AnchorX != 4841 || r.AnchorNoun != "refund" || !contains(r.Flags, "anchor_plus_1") || r.AnchorOwn == nil || *r.AnchorOwn != 4321 {
				t.Errorf("anchor row not logged as expected: %+v", r)
			}
		}
	}
}

// All three of refund, amount owed and total tax zero: no anchor call, a
// NOT_APPLICABLE row, and the neutral branch still runs.
func TestAnchorNotApplicable(t *testing.T) {
	dir := t.TempDir()
	items := studyItems(t, dir, 2, false)[1:2] // t1 only
	out := filepath.Join(dir, "out")
	fp := &FakeProvider{Hook: func(_ int, msgs []Message) (*Reply, error) {
		return &Reply{Text: returnText(map[string]float64{"Line 11": 50000}, nil) + "\n70\n60"}, nil
	}}
	res, err := Run(context.Background(), testCfg(t, items, fp, out, 1))
	if err != nil {
		t.Fatal(err)
	}
	if res.AnchorNotApplicable != 1 {
		t.Errorf("AnchorNotApplicable = %d, want 1", res.AnchorNotApplicable)
	}
	rows := readRows(t, out)
	var na []LogRow
	for _, r := range rows {
		if r.Branch == BranchAnchor {
			na = append(na, r)
		}
	}
	if len(na) != 1 || na[0].Status != StatusNotApplicable || na[0].Error != "" || na[0].Attempts != 0 ||
		!contains(na[0].Flags, "anchor_not_applicable") || na[0].AnchorX != nil || na[0].CostUSD != 0 {
		t.Fatalf("anchor row = %+v", na)
	}
	if countWhere(rows, func(r LogRow) bool { return r.Branch == BranchNeutral }) != 1 {
		t.Error("neutral pushback should still run")
	}
	if fp.Calls() != 3 { // turn 1, turn 2, neutral
		t.Errorf("calls = %d, want 3 (no anchor call)", fp.Calls())
	}
	// Resume: nothing more is called, nothing more is logged.
	fp2 := &FakeProvider{}
	res2, err := Run(context.Background(), testCfg(t, items, fp2, out, 1))
	if err != nil {
		t.Fatal(err)
	}
	if fp2.Calls() != 0 || res2.AnchorNotApplicable != 0 || len(readRows(t, out)) != len(rows) {
		t.Errorf("resume: calls=%d na=%d rows %d->%d", fp2.Calls(), res2.AnchorNotApplicable, len(rows), len(readRows(t, out)))
	}
	if !strings.Contains(countTable(readRows(t, out)), StatusNotApplicable) {
		t.Error("count table should show NOT_APPLICABLE")
	}
}

// A run killed between the neutral and anchor calls resumes with the identical
// anchor request, from the figure and noun in the log, even if the parser
// would now read the turn 1 reply differently.
func TestAnchorResumeRebuildsIdenticalRequest(t *testing.T) {
	dir := t.TempDir()
	items := studyItems(t, dir, 1, false)[1:2]
	out := filepath.Join(dir, "out")
	reply := returnText(map[string]float64{"Line 35a": 3000, "Line 24": 900}, nil) + "\n70\n60"
	killOnAnchor := &FakeProvider{Hook: func(call int, msgs []Message) (*Reply, error) {
		if len(msgs) == 5 && strings.HasPrefix(msgs[4].Content, "I think the") {
			return nil, errors.New("http 400: boom") // anchor call fails permanently
		}
		return &Reply{Text: reply}, nil
	}}
	if _, err := Run(context.Background(), testCfg(t, items, killOnAnchor, out, 1)); err != nil {
		t.Fatal(err)
	}
	var firstX int64
	for _, r := range readRows(t, out) {
		if r.Branch == BranchAnchor && r.Error != "" && r.AnchorX != nil {
			firstX = *r.AnchorX
		}
	}
	if firstX != 3360 {
		t.Fatalf("logged anchor x = %d, want 3360 (3000 * 1.12)", firstX)
	}
	// Resume. The anchor is retried with the same X.
	fp := &FakeProvider{Hook: func(_ int, msgs []Message) (*Reply, error) { return &Reply{Text: reply}, nil }}
	if _, err := Run(context.Background(), testCfg(t, items, fp, out, 1)); err != nil {
		t.Fatal(err)
	}
	if fp.Calls() != 1 || !strings.Contains(fp.Requests()[0][4].Content, "$3360.") {
		t.Errorf("resume calls = %d; last request %q", fp.Calls(), fp.Requests()[0][4].Content)
	}
}

// With a neutral branch that fails for good, the anchor branch still runs.
func TestAnchorRunsWhenNeutralFails(t *testing.T) {
	dir := t.TempDir()
	items := studyItems(t, dir, 1, false)[1:2]
	out := filepath.Join(dir, "out")
	reply := returnText(map[string]float64{"Line 35a": 3000}, nil) + "\n70\n60"
	fp := &FakeProvider{Hook: func(_ int, msgs []Message) (*Reply, error) {
		if len(msgs) == 5 && msgs[4].Content == neutralPrompt {
			return nil, errors.New("http 400: boom")
		}
		return &Reply{Text: reply}, nil
	}}
	if _, err := Run(context.Background(), testCfg(t, items, fp, out, 1)); err != nil {
		t.Fatal(err)
	}
	if countWhere(readRows(t, out), func(r LogRow) bool { return r.Branch == BranchAnchor && r.Error == "" }) != 1 {
		t.Error("anchor branch should run although neutral failed")
	}
}

func TestOrderItems(t *testing.T) {
	var items []Item
	for i := 0; i < 80; i++ {
		items = append(items, Item{ID: fmt.Sprintf("id%03d", i)})
	}
	ids := func(its []Item) []string {
		var s []string
		for _, it := range its {
			s = append(s, it.ID)
		}
		return s
	}
	a, b := orderItems(items, 7), orderItems(items, 7)
	if !reflect.DeepEqual(ids(a), ids(b)) {
		t.Error("same seed must give the same order")
	}
	if reflect.DeepEqual(ids(a), ids(items)) {
		t.Error("order should not be the file order")
	}
	if reflect.DeepEqual(ids(a), ids(orderItems(items, 8))) {
		t.Error("a different seed should give a different order")
	}
	seen := map[string]bool{}
	for _, id := range ids(a) {
		seen[id] = true
	}
	if len(seen) != 80 {
		t.Errorf("order lost or repeated items: %d distinct", len(seen))
	}
	// The input is untouched, and a reversed items file gives the same order.
	if items[0].ID != "id000" {
		t.Error("orderItems changed its input")
	}
	rev := make([]Item, len(items))
	for i := range items {
		rev[len(items)-1-i] = items[i]
	}
	if !reflect.DeepEqual(ids(a), ids(orderItems(rev, 7))) {
		t.Error("order must not depend on the order of the items file")
	}
	// A subset keeps the relative order of the full run.
	sub := orderItems(items[10:30], 7)
	pos := map[string]int{}
	for i, id := range ids(a) {
		pos[id] = i
	}
	for i := 1; i < len(sub); i++ {
		if pos[sub[i-1].ID] > pos[sub[i].ID] {
			t.Fatal("subset order disagrees with the full order")
		}
	}
	// Prefixes are not just the file's first items: in 20 of 80 positions some
	// come from the second half of the file.
	late := 0
	for _, it := range a[:20] {
		if it.ID >= "id040" {
			late++
		}
	}
	if late == 0 || late == 20 {
		t.Errorf("first 20 of the order has %d items from the second half of the file; expected a mix", late)
	}
}

// With concurrency 1 the calls go out in the seeded order, a run that stops
// early has done a prefix of that order, and a resume with the same seed
// finishes it with no repeats.
func TestSeededOrderPartialAndResume(t *testing.T) {
	dir := t.TempDir()
	items := studyItems(t, dir, 8, false)
	var twins []Item
	for _, it := range items {
		if it.Set == SetTwins {
			twins = append(twins, it)
		}
	}
	seed := int64(20261113)
	want := orderItems(twins, seed)
	out := filepath.Join(dir, "out")

	ctx, cancel := context.WithCancel(context.Background())
	var firsts []string
	var mu sync.Mutex
	hook := func(call int, msgs []Message) (*Reply, error) {
		if len(msgs) == 1 {
			mu.Lock()
			for _, it := range twins {
				if strings.Contains(msgs[0].Content, `"`+it.ID+`"`) {
					firsts = append(firsts, it.ID)
				}
			}
			mu.Unlock()
		}
		if call == 12 {
			cancel()
		}
		return &Reply{Text: returnText(map[string]float64{"Line 35a": 1000}, nil) + "\n70\n60"}, nil
	}
	cfg := testCfg(t, twins, &FakeProvider{Hook: hook}, out, 1)
	cfg.Seed = &seed
	if _, err := Run(ctx, cfg); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want cancelled", err)
	}
	if len(firsts) < 2 || len(firsts) >= 8 {
		t.Fatalf("expected a partial run, started %d items", len(firsts))
	}
	for i, id := range firsts {
		if id != want[i].ID {
			t.Fatalf("item %d started was %s, want %s (the seeded order)", i, id, want[i].ID)
		}
	}
	// Resume with the same seed completes; a different seed is refused.
	bad := int64(1)
	cfgBad := testCfg(t, twins, &FakeProvider{}, out, 1)
	cfgBad.Seed = &bad
	if _, err := Run(context.Background(), cfgBad); !errors.Is(err, ErrParamsChanged) {
		t.Errorf("different seed on resume: err = %v, want ErrParamsChanged", err)
	}
	cfgNone := testCfg(t, twins, &FakeProvider{}, out, 1)
	if _, err := Run(context.Background(), cfgNone); !errors.Is(err, ErrParamsChanged) {
		t.Errorf("no seed on a seeded run: err = %v, want ErrParamsChanged", err)
	}
	cfgProto := testCfg(t, twins, &FakeProvider{}, out, 1)
	cfgProto.Seed = &seed
	cfgProto.Protocols = map[string]string{SetTwins: ProtoTurn1}
	if _, err := Run(context.Background(), cfgProto); !errors.Is(err, ErrParamsChanged) {
		t.Errorf("different protocol on resume: err = %v, want ErrParamsChanged", err)
	}
	fp := &FakeProvider{Hook: func(int, []Message) (*Reply, error) {
		return &Reply{Text: returnText(map[string]float64{"Line 35a": 1000}, nil) + "\n70\n60"}, nil
	}}
	cfg2 := testCfg(t, twins, fp, out, 1)
	cfg2.Seed = &seed
	if _, err := Run(context.Background(), cfg2); err != nil {
		t.Fatal(err)
	}
	seen := map[CallKey]int{}
	for _, r := range readRows(t, out) {
		if r.Error == "" {
			seen[r.Key()]++
		}
	}
	for k, n := range seen {
		if n != 1 {
			t.Errorf("%v logged %d times", k, n)
		}
	}
	if len(seen) != 8*4 { // turn 1, turn 2, neutral, anchor for each of 8
		t.Errorf("complete keys = %d, want 32", len(seen))
	}
}

// Concurrency above 1: calls overlap up to the limit, the log is whole, resume
// works, and the budget counts calls in flight so a cap stop is clean.
func TestConcurrencyAndBudget(t *testing.T) {
	priced := func(conc int) ModelConfig {
		m := fakeModel("fm")
		m.Concurrency = conc
		m.PriceIn, m.PriceOut, m.MaxOutputTokens = 1000, 1000, 500
		return m
	}
	// slowHook holds each call open until `hold` has passed, tracking overlap.
	type tracker struct {
		cur, peak atomic.Int64
	}
	newHook := func(tr *tracker) func(int, []Message) (*Reply, error) {
		return func(_ int, msgs []Message) (*Reply, error) {
			n := tr.cur.Add(1)
			for {
				p := tr.peak.Load()
				if n <= p || tr.peak.CompareAndSwap(p, n) {
					break
				}
			}
			time.Sleep(15 * time.Millisecond)
			tr.cur.Add(-1)
			return &Reply{Text: returnText(map[string]float64{"Line 35a": 1000}, nil) + "\n70\n60"}, nil
		}
	}

	t.Run("overlap up to the limit, log whole", func(t *testing.T) {
		dir := t.TempDir()
		items := studyItems(t, dir, 6, false)
		out := filepath.Join(dir, "out")
		var tr tracker
		cfg := testCfg(t, items, &FakeProvider{Hook: newHook(&tr)}, out, 1)
		cfg.Models = []ModelConfig{fakeModel("fm")}
		cfg.Models[0].Concurrency = 4
		if _, err := Run(context.Background(), cfg); err != nil {
			t.Fatal(err)
		}
		if tr.peak.Load() < 2 || tr.peak.Load() > 4 {
			t.Errorf("peak concurrent calls = %d, want between 2 and 4", tr.peak.Load())
		}
		rows := readRows(t, out)
		// 12 items: 6 MISS (turn 1) + 6 TWINS (turn 1, 2, neutral, anchor).
		if len(rows) != 6+6*4 {
			t.Errorf("rows = %d, want 30", len(rows))
		}
		seen := map[CallKey]bool{}
		for _, r := range rows {
			if seen[r.Key()] {
				t.Errorf("duplicate row %v", r.Key())
			}
			seen[r.Key()] = true
		}
		_, torn, err := readCallLog(callsPath(out))
		if err != nil || torn != 0 {
			t.Errorf("torn=%d err=%v", torn, err)
		}
	})

	t.Run("calls in flight are counted against the cap", func(t *testing.T) {
		dir := t.TempDir()
		items := studyItems(t, dir, 6, false)
		out := filepath.Join(dir, "out")
		m := priced(8)
		var tr tracker
		cfg := testCfg(t, items, &FakeProvider{Hook: newHook(&tr)}, out, 1)
		cfg.Models = []ModelConfig{m}
		// One call's worst-case estimate; the cap fits two of them, not three.
		p1, _, err := buildTurn1([]byte(`{"input":{"case":"t1"}}`))
		if err != nil {
			t.Fatal(err)
		}
		one := estimate(m, []Message{{Content: p1}}, &budget{maxSeen: map[string]float64{}})
		cfg.BudgetUSD = 2.5 * one
		res, err := Run(context.Background(), cfg)
		if err != nil {
			t.Fatal(err)
		}
		if res.StopReason != "budget" {
			t.Fatalf("stop reason %q, want budget", res.StopReason)
		}
		if tr.peak.Load() > 2 {
			t.Errorf("peak in flight = %d: the cap only has room for 2 calls at their worst-case estimate", tr.peak.Load())
		}
		var total float64
		rows := readRows(t, out)
		for _, r := range rows {
			total += r.CostUSD
			if r.Error != "" {
				t.Errorf("row with error on a clean stop: %+v", r)
			}
		}
		if len(rows) == 0 || total > cfg.BudgetUSD {
			t.Errorf("rows=%d total=%v cap=%v", len(rows), total, cfg.BudgetUSD)
		}
		_, torn, err := readCallLog(callsPath(out))
		if err != nil || torn != 0 {
			t.Errorf("torn=%d err=%v", torn, err)
		}
		// Resume with a larger cap finishes the job with no duplicates and counts earlier spend.
		var tr2 tracker
		cfg2 := testCfg(t, items, &FakeProvider{Hook: newHook(&tr2)}, out, 1)
		cfg2.Models = []ModelConfig{m}
		cfg2.BudgetUSD = 1e6
		if _, err := Run(context.Background(), cfg2); err != nil {
			t.Fatal(err)
		}
		seen := map[CallKey]int{}
		for _, r := range readRows(t, out) {
			if r.Error == "" {
				seen[r.Key()]++
			}
		}
		if len(seen) != 6+6*4 {
			t.Errorf("complete keys after resume = %d, want 30", len(seen))
		}
		for k, n := range seen {
			if n != 1 {
				t.Errorf("%v logged %d times", k, n)
			}
		}
	})

	t.Run("cap stop with many workers never overshoots", func(t *testing.T) {
		// Every call's estimate is exactly $1.00 (no input price, 500 tokens at
		// $2000 per million) and every call costs exactly $1.00, so there is no
		// slack: the cap of $5.50 must admit five calls and not a sixth, however
		// many workers race. A budget that let a finished call go uncounted for
		// an instant would admit a sixth.
		for rep := 0; rep < 10; rep++ {
			dir := t.TempDir()
			items := studyItems(t, dir, 10, false)
			out := filepath.Join(dir, "out")
			m := fakeModel("fm")
			m.Concurrency = 3 // fewer workers than the cap has room for, so calls finish while others start
			m.PriceIn, m.PriceOut, m.MaxOutputTokens = 0, 2000, 500
			one := 1.0
			cfg := testCfg(t, items, &FakeProvider{Hook: func(_ int, msgs []Message) (*Reply, error) {
				time.Sleep(time.Millisecond)
				return &Reply{Text: returnText(map[string]float64{"Line 35a": 1000}, nil) + "\n70\n60", CostUSD: &one}, nil
			}}, out, 1)
			cfg.Models = []ModelConfig{m}
			cfg.BudgetUSD = 5.5
			res, err := Run(context.Background(), cfg)
			if err != nil {
				t.Fatal(err)
			}
			var total float64
			rows := readRows(t, out)
			for _, r := range rows {
				total += r.CostUSD
			}
			if res.StopReason != "budget" || total > cfg.BudgetUSD || len(rows) != 5 {
				t.Fatalf("rep %d: stop=%q total=%v cap=%v rows=%d, want a budget stop at exactly 5 calls", rep, res.StopReason, total, cfg.BudgetUSD, len(rows))
			}
		}
	})
}

// The budget swaps a reservation for the real cost in one step: at no moment
// is a finished call counted as neither reserved nor spent.
func TestBudgetSettleIsOneStep(t *testing.T) {
	b := &budget{cap: 10, maxSeen: map[string]float64{}}
	if !b.reserve(4) || !b.reserve(4) {
		t.Fatal("two reservations of 4 fit in 10")
	}
	if b.reserve(4) {
		t.Fatal("a third reservation of 4 must not fit")
	}
	if over := b.settle("m", 4, 1); over {
		t.Error("spend 1 is not over the cap")
	}
	if b.spent+b.reserved != 5 {
		t.Errorf("committed = %v, want 5 (1 spent + 4 reserved)", b.spent+b.reserved)
	}
	if !b.reserve(4) {
		t.Error("after the first settles, 5 committed leaves room for 4")
	}
	if over := b.settle("m", 4, 20); !over {
		t.Error("an actual cost past the cap must be reported")
	}
}

func TestRunRefusesBadProtocol(t *testing.T) {
	dir := t.TempDir()
	items := studyItems(t, dir, 1, false)
	cfg := testCfg(t, items, &FakeProvider{}, filepath.Join(dir, "out"), 1)
	cfg.Protocols = map[string]string{SetMiss: "turn7"}
	if _, err := Run(context.Background(), cfg); err == nil {
		t.Error("an invalid protocol should be refused")
	}
	if _, err := os.Stat(filepath.Join(dir, "out")); err == nil {
		t.Error("refused run created its output directory")
	}
}

func TestSeedRequiredOutsideDev(t *testing.T) {
	dir := t.TempDir()
	items := studyItems(t, dir, 1, false)
	itemsPath := filepath.Join(dir, "items.jsonl")
	if err := writeItems(context.Background(), itemsPath, items); err != nil {
		t.Fatal(err)
	}
	plan := filepath.Join(dir, "plan.md")
	if err := os.WriteFile(plan, []byte("p\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	frozen := filepath.Join(dir, "frozen.txt")
	if err := os.WriteFile(frozen, []byte("FROZEN\nplan-sha256: "+sha256Hex([]byte("p\n"))+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	models := filepath.Join(dir, "models.json")
	if err := os.WriteFile(models, []byte(`[{"name":"fa","kind":"fake"}]`), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "out")
	args := []string{"run", "-items", itemsPath, "-models", models, "-out", out, "-plan", plan, "-frozen", frozen, "-samples", "1"}
	var so, se strings.Builder
	if code := realMain(context.Background(), args, &so, &se); code == 0 || !strings.Contains(se.String(), "-seed is required") {
		t.Errorf("without -seed: code=%d stderr=%q", code, se.String())
	}
	if _, err := os.Stat(out); err == nil {
		t.Error("output directory created without a seed")
	}
	so.Reset()
	se.Reset()
	if code := realMain(context.Background(), append(args, "-seed", "5", "-protocol", "MISS=turn1,TWINS=full"), &so, &se); code != 0 {
		t.Fatalf("with -seed: code=%d stderr=%q", code, se.String())
	}
	if !strings.Contains(so.String(), "anchor branches not applicable") {
		t.Errorf("run summary lacks the anchor line: %s", so.String())
	}
}
