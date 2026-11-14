package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenAIProvider(t *testing.T) {
	const key = "sk-test-SECRET-123"
	var gotBody map[string]any
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Error(err)
		}
		w.Write([]byte(`{"model":"m-2026","choices":[{"message":{"content":"hello"},"finish_reason":"length"}],"usage":{"prompt_tokens":11,"completion_tokens":7}}`))
	}))
	defer srv.Close()
	p := newOpenAI(ModelConfig{Name: "o", Kind: KindOpenAI, BaseURL: srv.URL + "/v1", Model: "m"}, key)
	msgs := []Message{{Role: "user", Content: "q1"}, {Role: "assistant", Content: "a1"}, {Role: "user", Content: "q2"}}
	r, err := p.Complete(context.Background(), msgs)
	if err != nil {
		t.Fatal(err)
	}
	if r.Text != "hello" || !r.Truncated || r.StopReason != "length" || r.ModelReturned != "m-2026" ||
		r.Usage.InputTokens != 11 || r.Usage.OutputTokens != 7 {
		t.Errorf("reply = %+v", r)
	}
	if gotAuth != "Bearer "+key {
		t.Error("key not sent as bearer token")
	}
	if _, has := gotBody["temperature"]; has {
		t.Error("a temperature was sent")
	}
	if _, has := gotBody["max_tokens"]; has {
		t.Error("max_tokens sent though not configured")
	}
	if ms, _ := gotBody["messages"].([]any); len(ms) != 3 {
		t.Errorf("messages = %v, want a real 3-message array", gotBody["messages"])
	}
}

func TestOpenAIErrors(t *testing.T) {
	const key = "sk-test-SECRET-123"
	tests := []struct {
		status    int
		transient bool
	}{{429, true}, {500, true}, {503, true}, {400, false}, {401, false}}
	for _, tc := range tests {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(tc.status)
			w.Write([]byte("bad key " + r.Header.Get("Authorization")))
		}))
		p := newOpenAI(ModelConfig{BaseURL: srv.URL, Model: "m"}, key)
		_, err := p.Complete(context.Background(), []Message{{Role: "user", Content: "q"}})
		srv.Close()
		if err == nil {
			t.Fatalf("status %d: want error", tc.status)
		}
		if isTransient(err) != tc.transient {
			t.Errorf("status %d: transient=%v want %v", tc.status, isTransient(err), tc.transient)
		}
		if strings.Contains(err.Error(), key) {
			t.Errorf("status %d: error message leaks the key: %v", tc.status, err)
		}
	}
}

func TestOpenAIMaxTokensWhenConfigured(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&body)
		w.Write([]byte(`{"choices":[{"message":{"content":"x"},"finish_reason":"stop"}]}`))
	}))
	defer srv.Close()
	p := newOpenAI(ModelConfig{BaseURL: srv.URL, Model: "m", MaxOutputTokens: 4000}, "k")
	if _, err := p.Complete(context.Background(), []Message{{Role: "user", Content: "q"}}); err != nil {
		t.Fatal(err)
	}
	if body["max_tokens"] != float64(4000) {
		t.Errorf("max_tokens = %v", body["max_tokens"])
	}
}

func TestClaudeArgsAndTranscript(t *testing.T) {
	got := strings.Join(claudeArgs("opus-x"), " ")
	for _, want := range []string{
		"-p", "--model opus-x", "--tools ", "--setting-sources ", "--strict-mcp-config", "--disable-slash-commands",
		"--no-session-persistence", "--system-prompt You are a helpful assistant.", "--output-format json", "--max-budget-usd 1",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("args lack %q: %s", want, got)
		}
	}
	if s := transcriptPrompt([]Message{{Role: "user", Content: "only"}}); s != "only" {
		t.Errorf("single message = %q", s)
	}
	s := transcriptPrompt([]Message{{Role: "user", Content: "q1"}, {Role: "assistant", Content: "a1"}, {Role: "user", Content: "q2"}})
	if s != "[User]\nq1\n\n[Assistant]\na1\n\n[User]\nq2" {
		t.Errorf("transcript = %q", s)
	}
}

