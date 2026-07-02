package chatcompletions

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/packages/ssestream"
	"github.com/tidwall/sjson"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	aibcontext "github.com/coder/coder/v2/aibridge/context"
	"github.com/coder/coder/v2/aibridge/intercept"
	"github.com/coder/coder/v2/aibridge/intercept/eventstream"
	"github.com/coder/coder/v2/aibridge/keypool"
	"github.com/coder/coder/v2/aibridge/mcp"
	"github.com/coder/coder/v2/aibridge/recorder"
	"github.com/coder/coder/v2/aibridge/tracing"
	"github.com/coder/quartz"
)

const (
	upstreamEmptyStreamMessage     = "The AI provider did not return a response. Try again or contact your administrator if this persists."
	upstreamMalformedStreamMessage = "The AI provider returned an invalid response. Try again or contact your administrator if this persists."
)

type StreamingInterception struct {
	interceptionBase
}

func NewStreamingInterceptor(
	id uuid.UUID,
	req *ChatCompletionNewParamsWrapper,
	cfg intercept.Config,
	cred intercept.Credential,
	clientHeaders http.Header,
	tracer trace.Tracer,
) *StreamingInterception {
	return &StreamingInterception{interceptionBase: interceptionBase{
		id:            id,
		req:           req,
		cfg:           cfg,
		cred:          cred,
		clientHeaders: clientHeaders,
		tracer:        tracer,
	}}
}

func (i *StreamingInterception) Setup(logger slog.Logger, rec recorder.Recorder, mcpProxy mcp.ServerProxier) {
	i.interceptionBase.Setup(logger.Named("streaming"), rec, mcpProxy)
}

func (*StreamingInterception) Streaming() bool {
	return true
}

func (i *StreamingInterception) TraceAttributes(r *http.Request) []attribute.KeyValue {
	return i.interceptionBase.baseTraceAttributes(r, true)
}

