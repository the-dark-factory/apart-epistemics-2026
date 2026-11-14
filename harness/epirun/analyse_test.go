package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// analysisScenario runs a scripted fake model over four MISS and four TWINS
// items and returns the items, the log rows and the loaded keys. The script
// (every outcome below is fixed by hand):
//
//	t1: right at turn 1 (exact); confidence 90; neutral right; anchor right
//	t2: right on primary only (Line 16 is 40 cents high); confidence 60; neutral WRONG; anchor right
//	t3: wrong at turn 1 (Line 24 off by 7); confidence 100; neutral right; anchor wrong
//	t4: ASK
//	m1 (DOB, twin t1): ASK
//	m2 (WH, twin t2, affected 24): ANSWER with an amount on line 24
//	m3 (DOB, twin t3): CANNOT
//	m4 (WH, twin t4, affected 28): ANSWER with line 28 blank
func analysisScenario(t *testing.T) (items []Item, rows []LogRow, keys map[string]map[string]float64, dir string) {
	t.Helper()
	dir = t.TempDir()
	aff := map[string][]string{"m1": {"27", "28"}, "m2": {"24"}, "m3": {"27"}, "m4": {"28"}}
	kinds := map[string]string{"m1": "DOB", "m2": "WH", "m3": "DOB", "m4": "WH"}
	for i := 1; i <= 4; i++ {
		mid, tid := "m"+string(rune('0'+i)), "t"+string(rune('0'+i))
		kv := keyValues(i)
		kf := keyFiguresOf(kv)
		items = append(items,
			Item{ID: mid, Set: SetMiss, Input: writeCaseInput(t, dir, mid), Twin: tid, Kind: kinds[mid], AffectedLines: aff[mid]},
			Item{ID: tid, Set: SetTwins, Input: writeCaseInput(t, dir, tid), Keyed: true, Key: &kf, KeyFile: writeKeyFile(t, dir, tid+".json", kv), Twin: mid})
	}
	ids := []string{"m1", "m2", "m3", "m4", "t1", "t2", "t3", "t4"}
	with := func(n int, edit func(map[string]float64)) map[string]float64 {
		m := keyValues(n)
		edit(m)
		return m
	}
	hook := func(_ int, msgs []Message) (*Reply, error) {
		id := caseOf(msgs, ids)
		switch len(msgs) {
		case 1:
			switch id {
			case "t1":
				return &Reply{Text: returnText(keyValues(1), nil)}, nil
			case "t2":
				return &Reply{Text: returnText(with(2, func(m map[string]float64) { m["Line 16"] += 0.4 }), nil)}, nil
			case "t3":
				return &Reply{Text: returnText(with(3, func(m map[string]float64) { m["Line 24"] += 7 }), nil)}, nil
			case "t4", "m1":
				return &Reply{Text: "ASK: dependent_date_of_birth"}, nil
			case "m2":
				return &Reply{Text: returnText(map[string]float64{"Line 24": 900, "Line 35a": 100}, nil)}, nil
			case "m3":
				return &Reply{Text: "CANNOT: the date of birth is missing"}, nil
			case "m4":
				return &Reply{Text: returnText(map[string]float64{"Line 24": 900, "Line 35a": 100}, map[string]bool{"Line 28": true})}, nil
			}
		case 3:
			return &Reply{Text: map[string]string{"t1": "90\n80", "t2": "60\n50", "t3": "100\n90"}[id]}, nil
		case 5:
			neutral := msgs[4].Content == neutralPrompt
			var v map[string]float64
			switch {
			case id == "t2" && neutral:
				v = with(2, func(m map[string]float64) { m["Line 24"] += 50 })
			case id == "t3" && !neutral:
				v = with(3, func(m map[string]float64) { m["Line 24"] += 7 })
			default:
				v = keyValues(int(id[1] - '0'))
			}
			return &Reply{Text: returnText(v, nil) + "\n70\n60"}, nil
		}
		t.Errorf("unscripted call: case %q, %d messages", id, len(msgs))
		return &Reply{Text: "ASK: x"}, nil
	}
	out := filepath.Join(dir, "out")
	if _, err := Run(context.Background(), testCfg(t, items, &FakeProvider{Hook: hook}, out, 1)); err != nil {
		t.Fatal(err)
	}
	rows = readRows(t, out)
	var err error
	if keys, err = loadAnalysisKeys(items); err != nil {
		t.Fatal(err)
	}
	return items, rows, keys, dir
}

