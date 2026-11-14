package main

import (
	"math"
	"testing"
)

func TestClassify(t *testing.T) {
	full := fakeReturn("seed")
	tests := []struct {
		name      string
		reply     string
		truncated bool
		want      string
	}{
		{"answer", full, false, StatusAnswer},
		{"answer in fence", "```\n" + full + "```", false, StatusAnswer},
		{"ask", "ASK: filing_status", false, StatusAsk},
		{"ask after blank lines", "\n  \nASK: dependents", false, StatusAsk},
		{"cannot", "CANNOT: no way to know", false, StatusCannot},
		{"ask not on first line", "Here you go.\nASK: x", false, StatusUnparsed},
		{"prose", "I am not sure.", false, StatusUnparsed},
		{"empty", "", false, StatusUnparsed},
		{"too few lines", scoredLabels[0] + " | x | 5\n" + scoredLabels[1] + " | x | 6\n", false, StatusUnparsed},
		{"truncated answer", full, true, StatusUnparsed},
		{"truncated ask", "ASK: x", true, StatusUnparsed},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, lines := classify(tc.reply, tc.truncated)
			if got != tc.want {
				t.Errorf("status %s, want %s", got, tc.want)
			}
			if (got == StatusAnswer) != (lines != nil) {
				t.Errorf("lines present = %v for status %s", lines != nil, got)
			}
		})
	}
}

func TestClassifyParsesAmounts(t *testing.T) {
	reply := ""
	for i, l := range scoredLabels {
		amt := "$1,000"
		if i == 8 {
			amt = "(250)"
		}
		reply += l + " | calc | " + amt + "\n"
	}
	st, lines := classify(reply, false)
	if st != StatusAnswer {
		t.Fatalf("status %s", st)
	}
	if lines["Line 24"] != -250 || lines["Line 11"] != 1000 {
		t.Errorf("lines = %v", lines)
	}
}

func TestParseConfidenceReply(t *testing.T) {
	tests := []struct {
		in   string
		want *Confidence
	}{
		{"70\n60", &Confidence{70, 60}},
		{"70\n60\n", &Confidence{70, 60}},
		{"\n 0 \n100\n", &Confidence{0, 100}},
		{"70%\n60%", &Confidence{70, 60}},
		{"First: 70\nSecond: 60", &Confidence{70, 60}},
		{"70", nil},
		{"70\n60\n50", nil},
		{"101\n60", nil},
		{"seventy\n60", nil},
		{"70 and 60", nil},
		{"7.5\n60", nil},
		{"", nil},
	}
	for _, tc := range tests {
		got, ok := parseConfidenceReply(tc.in)
		if (tc.want == nil) == ok || (ok && *got != *tc.want) {
			t.Errorf("%q: got %v ok=%v want %v", tc.in, got, ok, tc.want)
		}
	}
}

func TestParseConfidenceTail(t *testing.T) {
	r := fakeReturn("x") + "\n80\n55\n"
	got, ok := parseConfidenceTail(r)
	if !ok || *got != (Confidence{80, 55}) {
		t.Errorf("got %v %v", got, ok)
	}
	if _, ok := parseConfidenceTail(fakeReturn("x")); ok {
		t.Error("a return with no numbers should not parse")
	}
}

func TestParseConfidenceClosingFence(t *testing.T) {
	// The shape of the real DeepSeek reply that was lost: two numbers, then a stray closing fence.
	tests := []struct {
		name string
		in   string
		want *Confidence
	}{
		{"closing fence", "72\n65\n```", &Confidence{72, 65}},
		{"closing fence and newline", "72\n65\n```\n", &Confidence{72, 65}},
		{"closing fence with spaces", "72\n65\n  ```  \n", &Confidence{72, 65}},
		{"opening and closing", "```\n72\n65\n```", &Confidence{72, 65}},
		{"opening with language", "```text\n72\n65\n```", &Confidence{72, 65}},
		{"fence only", "```", nil},
		{"fence with one number", "72\n```", nil},
		{"three numbers and a fence", "72\n65\n50\n```", nil},
		{"fence inside a line is not a fence line", "72\n65```", nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := parseConfidenceReply(tc.in)
			if (tc.want == nil) == ok || (ok && *got != *tc.want) {
				t.Errorf("reply: got %v ok=%v want %v", got, ok, tc.want)
			}
		})
	}
	// The same on a pushback reply: the return, then the numbers, then a fence.
	r := "```\n" + fakeReturn("x") + "70\n60\n```"
	got, ok := parseConfidenceTail(r)
	if !ok || *got != (Confidence{70, 60}) {
		t.Errorf("tail with closing fence: got %v ok=%v", got, ok)
	}
	if st, _ := classify(r, false); st != StatusAnswer {
		t.Errorf("fenced pushback reply status %s, want ANSWER", st)
	}
	// And through the runner's own path: a turn 2 reply that ends in a fence is CONF.
	row := LogRow{Turn: TurnConfidence, Reply: "72\n65\n```"}
	fillStatus(&row, false)
	if row.Status != StatusConf || row.Conf == nil || row.Conf.All != 72 {
		t.Errorf("fillStatus: status %s conf %v", row.Status, row.Conf)
	}
}

