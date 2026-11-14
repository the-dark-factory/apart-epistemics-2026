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
	"sync"
	"sync/atomic"
	"time"
)

// Defaults for retry.
const (
	defaultMaxAttempts = 4
	// defaultMaxOutputEstimate is the output size assumed for the budget
	// estimate when a model entry sets no max_output_tokens.
	defaultMaxOutputEstimate = 16000
	// claudeCallCap is the per-call cap passed to claude (--max-budget-usd).
	claudeCallCap = 1.0
)

// RunConfig is everything a run needs.
type RunConfig struct {
	// Items are the items to run, already filtered.
	Items []Item
	// Models are the config entries to run.
	Models []ModelConfig
	// Providers maps model name to its provider.
	Providers map[string]Provider
	// OutDir holds calls.jsonl.
	OutDir string
	// Samples is the number of samples per (model, item).
	Samples int
	// BudgetUSD caps accumulated actual cost; 0 means no cap (fake models only).
	BudgetUSD float64
	// MaxAttempts bounds tries per call; 0 means the default.
	MaxAttempts int
	// Backoff gives the wait before retry number attempt (1-based); nil means 1s doubling.
	Backoff func(attempt int) time.Duration
	// Now gives the clock; nil means time.Now.
	Now func() time.Time
	// Protocols maps a set to ProtoTurn1 or ProtoFull. A set not named gets
	// turn 1 only if it is MISS and the full conversation otherwise.
	Protocols map[string]string
	// Seed, when non-nil, runs the items in the fixed seeded random order for
	// that seed (see orderItems); nil keeps the order of Items. It is recorded
	// in run-params.json and a resume must use the same seed.
	Seed *int64
}

// RunResult says how a run ended.
type RunResult struct {
	// Made counts calls made in this run that were logged.
	Made int
	// Skipped counts calls skipped because they were already logged complete.
	Skipped int
	// Failed counts calls logged as failed in this run.
	Failed int
	// AnchorNotApplicable counts anchor branches skipped in this run because
	// every turn 1 figure (refund, amount owed, total tax) was zero or unparsable.
	AnchorNotApplicable int
	// TornLines counts unreadable lines found in an existing log.
	TornLines int
	// StopReason is "" for a finished run, "budget" for a budget stop.
	StopReason string
}

// ErrHistoryChanged means a logged call's request no longer matches what the
// harness would send now, so resuming would mix two protocols.
var ErrHistoryChanged = errors.New("logged request differs from the request built now")

// checkRunConfig refuses runs that would be unsafe or unpriced.
func checkRunConfig(c *RunConfig) error {
	if c.Samples < 1 {
		return errors.New("samples must be at least 1")
	}
	if len(c.Items) == 0 {
		return errors.New("no items to run")
	}
	for set, p := range c.Protocols {
		if !knownSets[set] || (p != ProtoTurn1 && p != ProtoFull) {
			return fmt.Errorf("protocol %s=%s is not valid", set, p)
		}
	}
	for _, m := range c.Models {
		if c.Providers[m.Name] == nil {
			return fmt.Errorf("model %q has no provider", m.Name)
		}
		if m.Kind == KindFake {
			continue
		}
		if c.BudgetUSD <= 0 {
			return fmt.Errorf("model %q is a real model: -budget-usd is mandatory", m.Name)
		}
		if m.Kind == KindOpenAI && (m.PriceIn <= 0 || m.PriceOut <= 0) {
			return fmt.Errorf("model %q has no price: refusing to start a priced run", m.Name)
		}
	}
	return nil
}

type turn1Prompt struct {
	text string
	sha  string
}

// budget tracks accumulated actual cost and the cost reserved by calls in flight.
type budget struct {
	mu       sync.Mutex
	cap      float64
	spent    float64
	reserved float64
	maxSeen  map[string]float64
}

// reserve takes est if spent+reserved+est stays within the cap.
func (b *budget) reserve(est float64) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.cap > 0 && b.spent+b.reserved+est > b.cap {
		return false
	}
	b.reserved += est
	return true
}

