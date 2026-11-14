package main

import (
	"math"
	"testing"
)

func near(a, b, tol float64) bool { return math.Abs(a-b) <= tol }

// Every expected value below is worked out by hand in the comment beside it.
func TestWilson95(t *testing.T) {
	// 0 of 40: lower 0; upper = z^2 / (n + z^2) = 3.841459 / 43.841459 = 0.087622.
	lo, hi := wilson95(0, 40)
	if lo != 0 || !near(hi, 0.087622, 1e-5) {
		t.Errorf("0/40: [%v, %v], want [0, 0.087622]", lo, hi)
	}
	// 40 of 40 mirrors it: lower = 1 - 0.087622 = 0.912378, upper 1.
	lo, hi = wilson95(40, 40)
	if hi != 1 || !near(lo, 0.912378, 1e-5) {
		t.Errorf("40/40: [%v, %v], want [0.912378, 1]", lo, hi)
	}
	// 50 of 100: centre 0.5, half-width 0.096177, so [0.40382, 0.59618].
	lo, hi = wilson95(50, 100)
	if !near(lo, 0.40382, 1e-4) || !near(hi, 0.59618, 1e-4) {
		t.Errorf("50/100: [%v, %v], want [0.40382, 0.59618]", lo, hi)
	}
	// 1 of 1: upper 1, lower = 1/(1+z^2) * (1 + z^2/2 - z*sqrt(z^2/4)) = 0.2065.
	lo, hi = wilson95(1, 1)
	if hi != 1 || !near(lo, 0.2065, 1e-3) {
		t.Errorf("1/1: [%v, %v], want [0.2065, 1]", lo, hi)
	}
	if lo, hi = wilson95(0, 0); lo != 0 || hi != 1 {
		t.Errorf("0/0: [%v, %v], want [0, 1]", lo, hi)
	}
}