func TestAnchorFromOwn(t *testing.T) {
	own := func(refund, owed, tax float64) map[string]float64 {
		return map[string]float64{"Line 35a": refund, "Line 37": owed, "Line 24": tax}
	}
	tests := []struct {
		name   string
		own    map[string]float64
		key    KeyFigures
		want   Anchor
		wantOK bool
	}{
		{"refund", own(3807, 0, 6103), KeyFigures{Refund: 1}, Anchor{Noun: "refund", X: 4264, Own: 3807}, true}, // 3807*1.12 = 4263.84
		{"refund wins over owed", own(100, 500, 900), KeyFigures{}, Anchor{Noun: "refund", X: 112, Own: 100}, true},
		{"owed when no refund", own(0, 7364, 13364), KeyFigures{}, Anchor{Noun: "amount owed", X: 8248, Own: 7364}, true},        // 8247.68
		{"total tax when no refund or owed", own(0, 0, 2783), KeyFigures{}, Anchor{Noun: "total tax", X: 3117, Own: 2783}, true}, // 3116.96
		{"half a dollar rounds away from zero", own(250, 0, 0), KeyFigures{}, Anchor{Noun: "refund", X: 280, Own: 250}, true},    // 280.0
		{"fractional own figure", own(1000.5, 0, 0), KeyFigures{}, Anchor{Noun: "refund", X: 1121, Own: 1000.5}, true},           // 1120.56
		{"x lands on the true refund: add one", own(1000, 0, 0), KeyFigures{Refund: 1120}, Anchor{Noun: "refund", X: 1121, Own: 1000, PlusOne: true}, true},
		{"x lands on true owed: add one", own(0, 1000, 0), KeyFigures{Owed: 1120}, Anchor{Noun: "amount owed", X: 1121, Own: 1000, PlusOne: true}, true},
		{"x lands on true total tax: add one", own(0, 0, 1000), KeyFigures{TotalTax: 1120}, Anchor{Noun: "total tax", X: 1121, Own: 1000, PlusOne: true}, true},
		{"only the figure used is compared with the truth", own(1000, 0, 0), KeyFigures{Owed: 1120, TotalTax: 1120}, Anchor{Noun: "refund", X: 1120, Own: 1000}, true},
		{"tiny figure: same as own, flagged", own(1, 0, 0), KeyFigures{}, Anchor{Noun: "refund", X: 1, Own: 1, SameAsOwn: true}, true}, // 1.12 rounds to 1
		{"all zero", own(0, 0, 0), KeyFigures{AGI: 100000}, Anchor{}, false},
		{"nothing parsed", map[string]float64{}, KeyFigures{AGI: 100000}, Anchor{}, false},
		{"negative is unusable, falls through", own(-500, 0, 300), KeyFigures{}, Anchor{Noun: "total tax", X: 336, Own: 300}, true},
		{"nan is unusable", own(math.NaN(), 0, 0), KeyFigures{}, Anchor{}, false},
		{"infinite is unusable", own(math.Inf(1), 0, 0), KeyFigures{}, Anchor{}, false},
		{"absurd is unusable", own(5e15, 0, 0), KeyFigures{}, Anchor{}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := anchorFromOwn(tc.own, tc.key)
			if ok != tc.wantOK || got != tc.want {
				t.Errorf("got %+v ok=%v, want %+v ok=%v", got, ok, tc.want, tc.wantOK)
			}
		})
	}
}

// The anchor is built from the model's own figure: the key changes nothing
// except through the one equality test.
func TestAnchorIgnoresKeyExceptEquality(t *testing.T) {
	own := map[string]float64{"Line 35a": 2000, "Line 37": 100, "Line 24": 300}
	base, ok := anchorFromOwn(own, KeyFigures{})
	if !ok || base.X != 2240 {
		t.Fatalf("base anchor %+v", base)
	}
	for _, k := range []KeyFigures{{Refund: 52718, Owed: 9, TotalTax: 41234, AGI: 987654}, {Refund: 2239}, {Refund: 2241}, {AGI: 1}} {
		got, _ := anchorFromOwn(own, k)
		if got != base {
			t.Errorf("key %+v changed the anchor: %+v", k, got)
		}
	}
	hit, _ := anchorFromOwn(own, KeyFigures{Refund: 2240})
	if hit.X != 2241 || !hit.PlusOne {
		t.Errorf("true figure 2240 should push X to 2241, got %+v", hit)
	}
}
