package chatdebug

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"reflect"
	"sync"
	"sync/atomic"
	"unicode/utf8"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	stringutil "github.com/coder/coder/v2/coderd/util/strings"
)

type debugModel struct {
	inner fantasy.LanguageModel
	svc   *Service
	opts  RecorderOptions
}

var _ fantasy.LanguageModel = (*debugModel)(nil)

// ErrNilModelResult is returned when the underlying language model
// returns a nil response or stream. Callers can match with
// errors.Is to distinguish this from provider-level failures.
var ErrNilModelResult = xerrors.New("language model returned nil result")

// normalizedCallOptions holds the optional model parameters shared by
// both regular and structured-output calls.
type normalizedCallOptions struct {
	MaxOutputTokens  *int64   `json:"max_output_tokens,omitempty"`
	Temperature      *float64 `json:"temperature,omitempty"`
	TopP             *float64 `json:"top_p,omitempty"`
	TopK             *int64   `json:"top_k,omitempty"`
	PresencePenalty  *float64 `json:"presence_penalty,omitempty"`
	FrequencyPenalty *float64 `json:"frequency_penalty,omitempty"`
}

// normalizedCallPayload is the rich envelope persisted for Generate /
// Stream calls. It carries the full message structure and tool
// metadata so the debug panel can render conversation context.
type normalizedCallPayload struct {
	Messages            []normalizedMessage   `json:"messages"`
	Tools               []normalizedTool      `json:"tools,omitempty"`
	Options             normalizedCallOptions `json:"options"`
	ToolChoice          string                `json:"tool_choice,omitempty"`
	ProviderOptionCount int                   `json:"provider_option_count"`
}

// normalizedObjectCallPayload is the rich envelope for
// GenerateObject / StreamObject calls, including schema metadata.
type normalizedObjectCallPayload struct {
	Messages            []normalizedMessage   `json:"messages"`
	Options             normalizedCallOptions `json:"options"`
	SchemaName          string                `json:"schema_name,omitempty"`
	SchemaDescription   string                `json:"schema_description,omitempty"`
	StructuredOutput    bool                  `json:"structured_output"`
	ProviderOptionCount int                   `json:"provider_option_count"`
}

// normalizedResponsePayload is the rich envelope for persisted model
// responses. It includes the full content parts, finish reason, token
// usage breakdown, and any provider warnings.
type normalizedResponsePayload struct {
	Content      []normalizedContentPart `json:"content"`
	FinishReason string                  `json:"finish_reason"`
	Usage        normalizedUsage         `json:"usage"`
	Warnings     []normalizedWarning     `json:"warnings,omitempty"`
}

// normalizedObjectResponsePayload is the rich envelope for
// structured-output responses. Raw text is bounded to length only.
type normalizedObjectResponsePayload struct {
	RawTextLength    int                 `json:"raw_text_length"`
	FinishReason     string              `json:"finish_reason"`
	Usage            normalizedUsage     `json:"usage"`
	Warnings         []normalizedWarning `json:"warnings,omitempty"`
	StructuredOutput bool                `json:"structured_output"`
}

// --------------- helper types ---------------

// normalizedMessage represents a single message in the prompt with
// its role and constituent parts.
type normalizedMessage struct {
	Role  string                  `json:"role"`
	Parts []normalizedMessagePart `json:"parts"`
}

// MaxMessagePartTextLength is the rune limit for bounded text stored
// in request message parts. Longer text is truncated with an ellipsis.
const MaxMessagePartTextLength = 10_000

// maxStreamDebugTextBytes caps accumulated streamed text persisted in
// debug responses.
const maxStreamDebugTextBytes = 50_000

// normalizedMessagePart captures the type and bounded metadata for a
// single part within a prompt message. Text-like payloads are truncated
// to MaxMessagePartTextLength runes so request payloads stay bounded
// while still giving the debug panel readable content.
type normalizedMessagePart struct {
	Type       string `json:"type"`
	Text       string `json:"text,omitempty"`
	TextLength int    `json:"text_length,omitempty"`
	Filename   string `json:"filename,omitempty"`
	MediaType  string `json:"media_type,omitempty"`
	ToolCallID string `json:"tool_call_id,omitempty"`
	ToolName   string `json:"tool_name,omitempty"`
	Arguments  string `json:"arguments,omitempty"`
	Result     string `json:"result,omitempty"`
}

// normalizedTool captures tool identity along with any JSON input
// schema needed by the debug panel.
type normalizedTool struct {
	Type           string          `json:"type"`
	Name           string          `json:"name"`
	Description    string          `json:"description,omitempty"`
	ID             string          `json:"id,omitempty"`
	HasInputSchema bool            `json:"has_input_schema,omitempty"`
	InputSchema    json.RawMessage `json:"input_schema,omitempty"`
}

// normalizedContentPart captures one piece of the model response.
// Text payloads are bounded to MaxMessagePartTextLength runes;
// TextLength stores the original rune count for truncation detection.
// Tool-call arguments are similarly bounded, and file data is never
// stored.
type normalizedContentPart struct {
	Type        string `json:"type"`
	Text        string `json:"text,omitempty"`
	TextLength  int    `json:"text_length,omitempty"`
	ToolCallID  string `json:"tool_call_id,omitempty"`
	ToolName    string `json:"tool_name,omitempty"`
	Arguments   string `json:"arguments,omitempty"`
	Result      string `json:"result,omitempty"`
	InputLength int    `json:"input_length,omitempty"`
	MediaType   string `json:"media_type,omitempty"`
	SourceType  string `json:"source_type,omitempty"`
	Title       string `json:"title,omitempty"`
	URL         string `json:"url,omitempty"`
}

