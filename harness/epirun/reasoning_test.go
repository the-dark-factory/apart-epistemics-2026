package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// reasoningServer is a fake chat-completions endpoint. It always returns a
// reasoning_content of "R<n>" for the n-th message of the conversation
// (unless omitReasoning), and records every raw request body.
type reasoningServer struct {
	srv            *httptest.Server
	mu             sync.Mutex
	bodies         [][]byte
	omitReasoning  bool // never return reasoning_content
	failPushbackTo bool // answer 400 to the 5-message pushback requests
}

func newReasoningServer(t *testing.T) *reasoningServer {
	t.Helper()
	rs := &reasoningServer{}
	rs.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		var req struct {
			Messages []Message `json:"messages"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			t.Error(err)
		}
		rs.mu.Lock()
		rs.bodies = append(rs.bodies, body)
		omit, fail := rs.omitReasoning, rs.failPushbackTo
		rs.mu.Unlock()
		if fail && len(req.Messages) == 5 {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("Missing required parameter: 'messages[1].reasoning_content'"))
			return
		}
		text := (&FakeProvider{}).script(req.Messages).Text
		msg := map[string]any{"content": text}
		if !omit {
			msg["reasoning_content"] = "R" + strconv.Itoa(len(req.Messages))
		}
		out, err := json.Marshal(map[string]any{
			"model":   "m",
			"choices": []any{map[string]any{"message": msg, "finish_reason": "stop"}},
			"usage":   map[string]any{"prompt_tokens": 10, "completion_tokens": 5},
		})
		if err != nil {
			t.Error(err)
		}
		w.Write(out)
	}))
	t.Cleanup(rs.srv.Close)
	return rs
}

// rawMessages decodes the messages array of one request body into per-message field maps.
func rawMessages(t *testing.T, body []byte) []map[string]json.RawMessage {
	t.Helper()
	var req struct {
		Messages []map[string]json.RawMessage `json:"messages"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		t.Fatal(err)
	}
	return req.Messages
}

func (rs *reasoningServer) model(echo bool) ModelConfig {
	return ModelConfig{Name: "oa", Kind: KindOpenAI, BaseURL: rs.srv.URL, Model: "m", KeyEnv: "K",
		PriceIn: 1, PriceOut: 1, Concurrency: 1, EchoReasoning: echo}
}

func (rs *reasoningServer) run(t *testing.T, echo bool, out string, items []Item) (RunResult, error) {
	t.Helper()
	m := rs.model(echo)
	return Run(context.Background(), RunConfig{
		Items: items, Models: []ModelConfig{m}, Providers: map[string]Provider{m.Name: newOpenAI(m, "k")},
		OutDir: out, Samples: 1, BudgetUSD: 100,
		Backoff: func(int) time.Duration { return 0 },
	})
}

func reasoningOf(t *testing.T, m map[string]json.RawMessage) (string, bool) {
	t.Helper()
	raw, ok := m["reasoning_content"]
	if !ok {
		return "", false
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		t.Fatal(err)
	}
	return s, true
}

func TestEchoOffRequestsUnchanged(t *testing.T) {
	rs := newReasoningServer(t) // the API returns reasoning, but echo is off
	dir := t.TempDir()
	items := testItems(t, dir)[:1]
	out := filepath.Join(dir, "out")
	if _, err := rs.run(t, false, out, items); err != nil {
		t.Fatal(err)
	}
	if len(rs.bodies) != 4 {
		t.Fatalf("requests = %d, want 4", len(rs.bodies))
	}
	for i, body := range rs.bodies {
		if bytes.Contains(body, []byte("reasoning")) {
			t.Errorf("request %d mentions reasoning: %s", i, body)
		}
		// The old wire shape: model and plain role/content messages only.
		var req struct {
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			t.Fatal(err)
		}
		want, err := json.Marshal(struct {
			Model    string `json:"model"`
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}{"m", req.Messages})
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(body, want) {
			t.Errorf("request %d bytes differ from the old shape:\n got %s\nwant %s", i, body, want)
		}
	}
	for _, r := range readRows(t, out) {
		if r.Reasoning != "" {
			t.Errorf("echo off logged reasoning on %d/%s", r.Turn, r.Branch)
		}
	}
	raw, err := os.ReadFile(callsPath(out))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte(`"reasoning"`)) {
		t.Error("echo off wrote a reasoning field to the log")
	}
}

func TestEchoOnSendsReasoning(t *testing.T) {
	rs := newReasoningServer(t)
	dir := t.TempDir()
	items := testItems(t, dir)[:1]
	out := filepath.Join(dir, "out")
	if _, err := rs.run(t, true, out, items); err != nil {
		t.Fatal(err)
	}
	if len(rs.bodies) != 4 {
		t.Fatalf("requests = %d, want 4 (turn 1, turn 2, two pushbacks)", len(rs.bodies))
	}
	tests := []struct {
		name string
		req  int
		// want maps message index to expected reasoning; other messages must have none.
		want map[int]string
	}{
		{"turn 1", 0, map[int]string{}},
		{"turn 2", 1, map[int]string{1: "R1"}},
		{"turn 3 neutral", 2, map[int]string{1: "R1", 3: "R3"}},
		{"turn 3 anchor", 3, map[int]string{1: "R1", 3: "R3"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			msgs := rawMessages(t, rs.bodies[tc.req])
			for i, m := range msgs {
				got, has := reasoningOf(t, m)
				want, wantHas := tc.want[i]
				if has != wantHas || got != want {
					t.Errorf("message %d reasoning = %q (present %v), want %q (present %v)", i, got, has, want, wantHas)
				}
			}
		})
	}
	// Both pushback branches carry byte-identical earlier turns.
	a, b := rawMessages(t, rs.bodies[2]), rawMessages(t, rs.bodies[3])
	for i := 0; i < 4; i++ {
		ja, err := json.Marshal(a[i])
		if err != nil {
			t.Fatal(err)
		}
		jb, err := json.Marshal(b[i])
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(ja, jb) {
			t.Errorf("message %d differs between branches", i)
		}
	}
	// The log holds the raw reasoning, and status comes from content only.
	for _, r := range readRows(t, out) {
		if r.Reasoning == "" {
			t.Errorf("turn %d/%s: reasoning not logged", r.Turn, r.Branch)
		}
		if r.Turn == TurnReturn && r.Status != StatusAnswer {
			t.Errorf("turn 1 status = %s", r.Status)
		}
	}
}

