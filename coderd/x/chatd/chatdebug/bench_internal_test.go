package chatdebug

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"testing"

	"charm.land/fantasy"
	"github.com/google/uuid"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
)

// noopStore implements database.Store by embedding it (nil) and
// overriding only the methods chatdebug's hot paths call. Every
// override returns immediately with a canned value and does no I/O,
// so any time spent inside Service methods that use this store is
// chatdebug's own CPU/allocation cost, not Postgres's.
type noopStore struct {
	database.Store
}

func (noopStore) GetChatDebugLoggingAllowUsers(context.Context) (bool, error) {
	return true, nil
}

func (noopStore) GetUserChatDebugLoggingEnabled(context.Context, uuid.UUID) (bool, error) {
	return true, nil
}

func (noopStore) InsertChatDebugStep(
	_ context.Context,
	arg database.InsertChatDebugStepParams,
) (database.ChatDebugStep, error) {
	return database.ChatDebugStep{
		ID:         uuid.New(),
		RunID:      arg.RunID,
		ChatID:     arg.ChatID,
		StepNumber: arg.StepNumber,
		Operation:  arg.Operation,
		Status:     arg.Status,
	}, nil
}

func (noopStore) UpdateChatDebugStep(
	_ context.Context,
	arg database.UpdateChatDebugStepParams,
) (database.ChatDebugStep, error) {
	return database.ChatDebugStep{
		ID:     arg.ID,
		ChatID: arg.ChatID,
	}, nil
}

// TouchChatDebugStepAndRun and GetChatDebugStepsByRunID aren't on the
// hot path these benchmarks exercise (heartbeat interval and step
// retry-collision handling respectively), but overriding them avoids
// a nil-dereference panic if a future benchmark configuration change
// reaches them through the embedded nil database.Store.
func (noopStore) TouchChatDebugStepAndRun(context.Context, database.TouchChatDebugStepAndRunParams) error {
	return nil
}

func (noopStore) GetChatDebugStepsByRunID(context.Context, uuid.UUID) ([]database.ChatDebugStep, error) {
	return nil, nil
}

var _ database.Store = noopStore{}

// benchService builds a Service backed by noopStore and an in-memory
// pubsub so publishEvent still marshals and dispatches DebugEvent
// payloads, mirroring a real deployment's wiring, without any network
// or disk I/O.
func benchService(b *testing.B) *Service {
	b.Helper()
	ps := pubsub.NewInMemory()
	b.Cleanup(func() { _ = ps.Close() })
	return NewService(noopStore{}, slog.Make(), ps)
}

// benchCall builds a realistic multi-turn prompt with tool
// definitions so normalizeCall walks a representative message/tool
// shape.
func benchCall(nMessages, nTools int) fantasy.Call {
	prompt := make(fantasy.Prompt, 0, nMessages)
	for i := range nMessages {
		if i%2 == 0 {
			prompt = append(prompt, fantasy.NewUserMessage(
				fmt.Sprintf("Please investigate issue #%d and summarize the relevant log lines around the failure.", i)))
			continue
		}
		prompt = append(prompt, fantasy.Message{
			Role: fantasy.MessageRoleAssistant,
			Content: []fantasy.MessagePart{
				fantasy.TextPart{Text: strings.Repeat("analysis details ", 40)},
			},
		})
	}

	tools := make([]fantasy.Tool, 0, nTools)
	for i := range nTools {
		tools = append(tools, fantasy.FunctionTool{
			Name:        fmt.Sprintf("tool_%d", i),
			Description: "Runs a workspace command and returns its output",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"command": map[string]any{"type": "string"},
					"timeout": map[string]any{"type": "integer"},
				},
				"required": []string{"command"},
			},
		})
	}

	return fantasy.Call{
		Prompt: prompt,
		Tools:  tools,
	}
}

func benchStreamParts(nDeltas int) []fantasy.StreamPart {
	parts := make([]fantasy.StreamPart, 0, nDeltas+2)
	for i := range nDeltas {
		parts = append(parts, fantasy.StreamPart{
			Type:  fantasy.StreamPartTypeTextDelta,
			ID:    "text-1",
			Delta: fmt.Sprintf("token-%d ", i),
		})
	}
	parts = append(parts,
		fantasy.StreamPart{Type: fantasy.StreamPartTypeToolCall, ID: "tool-1", ToolCallName: "tool_0"},
		fantasy.StreamPart{
			Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonStop,
			Usage: fantasy.Usage{InputTokens: 512, OutputTokens: int64(nDeltas), TotalTokens: 512 + int64(nDeltas)},
		},
	)
	return parts
}

// runContext returns a fresh RunContext/context pair for one
// benchmark iteration, mirroring a new chat turn.
func runContext(chatID uuid.UUID) context.Context {
	return ContextWithRun(context.Background(), &RunContext{RunID: uuid.New(), ChatID: chatID})
}