// normalizedUsage mirrors fantasy.Usage with the full token
// breakdown so the debug panel can display cost/cache info.
type normalizedUsage struct {
	InputTokens         int64 `json:"input_tokens"`
	OutputTokens        int64 `json:"output_tokens"`
	TotalTokens         int64 `json:"total_tokens"`
	ReasoningTokens     int64 `json:"reasoning_tokens"`
	CacheCreationTokens int64 `json:"cache_creation_tokens"`
	CacheReadTokens     int64 `json:"cache_read_tokens"`
}

// normalizedWarning captures a single provider warning.
type normalizedWarning struct {
	Type    string `json:"type"`
	Setting string `json:"setting,omitempty"`
	Details string `json:"details,omitempty"`
	Message string `json:"message,omitempty"`
}

type normalizedErrorPayload struct {
	Message        string `json:"message"`
	Type           string `json:"type"`
	ContextError   string `json:"context_error,omitempty"`
	ProviderTitle  string `json:"provider_title,omitempty"`
	ProviderStatus int    `json:"provider_status,omitempty"`
	IsRetryable    bool   `json:"is_retryable,omitempty"`
}

type streamSummary struct {
	FinishReason   string `json:"finish_reason,omitempty"`
	TextDeltaCount int    `json:"text_delta_count"`
	ToolCallCount  int    `json:"tool_call_count"`
	SourceCount    int    `json:"source_count"`
	WarningCount   int    `json:"warning_count"`
	ErrorCount     int    `json:"error_count"`
	LastError      string `json:"last_error,omitempty"`
	PartCount      int    `json:"part_count"`
}

type objectStreamSummary struct {
	FinishReason     string `json:"finish_reason,omitempty"`
	ObjectPartCount  int    `json:"object_part_count"`
	TextDeltaCount   int    `json:"text_delta_count"`
	ErrorCount       int    `json:"error_count"`
	LastError        string `json:"last_error,omitempty"`
	WarningCount     int    `json:"warning_count"`
	PartCount        int    `json:"part_count"`
	StructuredOutput bool   `json:"structured_output"`
}

func (d *debugModel) Generate(
	ctx context.Context,
	call fantasy.Call,
) (*fantasy.Response, error) {
	if d.svc == nil {
		return d.inner.Generate(ctx, call)
	}
	if _, ok := RunFromContext(ctx); !ok {
		return d.inner.Generate(ctx, call)
	}

	handle, enrichedCtx := beginStep(ctx, d.svc, d.opts, OperationGenerate,
		normalizeCall(call))
	if handle == nil {
		return d.inner.Generate(ctx, call)
	}

	// Keep the step alive during the blocking provider call so the
	// stale finalizer does not mark it as interrupted.
	heartbeatDone := make(chan struct{})
	launchHeartbeat(ctx, handle.svc, handle.stepCtx.StepID, handle.stepCtx.RunID, handle.stepCtx.ChatID, heartbeatDone)

	resp, err := d.inner.Generate(enrichedCtx, call)
	close(heartbeatDone)
	if err != nil {
		handle.finish(ctx, stepStatusForError(err), nil, nil, normalizeError(ctx, err), nil)
		return nil, err
	}
	if resp == nil {
		err = xerrors.Errorf("Generate: %w", ErrNilModelResult)
		handle.finish(ctx, StatusError, nil, nil, normalizeError(ctx, err), nil)
		return nil, err
	}

	handle.finish(ctx, StatusCompleted, normalizeResponse(resp), &resp.Usage, nil, nil)
	return resp, nil
}

func (d *debugModel) Stream(
	ctx context.Context,
	call fantasy.Call,
) (fantasy.StreamResponse, error) {
	if d.svc == nil {
		return d.inner.Stream(ctx, call)
	}
	if _, ok := RunFromContext(ctx); !ok {
		return d.inner.Stream(ctx, call)
	}

	handle, enrichedCtx := beginStep(ctx, d.svc, d.opts, OperationStream,
		normalizeCall(call))
	if handle == nil {
		return d.inner.Stream(ctx, call)
	}

	seq, err := d.inner.Stream(enrichedCtx, call)
	if err != nil {
		handle.finish(ctx, stepStatusForError(err), nil, nil, normalizeError(ctx, err), nil)
		return nil, err
	}
	if seq == nil {
		err = xerrors.Errorf("Stream: %w", ErrNilModelResult)
		handle.finish(ctx, StatusError, nil, nil, normalizeError(ctx, err), nil)
		return nil, err
	}

	return wrapStreamSeq(ctx, handle, seq), nil
}