func TestEchoOnMissingReasoningIsEmptyString(t *testing.T) {
	rs := newReasoningServer(t)
	rs.omitReasoning = true
	dir := t.TempDir()
	items := testItems(t, dir)[:1]
	out := filepath.Join(dir, "out")
	if _, err := rs.run(t, true, out, items); err != nil {
		t.Fatal(err)
	}
	for _, idx := range []int{1, 2, 3} {
		msgs := rawMessages(t, rs.bodies[idx])
		got, has := reasoningOf(t, msgs[1])
		if !has || got != "" {
			t.Errorf("request %d: message 1 reasoning = %q (present %v), want present and empty", idx, got, has)
		}
	}
	raw, err := os.ReadFile(callsPath(out))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte(`"reasoning"`)) {
		t.Error("empty reasoning should be omitted from the log")
	}
}

func TestEchoResumeRebuildsSameRequest(t *testing.T) {
	dir := t.TempDir()
	items := testItems(t, dir)[:1]

	// A clean run is the reference.
	rsFull := newReasoningServer(t)
	full := filepath.Join(dir, "full")
	if _, err := rsFull.run(t, true, full, items); err != nil {
		t.Fatal(err)
	}
	want := map[CallKey]string{}
	for _, r := range readRows(t, full) {
		want[r.Key()] = r.RequestSHA256
	}

	// A partial run: turns 1 and 2 land, both pushbacks fail with http 400.
	rs := newReasoningServer(t)
	rs.failPushbackTo = true
	out := filepath.Join(dir, "out")
	if _, err := rs.run(t, true, out, items); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(callsPath(out))
	if err != nil {
		t.Fatal(err)
	}

	// Resume against a healthy server: turns 1 and 2 come from the log.
	rs2 := newReasoningServer(t)
	res, err := rs2.run(t, true, out, items)
	if err != nil {
		t.Fatal(err)
	}
	if res.Skipped != 2 || len(rs2.bodies) != 2 {
		t.Errorf("skipped=%d requests=%d, want 2 and 2", res.Skipped, len(rs2.bodies))
	}
	after, err := os.ReadFile(callsPath(out))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(after, before) {
		t.Error("log was rewritten, not appended to")
	}
	got := map[CallKey]string{}
	for _, r := range readRows(t, out) {
		if r.Error == "" {
			got[r.Key()] = r.RequestSHA256
		}
	}
	if len(got) != len(want) {
		t.Fatalf("%d complete keys, want %d", len(got), len(want))
	}
	for k, sha := range want {
		if got[k] != sha {
			t.Errorf("key %v: resumed request sha %s, clean run %s", k, got[k], sha)
		}
	}
	// And the bytes on the wire match the clean run's pushback requests.
	if !bytes.Equal(rs2.bodies[0], rsFull.bodies[2]) || !bytes.Equal(rs2.bodies[1], rsFull.bodies[3]) {
		t.Error("resumed pushback request bytes differ from the clean run")
	}
}

func TestEchoSHACoversReasoning(t *testing.T) {
	m := ModelConfig{Name: "n", Model: "m", EchoReasoning: true}
	mk := func(r string) []Message {
		return []Message{{Role: "user", Content: "q"}, assistantMsg(m, LogRow{Reply: "a", Reasoning: r}), {Role: "user", Content: "q2"}}
	}
	a, err := requestSHA(m, mk("x"))
	if err != nil {
		t.Fatal(err)
	}
	b, err := requestSHA(m, mk("y"))
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Error("request sha ignores the reasoning that is sent")
	}
}

func TestEchoReasoningLoad(t *testing.T) {
	const openai = `"kind":"openai","base_url":"https://x.example/v1","model":"m","key_env":"K","price_in":1,"price_out":1`
	tests := []struct {
		name    string
		entry   string
		wantErr string
		want    bool
	}{
		{"openai true", `{"name":"a",` + openai + `,"echo_reasoning":true}`, "", true},
		{"openai default", `{"name":"a",` + openai + `}`, "", false},
		{"openai false", `{"name":"a",` + openai + `,"echo_reasoning":false}`, "", false},
		{"claude-headless", `{"name":"a","kind":"claude-headless","model":"m","echo_reasoning":true}`, "echo_reasoning", false},
		{"fake", `{"name":"a","kind":"fake","echo_reasoning":true}`, "echo_reasoning", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := filepath.Join(t.TempDir(), "models.json")
			if err := os.WriteFile(p, []byte("["+tc.entry+"]"), 0o644); err != nil {
				t.Fatal(err)
			}
			ms, err := loadModels(p)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want one mentioning %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if ms[0].EchoReasoning != tc.want {
				t.Errorf("EchoReasoning = %v, want %v", ms[0].EchoReasoning, tc.want)
			}
		})
	}
}
