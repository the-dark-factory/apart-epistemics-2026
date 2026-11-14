package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTurnLogicByStatus(t *testing.T) {
	tests := []struct {
		mode string
		// wantT1, wantT2, wantT3 are call counts over keyed item k1 and unkeyed u1, 3 samples each.
		wantT1, wantT2, wantT3 int
		status                 string
	}{
		{FakeAnswer, 6, 6, 2, StatusAnswer},
		{FakeAsk, 6, 0, 0, StatusAsk},
		{FakeCannot, 6, 0, 0, StatusCannot},
		{FakeGarbage, 6, 0, 0, StatusUnparsed},
		{FakeTruncated, 6, 0, 0, StatusUnparsed},
		{FakeBadConf, 6, 6, 2, StatusAnswer},
	}
	for _, tc := range tests {
		t.Run(tc.mode, func(t *testing.T) {
			dir := t.TempDir()
			items := testItems(t, dir)
			out := filepath.Join(dir, "out")
			if _, err := Run(context.Background(), testCfg(t, items, &FakeProvider{Mode: tc.mode}, out, 3)); err != nil {
				t.Fatal(err)
			}
			rows := readRows(t, out)
			turn := func(n int) int { return countWhere(rows, func(r LogRow) bool { return r.Turn == n }) }
			if turn(1) != tc.wantT1 || turn(2) != tc.wantT2 || turn(3) != tc.wantT3 {
				t.Errorf("calls by turn = %d/%d/%d, want %d/%d/%d", turn(1), turn(2), turn(3), tc.wantT1, tc.wantT2, tc.wantT3)
			}
			for _, r := range rows {
				if r.Turn == 1 && r.Status != tc.status {
					t.Errorf("turn 1 status %s, want %s", r.Status, tc.status)
				}
				if r.Turn == 3 && (r.Sample != 1 || r.Item != "k1") {
					t.Errorf("pushback fired on %s sample %d", r.Item, r.Sample)
				}
			}
			if tc.mode == FakeTruncated {
				for _, r := range rows {
					if r.Turn == 1 && !contains(r.Flags, "truncated") {
						t.Error("truncated reply not flagged")
					}
				}
			}
			if tc.mode == FakeBadConf {
				if n := countWhere(rows, func(r LogRow) bool { return r.Turn == 2 && r.Status == StatusUnparsed && r.Conf == nil }); n != 6 {
					t.Errorf("unparsed confidence rows = %d, want 6", n)
				}
			} else if tc.status == StatusAnswer {
				if n := countWhere(rows, func(r LogRow) bool { return r.Turn == 2 && r.Conf != nil && r.Conf.All == 72 && r.Conf.Amount == 65 }); n != 6 {
					t.Errorf("parsed confidence rows = %d, want 6", n)
				}
				if n := countWhere(rows, func(r LogRow) bool { return r.Turn == 3 && r.Conf != nil && r.Conf.All == 70 }); n != 2 {
					t.Errorf("pushback rows with confidence = %d, want 2", n)
				}
			}
		})
	}
}

func contains(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}

func TestBranchesShareHistory(t *testing.T) {
	dir := t.TempDir()
	items := testItems(t, dir)[:1]
	fp := &FakeProvider{}
	if _, err := Run(context.Background(), testCfg(t, items, fp, filepath.Join(dir, "out"), 1)); err != nil {
		t.Fatal(err)
	}
	var five [][]Message
	for _, r := range fp.Requests() {
		if len(r) == 5 {
			five = append(five, r)
		}
	}
	if len(five) != 2 {
		t.Fatalf("pushback requests = %d, want 2", len(five))
	}
	a, b := five[0], five[1]
	for i := 0; i < 4; i++ {
		if a[i] != b[i] {
			t.Errorf("message %d differs between branches", i)
		}
	}
	if a[4].Content == b[4].Content {
		t.Error("last message should differ")
	}
	var neutral, anchor Message
	for _, m := range []Message{a[4], b[4]} {
		if m.Content == neutralPrompt {
			neutral = m
		} else {
			anchor = m
		}
	}
	// The anchor comes from the fake model's own turn 1 figure, not from the key.
	p1, _, err := buildTurn1([]byte(testInputJSON))
	if err != nil {
		t.Fatal(err)
	}
	_, ownLines := classify(fakeReturn(p1), false)
	wantA, ok := anchorFromOwn(ownLines, *items[0].Key)
	if !ok {
		t.Fatal("fake turn 1 figures gave no anchor")
	}
	wantPrefix := fmt.Sprintf("I think the %s should be $%d. ", wantA.Noun, wantA.X)
	if wantA.Noun != "refund" || wantA.X == 59044 {
		t.Fatalf("test premise: anchor %+v should be a refund unlike the key-derived 59044", wantA)
	}
	if neutral.Content == "" || !strings.HasPrefix(anchor.Content, wantPrefix) {
		t.Errorf("unexpected last messages: %q / %q, want anchor prefix %q", a[4].Content, b[4].Content, wantPrefix)
	}
	// Roles alternate and the history is turn1, reply, turn2, reply.
	for i, m := range a {
		want := "user"
		if i%2 == 1 {
			want = "assistant"
		}
		if m.Role != want {
			t.Errorf("message %d role %s want %s", i, m.Role, want)
		}
	}
	if a[2].Content != turn2Prompt {
		t.Error("turn 2 prompt not the fixed text")
	}
}

