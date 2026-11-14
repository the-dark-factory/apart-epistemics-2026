package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
)

// StudyReport is what prepStudy checked, for the person who runs it. It holds
// counts only, never a key value.
type StudyReport struct {
	// Miss is the number of MISS items.
	Miss int
	// MissByKind counts MISS items by kind (DOB, WH).
	MissByKind map[string]int
	// Twins is the number of TWINS items.
	Twins int
	// TwinsKeyed counts TWINS items whose key has all 19 scored lines.
	TwinsKeyed int
	// Paired counts MISS items whose twin is present and keyed.
	Paired int
}

// readTSV reads a tab-separated file. With header true the first line names
// the columns and each row is returned keyed by column name; with header false
// the rows are keyed "0", "1", and so on.
func readTSV(path string, header bool) ([]map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()
	var cols []string
	var rows []map[string]string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 1<<16), 1<<22)
	first := true
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		fields := strings.Split(line, "\t")
		if first && header {
			cols = fields
			first = false
			continue
		}
		first = false
		row := map[string]string{}
		for i, v := range fields {
			if header {
				if i < len(cols) {
					row[cols[i]] = v
				}
			} else {
				row[fmt.Sprint(i)] = v
			}
		}
		rows = append(rows, row)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return rows, nil
}

// loadKeyFile reads a key JSON (label to number) and returns the value of each
// of the 19 scored lines keyed "Line 24" and so on. It fails if a line is
// missing or not a finite number, so a refusal stub ({"verdict":"REFUSE"}) or
// a partial key is caught.
func loadKeyFile(path string) (map[string]float64, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read key: %w", err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("parse key %s: %w", filepath.Base(path), err)
	}
	out := make(map[string]float64, len(scoredLabels))
	for _, l := range scoredLabels {
		rm, ok := m[l]
		if !ok {
			return nil, fmt.Errorf("key %s: line %q missing", filepath.Base(path), labelPrefix(l))
		}
		var v float64
		if err := json.Unmarshal(rm, &v); err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
			return nil, fmt.Errorf("key %s: line %q is not a number", filepath.Base(path), labelPrefix(l))
		}
		out[labelPrefix(l)] = v
	}
	return out, nil
}

// keyFiguresOf takes the four anchor figures out of a loaded key.
func keyFiguresOf(k map[string]float64) KeyFigures {
	return KeyFigures{Refund: k["Line 35a"], Owed: k["Line 37"], TotalTax: k["Line 24"], AGI: k["Line 11"]}
}

// affectedLabels turns "27,28,35a" into the scored-line prefixes "Line 27", and
// so on. A number that is not a scored line is an error.
func affectedLabels(s string) ([]string, error) {
	known := map[string]bool{}
	for _, l := range scoredLabels {
		known[labelPrefix(l)] = true
	}
	var out []string
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if !known["Line "+p] {
			return nil, fmt.Errorf("affected line %q is not one of the scored lines", p)
		}
		out = append(out, p)
	}
	return out, nil
}

// prepStudy builds the MISS items (missDir, from fresh/miss-*) and their
// full-facts TWINS items (setDir, from fresh/set-*) for the confirmatory run.
// The twin mapping is the MISS MANIFEST.tsv's twin column. It checks that every
// MISS input and twin input is a file named input.json, that the twin's input
// matches the sha256 in the set manifest, that the twin was keyed by the
// engine, that its key has all 19 lines, and that no twin is used twice.
// Models are only ever given Item.Input, which is always an input.json.
func prepStudy(ctx context.Context, missDir, setDir string) (items []Item, rep StudyReport, err error) {
	if err := ctx.Err(); err != nil {
		return nil, rep, err
	}
	missDir, err = filepath.Abs(missDir)
	if err != nil {
		return nil, rep, fmt.Errorf("miss dir: %w", err)
	}
	setDir, err = filepath.Abs(setDir)
	if err != nil {
		return nil, rep, fmt.Errorf("set dir: %w", err)
	}
	miss, err := readTSV(filepath.Join(missDir, "MANIFEST.tsv"), true)
	if err != nil {
		return nil, rep, err
	}
	setRows, err := readTSV(filepath.Join(setDir, "MANIFEST.tsv"), true)
	if err != nil {
		return nil, rep, err
	}
	setSHA := map[string]string{}
	for _, r := range setRows {
		setSHA[r["id"]] = r["sha256"]
	}
	spec, err := readTSV(filepath.Join(setDir, "audit", "under-spec.tsv"), false)
	if err != nil {
		return nil, rep, err
	}
	specStatus := map[string]string{}
	for _, r := range spec {
		specStatus[r["0"]] = r["2"]
	}
	if len(miss) == 0 {
		return nil, rep, errors.New("miss manifest has no rows")
	}
	rep.MissByKind = map[string]int{}
	usedTwin := map[string]bool{}
	var missItems, twinItems []Item
	for _, r := range miss {
		id, twin, kind := r["id"], r["twin"], r["kind"]
		if id == "" || twin == "" || (kind != "DOB" && kind != "WH") {
			return nil, rep, fmt.Errorf("miss manifest row %q: needs id, twin and kind DOB or WH", id)
		}
		if usedTwin[twin] {
			return nil, rep, fmt.Errorf("twin %s is used by two MISS items", twin)
		}
		usedTwin[twin] = true
		aff, err := affectedLabels(r["affected_lines"])
		if err != nil {
			return nil, rep, fmt.Errorf("miss %s: %w", id, err)
		}
		if len(aff) == 0 {
			return nil, rep, fmt.Errorf("miss %s: no affected lines listed", id)
		}
		missIn := filepath.Join(missDir, "inputs", id, "input.json")
		twinIn := filepath.Join(setDir, "inputs", twin, "input.json")
		for _, p := range []string{missIn, twinIn} {
			if _, err := os.Stat(p); err != nil {
				return nil, rep, fmt.Errorf("miss %s: %w", id, err)
			}
		}
		twinRaw, err := os.ReadFile(twinIn)
		if err != nil {
			return nil, rep, fmt.Errorf("twin %s: %w", twin, err)
		}
		if want, ok := setSHA[twin]; !ok || want != sha256Hex(twinRaw) {
			return nil, rep, fmt.Errorf("twin %s: input does not match the sha256 in the set manifest", twin)
		}
		if st := specStatus[twin]; st != "keyed" {
			return nil, rep, fmt.Errorf("twin %s: audit status is %q, want keyed", twin, st)
		}
		keyPath := filepath.Join(setDir, "key", twin+".json")
		key, err := loadKeyFile(keyPath)
		if err != nil {
			return nil, rep, fmt.Errorf("twin %s: %w", twin, err)
		}
		kf := keyFiguresOf(key)
		missItems = append(missItems, Item{ID: id, Set: SetMiss, Input: missIn, Twin: twin, Kind: kind, AffectedLines: aff})
		twinItems = append(twinItems, Item{ID: twin, Set: SetTwins, Input: twinIn, Keyed: true, Key: &kf, KeyFile: keyPath, Twin: id})
		rep.Miss++
		rep.MissByKind[kind]++
		rep.Twins++
		rep.TwinsKeyed++
		rep.Paired++
	}
	items = append(missItems, twinItems...)
	return items, rep, nil
}
