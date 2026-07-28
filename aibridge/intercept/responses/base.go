package responses

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared/constant"
	"github.com/tidwall/gjson"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	aibcontext "github.com/coder/coder/v2/aibridge/context"
	"github.com/coder/coder/v2/aibridge/intercept"
	"github.com/coder/coder/v2/aibridge/intercept/apidump"
	"github.com/coder/coder/v2/aibridge/keypool"
	"github.com/coder/coder/v2/aibridge/mcp"
	"github.com/coder/coder/v2/aibridge/recorder"
	"github.com/coder/coder/v2/aibridge/tracing"
	"github.com/coder/quartz"
)

const (
	requestTimeout = time.Second * 600
)

type responsesInterceptionBase struct {
	id         uuid.UUID
	reqPayload RequestPayload

	cfg  intercept.Config
	cred intercept.Credential

	// clientHeaders are the original HTTP headers from the client request.
	clientHeaders http.Header

	logger slog.Logger
	tracer trace.Tracer

	recorder recorder.Recorder
	mcpProxy mcp.ServerProxier
}

// newResponsesService builds the SDK service used for upstream calls.
func (i *responsesInterceptionBase) newResponsesService(ctx context.Context) responses.ResponseService {
	var opts []option.RequestOption
	// Only BYOK sets its credential here. Centralized keys are injected
	// per-attempt in the failover loop.
	if byok, ok := intercept.AsBYOK(i.cred); ok {
		i.logger.Debug(ctx, "using byok auth",
			slog.F("auth_header", byok.Header), slog.F("key_hint", byok.Hint()),
		)
		opts = append(opts, option.WithAPIKey(byok.Secret))
	}
	opts = append(opts, option.WithBaseURL(i.cfg.BaseURL))

	// Forward client headers to upstream. This middleware runs after the SDK
	// has built the request, and replaces the outgoing headers with the sanitized
	// client headers plus provider auth.
	if i.clientHeaders != nil {
		opts = append(opts, option.WithMiddleware(func(req *http.Request, next option.MiddlewareNext) (*http.Response, error) {
			req.Header = intercept.BuildUpstreamHeaders(req.Header, i.clientHeaders, i.cred.AuthHeader())
			return next(req)
		}))
	}

	// Add API dump middleware if configured
	if mw := apidump.NewBridgeMiddleware(i.cfg.APIDumpDir, i.cfg.ProviderName, i.Model(), i.id, i.logger, quartz.NewReal()); mw != nil {
		opts = append(opts, option.WithMiddleware(mw))
	}

	return responses.NewResponseService(opts...)
}

func (i *responsesInterceptionBase) ID() uuid.UUID {
	return i.id
}

func (i *responsesInterceptionBase) Credential() intercept.Credential {
	return i.cred
}

func (i *responsesInterceptionBase) Setup(logger slog.Logger, rec recorder.Recorder, mcpProxy mcp.ServerProxier) {
	i.logger = logger.With(slog.F("model", i.Model()))
	i.recorder = rec
	i.mcpProxy = mcpProxy
}

func (i *responsesInterceptionBase) Model() string {
	return i.reqPayload.model()
}

func (i *responsesInterceptionBase) CorrelatingToolCallID() *string {
	return i.reqPayload.correlatingToolCallID()
}

func (i *responsesInterceptionBase) baseTraceAttributes(r *http.Request, streaming bool) []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.String(tracing.RequestPath, r.URL.Path),
		attribute.String(tracing.InterceptionID, i.id.String()),
		attribute.String(tracing.InitiatorID, aibcontext.ActorIDFromContext(r.Context())),
		attribute.String(tracing.Provider, i.cfg.ProviderName),
		attribute.String(tracing.Model, i.Model()),
		attribute.Bool(tracing.Streaming, streaming),
	}
}

func (i *responsesInterceptionBase) validateRequest(ctx context.Context, w http.ResponseWriter) error {
	if i.reqPayload.background() {
		err := xerrors.New("background requests are currently not supported by AI Gateway")
		i.sendCustomErr(ctx, w, http.StatusNotImplemented, err)
		return err
	}

	return nil
}