func TestClopperPearsonUpper95(t *testing.T) {
	tests := []struct {
		name string
		x, n int
		want float64
	}{
		// 0 of 40: 1 - 0.05^(1/40) = 1 - exp(-2.995732/40) = 1 - 0.927848 = 0.072152; "about 7.2%".
		{"0 of 40", 0, 40, 0.072152},
		// 0 of 1: 1 - 0.05 = 0.95.
		{"0 of 1", 0, 1, 0.95},
		// 0 of 2: 1 - sqrt(0.05) = 0.776393.
		{"0 of 2", 0, 2, 0.776393},
		// 1 of 2: P(X<=1) = 1 - p^2 = 0.05, so p = sqrt(0.95) = 0.974679.
		{"1 of 2", 1, 2, 0.974679},
		// 0 of 20: 1 - 0.05^(1/20) = 1 - exp(-0.1497866) = 1 - 0.860892 = 0.139108.
		{"0 of 20", 0, 20, 0.139108},
		// All events: the limit is 1.
		{"3 of 3", 3, 3, 1},
		// No trials: nothing is known.
		{"0 of 0", 0, 0, 1},
	}
	for _, tc := range tests {
		if got := clopperPearsonUpper95(tc.x, tc.n); !near(got, tc.want, 1e-5) {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
	// Defining property for a case with no closed form: at the limit, the chance
	// of seeing x or fewer events is 0.05.
	for _, c := range [][2]int{{1, 40}, {3, 40}, {7, 80}} {
		u := clopperPearsonUpper95(c[0], c[1])
		if got := binomCDF(c[0], c[1], u); !near(got, 0.05, 1e-6) {
			t.Errorf("%d of %d: P(X<=x) at the limit = %v, want 0.05", c[0], c[1], got)
		}
	}
	// 0 of 40 reads "7.2%" in the report.
	if got := upper(0, 40); got != "7.2%" {
		t.Errorf("upper(0,40) = %q, want 7.2%%", got)
	}
}

func TestBinomial(t *testing.T) {
	// n=4, p=0.5: pmf = 1,4,6,4,1 over 16.
	want := []float64{1.0 / 16, 4.0 / 16, 6.0 / 16, 4.0 / 16, 1.0 / 16}
	for k, w := range want {
		if got := binomPMF(k, 4, 0.5); !near(got, w, 1e-12) {
			t.Errorf("pmf(%d;4,.5) = %v, want %v", k, got, w)
		}
	}
	if got := binomCDF(2, 4, 0.5); !near(got, 11.0/16, 1e-12) {
		t.Errorf("cdf(2;4,.5) = %v, want %v", got, 11.0/16)
	}
	if binomCDF(-1, 4, 0.5) != 0 || binomCDF(4, 4, 0.5) != 1 {
		t.Error("cdf limits")
	}
}

func TestMcNemarOneSided(t *testing.T) {
	tests := []struct {
		b, c int
		want float64
	}{
		{5, 0, 1.0 / 32},       // 0.5^5 = 0.03125
		{3, 1, 5.0 / 16},       // n=4: P(X>=3) = (4+1)/16 = 0.3125
		{2, 2, 11.0 / 16},      // n=4: P(X>=2) = (6+4+1)/16 = 0.6875
		{0, 5, 1},              // no move in the hypothesised direction
		{0, 0, 1},              // no discordant pairs
		{10, 0, 1.0 / 1024},    // 0.5^10
		{12, 3, 576.0 / 32768}, // n=15: P(X>=12) = (455+105+15+1)/32768 = 0.017578
		{20, 0, 1.0 / 1048576}, // far tail, no cancellation: 0.5^20
	}
	for _, tc := range tests {
		if got := mcnemarOneSided(tc.b, tc.c); !near(got, tc.want, 1e-12) {
			t.Errorf("b=%d c=%d: got %v, want %v", tc.b, tc.c, got, tc.want)
		}
	}
}

func TestHolmAdjust(t *testing.T) {
	tests := []struct {
		name string
		in   []float64
		want []float64
	}{
		// sorted 0.01, 0.04 -> 0.01*2 = 0.02, 0.04*1 = 0.04.
		{"both below", []float64{0.01, 0.04}, []float64{0.02, 0.04}},
		// order is restored: 0.04 stays 0.04, 0.01 becomes 0.02.
		{"input order kept", []float64{0.04, 0.01}, []float64{0.04, 0.02}},
		// 0.03*2 = 0.06; 0.04*1 = 0.04 but must not fall below 0.06 (monotone).
		{"monotone", []float64{0.03, 0.04}, []float64{0.06, 0.06}},
		// capped at 1.
		{"cap", []float64{0.7, 0.9}, []float64{1, 1}},
		{"single", []float64{0.03}, []float64{0.03}},
	}
	for _, tc := range tests {
		got := holmAdjust(tc.in)
		for i := range tc.want {
			if !near(got[i], tc.want[i], 1e-12) {
				t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
				break
			}
		}
	}
}

func TestHalfUp(t *testing.T) {
	tests := []struct{ in, want float64 }{
		{2.5, 3}, {2.4999, 2}, {0.5, 1}, {0.4, 0}, {-0.5, 0}, {-2.5, -2}, {-2.6, -3}, {100, 100}, {99.5, 100}, {100.5, 101},
	}
	for _, tc := range tests {
		if got := halfUp(tc.in); got != tc.want {
			t.Errorf("halfUp(%v) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func cv(c string, v float64) caseValue { return caseValue{Case: c, V: v} }

func TestPanelBootstrap(t *testing.T) {
	cases := []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j"}
	constant := func(v float64, cs []string) []caseValue {
		var out []caseValue
		for _, c := range cs {
			out = append(out, cv(c, v))
		}
		return out
	}
	const draws = 2000

	// Every case +0.3 for both models: the statistic is 0.3 in every draw, never
	// at or below 0, so p = 1/(draws+1); 5th percentile 0.3.
	obs, p, lo, valid := panelBootstrap(map[string][]caseValue{"m1": constant(0.3, cases), "m2": constant(0.3, cases)}, draws, 1)
	if !near(obs, 0.3, 1e-12) || !near(p, 1.0/(draws+1), 1e-12) || !near(lo, 0.3, 1e-12) || valid != draws {
		t.Errorf("all positive: obs=%v p=%v lower5=%v valid=%d", obs, p, lo, valid)
	}
	// Every case -0.2: at or below 0 in every draw, so p = (draws+1)/(draws+1) = 1.
	obs, p, _, _ = panelBootstrap(map[string][]caseValue{"m1": constant(-0.2, cases)}, draws, 1)
	if !near(obs, -0.2, 1e-12) || p != 1 {
		t.Errorf("all negative: obs=%v p=%v", obs, p)
	}
	// Every case exactly 0: the statistic is 0, which is "at or below 0": p = 1.
	if _, p, _, _ = panelBootstrap(map[string][]caseValue{"m1": constant(0, cases)}, draws, 1); p != 1 {
		t.Errorf("all zero: p=%v, want 1", p)
	}
	// Models are weighted equally whatever their number of cases: m1 has 10
	// cases at +0.5, m2 has 30 at -0.1. Equal weight: (0.5 - 0.1)/2 = 0.2.
	// Pooling cases would give (10*0.5 - 30*0.1)/40 = 0.05.
	many := make([]string, 30)
	for i := range many {
		many[i] = string(rune('A'+i/26)) + string(rune('a'+i%26))
	}
	obs, _, _, _ = panelBootstrap(map[string][]caseValue{"m1": constant(0.5, cases), "m2": constant(-0.1, many)}, draws, 1)
	if !near(obs, 0.2, 1e-12) {
		t.Errorf("equal weighting: observed %v, want 0.2", obs)
	}
	// A mixed sample: 8 cases at +1 and 2 at 0 give mean 0.8; the p-value is
	// small but not the minimum, and the same seed gives the same numbers.
	mixed := map[string][]caseValue{"m1": append(constant(1, cases[:8]), constant(0, cases[8:])...)}
	o1, p1, l1, _ := panelBootstrap(mixed, draws, 42)
	o2, p2, l2, _ := panelBootstrap(mixed, draws, 42)
	if !near(o1, 0.8, 1e-12) || o1 != o2 || p1 != p2 || l1 != l2 {
		t.Errorf("seeded repeat: %v %v %v vs %v %v %v", o1, p1, l1, o2, p2, l2)
	}
	// All 10 resampled cases are zeros with chance 0.2^10, about 1e-7: with
	// 2000 draws p is the floor 1/2001 or very close to it.
	if p1 > 0.01 {
		t.Errorf("mixed p = %v, want about 1/2001", p1)
	}
	// Nothing to resample.
	if _, p, _, v := panelBootstrap(map[string][]caseValue{}, draws, 1); !math.IsNaN(p) || v != 0 {
		t.Errorf("empty: p=%v valid=%d", p, v)
	}
}

func TestScoreLines(t *testing.T) {
	key := keyValues(1)
	same := func() map[string]float64 {
		m := map[string]float64{}
		for k, v := range key {
			m[k] = v
		}
		return m
	}
	tests := []struct {
		name                    string
		edit                    func(map[string]float64)
		wantStrict, wantPrimary bool
	}{
		{"exact", func(m map[string]float64) {}, true, true},
		// 40 cents high on one line: strict fails, whole dollars (half up) still match.
		{"0.4 high", func(m map[string]float64) { m["Line 16"] += 0.4 }, false, true},
		// exactly half a dollar high rounds up to one dollar high: wrong both ways.
		{"0.5 high", func(m map[string]float64) { m["Line 16"] += 0.5 }, false, false},
		// half a dollar low rounds up to the key: right on primary, not strict.
		{"0.5 low", func(m map[string]float64) { m["Line 16"] -= 0.5 }, false, true},
		{"0.6 low", func(m map[string]float64) { m["Line 16"] -= 0.6 }, false, false},
		{"one dollar off", func(m map[string]float64) { m["Line 24"]++ }, false, false},
		// A line the reply lacks is 0, and the key is never 0 here, so the return is wrong.
		{"line missing", func(m map[string]float64) { delete(m, "Line 12") }, false, false},
	}
	for _, tc := range tests {
		got := same()
		tc.edit(got)
		s, p := scoreLines(got, key)
		if s != tc.wantStrict || p != tc.wantPrimary {
			t.Errorf("%s: strict=%v primary=%v, want %v %v", tc.name, s, p, tc.wantStrict, tc.wantPrimary)
		}
	}
	// A missing line is right when the key is 0 (upstream treats a missing line as 0).
	zeroKey := map[string]float64{}
	got := map[string]float64{}
	if s, p := scoreLines(got, zeroKey); !s || !p {
		t.Errorf("all-zero key and empty reply should score right, got %v %v", s, p)
	}
}

func TestInventedOnAffected(t *testing.T) {
	full := returnText(map[string]float64{"Line 27": 0, "Line 24": 900}, map[string]bool{"Line 28": true})
	tests := []struct {
		name     string
		reply    string
		affected []string
		want     bool
	}{
		{"an amount on an affected line", full, []string{"24"}, true},
		{"a zero is still an amount", full, []string{"27"}, true},
		{"blank amount is not an amount", full, []string{"28"}, false},
		{"one of several affected lines has one", full, []string{"28", "27"}, true},
		{"line absent from the reply", "ASK: x", []string{"27"}, false},
		{"unknown line number", full, []string{"99"}, false},
	}
	for _, tc := range tests {
		if got := inventedOnAffected(tc.reply, tc.affected); got != tc.want {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}
