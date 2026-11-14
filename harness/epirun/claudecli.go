package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// claudeHeadless runs `claude -p` as a model route. The route has no way to
// supply assistant turns, so earlier turns go as a plain transcript inside
// the one prompt.
type claudeHeadless struct {
	cfg ModelConfig
}

func newClaudeHeadless(cfg ModelConfig) *claudeHeadless { return &claudeHeadless{cfg: cfg} }

// Channel reports that earlier turns are flattened into one prompt.
func (p *claudeHeadless) Channel() string { return "claude-headless-transcript" }

// claudeArgs are the arguments for one headless call.
func claudeArgs(model string) []string {
	return []string{
		"-p", "--model", model,
		"--tools", "",
		"--setting-sources", "",
		"--strict-mcp-config",
		"--disable-slash-commands",
		"--no-session-persistence",
		"--system-prompt", "You are a helpful assistant.",
		"--output-format", "json",
		"--max-budget-usd", "1",
	}
}

// transcriptPrompt flattens a conversation into one prompt. A single user
// message goes as is.
func transcriptPrompt(msgs []Message) string {
	if len(msgs) == 1 {
		return msgs[0].Content
	}
	var b strings.Builder
	for i, m := range msgs {
		if i > 0 {
			b.WriteString("\n\n")
		}
		if m.Role == "assistant" {
			b.WriteString("[Assistant]\n")
		} else {
			b.WriteString("[User]\n")
		}
		b.WriteString(m.Content)
	}
	return b.String()
}

type claudeJSON struct {
	Type         string   `json:"type"`
	Subtype      string   `json:"subtype"`
	IsError      bool     `json:"is_error"`
	Result       string   `json:"result"`
	StopReason   string   `json:"stop_reason"`
	TotalCostUSD *float64 `json:"total_cost_usd"`
	Usage        struct {
		InputTokens              int64 `json:"input_tokens"`
		OutputTokens             int64 `json:"output_tokens"`
		CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
		CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
	} `json:"usage"`
	ModelUsage map[string]json.RawMessage `json:"modelUsage"`
}

// Complete runs claude once from a fresh empty temp directory outside any repo.
func (p *claudeHeadless) Complete(ctx context.Context, msgs []Message) (Reply, error) {
	if len(msgs) == 0 {
		return Reply{}, errors.New("no messages")
	}
	dir, err := os.MkdirTemp("", "w")
	if err != nil {
		return Reply{}, fmt.Errorf("make temp dir: %w", err)
	}
	defer os.RemoveAll(dir)
	cmdName := p.cfg.Command
	if cmdName == "" {
		cmdName = "claude"
	}
	cctx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()
	cmd := exec.CommandContext(cctx, cmdName, claudeArgs(p.cfg.Model)...)
	cmd.Dir = dir
	cmd.Stdin = strings.NewReader(transcriptPrompt(msgs))
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	runErr := cmd.Run()
	if ctx.Err() != nil {
		return Reply{}, ctx.Err()
	}
	var out claudeJSON
	parsed := json.Unmarshal(stdout.Bytes(), &out) == nil
	if runErr != nil && !parsed {
		msg := strings.TrimSpace(stderr.String())
		if len(msg) > 500 {
			msg = msg[:500]
		}
		return Reply{}, &TransientError{Err: fmt.Errorf("claude: %v: %s", runErr, msg)}
	}
	if !parsed {
		return Reply{}, fmt.Errorf("claude: output is not JSON")
	}
	if out.IsError || runErr != nil {
		e := fmt.Errorf("claude: error result (subtype %q): %s", out.Subtype, truncateString(out.Result, 300))
		if out.Subtype == "error_max_budget_usd" {
			return Reply{}, e
		}
		return Reply{}, &TransientError{Err: e}
	}
	r := Reply{
		Text:       out.Result,
		StopReason: out.StopReason,
		Truncated:  out.StopReason == "max_tokens",
		CostUSD:    out.TotalCostUSD,
		Usage: Usage{
			InputTokens:  out.Usage.InputTokens + out.Usage.CacheCreationInputTokens + out.Usage.CacheReadInputTokens,
			OutputTokens: out.Usage.OutputTokens,
		},
	}
	for name := range out.ModelUsage {
		if r.ModelReturned == "" || name < r.ModelReturned {
			r.ModelReturned = name
		}
	}
	return r, nil
}

func truncateString(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