// writeUpstreamError marshals and writes a given error.
func (i *responsesInterceptionBase) writeUpstreamError(w http.ResponseWriter, oaiErr *intercept.ResponseError) {
	if oaiErr == nil {
		return
	}

	w.Header().Set("Content-Type", "application/json")
	// Set Retry-After when a cooldown is configured.
	if oaiErr.RetryAfter > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(int(math.Ceil(oaiErr.RetryAfter.Seconds()))))
	}
	w.WriteHeader(oaiErr.StatusCode)

	out, err := json.Marshal(oaiErr)
	if err != nil {
		i.logger.Warn(context.Background(), "failed to marshal upstream error", slog.Error(err), slog.F("error_payload", fmt.Sprintf("%+v", oaiErr)))
		// Response has to match expected format.
		_, _ = w.Write([]byte(`{
	"error": {
		"type": "error",
		"message":"error marshaling upstream error",
		"code": "server_error"
	}
}`))
	} else {
		_, _ = w.Write(out)
	}
}

// For centralized requests, markKeyOnError extracts an OpenAI
// SDK error from err and marks the key based on its status
// code. Returns true if the status was a key-specific failover
// trigger so callers can retry with the next key.
func (i *responsesInterceptionBase) markKeyOnError(ctx context.Context, key *keypool.Key, err error) bool {
	cp, ok := intercept.AsCentralizedPool(i.cred)
	if !ok {
		return false
	}
	var apiErr *openai.Error
	if !errors.As(err, &apiErr) {
		return false
	}
	return cp.Pool.MarkKeyOnStatus(
		ctx, key, apiErr.Response, i.logger,
	)
}

// sendCustomErr sends custom responses.Error error to the client
// it should only be called before any data is sent back to the client
func (i *responsesInterceptionBase) sendCustomErr(ctx context.Context, w http.ResponseWriter, code int, err error) {
	// Same JSON shape as responses.Error but using a plain struct because
	// responses.Error embeds *http.Request whose GetBody func field
	// is not JSON-marshalable (SA1026).
	respErr := struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}{
		Code:    strconv.Itoa(code),
		Message: err.Error(),
	}
	if b, err := json.Marshal(respErr); err != nil {
		i.logger.Warn(ctx, "failed to marshal custom error: ", slog.Error(err))
	} else {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		if _, err := w.Write(b); err != nil {
			i.logger.Warn(ctx, "failed to send custom error: ", slog.Error(err))
		}
	}
}

func (i *responsesInterceptionBase) requestOptions(respCopy *responseCopier) []option.RequestOption {
	opts := []option.RequestOption{
		// Sends original payload to solve json re-encoding issues
		// eg. Codex CLI produces requests without ID set in reasoning items: https://platform.openai.com/docs/api-reference/responses/create#responses_create-input-input_item_list-item-reasoning-id
		// when re-encoded, ID field is set to empty string which results
		// in bad request while not sending ID field at all somehow works.
		option.WithRequestBody("application/json", []byte(i.reqPayload)),

		// copyMiddleware copies body of original response body to the buffer in responseCopier,
		// also reference to headers and status code is kept responseCopier.
		// responseCopier is used by interceptors to forward response as it was received,
		// eliminating any possibility of JSON re-encoding issues.
		option.WithMiddleware(respCopy.copyMiddleware),
	}
	if !i.reqPayload.Stream() {
		opts = append(opts, option.WithRequestTimeout(requestTimeout))
	}
	return opts
}

func (i *responsesInterceptionBase) recordUserPrompt(ctx context.Context, responseID string, prompt string) {
	if responseID == "" {
		i.logger.Warn(ctx, "got empty response ID, skipping prompt recording")
		return
	}

	promptUsage := &recorder.PromptUsageRecord{
		InterceptionID: i.ID().String(),
		MsgID:          responseID,
		Prompt:         prompt,
	}
	if err := i.recorder.RecordPromptUsage(ctx, promptUsage); err != nil {
		i.logger.Warn(ctx, "failed to record prompt usage", slog.Error(err))
	}
}

func (i *responsesInterceptionBase) recordModelThoughts(ctx context.Context, response *responses.Response) {
	for _, t := range i.extractModelThoughts(response) {
		_ = i.recorder.RecordModelThought(ctx, &recorder.ModelThoughtRecord{
			InterceptionID: i.ID().String(),
			Content:        t.Content,
			Metadata:       t.Metadata,
		})
	}
}

