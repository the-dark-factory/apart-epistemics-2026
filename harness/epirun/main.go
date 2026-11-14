// Command epirun is the run harness for the epistemics study "Can you trust
// an LLM to do your taxes?". It puts TaxCalcBench TY24 returns to models in a
// fixed three-turn conversation (return, confidence, pushback) and writes an
// append-only raw log.
//
// Usage:
//
//	epirun prep -out items.jsonl [-dev-dir D] [-testdata T]
//	epirun prep-study -miss fresh/miss-X -set fresh/set-X -out items.jsonl
//	epirun run -items items.jsonl -models models.json -out dir
//	           [-only id,id] [-samples 3] [-budget-usd n] [-plan f -frozen f]
//	           [-seed n] [-protocol MISS=turn1,TWINS=full]
//	epirun analyse -items items.jsonl -run dir [-boot 10000] [-seed n] [-out report.txt]
//
// -seed puts the items in a fixed pseudo-random order (required for any set
// other than DEV); -protocol picks, per set, turn 1 only or the full
// conversation (MISS defaults to turn 1 only).
//
// Sets other than DEV are refused unless -frozen names a freeze file for the
// exact -plan. A real model needs -budget-usd.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	code := realMain(ctx, os.Args[1:], os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}

func realMain(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: epirun prep|prep-study|run|analyse [flags]")
		return 2
	}
	var err error
	switch args[0] {
	case "prep":
		err = prepCmd(ctx, args[1:], stdout)
	case "prep-study":
		err = prepStudyCmd(ctx, args[1:], stdout)
	case "run":
		err = runCmd(ctx, args[1:], stdout, stderr)
	case "analyse":
		err = analyseCmd(ctx, args[1:], stdout)
	default:
		fmt.Fprintf(stderr, "epirun: unknown subcommand %q\n", args[0])
		return 2
	}
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		fmt.Fprintf(stderr, "epirun: %v\n", err)
		return 1
	}
	return 0
}

func prepCmd(ctx context.Context, args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("prep", flag.ContinueOnError)
	out := fs.String("out", "", "items.jsonl to create (must not exist)")
	devDir := fs.String("dev-dir", defaultDevDir, "re-run directory holding dev-cases.txt, <case>.txt and <case>.json")
	testData := fs.String("testdata", defaultTestData, "upstream ty24 test_data directory")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *out == "" {
		return errors.New("prep: -out is required")
	}
	items, err := prepDev(*devDir, *testData)
	if err != nil {
		return fmt.Errorf("prep: %w", err)
	}
	if err := writeItems(ctx, *out, items); err != nil {
		return fmt.Errorf("prep: %w", err)
	}
	fmt.Fprintf(stdout, "wrote %d DEV items to %s\n", len(items), *out)
	return nil
}