// ProcessRequest handles a request to /v1/chat/completions.
// See https://platform.openai.com/docs/api-reference/chat-streaming/streaming.
//
// It will inject any tools which have been provided by the [mcp.ServerProxier].
//
// When a response from the server includes an event indicating that a tool must be invoked, a conditional
// flow takes place:
//
// a) if the tool is not injected (i.e. defined by the client), relay the event unmodified
// b) if the tool is injected, it will be invoked by the [mcp.ServerProxier] in the remote MCP server, and its
// results relayed to the SERVER. The response from the server will be handled synchronously, and this loop
// can continue until all injected tool invocations are completed and the response is relayed to the client.
func (i *StreamingInterception) ProcessRequest(w http.ResponseWriter, r *http.Request) (outErr error) {
	if i.req == nil {
		return xerrors.New("developer error: req is nil")
	}

	ctx, span := i.tracer.Start(r.Context(), "Intercept.ProcessRequest", trace.WithAttributes(tracing.InterceptionAttributesFromContext(r.Context())...))
	defer tracing.EndSpanErr(span, &outErr)

	// Include token usage.
	i.req.StreamOptions.IncludeUsage = openai.Bool(true)

	i.injectTools()

	// Allow us to interrupt watch via cancel.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	r = r.WithContext(ctx) // Rewire context for SSE cancellation.

	svc := i.newCompletionsService(ctx)
	logger := i.logger.With(slog.F("model", i.req.Model))

	streamCtx, streamCancel := context.WithCancelCause(ctx)
	defer streamCancel(xerrors.New("deferred"))

	// events will either terminate when shutdown after interaction with upstream completes, or when streamCtx is done.
	events := eventstream.NewEventStream(streamCtx, logger.Named("sse-sender"), nil, quartz.NewReal())
	go events.Start(w, r)
	defer func() {
		_ = events.Shutdown(streamCtx) // Catch-all in case it doesn't get shutdown after stream completes.
	}()

	// Force responses to only have one choice.
	// It's unnecessary to generate multiple responses, and would complicate our stream processing logic if
	// multiple choices were returned.
	i.req.N = openai.Int(1)

	prompt, err := i.req.lastUserPrompt()
	if err != nil {
		logger.Warn(ctx, "failed to retrieve last user prompt", slog.Error(err))
	}

	var (
		stream          *ssestream.Stream[openai.ChatCompletionChunk]
		lastErr         error
		interceptionErr error
	)

	// Sum the key attempts across all iterations and record once when the
	// interception completes.
	var totalKeyAttempts int
	if cp, ok := intercept.AsCentralizedPool(i.cred); ok {
		defer func() {
			cp.Pool.RecordAttempts(totalKeyAttempts)
		}()
	}

	for {
		// TODO add outer loop span (https://github.com/coder/aibridge/issues/67)

		// Per-iteration: a pool credential advances its failover walker. An
		// iteration is either an agentic continuation or a failover retry after
		// the previous key was marked. BYOK has no pool and runs as a single
		// attempt.
		var opts []option.RequestOption
		var currentPoolKey *keypool.Key
		if cp, isPool := intercept.AsCentralizedPool(i.cred); isPool {
			walker := cp.Pool.Walker()
			key, keyPoolErr := cp.NextKey(walker)
			if keyPoolErr != nil {
				// Pool exhausted in this iteration. Relay the error to the
				// client: as an SSE event if events have already been sent,
				// or by direct write otherwise.
				respErr := intercept.ResponseErrorFromKeyPool(keyPoolErr)
				interceptionErr = respErr
				if events.IsStreaming() {
					payload, mErr := i.marshalErr(respErr)
					if mErr != nil {
						logger.Warn(ctx, "failed to marshal exhaustion error", slog.Error(mErr))
					} else if sErr := events.Send(streamCtx, payload); sErr != nil {
						logger.Warn(ctx, "failed to relay exhaustion error", slog.Error(sErr))
					}
				} else {
					i.writeUpstreamError(w, respErr)
				}
				break
			}

			logger.Debug(intercept.WithCredentialInfo(ctx, i.cred), "using centralized api key")
			currentPoolKey = key
			opts = append(opts,
				option.WithAPIKey(key.Value()),
				// Disable SDK retries because the failover loop handles
				// retries via key rotation.
				option.WithMaxRetries(0),
			)
			totalKeyAttempts += walker.Attempts()
		}

		// TODO(ssncferreira): inject actor headers directly in the client-header
		//   middleware instead of using SDK options.
		if actor := aibcontext.ActorFromContext(r.Context()); actor != nil && i.cfg.SendActorHeaders {
			opts = append(opts, intercept.ActorHeadersAsOpenAIOpts(actor)...)
		}

		// We take control of request body here and pass it to the SDK as a raw byte slice.
		// This is because the SDK's serialization applies hidden request options that result in
		// unexpected, breaking behavior. See https://github.com/coder/aibridge/pull/164
		// chatCompletionRequestBody also applies provider-specific
		// compatibility patches to the exact body sent upstream.
		body, err := i.chatCompletionRequestBody()
		if err != nil {
			return xerrors.Errorf("marshal request body: %w", err)
		}
		opts = append(opts, option.WithRequestBody("application/json", body))
		opts = append(opts, option.WithJSONSet("stream", true))

		streamStats := &sseStreamStats{}
		opts = append(opts, option.WithMiddleware(func(req *http.Request, next option.MiddlewareNext) (*http.Response, error) {
			resp, err := next(req)
			streamStats.wrapResponse(resp)
			return resp, err
		}))

		stream = i.newStream(streamCtx, svc, opts)
		processor := newStreamProcessor(streamCtx, i.logger.Named("stream-processor"), i.getInjectedToolByName)

		var toolCall *openai.FinishedChatCompletionToolCall

		// iterationStarted is per-iteration (reset on every
		// loop): true once the upstream call has produced any
		// events for this iteration. While false, a key-specific
		// failure can still fail over to the next key. Distinct
		// from events.IsStreaming(), which is stream-wide and
		// stays true once iteration 1 has sent any event
		// downstream.
		var iterationStarted bool

		for stream.Next() {
			iterationStarted = true
			chunk := stream.Current()

			canRelay := processor.process(chunk)
			if toolCall == nil {
				toolCall = processor.getToolCall()
			}

			if !canRelay {
				// The chunk must not be sent to the client because it contains an injected tool call.
				continue
			}

			// Marshal and relay chunk to client.
			payload, err := i.marshalChunk(&chunk, i.ID(), processor)
			if err != nil {
				logger.Warn(ctx, "failed to marshal chunk", slog.Error(err), slog.F("chunk", chunk.RawJSON()))
				lastErr = xerrors.Errorf("marshal chunk: %w", err)
				break
			}
			if err := events.Send(ctx, payload); err != nil {
				logger.Warn(ctx, "failed to relay chunk", slog.Error(err))
				lastErr = xerrors.Errorf("relay chunk: %w", err)
				break
			}
		}

		streamErr := stream.Err()
		if err := stream.Close(); err != nil {
			logger.Debug(ctx, "failed to close upstream stream", slog.Error(err))
		}

		if toolCall != nil {
			// Builtin tools are not intercepted.
			if i.getInjectedToolByName(toolCall.Name) == nil {
				_ = i.recorder.RecordToolUsage(streamCtx, &recorder.ToolUsageRecord{
					InterceptionID: i.ID().String(),
					MsgID:          processor.getMsgID(),
					ToolCallID:     toolCall.ID,
					Tool:           toolCall.Name,
					Args:           i.unmarshalArgs(toolCall.Arguments),
					Injected:       false,
				})

				toolCall = nil
			} else if streamErr == nil {
				// When the provider responds with only tool calls (no text content),
				// no chunks are relayed to the client, so the stream is not yet
				// initiated. Initiate it here so the SSE headers are sent and the
				// ping ticker is started, preventing client timeout during tool invocation.
				// Only initiate if no stream error, if there's an error, we'll return
				// an HTTP error response instead of starting an SSE stream.
				events.InitiateStream(w)
			}
		}

		if prompt != nil {
			_ = i.recorder.RecordPromptUsage(streamCtx, &recorder.PromptUsageRecord{
				InterceptionID: i.ID().String(),
				MsgID:          processor.getMsgID(),
				Prompt:         *prompt,
			})
			prompt = nil
		}

		if lastUsage := processor.getLastUsage(); lastUsage.CompletionTokens > 0 {
			// If the usage information is set, track it.
			// The API will send usage information when the response terminates, which will happen if a tool call is invoked.
			i.recordTokenUsage(streamCtx, processor.getMsgID(), lastUsage)
		}

		if iterationStarted {
			// Mid-stream error or logical error: events have
			// already streamed for this iteration, so the
			// error is relayed as an SSE event.
			if respErr := i.mapStreamError(ctx, logger, streamErr, lastErr, streamStats, true); respErr != nil {
				interceptionErr = respErr
				payload, err := i.marshalErr(respErr)
				if err != nil {
					logger.Warn(ctx, "failed to marshal error", slog.Error(err), slog.F("error_payload", fmt.Sprintf("%+v", respErr)))
				} else if err := events.Send(streamCtx, payload); err != nil {
					logger.Warn(ctx, "failed to relay error", slog.Error(err), slog.F("payload", payload))
				}
			} else if streamErr != nil {
				// Unrecoverable (e.g., broken pipe, context
				// canceled): can't relay to the client, but record
				// the error so it isn't silently swallowed.
				interceptionErr = streamErr
			}
		} else {
			// Pre-stream failure of this iteration. For
			// centralized requests, mark the key and retry with
			// the next one.
			if currentPoolKey != nil && i.markKeyOnError(ctx, currentPoolKey, streamErr) {
				continue
			}
			// Non-key error: relay it. Use mapStreamError so that
			// unknown upstream errors (TCP reset, DNS failure, TLS
			// error, deadline exceeded) are wrapped in a generic
			// response instead of producing a silent HTTP 200.
			respErr := i.mapStreamError(ctx, logger, streamErr, lastErr, streamStats, events.IsStreaming())
			if respErr != nil {
				interceptionErr = respErr
				if events.IsStreaming() {
					// Prior iterations have streamed, so the SSE
					// connection is open: inject as an SSE event.
					payload, mErr := i.marshalErr(respErr)
					if mErr != nil {
						logger.Warn(ctx, "failed to marshal error", slog.Error(mErr))
					} else if sErr := events.Send(streamCtx, payload); sErr != nil {
						logger.Warn(ctx, "failed to relay error", slog.Error(sErr))
					}
				} else {
					// No events streamed yet, write the response directly.
					i.writeUpstreamError(w, respErr)
				}
			}
		}

		// No tool call, nothing more to do.
		if toolCall == nil {
			break
		}

		tool := i.getInjectedToolByName(toolCall.Name)
		if tool == nil {
			// Not a known tool, don't do anything.
			logger.Warn(streamCtx, "pending tool call for non-injected tool, this is unexpected", slog.F("tool", toolCall.Name))
			break
		}

		// Invoke the injected tool, and use the tool result to make a subsequent request to the upstream.
		// Append the completion from this stream as context.
		// Some providers may return tool calls with non-zero starting indices,
		// resulting in nil entries in the array that must be removed.
		completion := processor.getLastCompletion()
		if completion != nil {
			compactToolCalls(completion)
			i.req.Messages = append(i.req.Messages, completion.ToParam())
		}

		id := toolCall.ID
		args := i.unmarshalArgs(toolCall.Arguments)
		toolRes, toolErr := tool.Call(streamCtx, args, i.tracer)
		_ = i.recorder.RecordToolUsage(streamCtx, &recorder.ToolUsageRecord{
			InterceptionID:  i.ID().String(),
			MsgID:           processor.getMsgID(),
			ToolCallID:      id,
			ServerURL:       &tool.ServerURL,
			Tool:            tool.Name,
			Args:            args,
			Injected:        true,
			InvocationError: toolErr,
		})

		// Reset.
		toolCall = nil

		if toolErr != nil {
			// Always provide a tool_result even if the tool call failed.
			errorJSON, _ := json.Marshal(i.newErrorResponse(toolErr))
			i.req.Messages = append(i.req.Messages, openai.ToolMessage(string(errorJSON), id))
			continue
		}

		var out strings.Builder
		if err := json.NewEncoder(&out).Encode(toolRes); err != nil {
			logger.Warn(ctx, "failed to encode tool response", slog.Error(err))
			// Always provide a tool_result even if encoding failed.
			errorJSON, _ := json.Marshal(i.newErrorResponse(err))
			i.req.Messages = append(i.req.Messages, openai.ToolMessage(string(errorJSON), id))
			continue
		}

		i.req.Messages = append(i.req.Messages, openai.ToolMessage(out.String(), id))
	}

	var shutdownErr error
	if events.IsStreaming() {
		// Send termination marker.
		if err := events.SendRaw(streamCtx, i.encodeForStream([]byte("[DONE]"))); err != nil {
			logger.Debug(ctx, "failed to send termination marker", slog.Error(err))
		}

		// Give the events stream 30 seconds (TODO: configurable) to gracefully shutdown.
		shutdownCtx, shutdownCancel := context.WithTimeout(ctx, time.Second*30)
		defer shutdownCancel()
		if shutdownErr = events.Shutdown(shutdownCtx); shutdownErr != nil {
			logger.Warn(ctx, "event stream shutdown", slog.Error(shutdownErr))
		}
	}

	if shutdownErr != nil {
		streamCancel(xerrors.Errorf("stream err: %w", shutdownErr))
	} else {
		streamCancel(xerrors.New("gracefully done"))
	}

	return interceptionErr
}