func (i *responsesInterceptionBase) recordNonInjectedToolUsage(ctx context.Context, response *responses.Response) {
	if response == nil {
		i.logger.Warn(ctx, "got empty response, skipping tool usage recording")
		return
	}

	for _, item := range response.Output {
		var args recorder.ToolArgs

		// Whitelist the output item types that represent tool calls. Every
		// other output type (message, reasoning, *_output, etc.) is skipped.
		// Only function_call and custom_tool_call carry arguments we parse;
		// the remaining built-in tool calls are recorded for visibility but
		// have no uniform argument representation.
		switch item.Type {
		case string(constant.ValueOf[constant.FunctionCall]()):
			args = i.parseFunctionCallJSONArgs(ctx, item.Arguments)
		case string(constant.ValueOf[constant.CustomToolCall]()):
			args = item.Input
		case string(constant.ValueOf[constant.WebSearchCall]()),
			// computer_call has no SDK constant; only computer_call_output does.
			"computer_call",
			string(constant.ValueOf[constant.LocalShellCall]()),
			string(constant.ValueOf[constant.ShellCall]()),
			string(constant.ValueOf[constant.ApplyPatchCall]()),
			string(constant.ValueOf[constant.CodeInterpreterCall]()),
			string(constant.ValueOf[constant.McpCall]()),
			string(constant.ValueOf[constant.FileSearchCall]()),
			string(constant.ValueOf[constant.ImageGenerationCall]()):
			// Built-in tool calls carry no uniform argument payload.
		default:
			continue
		}

		// Built-in tools usually have no name, so fall back to the type.
		toolName := item.Name
		if toolName == "" {
			toolName = item.Type
		}

		if err := i.recorder.RecordToolUsage(ctx, &recorder.ToolUsageRecord{
			InterceptionID: i.ID().String(),
			MsgID:          response.ID,
			// ItemID is always present; ToolCallID (call_id) is empty for
			// hosted tools that the provider executes internally.
			ItemID:     item.ID,
			ToolCallID: item.CallID,
			Tool:       toolName,
			Args:       args,
			Injected:   false,
		}); err != nil {
			i.logger.Warn(ctx, "failed to record tool usage", slog.Error(err), slog.F("tool", toolName))
		}
	}
}

func (i *responsesInterceptionBase) parseFunctionCallJSONArgs(ctx context.Context, raw string) recorder.ToolArgs {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return trimmed
	}
	var args recorder.ToolArgs
	if err := json.Unmarshal([]byte(trimmed), &args); err != nil {
		i.logger.Warn(ctx, "failed to unmarshal tool args", slog.Error(err))
		return trimmed
	}
	return args
}

func (i *responsesInterceptionBase) recordTokenUsage(ctx context.Context, response *responses.Response) {
	if response == nil {
		i.logger.Warn(ctx, "got empty response, skipping token usage recording")
		return
	}

	usage := response.Usage

	// Keeping logic consistent with chat completions
	// Input *includes* the cached tokens, so we subtract them here to reflect actual input token usage.
	inputNonCacheTokens := max(0, usage.InputTokens-usage.InputTokensDetails.CachedTokens)

	if err := i.recorder.RecordTokenUsage(ctx, &recorder.TokenUsageRecord{
		InterceptionID:       i.ID().String(),
		MsgID:                response.ID,
		Input:                inputNonCacheTokens,
		Output:               usage.OutputTokens,
		CacheReadInputTokens: usage.InputTokensDetails.CachedTokens,
		ExtraTokenTypes: map[string]int64{
			"output_reasoning": usage.OutputTokensDetails.ReasoningTokens,
			"total_tokens":     usage.TotalTokens,
		},
	}); err != nil {
		i.logger.Warn(ctx, "failed to record token usage", slog.Error(err))
	}
}

