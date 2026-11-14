package main

import (
	"fmt"
	"strings"
	"testing"
)

func TestSignFlipP(t *testing.T) {
	rep := func(v float64, n int) []float64 {
		out := make([]float64, n)
		for i := range out {
			out[i] = v
		}
		return out
	}
	const draws = 20000
	tests := []struct {
		name    string
		d       []float64
		wantObs float64
		wantP   float64
		tol     float64
	}{
		// 5 cases all +1: only the all-plus sign pattern ties the mean, 1 of 2^5 = 1/32.
		{"five cases all +1", rep(1, 5), 1, 1.0 / 32, 0.006},
		// All zero: every draw ties the observed mean 0, so p is exactly 1.
		{"all zero", rep(0, 6), 0, 1, 1e-12},
		{"empty", nil, 0, 1, 1e-12},
		// +1 x3, -1 x3: observed 0; P(sum >= 0) = P(Bin(6,.5) >= 3) = (20+15+6+1)/64 = 42/64.
		{"balanced", append(rep(1, 3), rep(-1, 3)...), 0, 42.0 / 64, 0.02},
		// Mostly positive: +1 x4, 0 x2: observed 4/6; needs all four +: 1/16.
		{"zeros are inert", append(rep(1, 4), rep(0, 2)...), 4.0 / 6, 1.0 / 16, 0.008},
		// Negative direction can never be significant: all -1, p near 1.
		{"wrong direction", rep(-1, 5), -1, 1, 1e-12},
		// Fractional per-case values (models averaged): 0.5 x4: needs all four +: 1/16.
		{"fractional", rep(0.5, 4), 0.5, 1.0 / 16, 0.008},
	}
	for _, tc := range tests {
		obs, p := signFlipP(tc.d, draws, 11)
		if !near(obs, tc.wantObs, 1e-12) || !near(p, tc.wantP, tc.tol) {
			t.Errorf("%s: observed %v p %v, want %v and %v (+/- %v)", tc.name, obs, p, tc.wantObs, tc.wantP, tc.tol)
		}
	}
	// The balanced set is nowhere near significance.
	if _, p := signFlipP(append(rep(1, 3), rep(-1, 3)...), draws, 5); p < 0.3 {
		t.Errorf("balanced p = %v, want well above 0.05", p)
	}
	// Same seed, same p; the p-value is never 0.
	_, a := signFlipP(rep(1, 12), 500, 77)
	_, b := signFlipP(rep(1, 12), 500, 77)
	if a != b || a <= 0 {
		t.Errorf("seeded repeat: %v vs %v", a, b)
	}
}

// h4Rows builds the turn-1 rows for models x cases: every model asks on the
// MISS item of each case in askCases and answers elsewhere; twins are always answered.
func h4Rows(models []string, cases int, askCases map[int]bool) ([]Item, []LogRow) {
	var items []Item
	var rows []LogRow
	for c := 1; c <= cases; c++ {
		mid, tid := fmt.Sprintf("m%02d", c), fmt.Sprintf("t%02d", c)
		kv := keyValues(c)
		kf := keyFiguresOf(kv)
		items = append(items,
			Item{ID: mid, Set: SetMiss, Twin: tid, Kind: "DOB", AffectedLines: []string{"27"}},
			Item{ID: tid, Set: SetTwins, Keyed: true, Key: &kf, Twin: mid})
		for _, m := range models {
			st := StatusAnswer
			if askCases[c] {
				st = StatusAsk
			}
			rows = append(rows,
				LogRow{Model: m, Item: mid, Sample: 1, Turn: 1, Status: st},
				LogRow{Model: m, Item: tid, Sample: 1, Turn: 1, Status: StatusAnswer, Lines: map[string]float64{}})
		}
	}
	return items, rows
}

func h4Keys(items []Item) map[string]map[string]float64 {
	keys := map[string]map[string]float64{}
	for _, it := range items {
		if it.Set == SetTwins {
			keys[it.ID] = keyValues(1)
		}
	}
	return keys
}