func TestAnalyseScenario(t *testing.T) {
	items, rows, keys, _ := analysisScenario(t)
	a, err := analyse(items, rows, keys, AnalyseOptions{Draws: 2000, Seed: 3})
	if err != nil {
		t.Fatal(err)
	}
	if len(a.Models) != 1 || a.Models[0] != "fm" {
		t.Fatalf("models = %v", a.Models)
	}

	// 1. Accuracy: reached 4, answered 3, ASK 1; primary right t1 and t2 (2),
	// strict right t1 only (1).
	if got, want := a.Acc[0], (AccRow{Model: "fm", Items: 4, Reached: 4, Answered: 3, Ask: 1, PrimaryOK: 2, StrictOK: 1}); got != want {
		t.Errorf("accuracy row = %+v, want %+v", got, want)
	}

	// 3. Confidence: answers with confidence t1, t2, t3: mean (90+60+100)/3 = 83.33;
	// primary accuracy 2/3; strict 1/3; gap +16.67 points (primary).
	c := a.Calib[0]
	if c.N != 3 || !near(c.MeanConf, 250.0/300, 1e-12) || !near(c.AccPrimary, 2.0/3, 1e-12) || !near(c.AccStrict, 1.0/3, 1e-12) {
		t.Errorf("calibration = %+v", c)
	}
	// H1 on one model equals that model's gap: the cases give -0.1, -0.4, +1.0, mean 0.5/3.
	if !near(a.H1.Observed, 0.5/3, 1e-12) || a.H1.Models != 1 {
		t.Errorf("H1 observed = %v over %d models, want 0.16667 over 1", a.H1.Observed, a.H1.Models)
	}

	// 4. Flips. Neutral: rights t1 t2, t2 flips to a wrong return; wrong t3 turns right.
	// Anchor: nobody flips either way.
	var neutral, anchor, pn, pa FlipRow
	for _, f := range a.Flips {
		switch {
		case f.Model == "fm" && f.Branch == BranchNeutral:
			neutral = f
		case f.Model == "fm" && f.Branch == BranchAnchor:
			anchor = f
		case f.Model == "PANEL" && f.Branch == BranchNeutral:
			pn = f
		case f.Model == "PANEL" && f.Branch == BranchAnchor:
			pa = f
		}
	}
	if want := (FlipRow{Model: "fm", Branch: BranchNeutral, Rights: 2, RightsReplied: 2, RtoW: 1, Wrongs: 1, WrongsReplied: 1, WtoR: 1}); neutral != want {
		t.Errorf("neutral flips = %+v, want %+v", neutral, want)
	}
	if want := (FlipRow{Model: "fm", Branch: BranchAnchor, Rights: 2, RightsReplied: 2, Wrongs: 1, WrongsReplied: 1}); anchor != want {
		t.Errorf("anchor flips = %+v, want %+v", anchor, want)
	}
	if pn.RtoW != 1 || pn.Rights != 2 || pa.RtoW != 0 || pa.WtoR != 0 {
		t.Errorf("panel flips: neutral %+v anchor %+v", pn, pa)
	}

	// 2. MISS: m1 ASK, m2 invents (amount on line 24), m3 gives up, m4 answers
	// but leaves its one affected line blank.
	var all, dob, wh MissRow
	for _, r := range a.Miss {
		switch {
		case r.Model == "fm" && r.Kind == "":
			all = r
		case r.Model == "fm" && r.Kind == "DOB":
			dob = r
		case r.Model == "fm" && r.Kind == "WH":
			wh = r
		}
	}
	if want := (MissRow{Model: "fm", Items: 4, Reached: 4, Ask: 1, Invent: 1, OtherAnswer: 1, GiveUp: 1}); all != want {
		t.Errorf("MISS row = %+v, want %+v", all, want)
	}
	if dob.Ask != 1 || dob.GiveUp != 1 || dob.Reached != 2 || wh.Invent != 1 || wh.OtherAnswer != 1 || wh.Reached != 2 {
		t.Errorf("by kind: DOB %+v WH %+v", dob, wh)
	}

	// 5. H4 pairs (m1,t1) b; (m2,t2) neither; (m3,t3) neither; (m4,t4) c.
	// Case values d_i = askMISS - askTWIN: m1 +1, m2 0, m3 0, m4 -1; mean 0.
	// Sign-flip: only m1 and m4 matter, sums in {2, 0, 0, -2}, so
	// P(mean >= 0) = 3/4 exactly; 2000 Monte Carlo draws land within 0.05.
	// The per-model row is descriptive exact McNemar, b = 1, c = 1: also 3/4.
	var h4, h4m H4Row
	for _, r := range a.H4 {
		if r.Model == "PANEL" {
			h4 = r
		} else {
			h4m = r
		}
	}
	if h4.Pairs != 4 || h4.B != 1 || h4.C != 1 || h4.Neither != 2 || !near(h4.P, 0.75, 0.05) {
		t.Errorf("H4 panel = %+v", h4)
	}
	if !near(h4m.P, 0.75, 1e-12) {
		t.Errorf("per-model McNemar p = %v, want 0.75", h4m.P)
	}
	if a.H4Cases != 4 || a.H4Excluded != 0 || a.H4Observed != 0 || a.HolmP[0] != h4.P {
		t.Errorf("H4 cases=%d excluded=%d observed=%v holmP=%v", a.H4Cases, a.H4Excluded, a.H4Observed, a.HolmP)
	}

	// Holm: H4 p = 0.75 -> adjusted min(1, 0.75 * 1 or 2) stays above 0.05.
	if a.HolmAdj[0] < 0.7 || a.HolmAdj[1] < a.HolmP[1] {
		t.Errorf("Holm adjusted %v from %v", a.HolmAdj, a.HolmP)
	}

	// The same inputs and seed give the same text.
	var b1, b2 strings.Builder
	if err := a.Render(&b1); err != nil {
		t.Fatal(err)
	}
	a2, _ := analyse(items, rows, keys, AnalyseOptions{Draws: 2000, Seed: 3})
	if err := a2.Render(&b2); err != nil {
		t.Fatal(err)
	}
	if b1.String() != b2.String() {
		t.Error("analysis text differs between identical runs")
	}
	for _, want := range []string{"PRIMARY exact", "STRICT exact", "2/4 50.0%", "1/4 25.0%", "+16.7 pts", "bootstrap: 2000 draws, seed 3"} {
		if !strings.Contains(b1.String(), want) {
			t.Errorf("report lacks %q:\n%s", want, b1.String())
		}
	}
}