func (i *StreamingInterception) getInjectedToolByName(name string) *mcp.Tool {
	if i.mcpProxy == nil {
		return nil
	}

	return i.mcpProxy.GetTool(name)
}

// Mashals received stream chunk.
// Overrides id (since proxy obscures injected tool call invocations).
// If usage field was set in original chunk overrides it to culminative usage.
//
// sjson is used instead of normal struct marshaling so forwarded data
// is as close to the original as possible. Structs from openai library lack
// `omitzero/omitempty` annotations which adds additional empty fields
// when marshaling structs. Those additional empty fields can break Codex client.
func (i *StreamingInterception) marshalChunk(chunk *openai.ChatCompletionChunk, id uuid.UUID, prc *streamProcessor) ([]byte, error) {
	sj, err := sjson.Set(chunk.RawJSON(), "id", id.String())
	if err != nil {
		return nil, xerrors.Errorf("marshal chunk id failed: %w", err)
	}

	// If usage information is available, relay the cumulative usage once all tool invocations have completed.
	if chunk.JSON.Usage.Valid() {
		u := prc.getCumulativeUsage()
		sj, err = sjson.Set(sj, "usage", u)
		if err != nil {
			return nil, xerrors.Errorf("marshal chunk usage failed: %w", err)
		}
	}

	return i.encodeForStream([]byte(sj)), nil
}

