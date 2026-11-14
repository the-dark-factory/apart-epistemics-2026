package main

import (
	"os"
	"strings"
	"testing"
)

// bs is a backslash, kept out of literals so the expected text is unambiguous.
const bs = "\\"

func TestTurn1TemplateSentencesGone(t *testing.T) {
	p, _, err := buildTurn1([]byte(testInputJSON))
	if err != nil {
		t.Fatal(err)
	}
	for _, gone := range []string{
		"You should have all of the necessary taxpayer inputs",
		"It should have all of the necessary inputs",
		"{tax_year}", "{tool_use_hint}", "{input_data}", "{{", "\n}}\n",
	} {
		if strings.Contains(p, gone) {
			t.Errorf("prompt still contains %q", gone)
		}
	}
	for _, want := range []string{
		"`ASK: <field>`", "`CANNOT: <reason>`", "for the 2024 tax year",
		`"form_name": {`, // unescaped literal braces from the JSON description
		`{"input": {"form": {"wages": {"label": "Wages", "value": 12345}, "name": {"label": "Name", "value": "Zo` + bs + `u00eb {x}"}}}}`,
	} {
		if !strings.Contains(p, want) {
			t.Errorf("prompt lacks %q", want)
		}
	}
}

func TestTurn1Ending(t *testing.T) {
	p, _, err := buildTurn1([]byte(testInputJSON))
	if err != nil {
		t.Fatal(err)
	}
	const req4 = "4. Do not output any other introductory text or commentary."
	tests := []struct {
		name string
		sub  string
		want int
	}{
		{"new ending present once", endingNew, 1},
		{"old ending gone", endingOld, 0},
		{"requirement 4 unchanged", req4, 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := strings.Count(p, tc.sub); got != tc.want {
				t.Errorf("%q found %d times, want %d", tc.sub, got, tc.want)
			}
		})
	}
}

func TestTurn1Deterministic(t *testing.T) {
	a, sa, err := buildTurn1([]byte(testInputJSON))
	if err != nil {
		t.Fatal(err)
	}
	b, sb, err := buildTurn1([]byte(testInputJSON))
	if err != nil {
		t.Fatal(err)
	}
	if a != b || sa != sb {
		t.Error("prompt or template hash differs between builds")
	}
}

func TestTurn1FailsLoudly(t *testing.T) {
	orig := ty24Template
	defer func() { ty24Template = orig }()
	tests := []struct {
		name   string
		mutate func(string) string
	}{
		{"first sentence missing", func(s string) string { return strings.Replace(s, completenessReq1, "", 1) }},
		{"second sentence missing", func(s string) string { return strings.Replace(s, completenessJSON, "", 1) }},
		{"first sentence reworded", func(s string) string {
			return strings.Replace(s, "necessary taxpayer inputs", "needed taxpayer inputs", 1)
		}},
		{"ending missing", func(s string) string { return strings.Replace(s, endingOld, "", 1) }},
		{"ending twice", func(s string) string { return s + endingOld }},
		{"first sentence twice", func(s string) string { return s + completenessReq1 }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ty24Template = tc.mutate(orig)
			if _, _, err := buildTurn1([]byte(testInputJSON)); err == nil {
				t.Error("want an error")
			}
		})
	}
}

// TestTemplateMatchesUpstream compares the embedded template with the Python
// source it was copied from, when that source is present.
func TestTemplateMatchesUpstream(t *testing.T) {
	const path = "/Users/tony/dev/tax-calc-bench-upstream/tax_calc_bench/ty24_prompt.py"
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("upstream prompt not available: %v", err)
	}
	const open = `TAX_RETURN_GENERATION_PROMPT = """`
	s := string(raw)
	i := strings.Index(s, open)
	if i < 0 {
		t.Fatal("template start not found")
	}
	s = s[i+len(open):]
	j := strings.Index(s, `"""`)
	if j < 0 {
		t.Fatal("template end not found")
	}
	if s[:j] != ty24Template {
		t.Error("embedded template differs from upstream ty24_prompt.py")
	}
}

func TestPyJSONDumps(t *testing.T) {
	tests := []struct{ in, want string }{
		{`{"b":1,"a":[1,2,{"c":null}],"d":true}`, `{"b": 1, "a": [1, 2, {"c": null}], "d": true}`},
		{`{"s":"café \"q\" \\ \n"}`, `{"s": "caf` + bs + `u00e9 \"q\" \\ \n"}`},
		{`["😀"]`, `["` + bs + `ud83d` + bs + `ude00"]`},
		{`{"x":1.50,"y":-3}`, `{"x": 1.50, "y": -3}`},
		{`{}`, `{}`},
		{`[]`, `[]`},
	}
	for _, tc := range tests {
		got, err := pyJSONDumps([]byte(tc.in))
		if err != nil {
			t.Errorf("%s: %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("%s: got %s want %s", tc.in, got, tc.want)
		}
	}
	if _, err := pyJSONDumps([]byte(`{"a":1} extra`)); err == nil {
		t.Error("trailing data should fail")
	}
}