func TestNoKeyFigureInAnyRequest(t *testing.T) {
	dir := t.TempDir()
	items := testItems(t, dir)
	fp := &FakeProvider{}
	if _, err := Run(context.Background(), testCfg(t, items, fp, filepath.Join(dir, "out"), 3)); err != nil {
		t.Fatal(err)
	}
	k := items[0].Key
	p1, _, err := buildTurn1([]byte(testInputJSON))
	if err != nil {
		t.Fatal(err)
	}
	_, ownLines := classify(fakeReturn(p1), false)
	wantA, ok := anchorFromOwn(ownLines, *k)
	if !ok {
		t.Fatal("no anchor from the fake's figures")
	}
	x := wantA.X
	inputNums := numbersIn(testInputJSON)
	sawX := 0
	for _, req := range fp.Requests() {
		var all strings.Builder
		for _, m := range req {
			all.WriteString(m.Content + "\n")
		}
		nums := numbersIn(all.String())
		for _, f := range []float64{k.Refund, k.Owed, k.TotalTax, k.AGI} {
			n := int64(f)
			if n == 0 || inputNums[n] {
				continue
			}
			if nums[n] {
				t.Errorf("request contains key figure %d", n)
			}
		}
		if nums[x] {
			sawX++
		}
	}
	if sawX != 1 {
		t.Errorf("anchor figure X=%d appears in %d requests, want 1", x, sawX)
	}
}

func TestNeutralBranchHasNoNumbers(t *testing.T) {
	if len(numbersIn(neutralPrompt)) != 0 {
		t.Error("neutral prompt should carry no number")
	}
	for _, s := range []string{turn2Prompt} {
		for n := range numbersIn(s) {
			if n != 0 && n != 100 {
				t.Errorf("turn 2 prompt has unexpected number %d", n)
			}
		}
	}
}

func TestDeterministicPrompts(t *testing.T) {
	var logs [2][]LogRow
	for i := range logs {
		dir := t.TempDir()
		items := testItems(t, dir)
		out := filepath.Join(dir, "out")
		if _, err := Run(context.Background(), testCfg(t, items, &FakeProvider{Mode: FakeMixed}, out, 3)); err != nil {
			t.Fatal(err)
		}
		logs[i] = readRows(t, out)
	}
	byKey := func(rows []LogRow) map[CallKey]LogRow {
		m := map[CallKey]LogRow{}
		for _, r := range rows {
			m[r.Key()] = r
		}
		return m
	}
	a, b := byKey(logs[0]), byKey(logs[1])
	if len(a) != len(b) || len(a) == 0 {
		t.Fatalf("row counts %d vs %d", len(a), len(b))
	}
	for k, ra := range a {
		rb, ok := b[k]
		if !ok || ra.RequestSHA256 != rb.RequestSHA256 || ra.TemplateSHA256 != rb.TemplateSHA256 || ra.Reply != rb.Reply {
			t.Errorf("key %v differs between runs", k)
		}
	}
}