// settle releases a reservation and adds the actual cost in one step, so a
// call is always counted either as reserved or as spent, never as neither.
// It reports whether the spend alone is now past the cap (a call that cost
// more than its estimate), which the caller treats as a clean budget stop.
func (b *budget) settle(model string, est, actual float64) (over bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.reserved -= est
	b.spent += actual
	if actual > b.maxSeen[model] {
		b.maxSeen[model] = actual
	}
	return b.cap > 0 && b.spent > b.cap
}

func (b *budget) observedMax(model string) float64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.maxSeen[model]
}

// estimate is the most one call could cost, from the request size and the
// model's price; for a claude-headless entry with no price, from the largest
// cost seen so far, or the per-call cap before any is seen.
func estimate(m ModelConfig, msgs []Message, b *budget) float64 {
	if m.PriceIn > 0 || m.PriceOut > 0 {
		chars := 0
		for _, x := range msgs {
			chars += len(x.Content)
		}
		maxOut := m.MaxOutputTokens
		if maxOut <= 0 {
			maxOut = defaultMaxOutputEstimate
		}
		est := float64(chars)/3*m.PriceIn/1e6 + float64(maxOut)*m.PriceOut/1e6
		if m.Kind == KindClaudeHeadless && est > claudeCallCap {
			est = claudeCallCap
		}
		return est
	}
	if m.Kind == KindClaudeHeadless {
		if seen := b.observedMax(m.Name); seen > 0 {
			e := seen * 1.5
			if e > claudeCallCap {
				e = claudeCallCap
			}
			return e
		}
		return claudeCallCap
	}
	return 0
}

// actualCost is the cost charged for a reply: the provider's own figure, else tokens times price.
func actualCost(m ModelConfig, r Reply) (float64, error) {
	if r.CostUSD != nil {
		return *r.CostUSD, nil
	}
	if m.PriceIn > 0 || m.PriceOut > 0 {
		return float64(r.Usage.InputTokens)*m.PriceIn/1e6 + float64(r.Usage.OutputTokens)*m.PriceOut/1e6, nil
	}
	if m.Kind == KindFake {
		return 0, nil
	}
	return 0, fmt.Errorf("model %q: no cost reported and no price set", m.Name)
}

// assistantMsg rebuilds an assistant turn from its row, fresh or recalled from
// the log. For an echo_reasoning model it carries the logged reasoning (empty
// string when none); otherwise the message has no reasoning field, so the
// request bytes are as before. Resume therefore rebuilds the same conversation.
func assistantMsg(m ModelConfig, row LogRow) Message {
	msg := Message{Role: "assistant", Content: row.Reply}
	if m.EchoReasoning {
		r := row.Reasoning
		msg.Reasoning = &r
	}
	return msg
}

// requestSHA hashes the full request: model identity and every message.
func requestSHA(m ModelConfig, msgs []Message) (string, error) {
	b, err := json.Marshal(struct {
		Name     string    `json:"name"`
		Model    string    `json:"model"`
		Messages []Message `json:"messages"`
	}{m.Name, m.Model, msgs})
	if err != nil {
		return "", fmt.Errorf("hash request: %w", err)
	}
	return sha256Hex(b), nil
}

// runner holds the state of one run.
type runner struct {
	cfg      RunConfig
	log      *callLog
	done     map[CallKey]LogRow
	doneMu   sync.Mutex
	bud      *budget
	prompts  map[string]turn1Prompt
	stop     atomic.Value // string: why the run stopped early, "" while running
	fatalMu  sync.Mutex
	fatal    error
	made     atomic.Int64
	skipped  atomic.Int64
	failed   atomic.Int64
	anchorNA atomic.Int64
}

func (r *runner) stopped() bool {
	s, _ := r.stop.Load().(string)
	return s != ""
}

func (r *runner) setStop(reason string) {
	r.stop.CompareAndSwap("", reason)
}

func (r *runner) setFatal(err error) {
	r.fatalMu.Lock()
	if r.fatal == nil {
		r.fatal = err
	}
	r.fatalMu.Unlock()
	r.setStop("error")
}

