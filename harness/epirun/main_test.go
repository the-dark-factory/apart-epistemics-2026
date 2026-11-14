package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFreezeGuard(t *testing.T) {
	dir := t.TempDir()
	plan := filepath.Join(dir, "plan.md")
	if err := os.WriteFile(plan, []byte("the plan\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sha := sha256Hex([]byte("the plan\n"))
	nfrozen := 0
	frozen := func(body string) string {
		nfrozen++
		p := filepath.Join(dir, fmt.Sprintf("frozen%d.txt", nfrozen))
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	dev := []Item{{ID: "d", Set: SetDev}}
	for _, set := range []string{SetPubIn, SetPubOut, SetNear, SetSynth, SetUnclear, SetMiss} {
		items := []Item{{ID: "d", Set: SetDev}, {ID: "x", Set: set}}
		tests := []struct {
			name           string
			plan, frozenAt string
			wantErr        bool
		}{
			{"no flags", "", "", true},
			{"plan only", plan, "", true},
			{"frozen only", "", frozen("FROZEN\nplan-sha256: " + sha + "\n"), true},
			{"wrong hash", plan, frozen("FROZEN\nplan-sha256: " + strings.Repeat("0", 64) + "\n"), true},
			{"no FROZEN line", plan, frozen("plan-sha256: " + sha + "\n"), true},
			{"no hash line", plan, frozen("FROZEN\n"), true},
			{"short hash", plan, frozen("FROZEN\nplan-sha256: abc\n"), true},
			{"valid", plan, frozen("FROZEN\nplan-sha256: " + sha + "\n"), false},
			{"valid, upper-case hash", plan, frozen("FROZEN\nplan-sha256: " + strings.ToUpper(sha) + "\n"), false},
		}
		for _, tc := range tests {
			err := checkFreeze(items, tc.plan, tc.frozenAt)
			if (err != nil) != tc.wantErr {
				t.Errorf("%s/%s: err=%v wantErr=%v", set, tc.name, err, tc.wantErr)
			}
		}
		if err := checkFreeze(items, "", ""); !errors.Is(err, ErrNotFrozen) {
			t.Errorf("%s: want ErrNotFrozen, got %v", set, err)
		}
	}
	if err := checkFreeze(dev, "", ""); err != nil {
		t.Errorf("DEV alone should pass: %v", err)
	}
}

// TestRunRefusedBeforeAnyCall drives the command line: a NEAR item with no
// freeze must stop before models are loaded or the output directory is made.
func TestRunRefusedBeforeAnyCall(t *testing.T) {
	dir := t.TempDir()
	items := testItems(t, dir)
	items[0].Set = SetNear
	itemsPath := filepath.Join(dir, "items.jsonl")
	if err := writeItems(context.Background(), itemsPath, items); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "out")
	var so, se bytes.Buffer
	code := realMain(context.Background(), []string{"run", "-items", itemsPath, "-models", filepath.Join(dir, "absent.json"), "-out", out}, &so, &se)
	if code == 0 || !strings.Contains(se.String(), "frozen") {
		t.Errorf("code=%d stderr=%q", code, se.String())
	}
	if _, err := os.Stat(out); err == nil {
		t.Error("output directory was created")
	}
	// -only can leave just DEV items, which then pass the guard.
	models := filepath.Join(dir, "models.json")
	if err := os.WriteFile(models, []byte(`[{"name":"fa","kind":"fake"}]`), 0o644); err != nil {
		t.Fatal(err)
	}
	so.Reset()
	se.Reset()
	code = realMain(context.Background(), []string{"run", "-items", itemsPath, "-models", models, "-out", out, "-only", "u1", "-samples", "2"}, &so, &se)
	if code != 0 {
		t.Fatalf("code=%d stderr=%s", code, se.String())
	}
	if !strings.Contains(so.String(), "total") {
		t.Errorf("no count table: %s", so.String())
	}
	if _, err := os.Stat(filepath.Join(out, "counts.txt")); err != nil {
		t.Error(err)
	}
}

func TestPrepDev(t *testing.T) {
	dir := t.TempDir()
	dev := filepath.Join(dir, "dev")
	td := filepath.Join(dir, "td")
	for _, d := range []string{dev, filepath.Join(td, "a"), filepath.Join(td, "c")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	w := func(p, s string) {
		if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	w(filepath.Join(dev, "dev-cases.txt"), "a\nb\nc\n")
	w(filepath.Join(dev, "a.txt"), "Form 1040: x\n")
	w(filepath.Join(dev, "a.json"), `{"`+keyLabelRefund+`":3807,"`+keyLabelOwed+`":0,"`+keyLabelTax+`":6103,"`+keyLabelAGI+`":77671,"other":1}`)
	w(filepath.Join(dev, "b.txt"), "REFUSED: SELF_EMPLOYMENT\nNo return was computed\n")
	w(filepath.Join(dev, "b.json"), `{"verdict":"REFUSE"}`)
	w(filepath.Join(dev, "c.txt"), "Form 1040: y\n")
	w(filepath.Join(dev, "c.json"), `{"`+keyLabelRefund+`":0}`)
	w(filepath.Join(td, "a", "input.json"), "{}")
	w(filepath.Join(td, "c", "input.json"), "{}")

	if _, err := prepDev(dev, td); err == nil {
		t.Fatal("a case with missing key figures should fail loudly")
	}
	w(filepath.Join(dev, "dev-cases.txt"), "a\nb\n")
	items, err := prepDev(dev, td)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != "a" || items[0].Set != SetDev || !items[0].Keyed ||
		*items[0].Key != (KeyFigures{Refund: 3807, TotalTax: 6103, AGI: 77671}) ||
		items[0].Input != filepath.Join(td, "a", "input.json") {
		t.Errorf("items = %+v", items)
	}
	out := filepath.Join(dir, "items.jsonl")
	if err := writeItems(context.Background(), out, items); err != nil {
		t.Fatal(err)
	}
	if err := writeItems(context.Background(), out, items); err == nil {
		t.Error("writeItems must not overwrite")
	}
	back, err := readItems(out)
	if err != nil || len(back) != 1 || back[0].Key.Refund != 3807 {
		t.Errorf("round trip: %v %+v", err, back)
	}
}

func TestReadItemsValidation(t *testing.T) {
	dir := t.TempDir()
	cases := map[string]string{
		"unknown set":   `{"id":"a","set":"WAT","input":"x","keyed":false}`,
		"keyed no key":  `{"id":"a","set":"DEV","input":"x","keyed":true}`,
		"key not keyed": `{"id":"a","set":"DEV","input":"x","keyed":false,"key":{"refund":1}}`,
		"duplicate":     "{\"id\":\"a\",\"set\":\"DEV\",\"input\":\"x\",\"keyed\":false}\n{\"id\":\"a\",\"set\":\"DEV\",\"input\":\"x\",\"keyed\":false}",
		"empty id":      `{"id":"","set":"DEV","input":"x","keyed":false}`,
	}
	for name, body := range cases {
		p := filepath.Join(dir, "i.jsonl")
		if err := os.WriteFile(p, []byte(body+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := readItems(p); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}

func TestFilterItemsUnknown(t *testing.T) {
	items := []Item{{ID: "a"}, {ID: "b"}}
	if _, err := filterItems(items, []string{"a", "zz"}); err == nil {
		t.Error("unknown id should fail")
	}
	got, err := filterItems(items, []string{"b"})
	if err != nil || len(got) != 1 || got[0].ID != "b" {
		t.Errorf("got %v %v", got, err)
	}
}