// TestClaudeHeadlessWithStub runs the exec path against a stub script that
// stands in for the claude binary. No model is involved.
func TestClaudeHeadlessWithStub(t *testing.T) {
	dir := t.TempDir()
	rec := filepath.Join(dir, "record.txt")
	stub := filepath.Join(dir, "stubclaude")
	script := "#!/bin/sh\n{ pwd; echo \"$@\"; cat; } > \"$STUB_RECORD\"\n" +
		`echo '{"type":"result","subtype":"success","is_error":false,"result":"stub reply","stop_reason":"end_turn","total_cost_usd":0.0123,"usage":{"input_tokens":5,"output_tokens":2},"modelUsage":{"stub-model":{}}}'` + "\n"
	if err := os.WriteFile(stub, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("STUB_RECORD", rec)
	p := newClaudeHeadless(ModelConfig{Name: "c", Kind: KindClaudeHeadless, Model: "m1", Command: stub})
	r, err := p.Complete(context.Background(), []Message{{Role: "user", Content: "q1"}, {Role: "assistant", Content: "a1"}, {Role: "user", Content: "q2"}})
	if err != nil {
		t.Fatal(err)
	}
	if r.Text != "stub reply" || r.CostUSD == nil || *r.CostUSD != 0.0123 || r.ModelReturned != "stub-model" || r.Truncated {
		t.Errorf("reply = %+v", r)
	}
	if p.Channel() != "claude-headless-transcript" {
		t.Errorf("channel = %s", p.Channel())
	}
	b, err := os.ReadFile(rec)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.SplitN(string(b), "\n", 3)
	cwd := lines[0]
	if filepath.Base(cwd) == "" || !strings.HasPrefix(filepath.Base(cwd), "w") || strings.Contains(cwd, "ObVault") || strings.Contains(cwd, "/dev/") {
		t.Errorf("working directory %q is not a neutral temp directory", cwd)
	}
	if _, err := os.Stat(cwd); err == nil {
		t.Error("temp directory was not removed")
	}
	if !strings.Contains(lines[1], "--model m1") || !strings.HasSuffix(lines[2], "[User]\nq2\n") && !strings.Contains(lines[2], "[Assistant]\na1") {
		t.Errorf("recorded call: %q", string(b))
	}
}

func TestLoadModels(t *testing.T) {
	dir := t.TempDir()
	write := func(s string) string {
		p := filepath.Join(dir, "m.json")
		if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	bad := map[string]string{
		"placeholder url": `[{"name":"a","kind":"openai","base_url":"https://PLACEHOLDER/v1","model":"m","key_env":"K","price_in":1,"price_out":1}]`,
		"no price":        `[{"name":"a","kind":"openai","base_url":"https://x/v1","model":"m","key_env":"K"}]`,
		"unknown kind":    `[{"name":"a","kind":"carrier-pigeon"}]`,
		"duplicate":       `[{"name":"a","kind":"fake"},{"name":"a","kind":"fake"}]`,
		"empty":           `[]`,
	}
	for name, js := range bad {
		if _, err := loadModels(write(js)); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
	ms, err := loadModels(write(`[{"name":"a","kind":"fake"},{"name":"b","kind":"openai","base_url":"https://x/v1","model":"m","key_env":"K","price_in":1,"price_out":2}]`))
	if err != nil {
		t.Fatal(err)
	}
	if ms[0].Concurrency != 1 {
		t.Errorf("default concurrency = %d", ms[0].Concurrency)
	}
	// The shipped example is placeholders only and must not be runnable as it stands.
	if _, err := loadModels("models.example.json"); err == nil {
		t.Error("models.example.json should be refused until the seat fills it")
	}
}

func TestOpenAIMissingKeyEnv(t *testing.T) {
	t.Setenv("EPIRUN_TEST_KEY_UNSET", "")
	_, err := newProvider(ModelConfig{Name: "o", Kind: KindOpenAI, BaseURL: "https://x/v1", Model: "m", KeyEnv: "EPIRUN_TEST_KEY_UNSET"})
	if err == nil {
		t.Fatal("want an error for an empty key variable")
	}
	if strings.Contains(err.Error(), "sk-") {
		t.Error("error mentions a key")
	}
}