// extractModelThoughts extracts model thoughts from response output items.
// It captures both reasoning summary items and commentary messages (message
// output items with "phase": "commentary") as model thoughts.
func (*responsesInterceptionBase) extractModelThoughts(response *responses.Response) []*recorder.ModelThoughtRecord {
	if response == nil {
		return nil
	}

	var thoughts []*recorder.ModelThoughtRecord
	for _, item := range response.Output {
		switch item.Type {
		case string(constant.ValueOf[constant.Reasoning]()):
			reasoning := item.AsReasoning()
			for _, summary := range reasoning.Summary {
				if summary.Text == "" {
					continue
				}
				thoughts = append(thoughts, &recorder.ModelThoughtRecord{
					Content:  summary.Text,
					Metadata: recorder.Metadata{"source": recorder.ThoughtSourceReasoningSummary},
				})
			}

		case string(constant.ValueOf[constant.Message]()):
			// The API sometimes returns commentary messages instead of reasoning
			// summaries. These are assistant message output items with "phase": "commentary".
			// The SDK doesn't expose a Phase field, so we extract it from raw JSON.
			// TODO: revisit when the OpenAI SDK adds a proper Phase field.
			raw := item.RawJSON()
			if gjson.Get(raw, "role").String() != string(constant.ValueOf[constant.Assistant]()) ||
				gjson.Get(raw, "phase").String() != "commentary" {
				continue
			}
			msg := item.AsMessage()
			for _, part := range msg.Content {
				if part.Type != string(constant.ValueOf[constant.OutputText]()) {
					continue
				}
				if part.Text == "" {
					continue
				}
				thoughts = append(thoughts, &recorder.ModelThoughtRecord{
					Content:  part.Text,
					Metadata: recorder.Metadata{"source": recorder.ThoughtSourceCommentary},
				})
			}
		}
	}

	return thoughts
}

func (i *responsesInterceptionBase) hasInjectableTools() bool {
	return i.mcpProxy != nil && len(i.mcpProxy.ListTools()) > 0
}

// responseCopier helper struct to send original response to the client
type responseCopier struct {
	buff            deltaBuffer
	responseStatus  int
	responseHeaders http.Header

	// responseBody keeps reference to original ReadCloser.
	// TeeReader in copyMiddleware copies read bytes from
	// response body (read by SDK) to the buffer. In case
	// SDK doesns't read everything readAll method reads from
	// this closer to makes sure whole response body is in the buffer.
	responseBody io.ReadCloser

	// responseReceived flag is used to determine if AI Gateway needs to write custom error:
	// - If responseReceived is true, the upstream response is forwarded as-is.
	// - If responseReceived is false, no response was returned and there is nothing to forward (eg. connection/client error). Custom error will be returned.
	responseReceived atomic.Bool
}

func (r *responseCopier) copyMiddleware(req *http.Request, next option.MiddlewareNext) (*http.Response, error) {
	resp, err := next(req)
	if err != nil || resp == nil {
		return resp, err
	}

	r.responseReceived.Store(true)
	r.responseStatus = resp.StatusCode
	r.responseHeaders = resp.Header
	resp.Body = io.NopCloser(io.TeeReader(resp.Body, &r.buff))
	r.responseBody = resp.Body
	return resp, nil
}

// readAll reads all data from resp.Body returned by so TeeReader
// so it appends all read data to the buffer and returns buffer contents.
func (r *responseCopier) readAll() ([]byte, error) {
	if r.responseBody == nil {
		return []byte{}, nil
	}

	_, err := io.ReadAll(r.responseBody)
	return r.buff.readDelta(), err
}

// forwardResp writes whole response as received to ResponseWriter
func (r *responseCopier) forwardResp(w http.ResponseWriter) error {
	// no response was received, nothing to forward
	if !r.responseReceived.Load() {
		return nil
	}

	w.Header().Set("Content-Type", r.responseHeaders.Get("Content-Type"))
	// Preserve the upstream retry-after header so clients can honor it on
	// rate-limited or unavailable responses.
	if retryAfter := r.responseHeaders.Get("Retry-After"); retryAfter != "" {
		w.Header().Set("Retry-After", retryAfter)
	}
	w.WriteHeader(r.responseStatus)

	b, err := r.readAll()
	if err != nil {
		return xerrors.Errorf("failed to read response body: %w", err)
	}

	if _, err := w.Write(b); err != nil {
		return xerrors.Errorf("failed to write response body: %w", err)
	}
	return nil
}

// deltaBuffer is a thread safe byte buffer
// supports reading incremental data (added after last read)
type deltaBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (d *deltaBuffer) Write(p []byte) (int, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.buf.Write(p)
}

// readDelta returns only the bytes appended
// after the last readDelta call.
func (d *deltaBuffer) readDelta() []byte {
	d.mu.Lock()
	defer d.mu.Unlock()

	b := bytes.Clone(d.buf.Bytes())
	d.buf.Reset()
	return b
}
