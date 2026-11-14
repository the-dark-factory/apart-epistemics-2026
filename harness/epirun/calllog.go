package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
)

// Turn names for the log.
const (
	TurnReturn     = 1
	TurnConfidence = 2
	TurnPushback   = 3
)

// Pushback branches. Turns 1 and 2 have an empty branch.
const (
	BranchNeutral = "neutral"
	BranchAnchor  = "anchor"
)

// StatusConf is the status of a turn 2 reply whose two numbers parsed.
const StatusConf = "CONF"

// CallKey identifies one call. A re-run skips keys already logged complete.
type CallKey struct {
	Model  string
	Item   string
	Sample int
	Turn   int
	Branch string
}

// LogRow is one line of calls.jsonl.
type LogRow struct {
	// TimeUTC is when the call finished.
	TimeUTC string `json:"time_utc"`
	// Model is the model asked for (the models.json name).
	Model string `json:"model"`
	// ModelReturned is the model name the provider gave back, if any.
	ModelReturned string `json:"model_returned,omitempty"`
	// Item is the item id.
	Item string `json:"item"`
	// Set is the item's set.
	Set string `json:"set"`
	// Sample is 1-based.
	Sample int `json:"sample"`
	// Turn is 1 (return), 2 (confidence) or 3 (pushback).
	Turn int `json:"turn"`
	// Branch is neutral or anchor on turn 3, else empty.
	Branch string `json:"branch,omitempty"`
	// TemplateSHA256 is the sha256 of the turn's prompt template.
	TemplateSHA256 string `json:"template_sha256"`
	// RequestSHA256 is the sha256 of the full request (model and all messages).
	RequestSHA256 string `json:"request_sha256"`
	// Channel says how earlier turns reached the model.
	Channel string `json:"channel"`
	// Reply is the raw reply text.
	Reply string `json:"reply"`
	// Reasoning is the raw reasoning_content the API returned (echo_reasoning models
	// only); omitted when empty. Resume sends it back with this turn. Never scored.
	Reasoning string `json:"reasoning,omitempty"`
	// StopReason is the provider's stop reason.
	StopReason string `json:"stop_reason,omitempty"`
	// Status is ANSWER, ASK, CANNOT or UNPARSED (turns 1 and 3); CONF or UNPARSED (turn 2).
	Status string `json:"status"`
	// Flags lists notes such as truncated or conf_unparsed.
	Flags []string `json:"flags,omitempty"`
	// Conf is the parsed confidence pair (turn 2, and turn 3 answers); null when not parsed.
	Conf *Confidence `json:"conf"`
	// Lines holds the parsed scored line values of an ANSWER, keyed "Line 24" and so on.
	Lines map[string]float64 `json:"lines,omitempty"`
	// AnchorX and AnchorNoun are the wrong figure and word put in the anchor branch.
	AnchorX    *int64 `json:"anchor_x,omitempty"`
	AnchorNoun string `json:"anchor_noun,omitempty"`
	// AnchorOwn is the model's own turn 1 figure the anchor was built from.
	// Resume reads AnchorX and AnchorNoun back, so the request is rebuilt
	// identically even if the parser later changes.
	AnchorOwn *float64 `json:"anchor_own,omitempty"`
	// Usage is the reported token usage.
	Usage Usage `json:"usage"`
	// CostUSD is the cost charged to the budget for this call.
	CostUSD float64 `json:"cost_usd"`
	// LatencyMS is the time of the last attempt.
	LatencyMS int64 `json:"latency_ms"`
	// Attempts is how many tries the call took.
	Attempts int `json:"attempts"`
	// Error is empty for a complete call; a call with an error is failed and is retried on resume.
	Error string `json:"error,omitempty"`
}

// Key returns the row's call key.
func (r LogRow) Key() CallKey {
	return CallKey{Model: r.Model, Item: r.Item, Sample: r.Sample, Turn: r.Turn, Branch: r.Branch}
}

// callLog is the append-only calls.jsonl writer.
type callLog struct {
	mu sync.Mutex
	f  *os.File
}

// openCallLog opens path for appending, creating it if needed. If the file
// ends in a torn line (no final newline, e.g. after a kill), a newline is
// written first so the next row starts on its own line. Nothing is rewritten.
func openCallLog(path string) (*callLog, error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open call log: %w", err)
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("stat call log: %w", err)
	}
	if st.Size() > 0 {
		last := make([]byte, 1)
		if _, err := f.ReadAt(last, st.Size()-1); err != nil {
			f.Close()
			return nil, fmt.Errorf("read call log tail: %w", err)
		}
		if last[0] != '\n' {
			if _, err := f.Write([]byte("\n")); err != nil {
				f.Close()
				return nil, fmt.Errorf("repair call log tail: %w", err)
			}
		}
	}
	return &callLog{f: f}, nil
}

// Append writes one row as one line and syncs it to disk.
func (l *callLog) Append(r LogRow) error {
	line, err := json.Marshal(r)
	if err != nil {
		return fmt.Errorf("encode log row: %w", err)
	}
	line = append(line, '\n')
	l.mu.Lock()
	defer l.mu.Unlock()
	if _, err := l.f.Write(line); err != nil {
		return fmt.Errorf("write log row: %w", err)
	}
	if err := l.f.Sync(); err != nil {
		return fmt.Errorf("sync call log: %w", err)
	}
	return nil
}

// Close closes the log file.
func (l *callLog) Close() error { return l.f.Close() }

// readCallLog reads every row of a calls.jsonl. A line that is not valid JSON
// (a torn write left by a kill) is skipped and counted, never rewritten. A
// missing file gives no rows.
func readCallLog(path string) (rows []LogRow, torn int, err error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, 0, nil
	}
	if err != nil {
		return nil, 0, fmt.Errorf("read call log: %w", err)
	}
	rd := bufio.NewReaderSize(bytes.NewReader(data), 1<<20)
	n := 0
	for {
		line, rerr := rd.ReadBytes('\n')
		if len(line) > 0 {
			n++
			trimmed := bytes.TrimSpace(line)
			if len(trimmed) > 0 {
				var r LogRow
				if jerr := json.Unmarshal(trimmed, &r); jerr != nil {
					// A kill can leave half a line; opening the log for append
					// later ends it with a newline, so it may no longer be last.
					torn++
				} else {
					rows = append(rows, r)
				}
			}
		}
		if rerr != nil {
			if errors.Is(rerr, io.EOF) {
				break
			}
			return nil, 0, fmt.Errorf("read call log: %w", rerr)
		}
	}
	return rows, torn, nil
}
