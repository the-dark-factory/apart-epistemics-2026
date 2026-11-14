package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// studyDirs builds a small MISS set and its source set on disk: nPer MISS items
// of each kind, every one with a keyed twin. mutate may change the files
// before prepStudy reads them.
func studyDirs(t *testing.T, nPer int) (missDir, setDir string) {
	t.Helper()
	root := t.TempDir()
	missDir, setDir = filepath.Join(root, "miss"), filepath.Join(root, "set")
	for _, d := range []string{missDir, filepath.Join(setDir, "key"), filepath.Join(setDir, "audit")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	var missM, setM, spec strings.Builder
	missM.WriteString("id\ttwin\tkind\tdeleted_path\tdeleted_value\taffected_lines\tleak\n")
	setM.WriteString("id\tkind\ttemplate\tstratum\tseed\tsha256\n")
	n := 0
	for _, kind := range []string{"DOB", "WH"} {
		for i := 0; i < nPer; i++ {
			n++
			mid, tid := fmt.Sprintf("m%03d", n), fmt.Sprintf("f%04d", n)
			twinIn := fmt.Sprintf(`{"input":{"case":"%s"}}`, tid)
			write := func(p, s string) {
				if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			write(filepath.Join(missDir, "inputs", mid, "input.json"), fmt.Sprintf(`{"input":{"case":"%s","gap":1}}`, mid))
			write(filepath.Join(setDir, "inputs", tid, "input.json"), twinIn)
			m := map[string]float64{}
			for _, l := range scoredLabels {
				m[l] = float64(1000*n) + float64(len(l))
			}
			kb, err := json.Marshal(m)
			if err != nil {
				t.Fatal(err)
			}
			write(filepath.Join(setDir, "key", tid+".json"), string(kb))
			fmt.Fprintf(&missM, "%s\t%s\t%s\tp.q\t2010-01-01\t27,28,35a\t0\n", mid, tid, kind)
			fmt.Fprintf(&setM, "%s\tSYNTH\ttpl\tstr\t1\t%s\n", tid, sha256Hex([]byte(twinIn)))
			fmt.Fprintf(&spec, "%s\tSYNTH\tkeyed\th\tg\t-\t0\n", tid)
		}
	}
	for p, s := range map[string]string{
		filepath.Join(missDir, "MANIFEST.tsv"):           missM.String(),
		filepath.Join(setDir, "MANIFEST.tsv"):            setM.String(),
		filepath.Join(setDir, "audit", "under-spec.tsv"): spec.String(),
	} {
		if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return missDir, setDir
}

func TestPrepStudy(t *testing.T) {
	missDir, setDir := studyDirs(t, 3)
	items, rep, err := prepStudy(context.Background(), missDir, setDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 12 || rep.Miss != 6 || rep.MissByKind["DOB"] != 3 || rep.MissByKind["WH"] != 3 || rep.Twins != 6 || rep.TwinsKeyed != 6 || rep.Paired != 6 {
		t.Fatalf("items=%d report=%+v", len(items), rep)
	}
	byID := map[string]Item{}
	for _, it := range items {
		byID[it.ID] = it
		// A model is only ever given input.json.
		if filepath.Base(it.Input) != "input.json" || !filepath.IsAbs(it.Input) {
			t.Errorf("%s: input %q", it.ID, it.Input)
		}
	}
	m := byID["m001"]
	if m.Set != SetMiss || m.Keyed || m.Key != nil || m.KeyFile != "" || m.Twin != "f0001" || m.Kind != "DOB" || strings.Join(m.AffectedLines, ",") != "27,28,35a" {
		t.Errorf("MISS item = %+v", m)
	}
	tw := byID["f0001"]
	if tw.Set != SetTwins || !tw.Keyed || tw.Key == nil || tw.KeyFile == "" || tw.Twin != "m001" {
		t.Errorf("TWINS item = %+v", tw)
	}
	for _, l := range scoredLabels {
		if labelPrefix(l) == "Line 35a" && tw.Key.Refund != float64(1000)+float64(len(l)) {
			t.Errorf("twin refund figure = %v", tw.Key.Refund)
		}
	}
	// The file round-trips through the items reader, every MISS item's twin is
	// a keyed TWINS item, and nothing is written over an existing file.
	out := filepath.Join(t.TempDir(), "new", "items.jsonl")
	var so strings.Builder
	if code := realMain(context.Background(), []string{"prep-study", "-miss", missDir, "-set", setDir, "-out", out}, &so, &so); code != 0 {
		t.Fatalf("prep-study: %s", so.String())
	}
	back, err := readItems(out)
	if err != nil || len(back) != 12 {
		t.Fatalf("read back: %d items, %v", len(back), err)
	}
	ids := map[string]Item{}
	for _, it := range back {
		ids[it.ID] = it
	}
	for _, it := range back {
		if it.Set != SetMiss {
			continue
		}
		twin, ok := ids[it.Twin]
		if !ok || twin.Set != SetTwins || !twin.Keyed || twin.Twin != it.ID {
			t.Errorf("MISS %s: twin %q not a keyed TWINS item pointing back", it.ID, it.Twin)
		}
	}
	if code := realMain(context.Background(), []string{"prep-study", "-miss", missDir, "-set", setDir, "-out", out}, &so, &so); code == 0 {
		t.Error("prep-study overwrote an existing items file")
	}
	if !strings.Contains(so.String(), "6 MISS (DOB 3, WH 3)") {
		t.Errorf("summary: %s", so.String())
	}
}

func TestPrepStudyRefusals(t *testing.T) {
	tests := []struct {
		name   string
		break_ func(t *testing.T, missDir, setDir string)
		want   string
	}{
		{"refusal stub as key", func(t *testing.T, _, s string) {
			writeFile(t, filepath.Join(s, "key", "f0002.json"), `{"verdict":"REFUSE"}`)
		}, "missing"},
		{"key lacks a line", func(t *testing.T, _, s string) {
			writeFile(t, filepath.Join(s, "key", "f0002.json"), `{"Line 16: Tax": 5}`)
		}, "missing"},
		{"twin input changed since the manifest", func(t *testing.T, _, s string) {
			writeFile(t, filepath.Join(s, "inputs", "f0002", "input.json"), `{"changed":true}`)
		}, "sha256"},
		{"twin not keyed by the engine", func(t *testing.T, _, s string) {
			p := filepath.Join(s, "audit", "under-spec.tsv")
			b, err := os.ReadFile(p)
			if err != nil {
				t.Fatal(err)
			}
			writeFile(t, p, strings.Replace(string(b), "f0002\tSYNTH\tkeyed", "f0002\tSYNTH\trefused", 1))
		}, "want keyed"},
		{"MISS input missing", func(t *testing.T, m, _ string) {
			if err := os.Remove(filepath.Join(m, "inputs", "m001", "input.json")); err != nil {
				t.Fatal(err)
			}
		}, "no such file"},
		{"affected line that is not scored", func(t *testing.T, m, _ string) {
			p := filepath.Join(m, "MANIFEST.tsv")
			b, err := os.ReadFile(p)
			if err != nil {
				t.Fatal(err)
			}
			writeFile(t, p, strings.Replace(string(b), "27,28,35a", "27,99", 1))
		}, "not one of the scored lines"},
		{"a twin used twice", func(t *testing.T, m, _ string) {
			p := filepath.Join(m, "MANIFEST.tsv")
			b, err := os.ReadFile(p)
			if err != nil {
				t.Fatal(err)
			}
			writeFile(t, p, strings.Replace(string(b), "m002\tf0002", "m002\tf0001", 1))
		}, "two MISS items"},
		{"unknown kind", func(t *testing.T, m, _ string) {
			p := filepath.Join(m, "MANIFEST.tsv")
			b, err := os.ReadFile(p)
			if err != nil {
				t.Fatal(err)
			}
			writeFile(t, p, strings.Replace(string(b), "\tDOB\t", "\tDOBX\t", 1))
		}, "kind DOB or WH"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m, s := studyDirs(t, 2)
			tc.break_(t, m, s)
			_, _, err := prepStudy(context.Background(), m, s)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err = %v, want one mentioning %q", err, tc.want)
			}
		})
	}
}

func writeFile(t *testing.T, p, s string) {
	t.Helper()
	if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}
