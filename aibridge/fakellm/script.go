// Package fakellm is a spike for a fake LLM test double, in the spirit of
// provisioner/echo: instead of mocking an interface or hand-crafting
// provider-specific wire payloads per test, a test author writes a small,
// fully deterministic script describing exactly what the model says and
// exactly what every tool call "returns" — no template engine, no
// randomness, no wall-clock delays.
//
// A script is newline-delimited JSON (one step per line):
//
//	{"text": "let me check that"}
//	{"think": "I need to check if the file exists. I'll use the execute tool"}
//	{"tool_call": {"name": "execute", "args": {"command": "ls -l"}, "result": {"success": false, "output": "no such file or directory", "exit_code": 2}}}
//	{"text": "nope it's not there. should I create it?"}
//	{"tool_call": {"name": "user_choice", "args": {"options": ["yes", "no"]}, "result": {"choice": "yes"}}}
//
// Consecutive text/think lines accumulate into the current turn. One or
// more tool_call lines queue tool calls onto the current turn. A
// text/think line following one or more tool_call lines flushes the
// current turn (mirroring real provider APIs, which stop generating once
// tool calls are emitted) and starts a new one. EOF flushes whatever
// remains.
//
// tool_call.result is REQUIRED at parse time. A script that doesn't say
// what a tool call returned is treated as a bug in the test, not a
// silently-skipped assertion — this is deliberate: see the fakellm design
// discussion for why we rejected "skip the check if absent".
//
// Parallel tool calls (multiple tool_call lines with no intervening
// text/think, understood to be simultaneous) are NOT supported yet.
// Consecutive tool_call lines are treated as sequential single calls
// within one turn's ToolCalls slice; if/when a real test needs true
// parallel-call semantics, that should be an explicit script construct
// (e.g. a "parallel_tool_calls" line) rather than inferred from
// adjacency — deferred until there's a concrete need.
package fakellm

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"strings"

	"golang.org/x/xerrors"
)

// Step is the on-the-wire (well, on-the-line) representation of a single
// scripted step. Exactly one of Text, Think, ToolCall, or Error must be
// set.
type Step struct {
	Text     string     `json:"text,omitempty"`
	Think    string     `json:"think,omitempty"`
	ToolCall *ToolCall  `json:"tool_call,omitempty"`
	Error    *ErrorStep `json:"error,omitempty"`
}

// ToolCall is a single scripted tool call. Result is required: fakellm
// has no notion of "call a tool and see what happens" — the whole point
// is that every tool call's outcome is authored by the test, up front.
type ToolCall struct {
	Name   string          `json:"name"`
	Args   json.RawMessage `json:"args"`
	Result json.RawMessage `json:"result"`
}

// ErrorStep scripts a turn that fails instead of completing.
type ErrorStep struct {
	Message string `json:"message"`
}

// PartKind distinguishes the two kinds of content a turn's Parts can
// contain.
type PartKind int

const (
	PartText PartKind = iota
	PartThink
)

// Part is one piece of assistant-authored content (text or reasoning)
// within a Turn, in emission order.
type Part struct {
	Kind PartKind
	Text string
}

// Turn is one complete scripted model response: some number of text/
// think parts, optionally followed by one or more queued tool calls. A
// Turn with Err set represents an error response instead of a
// completion; its Parts/ToolCalls are unused.
type Turn struct {
	Parts     []Part
	ToolCalls []ToolCall
	Err       *ErrorStep
}

// Script is a fully-parsed, ordered sequence of Turns. Turn N is
// consumed by the (N+1)th call made against a Model/Server driven by
// this Script. There is no default/fallback turn: once Turns is
// exhausted, further calls fail loudly rather than silently reusing the
// last turn or echoing — see the package doc.
type Script struct {
	Turns []Turn
}