func (d *debugModel) GenerateObject(
	ctx context.Context,
	call fantasy.ObjectCall,
) (*fantasy.ObjectResponse, error) {
	if d.svc == nil {
		return d.inner.GenerateObject(ctx, call)
	}
	if _, ok := RunFromContext(ctx); !ok {
		return d.inner.GenerateObject(ctx, call)
	}

	handle, enrichedCtx := beginStep(ctx, d.svc, d.opts, OperationGenerate,
		normalizeObjectCall(call))
	if handle == nil {
		return d.inner.GenerateObject(ctx, call)
	}

	// Keep the step alive during the blocking provider call so the
	// stale finalizer does not mark it as interrupted.
	heartbeatDone := make(chan struct{})
	launchHeartbeat(ctx, handle.svc, handle.stepCtx.StepID, handle.stepCtx.RunID, handle.stepCtx.ChatID, heartbeatDone)

	resp, err := d.inner.GenerateObject(enrichedCtx, call)
	close(heartbeatDone)
	if err != nil {
		handle.finish(ctx, stepStatusForError(err), nil, nil, normalizeError(ctx, err),
			map[string]any{"structured_output": true})
		return nil, err
	}
	if resp == nil {
		err = xerrors.Errorf("GenerateObject: %w", ErrNilModelResult)
		handle.finish(ctx, StatusError, nil, nil, normalizeError(ctx, err),
			map[string]any{"structured_output": true})
		return nil, err
	}

	handle.finish(ctx, StatusCompleted, normalizeObjectResponse(resp), &resp.Usage,
		nil, map[string]any{"structured_output": true})
	return resp, nil
}

func (d *debugModel) StreamObject(
	ctx context.Context,
	call fantasy.ObjectCall,
) (fantasy.ObjectStreamResponse, error) {
	if d.svc == nil {
		return d.inner.StreamObject(ctx, call)
	}
	if _, ok := RunFromContext(ctx); !ok {
		return d.inner.StreamObject(ctx, call)
	}

	handle, enrichedCtx := beginStep(ctx, d.svc, d.opts, OperationStream,
		normalizeObjectCall(call))
	if handle == nil {
		return d.inner.StreamObject(ctx, call)
	}

	seq, err := d.inner.StreamObject(enrichedCtx, call)
	if err != nil {
		handle.finish(ctx, stepStatusForError(err), nil, nil, normalizeError(ctx, err),
			map[string]any{"structured_output": true})
		return nil, err
	}
	if seq == nil {
		err = xerrors.Errorf("StreamObject: %w", ErrNilModelResult)
		handle.finish(ctx, StatusError, nil, nil, normalizeError(ctx, err),
			map[string]any{"structured_output": true})
		return nil, err
	}

	return wrapObjectStreamSeq(ctx, handle, seq), nil
}

func (d *debugModel) Provider() string {
	return d.inner.Provider()
}

func (d *debugModel) Model() string {
	return d.inner.Model()
}

// launchHeartbeat starts a goroutine that periodically calls TouchStep
// to keep the step and run rows alive during long-running streams.  The
// goroutine also listens on the service's threshold-change channel so
// that a runtime SetStaleAfter call immediately resets the ticker
// instead of waiting for the old (possibly longer) period to elapse.
// The goroutine exits when done is closed or ctx is canceled.
func launchHeartbeat(ctx context.Context, svc *Service, stepID, runID, chatID uuid.UUID, done <-chan struct{}) {
	if svc == nil {
		return
	}
	go func() {
		interval := svc.heartbeatInterval()
		ticker := svc.clock.NewTicker(interval, "chatdebug", "heartbeat")
		defer ticker.Stop()
		thresholdCh := svc.thresholdChan()
		for {
			select {
			case <-ctx.Done():
				return
			case <-done:
				return
			case <-thresholdCh:
				// SetStaleAfter was called; re-read the interval
				// and reset the ticker immediately.
				thresholdCh = svc.thresholdChan()
				if newInterval := svc.heartbeatInterval(); newInterval != interval {
					interval = newInterval
					ticker.Reset(interval, "chatdebug", "heartbeat")
				}
			case <-ticker.C:
				if err := svc.TouchStep(ctx, stepID, runID, chatID); err != nil {
					svc.log.Debug(ctx, "heartbeat touch failed",
						slog.Error(err),
						slog.F("step_id", stepID),
					)
				}
				// Also re-read interval on every tick as a
				// secondary check.
				if newInterval := svc.heartbeatInterval(); newInterval != interval {
					interval = newInterval
					ticker.Reset(interval, "chatdebug", "heartbeat")
				}
			}
		}
	}()
}

