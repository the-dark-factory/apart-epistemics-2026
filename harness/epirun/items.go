package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Item sets. Only DEV may run before the freeze.
const (
	SetDev     = "DEV"
	SetPubIn   = "PUB-IN"
	SetPubOut  = "PUB-OUT"
	SetNear    = "NEAR"
	SetSynth   = "SYNTH"
	SetUnclear = "UNCLEAR"
	SetMiss    = "MISS"
	// SetTwins holds the full-facts original of each MISS return.
	SetTwins = "TWINS"
)

var knownSets = map[string]bool{
	SetDev: true, SetPubIn: true, SetPubOut: true, SetNear: true,
	SetSynth: true, SetUnclear: true, SetMiss: true, SetTwins: true,
}

// Item is one return to put to the models.
type Item struct {
	// ID names the item.
	ID string `json:"id"`
	// Set is one of the sets above.
	Set string `json:"set"`
	// Input is the path of the taxpayer input.json.
	Input string `json:"input"`
	// Keyed says whether Key is present. Pushback runs only on keyed items.
	Keyed bool `json:"keyed"`
	// Key holds the four figures the anchor needs; nil when not keyed.
	Key *KeyFigures `json:"key,omitempty"`
	// KeyFile is the path of the full answer key (all 19 scored lines) of a
	// keyed item. Only the analysis reads it; the runner and the prompts never do.
	KeyFile string `json:"key_file,omitempty"`
	// Twin pairs a MISS item with its full-facts TWINS item, and the other way round.
	Twin string `json:"twin,omitempty"`
	// Kind, on a MISS item, is the kind of fact deleted: DOB or WH.
	Kind string `json:"kind,omitempty"`
	// AffectedLines, on a MISS item, lists the scored lines ("27", "35a") that
	// the deleted fact changes in the engine.
	AffectedLines []string `json:"affected_lines,omitempty"`
}

// Default locations for the DEV set (the 2026-11-09 re-run and upstream data).
const (
	defaultDevDir   = "/Users/tony/dev/bustle-memory-20261027/guides/TAXCALC_TY24/runs/us1040-rerun-20261109/dev"
	defaultTestData = "/Users/tony/dev/tax-calc-bench-upstream/tax_calc_bench/ty24/test_data"
)

// Labels of the four key figures in a re-run <case>.json.
const (
	keyLabelRefund = "Line 35a: Amount of line 34 you want refunded to you."
	keyLabelOwed   = "Line 37: Subtract line 33 from line 24. This is the amount you owe"
	keyLabelTax    = "Line 24: Add lines 22 and 23. This is your total tax"
	keyLabelAGI    = "Line 11: Subtract line 10 from line 9. This is your adjusted gross income"
)

// prepDev builds the DEV items from the re-run directory: the cases listed in
// dev-cases.txt whose <case>.txt is not REFUSED, with the key figures taken
// from <case>.json and the input from testData/<case>/input.json.
func prepDev(devDir, testData string) ([]Item, error) {
	f, err := os.Open(filepath.Join(devDir, "dev-cases.txt"))
	if err != nil {
		return nil, fmt.Errorf("open dev-cases.txt: %w", err)
	}
	defer f.Close()
	var items []Item
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		id := strings.TrimSpace(sc.Text())
		if id == "" {
			continue
		}
		txt, err := os.ReadFile(filepath.Join(devDir, id+".txt"))
		if err != nil {
			return nil, fmt.Errorf("case %s: %w", id, err)
		}
		if strings.HasPrefix(strings.TrimSpace(string(txt)), "REFUSED") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(devDir, id+".json"))
		if err != nil {
			return nil, fmt.Errorf("case %s: %w", id, err)
		}
		var fig map[string]float64
		if err := json.Unmarshal(raw, &fig); err != nil {
			return nil, fmt.Errorf("case %s: read key figures: %w", id, err)
		}
		var k KeyFigures
		for _, p := range []struct {
			label string
			dst   *float64
		}{{keyLabelRefund, &k.Refund}, {keyLabelOwed, &k.Owed}, {keyLabelTax, &k.TotalTax}, {keyLabelAGI, &k.AGI}} {
			v, ok := fig[p.label]
			if !ok {
				return nil, fmt.Errorf("case %s: key figure %q missing in %s.json", id, p.label, id)
			}
			*p.dst = v
		}
		in := filepath.Join(testData, id, "input.json")
		if _, err := os.Stat(in); err != nil {
			return nil, fmt.Errorf("case %s: input: %w", id, err)
		}
		items = append(items, Item{ID: id, Set: SetDev, Input: in, Keyed: true, Key: &k})
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("read dev-cases.txt: %w", err)
	}
	if len(items) == 0 {
		return nil, errors.New("no computed DEV cases found")
	}
	return items, nil
}

// writeItems writes items as JSON lines to path, refusing to overwrite.
func writeItems(ctx context.Context, path string, items []Item) (err error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return fmt.Errorf("create items file: %w", err)
	}
	defer func() {
		if cerr := f.Close(); err == nil && cerr != nil {
			err = fmt.Errorf("close items file: %w", cerr)
		}
	}()
	enc := json.NewEncoder(f)
	for _, it := range items {
		if err := enc.Encode(it); err != nil {
			return fmt.Errorf("write item %s: %w", it.ID, err)
		}
	}
	return nil
}

// readItems reads and validates an items.jsonl file.
func readItems(path string) ([]Item, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open items: %w", err)
	}
	defer f.Close()
	var items []Item
	seen := map[string]bool{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 1<<16), 1<<24)
	n := 0
	for sc.Scan() {
		n++
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var it Item
		if err := json.Unmarshal([]byte(line), &it); err != nil {
			return nil, fmt.Errorf("items line %d: %w", n, err)
		}
		switch {
		case it.ID == "":
			return nil, fmt.Errorf("items line %d: empty id", n)
		case seen[it.ID]:
			return nil, fmt.Errorf("items line %d: duplicate id %q", n, it.ID)
		case !knownSets[it.Set]:
			return nil, fmt.Errorf("items line %d: unknown set %q", n, it.Set)
		case it.Input == "":
			return nil, fmt.Errorf("items line %d: empty input path", n)
		case it.Keyed != (it.Key != nil):
			return nil, fmt.Errorf("items line %d: keyed=%v disagrees with presence of key", n, it.Keyed)
		}
		seen[it.ID] = true
		items = append(items, it)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("read items: %w", err)
	}
	return items, nil
}