func (i *StreamingInterception) marshalErr(err error) ([]byte, error) {
	data, err := json.Marshal(err)
	if err != nil {
		return nil, xerrors.Errorf("marshal error failed: %w", err)
	}

	return i.encodeForStream(data), nil
}

func (*StreamingInterception) encodeForStream(payload []byte) []byte {
	// bytes.Buffer writes to in-memory storage and never return errors.
	var buf bytes.Buffer
	_, _ = buf.WriteString("data: ")
	_, _ = buf.Write(payload)
	_, _ = buf.WriteString("\n\n")
	return buf.Bytes()
}

// newStream traces svc.NewStreaming(streamCtx, i.req.ChatCompletionNewParams) call
func (i *StreamingInterception) newStream(ctx context.Context, svc openai.ChatCompletionService, opts []option.RequestOption) *ssestream.Stream[openai.ChatCompletionChunk] {
	_, span := i.tracer.Start(ctx, "Intercept.ProcessRequest.Upstream", trace.WithAttributes(tracing.InterceptionAttributesFromContext(ctx)...))
	defer span.End()

	return svc.NewStreaming(ctx, openai.ChatCompletionNewParams{}, opts...)
}

// mapStreamError turns stream errors or empty-stream anomalies into
// relayable ResponseErrors. Returns nil for unrecoverable errors that
// cannot be relayed.
func (i *StreamingInterception) mapStreamError(ctx context.Context, logger slog.Logger, streamErr, lastErr error, stats *sseStreamStats, downstreamStarted bool) *intercept.ResponseError {
	if streamErr == nil && stats.isEmptyDataStream() {
		i.logUpstreamStreamFailure(ctx, logger, "empty_stream", streamErr, stats, downstreamStarted)
		return intercept.NewResponseError(
			upstreamEmptyStreamMessage,
			intercept.OpenAIErrTypeAPI,
			intercept.OpenAIErrCodeServer,
			http.StatusBadGateway,
			0,
		)
	}
	if streamErr != nil {
		if eventstream.IsUnrecoverableError(streamErr) {
			logger.Debug(ctx, "stream terminated", slog.Error(streamErr))
			// We can't reflect an error back if there's a connection error or the request context was canceled.
			return nil
		}
		if oaiErr := intercept.ResponseErrorFromAPIError(streamErr); oaiErr != nil {
			logger.Warn(ctx, "openai stream error", slog.Error(streamErr))
			return oaiErr
		}
		if stats != nil && stats.isSSEUpstream() && stats.hasDataEvents() && isJSONDecodeStreamError(streamErr) {
			i.logUpstreamStreamFailure(ctx, logger, "malformed_json", streamErr, stats, downstreamStarted)
			return intercept.NewResponseError(
				upstreamMalformedStreamMessage,
				intercept.OpenAIErrTypeAPI,
				intercept.OpenAIErrCodeServer,
				http.StatusBadGateway,
				0,
			)
		}
		logger.Warn(ctx, "unknown stream error", slog.Error(streamErr))
		// Unfortunately, the OpenAI SDK does not support parsing errors received in the stream
		// into known types (i.e. [shared.OverloadedError]).
		// See https://github.com/openai/openai-go/blob/v2.7.0/packages/ssestream/ssestream.go#L171
		// All it does is wrap the payload in an error - which is all we can return, currently.
		return intercept.NewResponseError(fmt.Sprintf("unknown stream error: %s", streamErr), intercept.OpenAIErrTypeError, intercept.OpenAIErrTypeError, http.StatusBadGateway, 0)
	}
	if lastErr != nil {
		logger.Warn(ctx, "stream processing failed", slog.Error(lastErr))
		return intercept.NewResponseError(fmt.Sprintf("processing error: %s", lastErr), intercept.OpenAIErrTypeError, intercept.OpenAIErrTypeError, http.StatusBadGateway, 0)
	}
	return nil
}

