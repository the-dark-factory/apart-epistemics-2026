package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// returnText renders a Form 1040 reply from per-line values keyed "Line 24"
// and so on. A line not in vals is written as 0, and a line named in blank is
// written with no amount at all.
func returnText(vals map[string]float64, blank map[string]bool) string {
	var b strings.Builder
	b.WriteString("Form 1040: U.S. Individual Income Tax Return\n")
	for _, l := range scoredLabels {
		p := labelPrefix(l)
		amt := strconv.FormatFloat(vals[p], 'f', -1, 64)
		if blank[p] {
			amt = ""
		}
		b.WriteString(l + " | calc | " + amt + "\n")
	}
	return b.String()
}

// keyValues makes distinct, nonzero key values for case number n: every line
// differs from every other line and from every other case.
func keyValues(n int) map[string]float64 {
	out := map[string]float64{}
	for i, l := range scoredLabels {
		out[labelPrefix(l)] = float64(100000*(n+1) + 100*i + 7)
	}
	return out
}

// writeKeyFile writes a key JSON (label to value) and returns its path.
func writeKeyFile(t *testing.T, dir, name string, vals map[string]float64) string {
	t.Helper()
	m := map[string]float64{}
	for _, l := range scoredLabels {
		m[l] = vals[labelPrefix(l)]
	}
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, b, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// writeCaseInput writes an input.json that names its case, so a scripted
// fake can tell cases apart from the prompt.
func writeCaseInput(t *testing.T, dir, id string) string {
	t.Helper()
	d := filepath.Join(dir, "in-"+id)
	if err := os.MkdirAll(d, 0o755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(d, "input.json")
	if err := os.WriteFile(p, []byte(`{"input":{"case":"`+id+`"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// caseOf finds which case a conversation is about.
func caseOf(msgs []Message, ids []string) string {
	for _, id := range ids {
		if strings.Contains(msgs[0].Content, `"`+id+`"`) {
			return id
		}
	}
	return ""
}