func wrapStreamSeq(
	ctx context.Context,
	handle *stepHandle,
	seq iter.Seq[fantasy.StreamPart],
) fantasy.StreamResponse {
	// mu and finalized guard both the normal finalization path
	// inside the iterator and the safety-net AfterFunc below.
	// This ensures handle.finish is called exactly once regardless
	// of whether the caller iterates, drops the stream, or the
	// context is canceled mid-flight. We use a mutex rather than
	// sync.Once so the AfterFunc can yield to the normal path
	// when the stream already received its terminal chunk
	// (streamComplete), preventing the AfterFunc from clobbering
	// completed stream data with nil.
	var (
		mu             sync.Mutex
		finalized      bool
		streamComplete atomic.Bool
	)

	// heartbeatDone is closed when the stream finalizes (either
	// normally or via the safety net) to stop the heartbeat goroutine.
	heartbeatDone := make(chan struct{})

	// Safety net: if the caller drops the returned iterator without
	// consuming it (or abandons mid-stream and the context is
	// canceled), finalize the step so it does not remain permanently
	// in_progress once persistence lands in later branches.
	stop := context.AfterFunc(ctx, func() {
		mu.Lock()
		defer mu.Unlock()
		// If the stream already received a finish chunk, let
		// finalize handle it; it has the real response payload
		// and usage data that we would otherwise clobber.
		if finalized || streamComplete.Load() {
			return
		}
		finalized = true
		close(heartbeatDone)
		handle.finish(ctx, StatusInterrupted, nil, nil, nil, nil)
	})

	// startHeartbeat launches the heartbeat goroutine on first call.
	// Deferring the start until the caller begins consuming the stream
	// prevents leaked goroutines when the iterator is dropped without
	// being iterated.
	startHeartbeat := sync.OnceFunc(func() {
		launchHeartbeat(ctx, handle.svc, handle.stepCtx.StepID, handle.stepCtx.RunID, handle.stepCtx.ChatID, heartbeatDone)
	})

	return func(yield func(fantasy.StreamPart) bool) {
		startHeartbeat()
		var (
			summary          streamSummary
			latestUsage      fantasy.Usage
			usageSeen        bool
			finishSeen       bool
			finishReason     fantasy.FinishReason
			content          []normalizedContentPart
			warnings         []normalizedWarning
			streamDebugBytes int
			streamError      any
			streamStatus     = StatusCompleted
		)

		finalize := func(status Status) {
			// Cancel the safety net and heartbeat since we're finalizing.
			if stop != nil {
				stop()
			}
			mu.Lock()
			defer mu.Unlock()
			if finalized {
				return
			}
			finalized = true
			close(heartbeatDone)

			summary.FinishReason = string(finishReason)

			resp := normalizedResponsePayload{
				Content:      content,
				FinishReason: string(finishReason),
				Warnings:     warnings,
			}
			if usageSeen {
				resp.Usage = normalizeUsage(latestUsage)
			}

			var usage any
			if usageSeen {
				usage = &latestUsage
			}
			handle.finish(ctx, status, resp, usage, streamError, map[string]any{
				"stream_summary": summary,
			})
		}

		if seq != nil {
			seq(func(part fantasy.StreamPart) bool {
				summary.PartCount++
				summary.WarningCount += len(part.Warnings)
				if len(part.Warnings) > 0 {
					warnings = append(warnings, normalizeWarnings(part.Warnings)...)
				}

				switch part.Type {
				case fantasy.StreamPartTypeTextDelta:
					summary.TextDeltaCount++
				case fantasy.StreamPartTypeReasoningStart,
					fantasy.StreamPartTypeReasoningDelta:
				case fantasy.StreamPartTypeToolCall:
					summary.ToolCallCount++
				case fantasy.StreamPartTypeToolResult:
				case fantasy.StreamPartTypeSource:
					summary.SourceCount++
				case fantasy.StreamPartTypeFinish:
					finishReason = part.FinishReason
					latestUsage = part.Usage
					usageSeen = true
					finishSeen = true
					// Signal that the stream received its terminal
					// chunk so the AfterFunc safety net yields to
					// finalize, which has the real response payload.
					streamComplete.Store(true)
				}

				content = appendNormalizedStreamContent(content, part, &streamDebugBytes)

				if part.Type == fantasy.StreamPartTypeError || part.Error != nil {
					summary.ErrorCount++
					if part.Error != nil {
						summary.LastError = part.Error.Error()
						streamError = normalizeError(ctx, part.Error)
					} else {
						summary.LastError = "stream error part with nil error"
						streamError = map[string]string{"error": "stream error part with nil error"}
					}
					streamStatus = streamErrorStatus(streamStatus, part.Error)
				}

				if !yield(part) {
					// When the consumer stops iteration after
					// receiving a finish part, the stream completed
					// successfully; the consumer simply has nothing
					// left to read. Only mark as interrupted when the
					// consumer exits before the provider finished.
					switch {
					case streamStatus == StatusError:
						finalize(StatusError)
					case finishSeen:
						finalize(StatusCompleted)
					default:
						finalize(StatusInterrupted)
					}
					return false
				}

				return true
			})
		}

		// If the stream ended without a finish part and
		// without an explicit error, the provider closed
		// the connection prematurely. Record this as
		// interrupted so debug runs surface incomplete
		// output instead of falsely reporting success.
		if streamStatus == StatusCompleted && !finishSeen {
			streamStatus = StatusInterrupted
		}
		finalize(streamStatus)
	}
}