// callsPath is where the log lives under dir.
func callsPath(dir string) string { return filepath.Join(dir, "calls.jsonl") }

// Run executes the protocol for every (model, item, sample), appending to
// calls.jsonl and skipping calls already logged complete.
func Run(ctx context.Context, cfg RunConfig) (RunResult, error) {
	var res RunResult
	if err := checkRunConfig(&cfg); err != nil {
		return res, err
	}
	if cfg.MaxAttempts <= 0 {
		cfg.MaxAttempts = defaultMaxAttempts
	}
	if cfg.Backoff == nil {
		cfg.Backoff = func(a int) time.Duration { return time.Second << (a - 1) }
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	items := cfg.Items
	if cfg.Seed != nil {
		items = orderItems(items, *cfg.Seed)
	}
	if err := os.MkdirAll(cfg.OutDir, 0o755); err != nil {
		return res, fmt.Errorf("make out dir: %w", err)
	}
	params := RunParams{Seed: cfg.Seed, Protocols: map[string]string{}}
	for _, it := range items {
		params.Protocols[it.Set] = protocolFor(it.Set, cfg.Protocols)
	}
	if err := recordRunParams(ctx, cfg.OutDir, params); err != nil {
		return res, err
	}
	r := &runner{cfg: cfg, done: map[CallKey]LogRow{}, prompts: map[string]turn1Prompt{},
		bud: &budget{cap: cfg.BudgetUSD, maxSeen: map[string]float64{}}}
	r.stop.Store("")

	// Build every turn 1 prompt first, so a template or input problem fails before any call.
	for _, it := range items {
		raw, err := os.ReadFile(it.Input)
		if err != nil {
			return res, fmt.Errorf("item %s: read input: %w", it.ID, err)
		}
		p, sha, err := buildTurn1(raw)
		if err != nil {
			return res, fmt.Errorf("item %s: %w", it.ID, err)
		}
		r.prompts[it.ID] = turn1Prompt{text: p, sha: sha}
	}

	path := callsPath(cfg.OutDir)
	rows, torn, err := readCallLog(path)
	if err != nil {
		return res, err
	}
	res.TornLines = torn
	for _, row := range rows {
		r.bud.spent += row.CostUSD
		if row.Error == "" {
			if _, dup := r.done[row.Key()]; !dup {
				r.done[row.Key()] = row
			}
		}
	}
	lg, err := openCallLog(path)
	if err != nil {
		return res, err
	}
	defer lg.Close()
	r.log = lg

	sems := map[string]chan struct{}{}
	for _, m := range cfg.Models {
		sems[m.Name] = make(chan struct{}, m.Concurrency)
	}
	// One dispatcher per model hands out conversations in item order, taking
	// a concurrency slot before it starts each, so the first items in the
	// (seeded) order are the first to run and a partial run is a prefix of it.
	var wg sync.WaitGroup
	for _, m := range cfg.Models {
		wg.Add(1)
		go func(m ModelConfig) {
			defer wg.Done()
			sem := sems[m.Name]
			var chains sync.WaitGroup
			defer chains.Wait()
			for _, it := range items {
				for s := 1; s <= cfg.Samples; s++ {
					select {
					case sem <- struct{}{}:
					case <-ctx.Done():
						return
					}
					if r.stopped() {
						<-sem
						return
					}
					chains.Add(1)
					go func(it Item, s int) {
						defer chains.Done()
						defer func() { <-sem }()
						r.chain(ctx, m, it, s)
					}(it, s)
				}
			}
		}(m)
	}
	wg.Wait()

	res.Made = int(r.made.Load())
	res.Skipped = int(r.skipped.Load())
	res.Failed = int(r.failed.Load())
	res.AnchorNotApplicable = int(r.anchorNA.Load())
	r.fatalMu.Lock()
	fatal := r.fatal
	r.fatalMu.Unlock()
	if fatal != nil {
		return res, fatal
	}
	if err := ctx.Err(); err != nil {
		return res, err
	}
	if s, _ := r.stop.Load().(string); s != "" {
		res.StopReason = s
	}
	return res, nil
}

// chain runs turns 1, 2 and the pushback branches for one (model, item, sample).
func (r *runner) chain(ctx context.Context, m ModelConfig, it Item, sample int) {
	p1 := r.prompts[it.ID]
	msgs := []Message{{Role: "user", Content: p1.text}}
	row, ok := r.step(ctx, m, it, sample, TurnReturn, "", p1.sha, msgs, nil)
	if !ok || row.Status != StatusAnswer {
		return
	}
	if protocolFor(it.Set, r.cfg.Protocols) == ProtoTurn1 {
		return
	}
	msgs = append(msgs, assistantMsg(m, row))
	msgs = append(msgs, Message{Role: "user", Content: turn2Prompt})
	row2, ok := r.step(ctx, m, it, sample, TurnConfidence, "", sha256Hex([]byte(turn2Prompt)), msgs, nil)
	if !ok {
		return
	}
	if sample != 1 || !it.Keyed {
		return
	}
	// Both turn 3 prompts branch from the same history (turns 1 and 2) and
	// neither branch sees the other.
	base := append(append([]Message(nil), msgs...), assistantMsg(m, row2))
	neutral := append(append([]Message(nil), base...), Message{Role: "user", Content: neutralPrompt})
	if _, ok := r.step(ctx, m, it, sample, TurnPushback, BranchNeutral, sha256Hex([]byte(neutralPrompt)), neutral, nil); !ok && r.mustStop(ctx) {
		return
	}
	r.anchorBranch(ctx, m, it, sample, row, base)
}

// mustStop reports whether the run was stopped or cancelled, as opposed to one
// call having failed (the other branch can still run after a failed call).
func (r *runner) mustStop(ctx context.Context) bool {
	return r.stopped() || ctx.Err() != nil
}

// anchorBranch runs the wrong-anchor pushback from history base. The anchor is
// built from the model's own turn 1 figures in row1. On resume the figure and
// noun logged by the earlier run are used as logged, so the request is rebuilt
// byte for byte. If no figure is usable, no call is made and a NOT_APPLICABLE
// row records it.
func (r *runner) anchorBranch(ctx context.Context, m ModelConfig, it Item, sample int, row1 LogRow, base []Message) {
	key := CallKey{Model: m.Name, Item: it.ID, Sample: sample, Turn: TurnPushback, Branch: BranchAnchor}
	r.doneMu.Lock()
	prev, have := r.done[key]
	r.doneMu.Unlock()
	var a Anchor
	switch {
	case have && prev.Status == StatusNotApplicable:
		sha, err := requestSHA(m, base)
		if err != nil {
			r.setFatal(err)
			return
		}
		if sha != prev.RequestSHA256 {
			r.setFatal(fmt.Errorf("%w: %v", ErrHistoryChanged, key))
			return
		}
		r.skipped.Add(1)
		return
	case have && prev.AnchorX != nil:
		a = Anchor{Noun: prev.AnchorNoun, X: *prev.AnchorX}
		if prev.AnchorOwn != nil {
			a.Own = *prev.AnchorOwn
		}
	default:
		var ok bool
		a, ok = anchorFromOwn(row1.Lines, *it.Key)
		if !ok {
			r.logNotApplicable(ctx, m, it, sample, base)
			return
		}
	}
	prompt := fmt.Sprintf(anchorTemplate, a.Noun, strconv.FormatInt(a.X, 10))
	msgs := append(append([]Message(nil), base...), Message{Role: "user", Content: prompt})
	r.step(ctx, m, it, sample, TurnPushback, BranchAnchor, sha256Hex([]byte(anchorTemplate)), msgs, &a)
}

// logNotApplicable records, without any call, that the anchor branch cannot be
// built because the model's refund, amount owed and total tax were all zero or
// unparsable. The row carries a hash of the shared history so a resume still
// notices a changed conversation.
func (r *runner) logNotApplicable(ctx context.Context, m ModelConfig, it Item, sample int, base []Message) {
	if r.mustStop(ctx) {
		return
	}
	sha, err := requestSHA(m, base)
	if err != nil {
		r.setFatal(err)
		return
	}
	row := LogRow{
		TimeUTC:       r.cfg.Now().UTC().Format(time.RFC3339Nano),
		Model:         m.Name,
		Item:          it.ID,
		Set:           it.Set,
		Sample:        sample,
		Turn:          TurnPushback,
		Branch:        BranchAnchor,
		RequestSHA256: sha,
		Channel:       r.cfg.Providers[m.Name].Channel(),
		Status:        StatusNotApplicable,
		Flags:         []string{"anchor_not_applicable"},
	}
	if err := r.log.Append(row); err != nil {
		r.setFatal(err)
		return
	}
	r.anchorNA.Add(1)
	r.doneMu.Lock()
	r.done[row.Key()] = row
	r.doneMu.Unlock()
}

// step performs (or recalls) one call and returns its row. ok is false when the
// chain must stop: the run was stopped or cancelled, or the call failed.
func (r *runner) step(ctx context.Context, m ModelConfig, it Item, sample, turn int, branch, tmplSHA string,
	msgs []Message, anchor *Anchor) (LogRow, bool) {
	key := CallKey{Model: m.Name, Item: it.ID, Sample: sample, Turn: turn, Branch: branch}
	reqSHA, err := requestSHA(m, msgs)
	if err != nil {
		r.setFatal(err)
		return LogRow{}, false
	}
	r.doneMu.Lock()
	prev, have := r.done[key]
	r.doneMu.Unlock()
	if have {
		if prev.RequestSHA256 != reqSHA {
			r.setFatal(fmt.Errorf("%w: %v", ErrHistoryChanged, key))
			return LogRow{}, false
		}
		r.skipped.Add(1)
		return prev, true
	}
	if r.stopped() || ctx.Err() != nil {
		return LogRow{}, false
	}
	prov := r.cfg.Providers[m.Name]

	var reply Reply
	var callErr error
	var attempts int
	var latency time.Duration
	var cost float64
	var cerr error
	for attempts < r.cfg.MaxAttempts {
		if r.stopped() || ctx.Err() != nil {
			return LogRow{}, false
		}
		est := estimate(m, msgs, r.bud)
		if !r.bud.reserve(est) {
			r.setStop("budget")
			return LogRow{}, false
		}
		attempts++
		start := time.Now()
		reply, callErr = prov.Complete(ctx, msgs)
		latency = time.Since(start)
		// The reservation is held until the actual cost is known and swapped
		// for it in one step, so with several calls in flight none is ever
		// uncounted. A failed attempt costs nothing.
		var actual float64
		if callErr == nil {
			cost, cerr = actualCost(m, reply)
			if cerr == nil {
				actual = cost
			}
		}
		if r.bud.settle(m.Name, est, actual) {
			r.setStop("budget")
		}
		if callErr == nil {
			break
		}
		if ctx.Err() != nil {
			return LogRow{}, false
		}
		if !isTransient(callErr) {
			break
		}
		if attempts < r.cfg.MaxAttempts {
			select {
			case <-time.After(r.cfg.Backoff(attempts)):
			case <-ctx.Done():
				return LogRow{}, false
			}
		}
	}

	row := LogRow{
		TimeUTC:        r.cfg.Now().UTC().Format(time.RFC3339Nano),
		Model:          m.Name,
		Item:           it.ID,
		Set:            it.Set,
		Sample:         sample,
		Turn:           turn,
		Branch:         branch,
		TemplateSHA256: tmplSHA,
		RequestSHA256:  reqSHA,
		Channel:        prov.Channel(),
		LatencyMS:      latency.Milliseconds(),
		Attempts:       attempts,
	}
	if anchor != nil {
		x := anchor.X
		row.AnchorX = &x
		row.AnchorNoun = anchor.Noun
		if anchor.Own != 0 {
			own := anchor.Own
			row.AnchorOwn = &own
		}
		if anchor.PlusOne {
			row.Flags = append(row.Flags, "anchor_plus_1")
		}
		if anchor.SameAsOwn {
			row.Flags = append(row.Flags, "anchor_same_as_own")
		}
	}
	if callErr != nil {
		row.Error = callErr.Error()
		row.Status = StatusUnparsed
		if err := r.log.Append(row); err != nil {
			r.setFatal(err)
			return row, false
		}
		r.failed.Add(1)
		return row, false
	}

	row.Reply = reply.Text
	if m.EchoReasoning {
		row.Reasoning = reply.Reasoning
	}
	row.ModelReturned = reply.ModelReturned
	row.StopReason = reply.StopReason
	row.Usage = reply.Usage
	if reply.Truncated {
		row.Flags = append(row.Flags, "truncated")
	}
	if cerr != nil {
		// The call happened and spent money we cannot count: log it, then stop the run.
		row.Error = cerr.Error()
		row.Status = StatusUnparsed
		if err := r.log.Append(row); err != nil {
			r.setFatal(err)
			return row, false
		}
		r.setFatal(cerr)
		return row, false
	}
	row.CostUSD = cost
	fillStatus(&row, reply.Truncated)
	if err := r.log.Append(row); err != nil {
		r.setFatal(err)
		return row, false
	}
	r.made.Add(1)
	r.doneMu.Lock()
	r.done[key] = row
	r.doneMu.Unlock()
	return row, true
}

// fillStatus sets the status, parsed lines and parsed confidence of a row from its reply.
func fillStatus(row *LogRow, truncated bool) {
	if row.Turn == TurnConfidence {
		if c, ok := parseConfidenceReply(row.Reply); ok && !truncated {
			row.Status = StatusConf
			row.Conf = c
			return
		}
		row.Status = StatusUnparsed
		row.Flags = append(row.Flags, "conf_unparsed")
		return
	}
	row.Status, row.Lines = classify(row.Reply, truncated)
	if row.Turn == TurnPushback && row.Status == StatusAnswer {
		if c, ok := parseConfidenceTail(row.Reply); ok {
			row.Conf = c
		} else {
			row.Flags = append(row.Flags, "conf_unparsed")
		}
	}
}

// countTable renders the calls of a log as counts by model, turn, branch and status.
func countTable(rows []LogRow) string {
	type k struct {
		model  string
		turn   int
		branch string
		status string
	}
	counts := map[k]int{}
	failed := map[k]int{}
	for _, r := range rows {
		kk := k{r.Model, r.Turn, r.Branch, r.Status}
		if r.Error != "" {
			failed[k{r.Model, r.Turn, r.Branch, "FAILED"}]++
			continue
		}
		counts[kk]++
	}
	for kk, n := range failed {
		counts[kk] += n
	}
	keys := make([]k, 0, len(counts))
	for kk := range counts {
		keys = append(keys, kk)
	}
	sort.Slice(keys, func(i, j int) bool {
		a, b := keys[i], keys[j]
		if a.model != b.model {
			return a.model < b.model
		}
		if a.turn != b.turn {
			return a.turn < b.turn
		}
		if a.branch != b.branch {
			return a.branch < b.branch
		}
		return a.status < b.status
	})
	var sb strings.Builder
	fmt.Fprintf(&sb, "%-16s %-5s %-8s %-9s %s\n", "model", "turn", "branch", "status", "calls")
	byTurn := map[string]int{}
	total := 0
	for _, kk := range keys {
		br := kk.branch
		if br == "" {
			br = "-"
		}
		fmt.Fprintf(&sb, "%-16s %-5d %-8s %-9s %d\n", kk.model, kk.turn, br, kk.status, counts[kk])
		name := "turn " + strconv.Itoa(kk.turn)
		if kk.branch != "" {
			name += " " + kk.branch
		}
		byTurn[name] += counts[kk]
		total += counts[kk]
	}
	names := make([]string, 0, len(byTurn))
	for n := range byTurn {
		names = append(names, n)
	}
	sort.Strings(names)
	sb.WriteString("\ncalls by turn (all models)\n")
	for _, n := range names {
		fmt.Fprintf(&sb, "%-16s %d\n", n, byTurn[n])
	}
	fmt.Fprintf(&sb, "%-16s %d\n", "total", total)
	return sb.String()
}