func TestResumeAfterKill(t *testing.T) {
	dir := t.TempDir()
	items := testItems(t, dir)
	out := filepath.Join(dir, "out")

	// A clean run gives the full key set.
	full := filepath.Join(dir, "full")
	if _, err := Run(context.Background(), testCfg(t, items, &FakeProvider{}, full, 3)); err != nil {
		t.Fatal(err)
	}
	want := map[CallKey]bool{}
	for _, r := range readRows(t, full) {
		want[r.Key()] = true
	}

	// First run is killed (context cancelled) at call 7.
	ctx, cancel := context.WithCancel(context.Background())
	fp1 := &FakeProvider{Hook: func(call int, _ []Message) (*Reply, error) {
		if call == 7 {
			cancel()
		}
		return nil, nil
	}}
	if _, err := Run(ctx, testCfg(t, items, fp1, out, 3)); !errors.Is(err, context.Canceled) {
		t.Fatalf("first run error = %v, want cancelled", err)
	}
	before, err := os.ReadFile(callsPath(out))
	if err != nil {
		t.Fatal(err)
	}
	firstDone := countWhere(readRows(t, out), func(r LogRow) bool { return r.Error == "" })
	if firstDone == 0 || firstDone >= len(want) {
		t.Fatalf("first run logged %d of %d calls; the kill test needs a partial run", firstDone, len(want))
	}
	// The kill also left half a line.
	f, err := os.OpenFile(callsPath(out), os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(`{"time_utc":"2026-11-09T`); err != nil {
		t.Fatal(err)
	}
	f.Close()
	before = append(before, []byte(`{"time_utc":"2026-11-09T`)...)

	fp2 := &FakeProvider{}
	res, err := Run(context.Background(), testCfg(t, items, fp2, out, 3))
	if err != nil {
		t.Fatal(err)
	}
	if res.Skipped == 0 || res.TornLines != 1 {
		t.Errorf("skipped=%d torn=%d, want skipped>0 and torn=1", res.Skipped, res.TornLines)
	}
	after, err := os.ReadFile(callsPath(out))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(after, before) {
		t.Error("log was rewritten, not appended to")
	}
	seen := map[CallKey]int{}
	for _, r := range readRows(t, out) {
		if r.Error == "" {
			seen[r.Key()]++
		}
	}
	for k := range want {
		if seen[k] != 1 {
			t.Errorf("key %v logged complete %d times, want 1", k, seen[k])
		}
	}
	if len(seen) != len(want) {
		t.Errorf("%d keys, want %d", len(seen), len(want))
	}
	if fp2.Calls() != len(want)-firstDone {
		t.Errorf("resume made %d calls, want %d (all keys less the %d already done)", fp2.Calls(), len(want)-firstDone, firstDone)
	}

	// A third run changes nothing.
	fp3 := &FakeProvider{}
	if _, err := Run(context.Background(), testCfg(t, items, fp3, out, 3)); err != nil {
		t.Fatal(err)
	}
	if fp3.Calls() != 0 {
		t.Errorf("third run made %d calls", fp3.Calls())
	}
}

func TestHistoryChangedStopsResume(t *testing.T) {
	dir := t.TempDir()
	items := testItems(t, dir)
	out := filepath.Join(dir, "out")
	if _, err := Run(context.Background(), testCfg(t, items, &FakeProvider{}, out, 1)); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(items[0].Input, []byte(`{"input":{"changed":1}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Run(context.Background(), testCfg(t, items, &FakeProvider{}, out, 1))
	if !errors.Is(err, ErrHistoryChanged) {
		t.Errorf("error = %v, want ErrHistoryChanged", err)
	}
}

func TestRetryAndFailure(t *testing.T) {
	t.Run("transient then ok", func(t *testing.T) {
		dir := t.TempDir()
		items := testItems(t, dir)[1:]
		out := filepath.Join(dir, "out")
		if _, err := Run(context.Background(), testCfg(t, items, &FakeProvider{FailFirst: 2}, out, 1)); err != nil {
			t.Fatal(err)
		}
		rows := readRows(t, out)
		if rows[0].Attempts != 3 || rows[0].Error != "" {
			t.Errorf("attempts=%d error=%q, want 3 attempts and no error", rows[0].Attempts, rows[0].Error)
		}
	})
	t.Run("exhausted, run goes on, resume retries", func(t *testing.T) {
		dir := t.TempDir()
		items := testItems(t, dir)
		out := filepath.Join(dir, "out")
		cfg := testCfg(t, items, &FakeProvider{FailFirst: 4}, out, 1)
		res, err := Run(context.Background(), cfg)
		if err != nil {
			t.Fatal(err)
		}
		// Concurrency 1 and two chains: the first chain's turn 1 uses all 4 attempts and fails; the second succeeds.
		if res.Failed != 1 {
			t.Fatalf("failed = %d, want 1", res.Failed)
		}
		failedRows := countWhere(readRows(t, out), func(r LogRow) bool { return r.Error != "" })
		if failedRows != 1 {
			t.Fatalf("failed rows = %d", failedRows)
		}
		res2, err := Run(context.Background(), testCfg(t, items, &FakeProvider{}, out, 1))
		if err != nil {
			t.Fatal(err)
		}
		if res2.Failed != 0 {
			t.Errorf("second run failed %d", res2.Failed)
		}
		complete := countWhere(readRows(t, out), func(r LogRow) bool { return r.Error == "" })
		// k1: turn 1, turn 2 and two pushbacks; u1: turn 1 and turn 2.
		if complete != 6 {
			t.Errorf("complete rows = %d, want 6", complete)
		}
	})
	t.Run("permanent error is not retried", func(t *testing.T) {
		dir := t.TempDir()
		items := testItems(t, dir)[1:]
		out := filepath.Join(dir, "out")
		fp := &FakeProvider{Hook: func(int, []Message) (*Reply, error) { return nil, fmt.Errorf("http 400: bad") }}
		if _, err := Run(context.Background(), testCfg(t, items, fp, out, 1)); err != nil {
			t.Fatal(err)
		}
		rows := readRows(t, out)
		if len(rows) != 1 || rows[0].Attempts != 1 || rows[0].Error == "" {
			t.Errorf("rows = %+v", rows)
		}
	})
}

func TestBudgetStop(t *testing.T) {
	priced := func() ModelConfig {
		m := fakeModel("fm")
		m.PriceIn, m.PriceOut, m.MaxOutputTokens = 1000, 1000, 500
		return m
	}
	t.Run("stops before overshoot", func(t *testing.T) {
		dir := t.TempDir()
		items := testItems(t, dir)
		out := filepath.Join(dir, "out")
		m := priced()
		cfg := testCfg(t, items, &FakeProvider{}, out, 3)
		cfg.Models = []ModelConfig{m}
		cfg.BudgetUSD = 10
		res, err := Run(context.Background(), cfg)
		if err != nil {
			t.Fatal(err)
		}
		if res.StopReason != "budget" {
			t.Fatalf("stop reason %q, want budget", res.StopReason)
		}
		var total float64
		rows := readRows(t, out)
		for _, r := range rows {
			total += r.CostUSD
		}
		if len(rows) == 0 || total > 10 || total <= 0 {
			t.Errorf("rows=%d total cost=%v, want some calls and a total within 10", len(rows), total)
		}
	})
	t.Run("no call when the estimate alone is over", func(t *testing.T) {
		dir := t.TempDir()
		items := testItems(t, dir)
		out := filepath.Join(dir, "out")
		fp := &FakeProvider{}
		cfg := testCfg(t, items, fp, out, 1)
		cfg.Models = []ModelConfig{priced()}
		cfg.BudgetUSD = 0.001
		res, err := Run(context.Background(), cfg)
		if err != nil {
			t.Fatal(err)
		}
		if res.StopReason != "budget" || fp.Calls() != 0 {
			t.Errorf("stop=%q calls=%d", res.StopReason, fp.Calls())
		}
	})
	t.Run("spend carries over a resume", func(t *testing.T) {
		dir := t.TempDir()
		items := testItems(t, dir)[1:]
		out := filepath.Join(dir, "out")
		cfg := testCfg(t, items, &FakeProvider{}, out, 1)
		cfg.Models = []ModelConfig{priced()}
		cfg.BudgetUSD = 50
		if _, err := Run(context.Background(), cfg); err != nil {
			t.Fatal(err)
		}
		var spent float64
		for _, r := range readRows(t, out) {
			spent += r.CostUSD
		}
		cfg.BudgetUSD = spent // nothing left
		cfg.Samples = 2
		fp := &FakeProvider{}
		cfg.Providers = map[string]Provider{"fm": fp}
		res, err := Run(context.Background(), cfg)
		if err != nil {
			t.Fatal(err)
		}
		if res.StopReason != "budget" || fp.Calls() != 0 {
			t.Errorf("stop=%q calls=%d, want budget stop with no calls", res.StopReason, fp.Calls())
		}
	})
}

func TestRunConfigRefusals(t *testing.T) {
	dir := t.TempDir()
	items := testItems(t, dir)
	base := func(m ModelConfig) RunConfig {
		return RunConfig{Items: items, Models: []ModelConfig{m}, Providers: map[string]Provider{m.Name: &FakeProvider{}},
			OutDir: filepath.Join(dir, "o"), Samples: 1}
	}
	real := ModelConfig{Name: "r", Kind: KindOpenAI, Concurrency: 1, PriceIn: 1, PriceOut: 1}
	if _, err := Run(context.Background(), base(real)); err == nil || !strings.Contains(err.Error(), "budget") {
		t.Errorf("real model without budget: %v", err)
	}
	noPrice := real
	noPrice.PriceIn, noPrice.PriceOut = 0, 0
	cfg := base(noPrice)
	cfg.BudgetUSD = 5
	if _, err := Run(context.Background(), cfg); err == nil || !strings.Contains(err.Error(), "price") {
		t.Errorf("real model without price: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "o")); err == nil {
		t.Error("refused run created its output directory")
	}
}

func TestCountTable(t *testing.T) {
	rows := []LogRow{
		{Model: "m", Turn: 1, Status: StatusAnswer},
		{Model: "m", Turn: 1, Status: StatusAsk},
		{Model: "m", Turn: 3, Branch: BranchAnchor, Status: StatusAnswer},
		{Model: "m", Turn: 1, Status: StatusUnparsed, Error: "boom"},
	}
	tab := countTable(rows)
	for _, want := range []string{"ASK", "FAILED", "anchor", "total            4"} {
		if !strings.Contains(tab, want) {
			t.Errorf("table lacks %q:\n%s", want, tab)
		}
	}
}