func wrapObjectStreamSeq(
	ctx context.Context,
	handle *stepHandle,
	seq iter.Seq[fantasy.ObjectStreamPart],
) fantasy.ObjectStreamResponse {
	// Same safety-net pattern as wrapStreamSeq: a mutex rather
	// than sync.Once lets the AfterFunc yield to the normal
	// finalization path when the stream has already completed.
	var (
		mu             sync.Mutex
		finalized      bool
		streamComplete atomic.Bool
	)

	heartbeatDone := make(chan struct{})

	stop := context.AfterFunc(ctx, func() {
		mu.Lock()
		defer mu.Unlock()
		if finalized || streamComplete.Load() {
			return
		}
		finalized = true
		close(heartbeatDone)
		handle.finish(ctx, StatusInterrupted, nil, nil, nil, nil)
	})

	// Deferred heartbeat: start the heartbeat goroutine only when the
	// caller begins consuming the stream.
	startHeartbeat := sync.OnceFunc(func() {
		launchHeartbeat(ctx, handle.svc, handle.stepCtx.StepID, handle.stepCtx.RunID, handle.stepCtx.ChatID, heartbeatDone)
	})

	return func(yield func(fantasy.ObjectStreamPart) bool) {
		startHeartbeat()
		var (
			summary       = objectStreamSummary{StructuredOutput: true}
			latestUsage   fantasy.Usage
			usageSeen     bool
			finishSeen    bool
			finishReason  fantasy.FinishReason
			rawTextLength int
			warnings      []normalizedWarning
			streamError   any
			streamStatus  = StatusCompleted
		)

		finalize := func(status Status) {
			if stop != nil {
				stop()
			}
			mu.Lock()
			defer mu.Unlock()
			if finalized {
				return
			}
			finalized = true
			close(heartbeatDone)

			summary.FinishReason = string(finishReason)

			resp := normalizedObjectResponsePayload{
				RawTextLength:    rawTextLength,
				FinishReason:     string(finishReason),
				Warnings:         warnings,
				StructuredOutput: true,
			}
			if usageSeen {
				resp.Usage = normalizeUsage(latestUsage)
			}

			var usage any
			if usageSeen {
				usage = &latestUsage
			}
			handle.finish(ctx, status, resp, usage, streamError, map[string]any{
				"structured_output": true,
				"stream_summary":    summary,
			})
		}

		if seq != nil {
			seq(func(part fantasy.ObjectStreamPart) bool {
				summary.PartCount++
				summary.WarningCount += len(part.Warnings)
				if len(part.Warnings) > 0 {
					warnings = append(warnings, normalizeWarnings(part.Warnings)...)
				}

				switch part.Type {
				case fantasy.ObjectStreamPartTypeObject:
					summary.ObjectPartCount++
				case fantasy.ObjectStreamPartTypeTextDelta:
					summary.TextDeltaCount++
					rawTextLength += utf8.RuneCountInString(part.Delta)
				case fantasy.ObjectStreamPartTypeFinish:
					finishReason = part.FinishReason
					latestUsage = part.Usage
					usageSeen = true
					finishSeen = true
					streamComplete.Store(true)
				}

				if part.Type == fantasy.ObjectStreamPartTypeError || part.Error != nil {
					summary.ErrorCount++
					if part.Error != nil {
						summary.LastError = part.Error.Error()
						streamError = normalizeError(ctx, part.Error)
					} else {
						summary.LastError = "stream error part with nil error"
						streamError = map[string]string{"error": "stream error part with nil error"}
					}
					streamStatus = streamErrorStatus(streamStatus, part.Error)
				}

				if !yield(part) {
					// Same as the regular stream wrapper: if a
					// finish part was already seen, the consumer
					// exited normally after completion.
					switch {
					case streamStatus == StatusError:
						finalize(StatusError)
					case finishSeen:
						finalize(StatusCompleted)
					default:
						finalize(StatusInterrupted)
					}
					return false
				}

				return true
			})
		}

		// Same as the regular stream wrapper: treat a
		// stream that ended without a finish part as
		// interrupted rather than falsely completed.
		if streamStatus == StatusCompleted && !finishSeen {
			streamStatus = StatusInterrupted
		}
		finalize(streamStatus)
	}
}

// --------------- helper functions ---------------

// normalizeMessages converts a fantasy.Prompt into a slice of
// normalizedMessage values with bounded part metadata.
func normalizeMessages(prompt fantasy.Prompt) []normalizedMessage {
	msgs := make([]normalizedMessage, 0, len(prompt))
	for _, m := range prompt {
		msgs = append(msgs, normalizedMessage{
			Role:  string(m.Role),
			Parts: normalizeMessageParts(m.Content),
		})
	}
	return msgs
}

// boundText truncates s to MaxMessagePartTextLength runes, appending
// an ellipsis if truncation occurs.
func boundText(s string) string {
	return stringutil.Truncate(s, MaxMessagePartTextLength, stringutil.TruncateWithEllipsis)
}

// safeMarshalJSON marshals value to JSON. On failure it returns a
// diagnostic error object rather than panicking, which is appropriate
// for debug telemetry where a marshal failure should not crash the
// caller.
func safeMarshalJSON(label string, value any) json.RawMessage {
	data, err := json.Marshal(value)
	if err != nil {
		fallback, fallbackErr := json.Marshal(map[string]string{
			"error": fmt.Sprintf("chatdebug: failed to marshal %s: %v", label, err),
		})
		if fallbackErr == nil {
			return append(json.RawMessage(nil), fallback...)
		}
		return json.RawMessage(`{"error":"chatdebug: failed to marshal value"}`)
	}
	return append(json.RawMessage(nil), data...)
}

func appendStreamContentText(
	content []normalizedContentPart,
	partType string,
	delta string,
	streamDebugBytes *int,
) []normalizedContentPart {
	if delta == "" {
		return content
	}

	remaining := maxStreamDebugTextBytes
	if streamDebugBytes != nil {
		remaining -= *streamDebugBytes
	}
	if remaining <= 0 {
		return content
	}
	if len(delta) > remaining {
		cut := 0
		for _, r := range delta {
			size := utf8.RuneLen(r)
			if size < 0 {
				size = 1
			}
			if cut+size > remaining {
				break
			}
			cut += size
		}
		delta = delta[:cut]
	}
	if delta == "" {
		return content
	}

	if len(content) == 0 || content[len(content)-1].Type != partType {
		content = append(content, normalizedContentPart{Type: partType})
	}
	last := &content[len(content)-1]
	last.Text += delta
	if streamDebugBytes != nil {
		*streamDebugBytes += len(delta)
	}
	return content
}

