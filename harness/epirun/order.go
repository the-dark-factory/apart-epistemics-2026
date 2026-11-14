package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Per-set protocols. ProtoTurn1 runs the return turn only; ProtoFull runs the
// return, then (after an ANSWER) the confidence turn and, on keyed items, the
// neutral and wrong-anchor pushback branches.
const (
	ProtoTurn1 = "turn1"
	ProtoFull  = "full"
)

// protocolFor gives the protocol for a set: the explicit choice, else turn 1
// only for MISS (it has no key and the right move is to ask), else the full
// conversation.
func protocolFor(set string, explicit map[string]string) string {
	if p, ok := explicit[set]; ok {
		return p
	}
	if set == SetMiss {
		return ProtoTurn1
	}
	return ProtoFull
}

// parseProtocols reads "MISS=turn1,TWINS=full" into a set-to-protocol map.
// An empty string gives an empty map. Unknown sets and protocols are errors.
func parseProtocols(s string) (map[string]string, error) {
	out := map[string]string{}
	if s == "" {
		return out, nil
	}
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		set, proto, _ := strings.Cut(part, "=")
		if set == "" || proto == "" {
			return nil, fmt.Errorf("protocol %q: want SET=turn1 or SET=full", part)
		}
		if !knownSets[set] {
			return nil, fmt.Errorf("protocol %q: unknown set %q", part, set)
		}
		if proto != ProtoTurn1 && proto != ProtoFull {
			return nil, fmt.Errorf("protocol %q: protocol must be %s or %s", part, ProtoTurn1, ProtoFull)
		}
		if _, dup := out[set]; dup {
			return nil, fmt.Errorf("protocol for set %s given twice", set)
		}
		out[set] = proto
	}
	return out, nil
}

// orderItems returns items in the fixed pseudo-random order for seed: sorted
// by sha256("epirun-order:<seed>:<id>"). The order depends only on the seed and
// the ids, not on the order of the items file or on which items are present, so
// the same seed gives the same order, a subset keeps the full run's relative
// order, and any prefix of the order is a random subset. The input is not changed.
func orderItems(items []Item, seed int64) []Item {
	type kv struct {
		key string
		it  Item
	}
	ks := make([]kv, len(items))
	for i, it := range items {
		ks[i] = kv{sha256Hex([]byte("epirun-order:" + strconv.FormatInt(seed, 10) + ":" + it.ID)), it}
	}
	sort.Slice(ks, func(i, j int) bool { return ks[i].key < ks[j].key })
	out := make([]Item, len(ks))
	for i := range ks {
		out[i] = ks[i].it
	}
	return out
}

// runParamsFile is the name of the file recording the parameters of a run.
const runParamsFile = "run-params.json"

// ErrParamsChanged means the run directory was started with a different seed
// or protocol, so resuming would mix two designs.
var ErrParamsChanged = errors.New("run parameters differ from those recorded in this run directory")

// RunParams are the parameters that must not change between a run and its resume.
type RunParams struct {
	// Seed is the item-order seed; nil means items ran in the order of the items file.
	Seed *int64 `json:"seed"`
	// Protocols maps each set present to the protocol used for it.
	Protocols map[string]string `json:"protocols"`
}

// recordRunParams writes want to dir/run-params.json, or checks it against the
// file already there. A seed or a protocol that differs for a set in both is
// ErrParamsChanged. Sets new to this run are added to the file.
func recordRunParams(ctx context.Context, dir string, want RunParams) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	path := filepath.Join(dir, runParamsFile)
	raw, err := os.ReadFile(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return writeRunParams(path, want)
	case err != nil:
		return fmt.Errorf("read run params: %w", err)
	}
	var have RunParams
	if err := json.Unmarshal(raw, &have); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	if (have.Seed == nil) != (want.Seed == nil) || (have.Seed != nil && *have.Seed != *want.Seed) {
		return fmt.Errorf("%w: seed %s here, %s recorded", ErrParamsChanged, seedString(want.Seed), seedString(have.Seed))
	}
	added := false
	if have.Protocols == nil {
		have.Protocols = map[string]string{}
	}
	for set, p := range want.Protocols {
		if old, ok := have.Protocols[set]; ok {
			if old != p {
				return fmt.Errorf("%w: set %s protocol %s here, %s recorded", ErrParamsChanged, set, p, old)
			}
			continue
		}
		have.Protocols[set] = p
		added = true
	}
	if added {
		return writeRunParams(path, have)
	}
	return nil
}

func seedString(s *int64) string {
	if s == nil {
		return "none"
	}
	return strconv.FormatInt(*s, 10)
}

func writeRunParams(path string, p RunParams) error {
	b, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return fmt.Errorf("encode run params: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o644); err != nil {
		return fmt.Errorf("write run params: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("replace run params: %w", err)
	}
	return nil
}

// readRunParams reads dir/run-params.json; a missing file gives the zero value and false.
func readRunParams(dir string) (RunParams, bool, error) {
	raw, err := os.ReadFile(filepath.Join(dir, runParamsFile))
	if errors.Is(err, os.ErrNotExist) {
		return RunParams{}, false, nil
	}
	if err != nil {
		return RunParams{}, false, fmt.Errorf("read run params: %w", err)
	}
	var p RunParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return RunParams{}, false, fmt.Errorf("parse run params: %w", err)
	}
	return p, true, nil
}