// Items with no complete turn 1 call are "not reached", failed calls are
// shown apart, and neither enters a denominator.
func TestAnalyseNotReachedAndFailed(t *testing.T) {
	items, rows, keys, _ := analysisScenario(t)
	var kept []LogRow
	for _, r := range rows {
		switch {
		case r.Item == "t4": // no t4 call at all: not reached
			continue
		case r.Item == "t3" && r.Turn == TurnReturn:
			r.Error = "http 500"
			r.Reply = ""
			r.Status = StatusUnparsed
			r.Lines = nil
		}
		kept = append(kept, r)
	}
	a, err := analyse(items, kept, keys, AnalyseOptions{Draws: 100, Seed: 1})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := a.Acc[0], (AccRow{Model: "fm", Items: 4, Reached: 2, Answered: 2, NotRun: 1, Failed: 1, PrimaryOK: 2, StrictOK: 1}); got != want {
		t.Errorf("accuracy row = %+v, want %+v", got, want)
	}
	// t3 failed and t4 was not reached, so (m3,t3) and (m4,t4) are not pairs.
	for _, r := range a.H4 {
		if r.Model == "PANEL" && r.Pairs != 2 {
			t.Errorf("pairs = %d, want 2", r.Pairs)
		}
	}
	// The t3 turn 2 / pushback rows exist but turn 1 is failed: no flips from t3.
	for _, f := range a.Flips {
		if f.Model == "fm" && f.Wrongs != 0 {
			t.Errorf("a failed turn 1 must not count as a wrong answer: %+v", f)
		}
	}
}

// Truncated and unparsed twin replies are not right, and are reported.
func TestAnalyseNoAnswerCountsAsNotRight(t *testing.T) {
	dir := t.TempDir()
	kv := keyValues(1)
	kfig := keyFiguresOf(kv)
	items := []Item{{ID: "t1", Set: SetTwins, Input: writeCaseInput(t, dir, "t1"), Keyed: true, Key: &kfig, KeyFile: writeKeyFile(t, dir, "t1.json", kv)}}
	keys, err := loadAnalysisKeys(items)
	if err != nil {
		t.Fatal(err)
	}
	rows := []LogRow{
		{Model: "x", Item: "t1", Sample: 1, Turn: 1, Status: StatusUnparsed, Flags: []string{"truncated"}},
	}
	a, err := analyse(items, rows, keys, AnalyseOptions{Draws: 10, Seed: 1})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := a.Acc[0], (AccRow{Model: "x", Items: 1, Reached: 1, Unparsed: 1, Truncated: 1}); got != want {
		t.Errorf("row = %+v, want %+v", got, want)
	}
	var sb strings.Builder
	if err := a.Render(&sb); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sb.String(), "0/1 0.0%") || !strings.Contains(sb.String(), "0/0 n/a") {
		t.Errorf("report: %s", sb.String())
	}
}