// appendStreamToolInput accumulates incremental tool-input deltas
// per tool call ID so that parallel or sequential tool invocations
// remain distinguishable in interrupted stream debug payloads.
func appendStreamToolInput(
	content []normalizedContentPart,
	part fantasy.StreamPart,
	streamDebugBytes *int,
) []normalizedContentPart {
	if part.Delta == "" {
		return content
	}

	remaining := maxStreamDebugTextBytes
	if streamDebugBytes != nil {
		remaining -= *streamDebugBytes
	}
	if remaining <= 0 {
		return content
	}
	delta := part.Delta
	if len(delta) > remaining {
		cut := 0
		for _, r := range delta {
			size := utf8.RuneLen(r)
			if size < 0 {
				size = 1
			}
			if cut+size > remaining {
				break
			}
			cut += size
		}
		delta = delta[:cut]
	}
	if delta == "" {
		return content
	}

	// Find the existing tool_input part for this specific tool call ID.
	// Scan backwards through all content; tool_input deltas for the
	// same call may be separated by text, reasoning, or source parts
	// when streams interleave multiple tool invocations.
	for i := len(content) - 1; i >= 0; i-- {
		if content[i].Type == "tool_input" && content[i].ToolCallID == part.ID {
			content[i].Arguments += delta
			if streamDebugBytes != nil {
				*streamDebugBytes += len(delta)
			}
			return content
		}
	}

	content = append(content, normalizedContentPart{
		Type:       "tool_input",
		ToolCallID: part.ID,
		ToolName:   part.ToolCallName,
		Arguments:  delta,
	})
	if streamDebugBytes != nil {
		*streamDebugBytes += len(delta)
	}
	return content
}

func canonicalContentType(partType string) string {
	switch partType {
	case string(fantasy.StreamPartTypeToolCall), string(fantasy.ContentTypeToolCall):
		return string(fantasy.ContentTypeToolCall)
	case string(fantasy.StreamPartTypeToolResult), string(fantasy.ContentTypeToolResult):
		return string(fantasy.ContentTypeToolResult)
	default:
		return partType
	}
}

func appendNormalizedStreamContent(
	content []normalizedContentPart,
	part fantasy.StreamPart,
	streamDebugBytes *int,
) []normalizedContentPart {
	switch part.Type {
	case fantasy.StreamPartTypeTextDelta:
		return appendStreamContentText(content, "text", part.Delta, streamDebugBytes)
	case fantasy.StreamPartTypeReasoningStart, fantasy.StreamPartTypeReasoningDelta:
		return appendStreamContentText(content, "reasoning", part.Delta, streamDebugBytes)
	case fantasy.StreamPartTypeToolInputStart,
		fantasy.StreamPartTypeToolInputDelta,
		fantasy.StreamPartTypeToolInputEnd:
		// Incremental tool input parts are emitted before the final
		// tool_call summary. Attribute each chunk to its tool call
		// so interrupted streams can reconstruct which partial input
		// belonged to which invocation.
		return appendStreamToolInput(content, part, streamDebugBytes)
	case fantasy.StreamPartTypeToolCall:
		return append(content, normalizedContentPart{
			Type:        canonicalContentType(string(part.Type)),
			ToolCallID:  part.ID,
			ToolName:    part.ToolCallName,
			Arguments:   boundText(part.ToolCallInput),
			InputLength: utf8.RuneCountInString(part.ToolCallInput),
		})
	case fantasy.StreamPartTypeToolResult:
		return append(content, normalizedContentPart{
			Type:       canonicalContentType(string(part.Type)),
			ToolCallID: part.ID,
			ToolName:   part.ToolCallName,
			Result:     boundText(part.ToolCallInput),
		})
	case fantasy.StreamPartTypeSource:
		return append(content, normalizedContentPart{
			Type:       string(part.Type),
			SourceType: string(part.SourceType),
			Title:      part.Title,
			URL:        part.URL,
		})
	default:
		return content
	}
}

func normalizeToolResultOutput(output fantasy.ToolResultOutputContent) string {
	switch v := output.(type) {
	case fantasy.ToolResultOutputContentText:
		return boundText(v.Text)
	case *fantasy.ToolResultOutputContentText:
		if v == nil {
			return ""
		}
		return boundText(v.Text)
	case fantasy.ToolResultOutputContentError:
		if v.Error == nil {
			return ""
		}
		return boundText(v.Error.Error())
	case *fantasy.ToolResultOutputContentError:
		if v == nil || v.Error == nil {
			return ""
		}
		return boundText(v.Error.Error())
	case fantasy.ToolResultOutputContentMedia:
		if v.Text != "" {
			return boundText(v.Text)
		}
		if v.MediaType == "" {
			return "[media output]"
		}
		return fmt.Sprintf("[media output: %s]", v.MediaType)
	case *fantasy.ToolResultOutputContentMedia:
		if v == nil {
			return ""
		}
		if v.Text != "" {
			return boundText(v.Text)
		}
		if v.MediaType == "" {
			return "[media output]"
		}
		return fmt.Sprintf("[media output: %s]", v.MediaType)
	default:
		if output == nil {
			return ""
		}
		return boundText(string(safeMarshalJSON("tool result output", output)))
	}
}

// isNilInterfaceValue reports whether v is nil or holds a nil pointer,
// map, slice, channel, or func.
func isNilInterfaceValue(v any) bool {
	if v == nil {
		return true
	}

	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Chan, reflect.Func, reflect.Map, reflect.Pointer, reflect.Slice:
		return rv.IsNil()
	default:
		return false
	}
}