// Parse reads one JSON object per line from r and compiles it into a
// Script, applying the turn-boundary and required-result rules described
// in the package doc. Blank lines and lines starting with "//" are
// ignored, so scripts can be written as readable Go raw string literals
// with comments.
func Parse(r io.Reader) (*Script, error) {
	script := &Script{}
	var cur *Turn

	flush := func() {
		if cur != nil {
			script.Turns = append(script.Turns, *cur)
			cur = nil
		}
	}

	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 || bytes.HasPrefix(line, []byte("//")) {
			continue
		}

		var step Step
		if err := json.Unmarshal(line, &step); err != nil {
			return nil, xerrors.Errorf("fakellm: line %d: invalid step JSON: %w", lineNo, err)
		}
		if err := validateStep(step, lineNo); err != nil {
			return nil, err
		}

		switch {
		case step.Error != nil:
			// An error is its own isolated turn: flush whatever was
			// accumulating, then flush the error turn immediately.
			flush()
			script.Turns = append(script.Turns, Turn{Err: step.Error})
		case step.ToolCall != nil:
			if cur == nil {
				cur = &Turn{}
			}
			cur.ToolCalls = append(cur.ToolCalls, *step.ToolCall)
		default: // text or think
			if cur != nil && len(cur.ToolCalls) > 0 {
				// A tool call already queued on the current turn means
				// the model would have stopped generating to wait for
				// the result; this text/think line starts a new turn.
				flush()
			}
			if cur == nil {
				cur = &Turn{}
			}
			kind := PartText
			text := step.Text
			if step.Think != "" {
				kind = PartThink
				text = step.Think
			}
			cur.Parts = append(cur.Parts, Part{Kind: kind, Text: text})
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, xerrors.Errorf("fakellm: reading script: %w", err)
	}
	flush()

	if len(script.Turns) == 0 {
		return nil, xerrors.New("fakellm: script has no turns")
	}
	return script, nil
}

// ParseString is Parse for a script already held as a string, typically a
// Go raw string literal inline in a test.
func ParseString(src string) (*Script, error) {
	return Parse(strings.NewReader(src))
}

// MustParseString is ParseString, panicking on error. Intended for use in
// test setup where a parse failure is a bug in the test itself.
func MustParseString(src string) *Script {
	s, err := ParseString(src)
	if err != nil {
		panic(err)
	}
	return s
}

func validateStep(step Step, lineNo int) error {
	set := 0
	if step.Text != "" {
		set++
	}
	if step.Think != "" {
		set++
	}
	if step.ToolCall != nil {
		set++
	}
	if step.Error != nil {
		set++
	}
	if set != 1 {
		return xerrors.Errorf("fakellm: line %d: exactly one of text/think/tool_call/error must be set, got %d", lineNo, set)
	}
	if step.ToolCall != nil {
		if step.ToolCall.Name == "" {
			return xerrors.Errorf("fakellm: line %d: tool_call.name is required", lineNo)
		}
		if len(step.ToolCall.Result) == 0 || bytes.Equal(bytes.TrimSpace(step.ToolCall.Result), []byte("null")) {
			return xerrors.Errorf(
				"fakellm: line %d: tool_call %q has no result; every scripted tool call must specify its result explicitly (fakellm does not silently skip this check)",
				lineNo, step.ToolCall.Name,
			)
		}
	}
	return nil
}

// FinishedToolCalls reports whether the turn ends by queuing tool calls
// (as opposed to a plain text/think completion).
func (t Turn) FinishedToolCalls() bool {
	return len(t.ToolCalls) > 0
}

// Text concatenates all PartText parts, for tests that just want "what
// did it say".
func (t Turn) Text() string {
	var b strings.Builder
	for _, p := range t.Parts {
		if p.Kind == PartText {
			_, _ = b.WriteString(p.Text)
		}
	}
	return b.String()
}

func (e *ErrorStep) error() error {
	return xerrors.Errorf("fakellm: scripted error: %s", e.Message)
}
