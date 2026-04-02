package chatdebug

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"reflect"
	"strings"
	"sync"
	"unicode/utf8"

	"charm.land/fantasy"
	"golang.org/x/xerrors"
)

type debugModel struct {
	inner fantasy.LanguageModel
	svc   *Service
	opts  RecorderOptions
}

var _ fantasy.LanguageModel = (*debugModel)(nil)

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
// Text is stored in full (the UI needs it), tool-call arguments are
// stored in bounded form while retaining their original length, and
// file data is never stored.
type normalizedContentPart struct {
	Type        string `json:"type"`
	Text        string `json:"text,omitempty"`
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

	resp, err := d.inner.Generate(enrichedCtx, call)
	if err != nil {
		handle.finish(ctx, stepStatusForError(err), nil, nil, normalizeError(ctx, err), nil)
		return nil, err
	}
	if resp == nil {
		err = xerrors.New("chatdebug: language model returned nil response")
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

	resp, err := d.inner.GenerateObject(enrichedCtx, call)
	if err != nil {
		handle.finish(ctx, stepStatusForError(err), nil, nil, normalizeError(ctx, err),
			map[string]any{"structured_output": true})
		return nil, err
	}
	if resp == nil {
		err = xerrors.New("chatdebug: language model returned nil object response")
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

	return wrapObjectStreamSeq(ctx, handle, seq), nil
}

func (d *debugModel) Provider() string {
	return d.inner.Provider()
}

func (d *debugModel) Model() string {
	return d.inner.Model()
}

func appendStreamDebugText(buf *strings.Builder, text string) {
	remaining := maxStreamDebugTextBytes - buf.Len()
	if remaining <= 0 || text == "" {
		return
	}
	if len(text) > remaining {
		cut := 0
		for _, r := range text {
			size := utf8.RuneLen(r)
			if size < 0 {
				size = 1
			}
			if cut+size > remaining {
				break
			}
			cut += size
		}
		text = text[:cut]
	}
	_, _ = buf.WriteString(text)
}

func wrapStreamSeq(
	ctx context.Context,
	handle *stepHandle,
	seq iter.Seq[fantasy.StreamPart],
) fantasy.StreamResponse {
	return func(yield func(fantasy.StreamPart) bool) {
		var (
			summary      streamSummary
			latestUsage  fantasy.Usage
			usageSeen    bool
			finishReason fantasy.FinishReason
			textBuf      strings.Builder
			reasoningBuf strings.Builder
			toolCalls    []normalizedContentPart
			sources      []normalizedContentPart
			warnings     []normalizedWarning
			streamError  any
			streamStatus = StatusCompleted
			once         sync.Once
		)

		finalize := func(status Status) {
			once.Do(func() {
				summary.FinishReason = string(finishReason)

				var content []normalizedContentPart
				if text := textBuf.String(); text != "" {
					content = append(content, normalizedContentPart{
						Type: "text",
						Text: text,
					})
				}
				if reasoning := reasoningBuf.String(); reasoning != "" {
					content = append(content, normalizedContentPart{
						Type: "reasoning",
						Text: reasoning,
					})
				}
				content = append(content, toolCalls...)
				content = append(content, sources...)

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
					appendStreamDebugText(&textBuf, part.Delta)
				case fantasy.StreamPartTypeReasoningDelta:
					appendStreamDebugText(&reasoningBuf, part.Delta)
				case fantasy.StreamPartTypeToolCall:
					summary.ToolCallCount++
					toolCalls = append(toolCalls, normalizedContentPart{
						Type:        "tool-call",
						ToolCallID:  part.ID,
						ToolName:    part.ToolCallName,
						Arguments:   boundText(part.ToolCallInput),
						InputLength: len(part.ToolCallInput),
					})
				case fantasy.StreamPartTypeToolResult:
					toolCalls = append(toolCalls, normalizedContentPart{
						Type:       "tool-result",
						ToolCallID: part.ID,
						ToolName:   part.ToolCallName,
						Result:     boundText(part.ToolCallInput),
					})
				case fantasy.StreamPartTypeSource:
					summary.SourceCount++
					sources = append(sources, normalizedContentPart{
						Type:       string(part.Type),
						SourceType: string(part.SourceType),
						Title:      part.Title,
						URL:        part.URL,
					})
				case fantasy.StreamPartTypeFinish:
					finishReason = part.FinishReason
					latestUsage = part.Usage
					usageSeen = true
				}

				if part.Type == fantasy.StreamPartTypeError || part.Error != nil {
					summary.ErrorCount++
					streamStatus = StatusError
					if part.Error != nil {
						summary.LastError = part.Error.Error()
						streamError = normalizeError(ctx, part.Error)
					}
				}

				if !yield(part) {
					if streamStatus == StatusError {
						finalize(StatusError)
					} else {
						finalize(StatusInterrupted)
					}
					return false
				}

				return true
			})
		}

		if streamStatus == StatusCompleted && ctx.Err() != nil {
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
	return func(yield func(fantasy.ObjectStreamPart) bool) {
		var (
			summary      = objectStreamSummary{StructuredOutput: true}
			latestUsage  fantasy.Usage
			usageSeen    bool
			finishReason fantasy.FinishReason
			textBuf      strings.Builder
			warnings     []normalizedWarning
			streamError  any
			streamStatus = StatusCompleted
			once         sync.Once
		)

		finalize := func(status Status) {
			once.Do(func() {
				summary.FinishReason = string(finishReason)

				var content []normalizedContentPart
				if text := textBuf.String(); text != "" {
					content = append(content, normalizedContentPart{
						Type: "text",
						Text: text,
					})
				}

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
					"structured_output": true,
					"stream_summary":    summary,
				})
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
					appendStreamDebugText(&textBuf, part.Delta)
				case fantasy.ObjectStreamPartTypeFinish:
					finishReason = part.FinishReason
					latestUsage = part.Usage
					usageSeen = true
				}

				if part.Type == fantasy.ObjectStreamPartTypeError || part.Error != nil {
					summary.ErrorCount++
					streamStatus = StatusError
					if part.Error != nil {
						summary.LastError = part.Error.Error()
						streamError = normalizeError(ctx, part.Error)
					}
				}

				if !yield(part) {
					if streamStatus == StatusError {
						finalize(StatusError)
					} else {
						finalize(StatusInterrupted)
					}
					return false
				}

				return true
			})
		}

		if streamStatus == StatusCompleted && ctx.Err() != nil {
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
	if utf8.RuneCountInString(s) <= MaxMessagePartTextLength {
		return s
	}
	runes := []rune(s)
	return string(runes[:MaxMessagePartTextLength]) + "…"
}

func mustMarshalJSON(label string, value any) json.RawMessage {
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
		return boundText(string(mustMarshalJSON("tool result output", output)))
	}
}

// normalizeMessageParts extracts type and bounded metadata from each
// MessagePart. Text-like payloads are bounded to
// MaxMessagePartTextLength runes so the debug panel can display
// readable content.
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

func normalizeMessageParts(parts []fantasy.MessagePart) []normalizedMessagePart {
	result := make([]normalizedMessagePart, 0, len(parts))
	for _, p := range parts {
		if isNilInterfaceValue(p) {
			continue
		}
		np := normalizedMessagePart{
			Type: string(p.GetType()),
		}
		switch v := p.(type) {
		case fantasy.TextPart:
			np.Text = boundText(v.Text)
			np.TextLength = len(v.Text)
		case *fantasy.TextPart:
			np.Text = boundText(v.Text)
			np.TextLength = len(v.Text)
		case fantasy.ReasoningPart:
			np.Text = boundText(v.Text)
			np.TextLength = len(v.Text)
		case *fantasy.ReasoningPart:
			np.Text = boundText(v.Text)
			np.TextLength = len(v.Text)
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
				nt.InputSchema = mustMarshalJSON(
					fmt.Sprintf("tool %q input schema", v.Name),
					v.InputSchema,
				)
			}
		case *fantasy.FunctionTool:
			nt.Description = v.Description
			nt.HasInputSchema = len(v.InputSchema) > 0
			if nt.HasInputSchema {
				nt.InputSchema = mustMarshalJSON(
					fmt.Sprintf("tool %q input schema", v.Name),
					v.InputSchema,
				)
			}
		case fantasy.ProviderDefinedTool:
			nt.ID = v.ID
		case *fantasy.ProviderDefinedTool:
			nt.ID = v.ID
		}
		result = append(result, nt)
	}
	return result
}