// normalizeMessageParts extracts type and bounded metadata from each
// MessagePart. Text-like payloads are bounded to
// MaxMessagePartTextLength runes so the debug panel can display
// readable content.
func normalizeMessageParts(parts []fantasy.MessagePart) []normalizedMessagePart {
	result := make([]normalizedMessagePart, 0, len(parts))
	for _, p := range parts {
		if isNilInterfaceValue(p) {
			continue
		}
		np := normalizedMessagePart{
			Type: canonicalContentType(string(p.GetType())),
		}
		switch v := p.(type) {
		case fantasy.TextPart:
			np.Text = boundText(v.Text)
			np.TextLength = utf8.RuneCountInString(v.Text)
		case *fantasy.TextPart:
			np.Text = boundText(v.Text)
			np.TextLength = utf8.RuneCountInString(v.Text)
		case fantasy.ReasoningPart:
			np.Text = boundText(v.Text)
			np.TextLength = utf8.RuneCountInString(v.Text)
		case *fantasy.ReasoningPart:
			np.Text = boundText(v.Text)
			np.TextLength = utf8.RuneCountInString(v.Text)
		case fantasy.FilePart:
			np.Filename = v.Filename
			np.MediaType = v.MediaType
		case *fantasy.FilePart:
			np.Filename = v.Filename
			np.MediaType = v.MediaType
		case fantasy.ToolCallPart:
			np.ToolCallID = v.ToolCallID
			np.ToolName = v.ToolName
			np.Arguments = boundText(v.Input)
		case *fantasy.ToolCallPart:
			np.ToolCallID = v.ToolCallID
			np.ToolName = v.ToolName
			np.Arguments = boundText(v.Input)
		case fantasy.ToolResultPart:
			np.ToolCallID = v.ToolCallID
			np.Result = normalizeToolResultOutput(v.Output)
		case *fantasy.ToolResultPart:
			np.ToolCallID = v.ToolCallID
			np.Result = normalizeToolResultOutput(v.Output)
		}
		result = append(result, np)
	}
	return result
}

// normalizeTools converts the tool list into lightweight descriptors.
// Function tool schemas are preserved so the debug panel can render
// parameter details without re-fetching provider metadata.
func normalizeTools(tools []fantasy.Tool) []normalizedTool {
	if len(tools) == 0 {
		return nil
	}
	result := make([]normalizedTool, 0, len(tools))
	for _, t := range tools {
		if isNilInterfaceValue(t) {
			continue
		}
		nt := normalizedTool{
			Type: string(t.GetType()),
			Name: t.GetName(),
		}
		switch v := t.(type) {
		case fantasy.FunctionTool:
			nt.Description = v.Description
			nt.HasInputSchema = len(v.InputSchema) > 0
			if nt.HasInputSchema {
				nt.InputSchema = safeMarshalJSON(
					fmt.Sprintf("tool %q input schema", v.Name),
					v.InputSchema,
				)
			}
		case *fantasy.FunctionTool:
			nt.Description = v.Description
			nt.HasInputSchema = len(v.InputSchema) > 0
			if nt.HasInputSchema {
				nt.InputSchema = safeMarshalJSON(
					fmt.Sprintf("tool %q input schema", v.Name),
					v.InputSchema,
				)
			}
		case fantasy.ProviderDefinedTool:
			nt.ID = v.ID
		case *fantasy.ProviderDefinedTool:
			nt.ID = v.ID
		case fantasy.ExecutableProviderTool:
			nt.ID = v.Definition().ID
		case *fantasy.ExecutableProviderTool:
			nt.ID = v.Definition().ID
		}
		result = append(result, nt)
	}
	return result
}

// normalizeContentParts converts the response content into a slice
// of normalizedContentPart values. Text payloads are bounded to
// MaxMessagePartTextLength runes per part; tool-call arguments are
// similarly bounded. File data is never stored.
//
// Unlike the stream path which caps total accumulated text at
// maxStreamDebugTextBytes, the Generate path bounds each part
// individually. This is intentional: stream deltas are many small
// fragments that accumulate unboundedly, while Generate responses
// contain a fixed number of discrete content parts, each
// independently bounded by MaxMessagePartTextLength.
func normalizeContentParts(content fantasy.ResponseContent) []normalizedContentPart {
	result := make([]normalizedContentPart, 0, len(content))
	for _, c := range content {
		if isNilInterfaceValue(c) {
			continue
		}
		np := normalizedContentPart{
			Type: canonicalContentType(string(c.GetType())),
		}
		switch v := c.(type) {
		case fantasy.TextContent:
			np.Text = boundText(v.Text)
			np.TextLength = utf8.RuneCountInString(v.Text)
		case *fantasy.TextContent:
			np.Text = boundText(v.Text)
			np.TextLength = utf8.RuneCountInString(v.Text)
		case fantasy.ReasoningContent:
			np.Text = boundText(v.Text)
			np.TextLength = utf8.RuneCountInString(v.Text)
		case *fantasy.ReasoningContent:
			np.Text = boundText(v.Text)
			np.TextLength = utf8.RuneCountInString(v.Text)
		case fantasy.ToolCallContent:
			np.ToolCallID = v.ToolCallID
			np.ToolName = v.ToolName
			np.Arguments = boundText(v.Input)
			np.InputLength = utf8.RuneCountInString(v.Input)
		case *fantasy.ToolCallContent:
			np.ToolCallID = v.ToolCallID
			np.ToolName = v.ToolName
			np.Arguments = boundText(v.Input)
			np.InputLength = utf8.RuneCountInString(v.Input)
		case fantasy.FileContent:
			np.MediaType = v.MediaType
		case *fantasy.FileContent:
			np.MediaType = v.MediaType
		case fantasy.SourceContent:
			np.SourceType = string(v.SourceType)
			np.Title = v.Title
			np.URL = v.URL
		case *fantasy.SourceContent:
			np.SourceType = string(v.SourceType)
			np.Title = v.Title
			np.URL = v.URL
		case fantasy.ToolResultContent:
			np.ToolCallID = v.ToolCallID
			np.ToolName = v.ToolName
			np.Result = normalizeToolResultOutput(v.Result)
		case *fantasy.ToolResultContent:
			if v != nil {
				np.ToolCallID = v.ToolCallID
				np.ToolName = v.ToolName
				np.Result = normalizeToolResultOutput(v.Result)
			}
		}
		result = append(result, np)
	}
	return result
}