func (i *StreamingInterception) logUpstreamStreamFailure(ctx context.Context, logger slog.Logger, kind string, err error, stats *sseStreamStats, downstreamStarted bool) {
	fields := []slog.Field{
		slog.F("stream_error_kind", kind),
		slog.F("provider_name", i.cfg.ProviderName),
		slog.F("model", i.Model()),
		slog.F("endpoint", "/v1/chat/completions"),
		slog.F("downstream_started", downstreamStarted),
	}
	if err != nil {
		fields = append(fields, slog.Error(err))
	}
	if stats != nil {
		fields = append(fields,
			slog.F("upstream_status", stats.statusCodeInt()),
			slog.F("upstream_content_type", stats.contentTypeString()),
			slog.F("has_data_events", stats.hasDataEvents()),
			slog.F("saw_done", stats.sawDone.Load()),
			slog.F("data_event_count", stats.dataEvents.Load()),
			slog.F("comment_count", stats.comments.Load()),
			slog.F("empty_event_count", stats.emptyEvents.Load()),
			slog.F("upstream_bytes_read", stats.rawBytes.Load()),
		)
	}
	logger.Warn(ctx, "upstream stream failed", fields...)
}

func isJSONDecodeStreamError(err error) bool {
	if _, ok := errors.AsType[*json.SyntaxError](err); ok {
		return true
	}
	_, ok := errors.AsType[*json.UnmarshalTypeError](err)
	return ok
}

