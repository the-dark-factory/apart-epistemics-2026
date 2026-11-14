package main

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

// Fake modes for the fake provider.
const (
	FakeAnswer    = "answer"
	FakeAsk       = "ask"
	FakeCannot    = "cannot"
	FakeGarbage   = "garbage"
	FakeTruncated = "truncated"
	FakeBadConf   = "badconf"
	FakeMixed     = "mixed"
)

// FakeProvider is a deterministic stand-in for a model. It makes no network
// call and spends nothing. Replies depend only on the conversation, so two
// runs give the same log apart from times.
type FakeProvider struct {
	// Mode picks the behaviour; empty means FakeAnswer.
	Mode string
	// FailFirst makes the first n calls fail with a transient error.
	FailFirst int
	// Hook, if set, is called first with the 1-based call number and may
	// return a reply or an error to use instead of the scripted one.
	Hook func(call int, msgs []Message) (*Reply, error)

	mu       sync.Mutex
	calls    int
	requests [][]Message
}

func newFakeFromConfig(m ModelConfig) (*FakeProvider, error) {
	switch m.FakeMode {
	case "", FakeAnswer, FakeAsk, FakeCannot, FakeGarbage, FakeTruncated, FakeBadConf, FakeMixed:
	default:
		return nil, fmt.Errorf("model %q: unknown fake_mode %q", m.Name, m.FakeMode)
	}
	return &FakeProvider{Mode: m.FakeMode, FailFirst: m.FakeFailFirst}, nil
}

// Channel reports the fake's channel.
func (p *FakeProvider) Channel() string { return "fake-messages" }

// Requests returns a copy of every conversation sent so far.
func (p *FakeProvider) Requests() [][]Message {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([][]Message, len(p.requests))
	copy(out, p.requests)
	return out
}

// Calls returns how many calls have been made.
func (p *FakeProvider) Calls() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls
}

// Complete returns the scripted reply for the conversation.
func (p *FakeProvider) Complete(ctx context.Context, msgs []Message) (Reply, error) {
	if err := ctx.Err(); err != nil {
		return Reply{}, err
	}
	p.mu.Lock()
	p.calls++
	n := p.calls
	cp := make([]Message, len(msgs))
	copy(cp, msgs)
	p.requests = append(p.requests, cp)
	p.mu.Unlock()

	if n <= p.FailFirst {
		return Reply{}, &TransientError{Err: fmt.Errorf("fake transient failure %d", n)}
	}
	if p.Hook != nil {
		r, err := p.Hook(n, msgs)
		if err != nil {
			return Reply{}, err
		}
		if r != nil {
			return withFakeUsage(*r, msgs), nil
		}
	}
	return withFakeUsage(p.script(msgs), msgs), nil
}

func withFakeUsage(r Reply, msgs []Message) Reply {
	var in int
	for _, m := range msgs {
		in += len(m.Content)
	}
	r.Usage = Usage{InputTokens: int64(in / 4), OutputTokens: int64(len(r.Text) / 4)}
	if r.ModelReturned == "" {
		r.ModelReturned = "fake-model"
	}
	if r.StopReason == "" {
		r.StopReason = "stop"
	}
	return r
}

// script gives the scripted reply by position in the conversation: one
// message is turn 1, three is turn 2, five is a pushback turn.
func (p *FakeProvider) script(msgs []Message) Reply {
	seed := msgs[0].Content
	mode := p.Mode
	if mode == "" {
		mode = FakeAnswer
	}
	if mode == FakeMixed {
		switch hashMod(seed, 7) {
		case 0:
			mode = FakeAsk
		case 1:
			mode = FakeCannot
		case 2:
			mode = FakeGarbage
		case 3:
			mode = FakeTruncated
		case 4:
			mode = FakeBadConf
		default:
			mode = FakeAnswer
		}
	}
	switch len(msgs) {
	case 1:
		switch mode {
		case FakeAsk:
			return Reply{Text: "ASK: filing_status"}
		case FakeCannot:
			return Reply{Text: "CANNOT: the inputs do not cover this return"}
		case FakeGarbage:
			return Reply{Text: "I am not sure how to approach this."}
		case FakeTruncated:
			full := fakeReturn(seed)
			return Reply{Text: full[:len(full)/2], StopReason: "length", Truncated: true}
		}
		return Reply{Text: fakeReturn(seed)}
	case 3:
		if mode == FakeBadConf {
			return Reply{Text: "I would say about seventy percent."}
		}
		return Reply{Text: "72\n65"}
	default:
		return Reply{Text: fakeReturn(seed+"|"+msgs[len(msgs)-1].Content) + "\n70\n60"}
	}
}

// hashMod returns a deterministic number in [0, n) from s.
func hashMod(s string, n int) int {
	h := sha256Hex([]byte(s))
	v := 0
	for _, c := range h[:8] {
		v = v*16 + int(strings.IndexRune("0123456789abcdef", c))
	}
	return v % n
}

// fakeReturn builds a parseable return whose amounts come from seed. Amounts
// are below 10000.
func fakeReturn(seed string) string {
	var b strings.Builder
	b.WriteString("Form 1040: U.S. Individual Income Tax Return\n")
	b.WriteString("===========================================\n")
	for i, l := range scoredLabels {
		amt := hashMod(fmt.Sprintf("%s#%d", seed, i), 10000)
		fmt.Fprintf(&b, "%s | fake | %d\n", l, amt)
	}
	return b.String()
}
