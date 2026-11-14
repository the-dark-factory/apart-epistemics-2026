package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// callTimeout bounds one provider call.
const callTimeout = 15 * time.Minute

// openAIProvider talks to an OpenAI chat-completions compatible endpoint with
// a real multi-turn messages array. It never sets a temperature.
type openAIProvider struct {
	cfg    ModelConfig
	key    string
	client *http.Client
}

func newOpenAI(cfg ModelConfig, key string) *openAIProvider {
	return &openAIProvider{cfg: cfg, key: key, client: &http.Client{Timeout: callTimeout}}
}

// Channel reports that earlier turns go as real assistant messages.
func (p *openAIProvider) Channel() string { return "openai-messages" }

type oaRequest struct {
	Model     string    `json:"model"`
	Messages  []Message `json:"messages"`
	MaxTokens int       `json:"max_tokens,omitempty"`
}

type oaResponse struct {
	Model   string `json:"model"`
	Choices []struct {
		Message struct {
			Content          string `json:"content"`
			ReasoningContent string `json:"reasoning_content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int64 `json:"prompt_tokens"`
		CompletionTokens int64 `json:"completion_tokens"`
	} `json:"usage"`
}

// Complete sends the conversation to the endpoint.
func (p *openAIProvider) Complete(ctx context.Context, msgs []Message) (Reply, error) {
	body, err := json.Marshal(oaRequest{Model: p.cfg.Model, Messages: msgs, MaxTokens: p.cfg.MaxOutputTokens})
	if err != nil {
		return Reply{}, fmt.Errorf("encode request: %w", err)
	}
	url := strings.TrimRight(p.cfg.BaseURL, "/") + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return Reply{}, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.key)
	resp, err := p.client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return Reply{}, ctx.Err()
		}
		return Reply{}, &TransientError{Err: errors.New(redact(err.Error(), p.key))}
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return Reply{}, &TransientError{Err: fmt.Errorf("read response: %s", redact(err.Error(), p.key))}
	}
	if resp.StatusCode != http.StatusOK {
		snippet := redact(string(data), p.key)
		if len(snippet) > 500 {
			snippet = snippet[:500]
		}
		e := fmt.Errorf("http %d: %s", resp.StatusCode, snippet)
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 || resp.StatusCode == http.StatusRequestTimeout {
			return Reply{}, &TransientError{Err: e}
		}
		return Reply{}, e
	}
	var out oaResponse
	if err := json.Unmarshal(data, &out); err != nil {
		return Reply{}, fmt.Errorf("decode response: %w", err)
	}
	if len(out.Choices) == 0 {
		return Reply{}, errors.New("response has no choices")
	}
	c := out.Choices[0]
	return Reply{
		Text:          c.Message.Content,
		Reasoning:     c.Message.ReasoningContent,
		StopReason:    c.FinishReason,
		Truncated:     c.FinishReason == "length",
		ModelReturned: out.Model,
		Usage:         Usage{InputTokens: out.Usage.PromptTokens, OutputTokens: out.Usage.CompletionTokens},
	}, nil
}