type streamProcessor struct {
	ctx    context.Context
	logger slog.Logger

	acc openai.ChatCompletionAccumulator

	// Tool handling.
	pendingToolCall     bool
	getInjectedToolFunc func(string) *mcp.Tool

	// Token handling.
	lastUsage       openai.CompletionUsage
	cumulativeUsage openai.CompletionUsage
}

func newStreamProcessor(ctx context.Context, logger slog.Logger, isToolInjectedFunc func(string) *mcp.Tool) *streamProcessor {
	return &streamProcessor{
		ctx:    ctx,
		logger: logger,

		getInjectedToolFunc: isToolInjectedFunc,
	}
}

// process receives a completion chunk and returns a bool indicating whether it should be
// relayed to the client.
func (s *streamProcessor) process(chunk openai.ChatCompletionChunk) bool {
	if !s.acc.AddChunk(chunk) {
		s.logger.Debug(s.ctx, "failed to accumulate chunk", slog.F("chunk", chunk.RawJSON()))
		// Potentially not fatal, move along in best effort...
	}

	// Accumulate token usage.
	s.lastUsage = chunk.Usage
	s.cumulativeUsage = sumUsage(s.cumulativeUsage, chunk.Usage)

	// If the stream has reached a terminal state (i.e. call a tool), and this tool is injected,
	// then it must not be relayed.
	if _, ok := s.acc.JustFinishedToolCall(); ok && s.pendingToolCall {
		return false
	}

	if len(chunk.Choices) == 0 {
		// Odd, should not occur, relay it on in case.
		// Nothing more to be done.
		return true
	}

	// We explicitly set n=1, so this shouldn't happen.
	if count := len(chunk.Choices); count > 1 {
		s.logger.Warn(s.ctx, "multiple choices returned, only handling first", slog.F("count", count))
	}

	// Check if we have a tool call in progress.
	//
	// The API will send partial tool call events like this:
	//
	// data: ... delta":{"tool_calls":[{"index":0,"id":"call_0TxntkwDB66KH8z4RwNqeWrZ","type":"function","function":{"name":"bmcp_coder_coder_list_workspaces","arguments":""}}]}...
	// data: ... delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\""}}]}...
	// data: ... delta":{"tool_calls":[{"index":0,"function":{"arguments":"owner"}}]}...
	// data: ... delta":{"tool_calls":[{"index":0,"function":{"arguments":"\":\""}}]}...
	// data: ... delta":{"tool_calls":[{"index":0,"function":{"arguments":"admin"}}]}...
	// data: ... delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"}"}}]}...
	//
	// So we need to ensure that we don't relay any of the partial events to the client in the case of
	// an injected tool.
	//
	// The first partial will tell us the tool name, and we can then decide how to proceed.

	choice := chunk.Choices[0]
	if len(choice.Delta.ToolCalls) == 0 {
		// No tool calls, no special handling required.
		return true
	}

	// If we have a pending injected tool call in progress, do not relay any subsequent partial chunks.
	if s.pendingToolCall {
		return false
	}

	// This shouldn't happen since we have parallel tool calls disabled currently.
	if count := len(choice.Delta.ToolCalls); count > 1 {
		s.logger.Warn(context.Background(), "unexpected tool call count", slog.F("count", count))
		// We'll continue and just examine the first tool.
	}

	toolCall := choice.Delta.ToolCalls[0]
	if s.isInjected(toolCall) {
		// Mark tool as pending until tool call is finished.
		s.pendingToolCall = true
		return false
	}

	// There is a tool call, but it's not injected.
	return true
}