func prepStudyCmd(ctx context.Context, args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("prep-study", flag.ContinueOnError)
	out := fs.String("out", "", "items.jsonl to create (must not exist)")
	missDir := fs.String("miss", "", "MISS set directory (fresh/miss-...)")
	setDir := fs.String("set", "", "full-facts set directory (fresh/set-...)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *out == "" || *missDir == "" || *setDir == "" {
		return errors.New("prep-study: -miss, -set and -out are required")
	}
	items, rep, err := prepStudy(ctx, *missDir, *setDir)
	if err != nil {
		return fmt.Errorf("prep-study: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil {
		return fmt.Errorf("prep-study: %w", err)
	}
	if err := writeItems(ctx, *out, items); err != nil {
		return fmt.Errorf("prep-study: %w", err)
	}
	fmt.Fprintf(stdout, "wrote %d items to %s: %d MISS (DOB %d, WH %d), %d TWINS keyed %d, %d pairs with a keyed twin\n",
		len(items), *out, rep.Miss, rep.MissByKind["DOB"], rep.MissByKind["WH"], rep.Twins, rep.TwinsKeyed, rep.Paired)
	return nil
}

func runCmd(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	itemsPath := fs.String("items", "", "items.jsonl")
	modelsPath := fs.String("models", "", "models.json")
	outDir := fs.String("out", "", "output directory (calls.jsonl is appended here)")
	only := fs.String("only", "", "comma-separated item ids to run")
	samples := fs.Int("samples", 3, "samples per (model, item)")
	budgetUSD := fs.Float64("budget-usd", 0, "stop before a call that could pass this total cost (mandatory for real models)")
	planPath := fs.String("plan", "", "plan file the freeze file must match")
	frozenPath := fs.String("frozen", "", "freeze file holding FROZEN and plan-sha256")
	seed := fs.Int64("seed", 0, "seed for the fixed random item order (required for any set other than DEV)")
	protoFlag := fs.String("protocol", "", "per-set protocol, e.g. MISS=turn1,TWINS=full (MISS defaults to turn1, others to full)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	seedGiven := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "seed" {
			seedGiven = true
		}
	})
	protocols, err := parseProtocols(*protoFlag)
	if err != nil {
		return fmt.Errorf("run: %w", err)
	}
	if *itemsPath == "" || *modelsPath == "" || *outDir == "" {
		return errors.New("run: -items, -models and -out are required")
	}
	items, err := readItems(*itemsPath)
	if err != nil {
		return err
	}
	if *only != "" {
		items, err = filterItems(items, strings.Split(*only, ","))
		if err != nil {
			return err
		}
	}
	// The freeze guard comes before anything else is touched.
	if err := checkFreeze(items, *planPath, *frozenPath); err != nil {
		return err
	}
	var seedPtr *int64
	if seedGiven {
		s := *seed
		seedPtr = &s
	}
	if seedPtr == nil {
		for _, it := range items {
			if it.Set != SetDev {
				return fmt.Errorf("run: -seed is required for set %s: a partial run must be a random subset, in a fixed order", it.Set)
			}
		}
	}
	models, err := loadModels(*modelsPath)
	if err != nil {
		return err
	}
	provs := map[string]Provider{}
	for _, m := range models {
		p, err := newProvider(m)
		if err != nil {
			return err
		}
		provs[m.Name] = p
	}
	res, err := Run(ctx, RunConfig{
		Items: items, Models: models, Providers: provs, OutDir: *outDir,
		Samples: *samples, BudgetUSD: *budgetUSD, Seed: seedPtr, Protocols: protocols,
	})
	if res.TornLines > 0 {
		fmt.Fprintf(stderr, "epirun: warning: %d unreadable line(s) in the existing log were skipped, not rewritten\n", res.TornLines)
	}
	rows, _, rerr := readCallLog(callsPath(*outDir))
	if rerr != nil && err == nil {
		err = rerr
	}
	table := countTable(rows)
	fmt.Fprint(stdout, table)
	fmt.Fprintf(stdout, "\nthis run: made %d, skipped %d (already logged), failed %d, anchor branches not applicable %d\n",
		res.Made, res.Skipped, res.Failed, res.AnchorNotApplicable)
	if werr := os.WriteFile(filepath.Join(*outDir, "counts.txt"), []byte(table), 0o644); werr != nil && err == nil {
		err = fmt.Errorf("write counts.txt: %w", werr)
	}
	if res.StopReason == "budget" {
		fmt.Fprintln(stdout, "run stopped cleanly: the next call could pass -budget-usd")
	}
	return err
}

// filterItems keeps the named items, in the order of the items file.
func filterItems(items []Item, ids []string) ([]Item, error) {
	want := map[string]bool{}
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id != "" {
			want[id] = true
		}
	}
	var out []Item
	for _, it := range items {
		if want[it.ID] {
			out = append(out, it)
			delete(want, it.ID)
		}
	}
	if len(want) > 0 {
		var missing []string
		for id := range want {
			missing = append(missing, id)
		}
		return nil, fmt.Errorf("-only names unknown item(s): %s", strings.Join(missing, ", "))
	}
	return out, nil
}