// Clustering: four models that all ask on the same few cases are four
// looks at the same cases, not four times the evidence.
func TestH4ClusteringMatters(t *testing.T) {
	models := []string{"a", "b", "c", "d"}
	items, rows := h4Rows(models, 4, map[int]bool{1: true, 2: true, 3: true, 4: true})
	a, err := analyse(items, rows, h4Keys(items), AnalyseOptions{Draws: 20000, Seed: 1})
	if err != nil {
		t.Fatal(err)
	}
	var panel H4Row
	for _, r := range a.H4 {
		if r.Model == "PANEL" {
			panel = r
		}
	}
	// Pooled, that is 16 pairs all in one direction: McNemar 0.5^16 = 1.5e-5.
	if panel.B != 16 || panel.C != 0 {
		t.Fatalf("pooled counts b=%d c=%d, want 16 and 0", panel.B, panel.C)
	}
	pooled := mcnemarOneSided(panel.B, panel.C)
	if pooled > 0.0001 {
		t.Fatalf("pooled McNemar %v should be tiny", pooled)
	}
	// By case: 4 cases all +1, p = 1/16 = 0.0625, which does not reach 0.05.
	if a.H4Cases != 4 || !near(a.H4Observed, 1, 1e-12) || !near(panel.P, 1.0/16, 0.008) || panel.P <= 0.05 {
		t.Errorf("cases=%d observed=%v case-level p=%v, want 4, 1 and about 0.0625 (not below 0.05)", a.H4Cases, a.H4Observed, panel.P)
	}
	if a.HolmP[0] != panel.P {
		t.Errorf("Holm input %v is not the case-level p %v", a.HolmP[0], panel.P)
	}
	// Eight cases all +1 would reach it: 1/256.
	items8, rows8 := h4Rows(models, 8, map[int]bool{1: true, 2: true, 3: true, 4: true, 5: true, 6: true, 7: true, 8: true})
	a8, err := analyse(items8, rows8, h4Keys(items8), AnalyseOptions{Draws: 20000, Seed: 1})
	if err != nil {
		t.Fatal(err)
	}
	if p := a8.H4[len(a8.H4)-1].P; !near(p, 1.0/256, 0.002) {
		t.Errorf("eight cases p = %v, want about 1/256", p)
	}
}

// Models are averaged within a case; cases with no model having both rows are
// left out and counted; the report says the panel test is not McNemar.
func TestH4CaseValuesAndReport(t *testing.T) {
	items, rows := h4Rows([]string{"a", "b"}, 4, map[int]bool{1: true})
	var kept []LogRow
	for _, r := range rows {
		switch {
		case r.Item == "t02": // case 2: no twin row for either model
			continue
		case r.Model == "b" && r.Item == "m01": // case 1: model b did not ask
			r.Status = StatusAnswer
		case r.Model == "a" && r.Item == "t03": // case 3: only b has both rows
			continue
		case r.Model == "a" && r.Item == "m03":
			r.Status = StatusAsk
		}
		kept = append(kept, r)
	}
	a, err := analyse(items, kept, h4Keys(items), AnalyseOptions{Draws: 100, Seed: 1})
	if err != nil {
		t.Fatal(err)
	}
	// Case 1: a +1, b 0 -> 0.5. Case 2: left out. Case 3: only b, 0. Case 4: both 0.
	// Cases in test 3, left out 1, mean (0.5 + 0 + 0)/3.
	if a.H4Cases != 3 || a.H4Excluded != 1 || !near(a.H4Observed, 0.5/3, 1e-12) {
		t.Errorf("cases=%d excluded=%d observed=%v", a.H4Cases, a.H4Excluded, a.H4Observed)
	}
	var sb strings.Builder
	if err := a.Render(&sb); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"case-level sign-flip test, not McNemar", "left out (no model had both turn-1 rows) 1", "DESCRIPTIVE", "descriptive McNemar"} {
		if !strings.Contains(sb.String(), want) {
			t.Errorf("report lacks %q", want)
		}
	}
	// All zero differences: p = 1.
	itemsZ, rowsZ := h4Rows([]string{"a"}, 5, nil)
	az, err := analyse(itemsZ, rowsZ, h4Keys(itemsZ), AnalyseOptions{Draws: 100, Seed: 1})
	if err != nil {
		t.Fatal(err)
	}
	if az.HolmP[0] != 1 {
		t.Errorf("all-zero H4 p = %v, want 1", az.HolmP[0])
	}
}