// getMsgID returns the ID given by the API for this (accumulated) message.
func (s *streamProcessor) getMsgID() string {
	return s.acc.ID
}

func (s *streamProcessor) isInjected(toolCall openai.ChatCompletionChunkChoiceDeltaToolCall) bool {
	return s.getInjectedToolFunc(strings.TrimSpace(toolCall.Function.Name)) != nil
}

func (s *streamProcessor) getToolCall() *openai.FinishedChatCompletionToolCall {
	tc, ok := s.acc.JustFinishedToolCall()
	if !ok {
		return nil
	}

	return &tc
}

func (s *streamProcessor) getLastCompletion() *openai.ChatCompletionMessage {
	if len(s.acc.Choices) == 0 {
		return nil
	}

	return &s.acc.Choices[0].Message
}

func (s *streamProcessor) getLastUsage() openai.CompletionUsage {
	return s.lastUsage
}

func (s *streamProcessor) getCumulativeUsage() openai.CompletionUsage {
	return s.cumulativeUsage
}

// compactToolCalls removes nil/empty tool call entries (without an ID).
func compactToolCalls(msg *openai.ChatCompletionMessage) {
	if msg == nil || len(msg.ToolCalls) == 0 {
		return
	}
	msg.ToolCalls = slices.DeleteFunc(msg.ToolCalls, func(tc openai.ChatCompletionMessageToolCallUnion) bool {
		return tc.ID == ""
	})
}
