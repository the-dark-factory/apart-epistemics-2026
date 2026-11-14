package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
)

// Provider kinds.
const (
	KindOpenAI         = "openai"
	KindClaudeHeadless = "claude-headless"
	KindFake           = "fake"
)

// Message is one turn of a conversation. Role is "user" or "assistant".
type Message struct {
	// Role is "user" or "assistant".
	Role string `json:"role"`
	// Content is the message text.
	Content string `json:"content"`
	// Reasoning, when non-nil, is sent as reasoning_content on this assistant
	// message (echo_reasoning models only). Nil leaves the field out of the request.
	Reasoning *string `json:"reasoning_content,omitempty"`
}

// Usage is the token count a provider reported.
type Usage struct {
	// InputTokens is the prompt size.
	InputTokens int64 `json:"input_tokens"`
	// OutputTokens is the reply size.
	OutputTokens int64 `json:"output_tokens"`
}

// Reply is one provider answer.
type Reply struct {
	// Text is the reply text, raw.
	Text string
	// StopReason is the provider's own stop reason, if given.
	StopReason string
	// Truncated is true when the provider says it stopped for length.
	Truncated bool
	// ModelReturned is the model name the provider reported, if any.
	ModelReturned string
	// Usage is the reported token usage.
	Usage Usage
	// CostUSD is the provider's own cost figure (claude's total_cost_usd); nil when none.
	CostUSD *float64
	// Reasoning is the reasoning_content the API returned, raw; empty when none.
	// It is logged and echoed back, never parsed or scored.
	Reasoning string
}

// Provider sends a conversation and returns the next assistant turn.
type Provider interface {
	// Channel names how earlier turns reach the model, for the log.
	Channel() string
	// Complete sends msgs (alternating user and assistant, ending with user).
	Complete(ctx context.Context, msgs []Message) (Reply, error)
}

// TransientError marks an error worth retrying (rate limit, server error, network).
type TransientError struct {
	// Err is the underlying error.
	Err error
}

// Error implements error.
func (e *TransientError) Error() string { return "transient: " + e.Err.Error() }

// Unwrap returns the underlying error.
func (e *TransientError) Unwrap() error { return e.Err }

// isTransient reports whether err is, or wraps, a TransientError.
func isTransient(err error) bool {
	var t *TransientError
	return errors.As(err, &t)
}

// ModelConfig is one entry of models.json.
type ModelConfig struct {
	// Name is the label used in the log and for -only-style selection.
	Name string `json:"name"`
	// Kind is openai, claude-headless or fake.
	Kind string `json:"kind"`
	// BaseURL is the chat-completions base URL (openai kind).
	BaseURL string `json:"base_url,omitempty"`
	// Model is the model id sent to the provider.
	Model string `json:"model"`
	// KeyEnv names the environment variable holding the API key. The key itself is never stored or logged.
	KeyEnv string `json:"key_env,omitempty"`
	// PriceIn is dollars per million input tokens.
	PriceIn float64 `json:"price_in"`
	// PriceOut is dollars per million output tokens.
	PriceOut float64 `json:"price_out"`
	// Concurrency is how many conversations run at once for this model (minimum 1).
	Concurrency int `json:"concurrency"`
	// MaxOutputTokens, if set, is sent as max_tokens (openai kind) and used in the budget estimate; 0 sends nothing.
	MaxOutputTokens int `json:"max_output_tokens,omitempty"`
	// Command is the claude binary for claude-headless (default "claude").
	Command string `json:"command,omitempty"`
	// FakeMode picks the scripted behaviour of the fake kind.
	FakeMode string `json:"fake_mode,omitempty"`
	// FakeFailFirst makes the fake kind fail its first n calls with a transient error.
	FakeFailFirst int `json:"fake_fail_first,omitempty"`
	// EchoReasoning, openai kind only, sends each assistant turn back with the
	// reasoning_content the API returned for it. Default false.
	EchoReasoning bool `json:"echo_reasoning,omitempty"`
}

// loadModels reads and validates a models.json (an array of entries).
func loadModels(path string) ([]ModelConfig, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read models: %w", err)
	}
	var ms []ModelConfig
	if err := json.Unmarshal(raw, &ms); err != nil {
		return nil, fmt.Errorf("parse models: %w", err)
	}
	if len(ms) == 0 {
		return nil, errors.New("models file has no entries")
	}
	seen := map[string]bool{}
	for i := range ms {
		m := &ms[i]
		if m.Name == "" {
			return nil, fmt.Errorf("models entry %d: empty name", i)
		}
		if seen[m.Name] {
			return nil, fmt.Errorf("models entry %q: duplicate name", m.Name)
		}
		seen[m.Name] = true
		if m.Concurrency < 1 {
			m.Concurrency = 1
		}
		if m.EchoReasoning && m.Kind != KindOpenAI {
			return nil, fmt.Errorf("models entry %q: echo_reasoning is valid for the openai kind only", m.Name)
		}
		switch m.Kind {
		case KindOpenAI:
			if m.BaseURL == "" || m.Model == "" || m.KeyEnv == "" {
				return nil, fmt.Errorf("models entry %q: openai kind needs base_url, model and key_env", m.Name)
			}
			if strings.Contains(m.BaseURL, "PLACEHOLDER") || strings.Contains(m.KeyEnv, "PLACEHOLDER") {
				return nil, fmt.Errorf("models entry %q: still has placeholder values", m.Name)
			}
			if m.PriceIn <= 0 || m.PriceOut <= 0 {
				return nil, fmt.Errorf("models entry %q: priced run needs price_in and price_out", m.Name)
			}
		case KindClaudeHeadless:
			if m.Model == "" {
				return nil, fmt.Errorf("models entry %q: claude-headless kind needs model", m.Name)
			}
			if strings.Contains(m.Model, "PLACEHOLDER") {
				return nil, fmt.Errorf("models entry %q: still has placeholder values", m.Name)
			}
		case KindFake:
		default:
			return nil, fmt.Errorf("models entry %q: unknown kind %q", m.Name, m.Kind)
		}
	}
	return ms, nil
}

// newProvider builds the provider for a config entry.
func newProvider(m ModelConfig) (Provider, error) {
	switch m.Kind {
	case KindOpenAI:
		key := os.Getenv(m.KeyEnv)
		if key == "" {
			return nil, fmt.Errorf("model %q: environment variable %s is empty", m.Name, m.KeyEnv)
		}
		return newOpenAI(m, key), nil
	case KindClaudeHeadless:
		return newClaudeHeadless(m), nil
	case KindFake:
		return newFakeFromConfig(m)
	}
	return nil, fmt.Errorf("model %q: unknown kind %q", m.Name, m.Kind)
}

// redact removes secret from s so a key can never reach the log.
func redact(s, secret string) string {
	if secret == "" {
		return s
	}
	return strings.ReplaceAll(s, secret, "[REDACTED]")
}