// normalizeContentParts converts the response content into a slice
// of normalizedContentPart values. Text is stored in full (needed
// by the UI); tool-call arguments are stored in bounded form while
// preserving their original length; file data is never stored.
func normalizeContentParts(content fantasy.ResponseContent) []normalizedContentPart {
	result := make([]normalizedContentPart, 0, len(content))
	for _, c := range content {
		if isNilInterfaceValue(c) {
			continue
		}
		np := normalizedContentPart{
			Type: string(c.GetType()),
		}
		switch v := c.(type) {
		case fantasy.TextContent:
			np.Text = v.Text
		case *fantasy.TextContent:
			np.Text = v.Text
		case fantasy.ReasoningContent:
			np.Text = v.Text
		case *fantasy.ReasoningContent:
			np.Text = v.Text
		case fantasy.ToolCallContent:
			np.ToolCallID = v.ToolCallID
			np.ToolName = v.ToolName
			np.Arguments = boundText(v.Input)
			np.InputLength = len(v.Input)
		case *fantasy.ToolCallContent:
			np.ToolCallID = v.ToolCallID
			np.ToolName = v.ToolName
			np.Arguments = boundText(v.Input)
			np.InputLength = len(v.Input)
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
		RawTextLength:    len(resp.RawText),
		FinishReason:     string(resp.FinishReason),
		Usage:            normalizeUsage(resp.Usage),
		Warnings:         normalizeWarnings(resp.Warnings),
		StructuredOutput: true,
	}
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