// normalizeUsage maps the full fantasy.Usage token breakdown into
// the debug-friendly normalizedUsage struct.
func normalizeUsage(u fantasy.Usage) normalizedUsage {
	return normalizedUsage{
		InputTokens:         u.InputTokens,
		OutputTokens:        u.OutputTokens,
		TotalTokens:         u.TotalTokens,
		ReasoningTokens:     u.ReasoningTokens,
		CacheCreationTokens: u.CacheCreationTokens,
		CacheReadTokens:     u.CacheReadTokens,
	}
}

// normalizeWarnings converts provider call warnings into their
// normalized form. Returns nil for empty input to keep JSON clean.
func normalizeWarnings(warnings []fantasy.CallWarning) []normalizedWarning {
	if len(warnings) == 0 {
		return nil
	}
	result := make([]normalizedWarning, 0, len(warnings))
	for _, w := range warnings {
		result = append(result, normalizedWarning{
			Type:    string(w.Type),
			Setting: w.Setting,
			Details: w.Details,
			Message: w.Message,
		})
	}
	return result
}

// --------------- normalize functions ---------------

func normalizeCall(call fantasy.Call) normalizedCallPayload {
	payload := normalizedCallPayload{
		Messages: normalizeMessages(call.Prompt),
		Tools:    normalizeTools(call.Tools),
		Options: normalizedCallOptions{
			MaxOutputTokens:  call.MaxOutputTokens,
			Temperature:      call.Temperature,
			TopP:             call.TopP,
			TopK:             call.TopK,
			PresencePenalty:  call.PresencePenalty,
			FrequencyPenalty: call.FrequencyPenalty,
		},
		ProviderOptionCount: len(call.ProviderOptions),
	}
	if call.ToolChoice != nil {
		payload.ToolChoice = string(*call.ToolChoice)
	}
	return payload
}

func normalizeObjectCall(call fantasy.ObjectCall) normalizedObjectCallPayload {
	return normalizedObjectCallPayload{
		Messages: normalizeMessages(call.Prompt),
		Options: normalizedCallOptions{
			MaxOutputTokens:  call.MaxOutputTokens,
			Temperature:      call.Temperature,
			TopP:             call.TopP,
			TopK:             call.TopK,
			PresencePenalty:  call.PresencePenalty,
			FrequencyPenalty: call.FrequencyPenalty,
		},
		SchemaName:          call.SchemaName,
		SchemaDescription:   call.SchemaDescription,
		StructuredOutput:    true,
		ProviderOptionCount: len(call.ProviderOptions),
	}
}

func normalizeResponse(resp *fantasy.Response) normalizedResponsePayload {
	if resp == nil {
		return normalizedResponsePayload{}
	}

	return normalizedResponsePayload{
		Content:      normalizeContentParts(resp.Content),
		FinishReason: string(resp.FinishReason),
		Usage:        normalizeUsage(resp.Usage),
		Warnings:     normalizeWarnings(resp.Warnings),
	}
}

func normalizeObjectResponse(resp *fantasy.ObjectResponse) normalizedObjectResponsePayload {
	if resp == nil {
		return normalizedObjectResponsePayload{StructuredOutput: true}
	}

	return normalizedObjectResponsePayload{
		RawTextLength:    utf8.RuneCountInString(resp.RawText),
		FinishReason:     string(resp.FinishReason),
		Usage:            normalizeUsage(resp.Usage),
		Warnings:         normalizeWarnings(resp.Warnings),
		StructuredOutput: true,
	}
}

func streamErrorStatus(current Status, err error) Status {
	if current == StatusError {
		return current
	}
	if err == nil {
		return StatusError
	}
	return stepStatusForError(err)
}

func stepStatusForError(err error) Status {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return StatusInterrupted
	}
	return StatusError
}

func normalizeError(ctx context.Context, err error) normalizedErrorPayload {
	payload := normalizedErrorPayload{}
	if err == nil {
		return payload
	}

	payload.Message = err.Error()
	payload.Type = fmt.Sprintf("%T", err)
	if ctxErr := ctx.Err(); ctxErr != nil {
		payload.ContextError = ctxErr.Error()
	}

	var providerErr *fantasy.ProviderError
	if errors.As(err, &providerErr) {
		payload.ProviderTitle = providerErr.Title
		payload.ProviderStatus = providerErr.StatusCode
		payload.IsRetryable = providerErr.IsRetryable()
	}

	return payload
}
