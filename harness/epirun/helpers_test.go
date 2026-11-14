package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

const testInputJSON = `{"input":{"form":{"wages":{"label":"Wages","value":12345},"name":{"label":"Name","value":"Zoë {x}"}}}}`

// testItems makes one keyed item and one unkeyed item. The key figures are
// five or six digits and absent from the input, so a leak is easy to see.
func testItems(t *testing.T, dir string) []Item {
	t.Helper()
	in := filepath.Join(dir, "input.json")
	if err := os.WriteFile(in, []byte(testInputJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	return []Item{
		{ID: "k1", Set: SetDev, Input: in, Keyed: true, Key: &KeyFigures{Refund: 52718, Owed: 0, TotalTax: 41234, AGI: 987654}},
		{ID: "u1", Set: SetDev, Input: in},
	}
}

func fakeModel(name string) ModelConfig {
	return ModelConfig{Name: name, Kind: KindFake, Model: "fake", Concurrency: 1}
}

func testCfg(t *testing.T, items []Item, p Provider, out string, samples int) RunConfig {
	t.Helper()
	m := fakeModel("fm")
	return RunConfig{
		Items: items, Models: []ModelConfig{m}, Providers: map[string]Provider{m.Name: p},
		OutDir: out, Samples: samples,
		Backoff: func(int) time.Duration { return 0 },
	}
}

func readRows(t *testing.T, out string) []LogRow {
	t.Helper()
	rows, _, err := readCallLog(callsPath(out))
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

func countWhere(rows []LogRow, f func(LogRow) bool) int {
	n := 0
	for _, r := range rows {
		if f(r) {
			n++
		}
	}
	return n
}

var numTok = regexp.MustCompile(`\d[\d,]*`)

// numbersIn returns the set of numbers in s, commas dropped.
func numbersIn(s string) map[int64]bool {
	out := map[int64]bool{}
	for _, m := range numTok.FindAllString(s, -1) {
		n, err := strconv.ParseInt(strings.ReplaceAll(strings.TrimRight(m, ","), ",", ""), 10, 64)
		if err == nil {
			out[n] = true
		}
	}
	return out
}
