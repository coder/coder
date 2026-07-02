package fakellm

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"

	"charm.land/fantasy"
	"golang.org/x/xerrors"
)

// Model is a fantasy.LanguageModel driven by a Script, with no HTTP/wire
// involvement at all — it replaces chattest.FakeModel's per-test-file
// hand-built fantasy.StreamResponse/textMessage helpers for the common
// "scripted echo/tool-call conversation" case.
//
// Because every scripted tool_call carries its own required Result,
// Model never needs a real tool to actually execute: a test can drive a
// full multi-turn tool-calling conversation by calling Generate/Stream,
// inspecting the returned ToolCallContent, fetching the matching
// canned result via ResultFor, and feeding a synthesized
// fantasy.ToolResultPart back in on the next call — all deterministic,
// no real tool dispatch required. (Tests that *do* want real tool
// dispatch to run should exercise chatloop/chatd end-to-end instead;
// Model's job is only to be the model.)
type Model struct {
	ProviderName string
	ModelName    string

	script *Script
	calls  atomic.Int64

	mu      sync.Mutex
	results map[string]json.RawMessage // tool call ID -> scripted result
}

var _ fantasy.LanguageModel = (*Model)(nil)

// NewModel returns a Model driven by script.
func NewModel(script *Script) *Model {
	return &Model{
		ProviderName: "fakellm",
		ModelName:    "fakellm",
		script:       script,
		results:      make(map[string]json.RawMessage),
	}
}

// ResultFor returns the scripted result for a tool call previously
// returned by Generate/Stream, keyed by the ToolCallID that was
// generated for it. Used by a test's own harness to build the
// fantasy.ToolResultPart it feeds back into the next Call, without
// needing any real tool to execute.
func (m *Model) ResultFor(toolCallID string) (json.RawMessage, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.results[toolCallID]
	return r, ok
}

// Calls returns the number of Generate/Stream calls made so far.
func (m *Model) Calls() int64 {
	return m.calls.Load()
}

func (m *Model) next() (Turn, int64, error) {
	idx := m.calls.Add(1) - 1
	if idx >= int64(len(m.script.Turns)) {
		return Turn{}, idx, xerrors.Errorf("fakellm: script exhausted: call %d made, but only %d turn(s) scripted", idx+1, len(m.script.Turns))
	}
	return m.script.Turns[idx], idx, nil
}

func toolCallID(turnIdx int64, callIdx int) string {
	return fmt.Sprintf("fakellm-call-%d-%d", turnIdx, callIdx)
}

func (m *Model) recordResult(id string, result json.RawMessage) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.results[id] = result
}

// Generate implements fantasy.LanguageModel.
func (m *Model) Generate(_ context.Context, _ fantasy.Call) (*fantasy.Response, error) {
	turn, idx, err := m.next()
	if err != nil {
		return nil, err
	}
	if turn.Err != nil {
		return nil, turn.Err.error()
	}

	var content fantasy.ResponseContent
	for _, p := range turn.Parts {
		switch p.Kind {
		case PartText:
			content = append(content, fantasy.TextContent{Text: p.Text})
		case PartThink:
			content = append(content, fantasy.ReasoningContent{Text: p.Text})
		}
	}
	finish := fantasy.FinishReasonStop
	for i, tc := range turn.ToolCalls {
		id := toolCallID(idx, i)
		m.recordResult(id, tc.Result)
		content = append(content, fantasy.ToolCallContent{
			ToolCallID: id,
			ToolName:   tc.Name,
			Input:      string(tc.Args),
		})
		finish = fantasy.FinishReasonToolCalls
	}

	return &fantasy.Response{
		Content:      content,
		FinishReason: finish,
	}, nil
}

// Stream implements fantasy.LanguageModel by replaying the same content
// Generate would return as a single-shot stream (one delta per part/tool
// call, no artificial chunking or delay).
func (m *Model) Stream(_ context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
	turn, idx, err := m.next()
	if err != nil {
		return nil, err
	}

	return func(yield func(fantasy.StreamPart) bool) {
		if turn.Err != nil {
			yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeError, Error: turn.Err.error()})
			return
		}

		for _, p := range turn.Parts {
			var start, delta, end fantasy.StreamPartType
			switch p.Kind {
			case PartText:
				start, delta, end = fantasy.StreamPartTypeTextStart, fantasy.StreamPartTypeTextDelta, fantasy.StreamPartTypeTextEnd
			case PartThink:
				start, delta, end = fantasy.StreamPartTypeReasoningStart, fantasy.StreamPartTypeReasoningDelta, fantasy.StreamPartTypeReasoningEnd
			}
			if !yield(fantasy.StreamPart{Type: start}) {
				return
			}
			if !yield(fantasy.StreamPart{Type: delta, Delta: p.Text}) {
				return
			}
			if !yield(fantasy.StreamPart{Type: end}) {
				return
			}
		}

		finish := fantasy.FinishReasonStop
		for i, tc := range turn.ToolCalls {
			id := toolCallID(idx, i)
			m.recordResult(id, tc.Result)
			if !yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeToolInputStart, ID: id, ToolCallName: tc.Name}) {
				return
			}
			if !yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeToolInputDelta, ID: id, ToolCallInput: string(tc.Args)}) {
				return
			}
			if !yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeToolInputEnd, ID: id}) {
				return
			}
			if !yield(fantasy.StreamPart{
				Type:          fantasy.StreamPartTypeToolCall,
				ID:            id,
				ToolCallName:  tc.Name,
				ToolCallInput: string(tc.Args),
			}) {
				return
			}
			finish = fantasy.FinishReasonToolCalls
		}

		yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeFinish, FinishReason: finish})
	}, nil
}

// GenerateObject and StreamObject are not supported by this spike: fakellm
// scripts describe text/reasoning/tool-call conversations only. Calling
// either panics, matching chattest.FakeModel's "be explicit about which
// methods you expect invoked" behavior.
func (*Model) GenerateObject(context.Context, fantasy.ObjectCall) (*fantasy.ObjectResponse, error) {
	panic("fakellm: Model.GenerateObject is not supported by this spike")
}

func (*Model) StreamObject(context.Context, fantasy.ObjectCall) (fantasy.ObjectStreamResponse, error) {
	panic("fakellm: Model.StreamObject is not supported by this spike")
}

func (m *Model) Provider() string { return m.ProviderName }
func (m *Model) Model() string    { return m.ModelName }