func BenchmarkWrapModel_Generate(b *testing.B) {
	svc := benchService(b)
	chatID, ownerID := uuid.New(), uuid.New()
	call := benchCall(6, 4)
	resp := &fantasy.Response{
		Content: fantasy.ResponseContent{
			fantasy.TextContent{Text: strings.Repeat("response text ", 100)},
			fantasy.ToolCallContent{ToolCallID: "tool-1", ToolName: "tool_0", Input: `{"command":"ls"}`},
		},
		FinishReason: fantasy.FinishReasonStop,
		Usage:        fantasy.Usage{InputTokens: 512, OutputTokens: 128, TotalTokens: 640},
	}
	inner := &chattest.FakeModel{GenerateFn: func(context.Context, fantasy.Call) (*fantasy.Response, error) {
		return resp, nil
	}}
	model := WrapModel(inner, svc, RecorderOptions{ChatID: chatID, OwnerID: ownerID})

	b.ReportAllocs()
	for b.Loop() {
		ctx := runContext(chatID)
		if _, err := model.Generate(ctx, call); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkWrapModel_Stream(b *testing.B) {
	svc := benchService(b)
	chatID, ownerID := uuid.New(), uuid.New()
	call := benchCall(6, 4)
	parts := benchStreamParts(200)
	inner := &chattest.FakeModel{StreamFn: func(context.Context, fantasy.Call) (fantasy.StreamResponse, error) {
		return slices.Values(parts), nil
	}}
	model := WrapModel(inner, svc, RecorderOptions{ChatID: chatID, OwnerID: ownerID})

	b.ReportAllocs()
	for b.Loop() {
		ctx := runContext(chatID)
		seq, err := model.Stream(ctx, call)
		if err != nil {
			b.Fatal(err)
		}
		for range seq { //nolint:revive // draining the stream is the point of the benchmark.
		}
	}
}

// cannedRoundTripper replays a fixed response body for every request,
// isolating RecordingTransport's own redaction/buffering cost from any
// real network or provider latency.
type cannedRoundTripper struct {
	status    int
	header    http.Header
	body      []byte
	chunkSize int // 0 means return the whole body in one Read.
}

func (rt *cannedRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	var body io.ReadCloser
	if rt.chunkSize > 0 {
		body = io.NopCloser(&chunkedReader{data: rt.body, chunkSize: rt.chunkSize})
	} else {
		body = io.NopCloser(bytes.NewReader(rt.body))
	}
	header := rt.header.Clone()
	return &http.Response{
		StatusCode:    rt.status,
		Header:        header,
		Body:          body,
		ContentLength: -1,
		Request:       req,
	}, nil
}

// chunkedReader splits data into fixed-size Read() calls so the
// benchmark exercises recordingBody's incremental accumulation path
// the way a real SSE stream would, instead of handing back the whole
// body in a single Read.
type chunkedReader struct {
	data      []byte
	chunkSize int
}

func (c *chunkedReader) Read(p []byte) (int, error) {
	if len(c.data) == 0 {
		return 0, io.EOF
	}
	n := min(c.chunkSize, len(c.data), len(p))
	copy(p, c.data[:n])
	c.data = c.data[n:]
	return n, nil
}

func benchJSONResponseBody(n int) []byte {
	type choice struct {
		Index int    `json:"index"`
		Text  string `json:"text"`
	}
	payload := struct {
		ID      string   `json:"id"`
		Object  string   `json:"object"`
		Model   string   `json:"model"`
		Choices []choice `json:"choices"`
		Usage   struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
	}{
		ID:     "resp-bench",
		Object: "chat.completion",
		Model:  "bench-model",
	}
	for i := range n {
		payload.Choices = append(payload.Choices, choice{
			Index: i,
			Text:  strings.Repeat("word ", 50),
		})
	}
	data, err := json.Marshal(payload)
	if err != nil {
		panic(err)
	}
	return data
}

func benchSSEResponseBody(nEvents int) []byte {
	var buf bytes.Buffer
	for i := range nEvents {
		// bytes.Buffer.Write* never returns an error.
		_, _ = fmt.Fprintf(&buf, "data: {\"delta\":\"token-%d \",\"index\":%d}\n\n", i, i)
	}
	_, _ = buf.WriteString("data: [DONE]\n\n")
	return buf.Bytes()
}

func newBenchRequest(b *testing.B, sink *attemptSink) *http.Request {
	b.Helper()
	req, err := http.NewRequestWithContext(
		withAttemptSink(context.Background(), sink),
		http.MethodPost,
		"https://api.example.com/v1/chat/completions",
		bytes.NewReader([]byte(`{"model":"bench-model","messages":[{"role":"user","content":"hi"}]}`)),
	)
	if err != nil {
		b.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer sk-should-be-redacted")
	req.Header.Set("Content-Type", "application/json")
	return req
}

func BenchmarkRecordingTransport_RoundTrip_JSON(b *testing.B) {
	header := http.Header{"Content-Type": []string{"application/json"}}
	body := benchJSONResponseBody(20)
	transport := &RecordingTransport{Base: &cannedRoundTripper{status: 200, header: header, body: body}}

	b.ReportAllocs()
	for b.Loop() {
		sink := &attemptSink{}
		req := newBenchRequest(b, sink)
		resp, err := transport.RoundTrip(req)
		if err != nil {
			b.Fatal(err)
		}
		if _, err := io.Copy(io.Discard, resp.Body); err != nil {
			b.Fatal(err)
		}
		if err := resp.Body.Close(); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRecordingTransport_RoundTrip_SSE(b *testing.B) {
	header := http.Header{"Content-Type": []string{"text/event-stream"}}
	body := benchSSEResponseBody(200)
	transport := &RecordingTransport{Base: &cannedRoundTripper{status: 200, header: header, body: body, chunkSize: 256}}

	b.ReportAllocs()
	for b.Loop() {
		sink := &attemptSink{}
		req := newBenchRequest(b, sink)
		resp, err := transport.RoundTrip(req)
		if err != nil {
			b.Fatal(err)
		}
		if _, err := io.Copy(io.Discard, resp.Body); err != nil {
			b.Fatal(err)
		}
		if err := resp.Body.Close(); err != nil {
			b.Fatal(err)
		}
	}
}
