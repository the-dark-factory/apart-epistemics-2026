package main

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
)

// ErrNotFrozen is returned when a set other than DEV is requested without a valid freeze.
var ErrNotFrozen = errors.New("sets other than DEV need -plan and -frozen: the plan must be frozen first")

var planSHALine = regexp.MustCompile(`^plan-sha256:\s*([0-9a-fA-F]{64})$`)

// checkFreeze enforces the freeze guard. Items of the DEV set always pass.
// Any other set is refused unless frozenPath names a file with a line
// "FROZEN" and a line "plan-sha256: <64 hex>" equal to the sha256 of the
// plan file at planPath. It returns the sets that needed the freeze.
func checkFreeze(items []Item, planPath, frozenPath string) error {
	var gated []string
	seen := map[string]bool{}
	for _, it := range items {
		if it.Set != SetDev && !seen[it.Set] {
			seen[it.Set] = true
			gated = append(gated, it.Set)
		}
	}
	if len(gated) == 0 {
		return nil
	}
	if planPath == "" || frozenPath == "" {
		return fmt.Errorf("%w (sets requested: %s)", ErrNotFrozen, strings.Join(gated, ", "))
	}
	plan, err := os.ReadFile(planPath)
	if err != nil {
		return fmt.Errorf("freeze guard: read plan: %w", err)
	}
	fz, err := os.ReadFile(frozenPath)
	if err != nil {
		return fmt.Errorf("freeze guard: read frozen file: %w", err)
	}
	var hasFrozen bool
	var sha string
	for _, ln := range strings.Split(string(fz), "\n") {
		ln = strings.TrimSpace(ln)
		if ln == "FROZEN" {
			hasFrozen = true
		}
		if m := planSHALine.FindStringSubmatch(ln); m != nil {
			sha = strings.ToLower(m[1])
		}
	}
	switch {
	case !hasFrozen:
		return fmt.Errorf("freeze guard: %s has no FROZEN line", frozenPath)
	case sha == "":
		return fmt.Errorf("freeze guard: %s has no plan-sha256 line", frozenPath)
	case sha != sha256Hex(plan):
		return fmt.Errorf("freeze guard: plan-sha256 in %s does not match the plan file", frozenPath)
	}
	return nil
}