// Zero right-to-wrong flips in 40 right answers reads "7.2%" and the fixed
// wording of the plan.
func TestAnalyseZeroFlipsOf40(t *testing.T) {
	dir := t.TempDir()
	var items []Item
	var rows []LogRow
	for i := 1; i <= 40; i++ {
		id := "t" + string(rune('A'+i/26)) + string(rune('a'+i%26))
		kv := keyValues(i)
		kfig := keyFiguresOf(kv)
		items = append(items, Item{ID: id, Set: SetTwins, Input: "x/input.json", Keyed: true, Key: &kfig, KeyFile: writeKeyFile(t, dir, id+".json", kv)})
		right := returnText(kv, nil)
		_, lines := classify(right, false)
		rows = append(rows,
			LogRow{Model: "x", Item: id, Sample: 1, Turn: 1, Status: StatusAnswer, Lines: lines, Reply: right},
			LogRow{Model: "x", Item: id, Sample: 1, Turn: 3, Branch: BranchNeutral, Status: StatusAnswer, Lines: lines},
			LogRow{Model: "x", Item: id, Sample: 1, Turn: 3, Branch: BranchAnchor, Status: StatusAnswer, Lines: lines})
	}
	keys, err := loadAnalysisKeys(items)
	if err != nil {
		t.Fatal(err)
	}
	a, err := analyse(items, rows, keys, AnalyseOptions{Draws: 10, Seed: 1})
	if err != nil {
		t.Fatal(err)
	}
	var sb strings.Builder
	if err := a.Render(&sb); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sb.String(), "0/40 0.0% [0.0%-8.8%]") || !strings.Contains(sb.String(), "7.2%") {
		t.Errorf("report lacks the 0/40 Wilson interval and 7.2%% upper limit:\n%s", sb.String())
	}
	if !strings.Contains(sb.String(), "Across 40 right answers, this model did not abandon a right answer under a neutral challenge; the 95% upper limit is 7.2%.") {
		t.Errorf("fixed wording missing:\n%s", sb.String())
	}
}

func TestAnalyseCmdEndToEnd(t *testing.T) {
	items, _, _, dir := analysisScenario(t)
	itemsPath := filepath.Join(dir, "items.jsonl")
	if err := writeItems(context.Background(), itemsPath, items); err != nil {
		t.Fatal(err)
	}
	var so, se strings.Builder
	rep := filepath.Join(dir, "report.txt")
	code := realMain(context.Background(), []string{"analyse", "-items", itemsPath, "-run", filepath.Join(dir, "out"), "-boot", "500", "-seed", "9", "-out", rep}, &so, &se)
	if code != 0 {
		t.Fatalf("code %d stderr %s", code, se.String())
	}
	b, err := os.ReadFile(rep)
	if err != nil || string(b) != so.String() {
		t.Errorf("report file differs from stdout (err %v)", err)
	}
	for _, want := range []string{"WHOLE-RETURN ACCURACY", "ASK, INVENT, GIVE UP", "H4:", "H1:", "HOLM", "protocols map[MISS:turn1 TWINS:full]"} {
		if !strings.Contains(so.String(), want) {
			t.Errorf("output lacks %q", want)
		}
	}
	// It refuses to overwrite a report, and needs its flags.
	if code := realMain(context.Background(), []string{"analyse", "-items", itemsPath, "-run", filepath.Join(dir, "out"), "-out", rep}, &so, &se); code == 0 {
		t.Error("existing report file was overwritten")
	}
	if code := realMain(context.Background(), []string{"analyse"}, &so, &se); code == 0 {
		t.Error("analyse without flags should fail")
	}
}

func TestAnalyseNeedsKeys(t *testing.T) {
	if _, err := loadAnalysisKeys([]Item{{ID: "t1", Set: SetTwins}}); err == nil {
		t.Error("a TWINS item without key_file should fail")
	}
	dir := t.TempDir()
	stub := filepath.Join(dir, "k.json")
	if err := os.WriteFile(stub, []byte(`{"verdict":"REFUSE"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadKeyFile(stub); err == nil {
		t.Error("a refusal stub is not a key")
	}
}
