package messages

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/anthropics/anthropic-sdk-go/packages/ssestream"
	"github.com/anthropics/anthropic-sdk-go/shared/constant"
	"github.com/google/uuid"
	mcplib "github.com/mark3labs/mcp-go/mcp"
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

type StreamingInterception struct {
	interceptionBase
}

func NewStreamingInterceptor(
	id uuid.UUID,
	reqPayload RequestPayload,
	cfg intercept.Config,
	cred intercept.Credential,
	bedrock *BedrockRuntime,
	clientHeaders http.Header,
	tracer trace.Tracer,
) *StreamingInterception {
	return &StreamingInterception{interceptionBase: interceptionBase{
		id:            id,
		reqPayload:    reqPayload,
		cfg:           cfg,
		cred:          cred,
		bedrock:       bedrock,
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

// ProcessRequest handles a request to /v1/messages.
// This API has a state-machine behind it, which is described in https://docs.claude.com/en/docs/build-with-claude/streaming#event-types.
//
// Each stream uses the following event flow:
// - `message_start`: contains a Message object with empty content.
// - A series of content blocks, each of which have a `content_block_start`, one or more `content_block_delta` events, and a `content_block_stop` event.
// - Each content block will have an index that corresponds to its index in the final Message content array.
// - One or more `message_delta` events, indicating top-level changes to the final Message object.
// - A final `message_stop` event.
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
	if len(i.reqPayload) == 0 {
		return xerrors.New("developer error: request payload is empty")
	}

	ctx, span := i.tracer.Start(r.Context(), "Intercept.ProcessRequest", trace.WithAttributes(tracing.InterceptionAttributesFromContext(r.Context())...))
	defer tracing.EndSpanErr(span, &outErr)

	// Allow us to interrupt watch via cancel.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	r = r.WithContext(ctx) // Rewire context for SSE cancellation.

	logger := i.logger.With(slog.F("model", i.Model()))

	var (
		prompt      string
		promptFound bool
		err         error
	)

	prompt, promptFound, err = i.reqPayload.lastUserPrompt()
	if err != nil {
		logger.Warn(ctx, "failed to determine last user prompt", slog.Error(err))
	}

	// Claude Code uses a "small/fast model" for certain tasks.
	if !i.isSmallFastModel() {
		// Only inject tools into "actual" request.
		i.injectTools()
	}

	streamCtx, streamCancel := context.WithCancelCause(ctx)
	defer streamCancel(xerrors.New("deferred"))

	// TODO(ssncferreira): inject actor headers directly in the client-header
	//   middleware instead of using SDK options.
	var opts []option.RequestOption
	if actor := aibcontext.ActorFromContext(ctx); actor != nil && i.cfg.SendActorHeaders {
		opts = append(opts, intercept.ActorHeadersAsAnthropicOpts(actor)...)
	}

	svc, err := i.newMessagesService(streamCtx, opts...)
	if err != nil {
		err = xerrors.Errorf("create anthropic client: %w", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return err
	}

	// events will either terminate when shutdown after interaction with upstream completes, or when streamCtx is done.
	events := eventstream.NewEventStream(streamCtx, logger.Named("sse-sender"), i.pingPayload(), quartz.NewReal())
	go events.Start(w, r)
	defer func() {
		_ = events.Shutdown(streamCtx) // Catch-all in case it doesn't get shutdown after stream completes.
	}()

	// Accumulate usage across the entire streaming interaction (including tool reinvocations).
	var cumulativeUsage anthropic.Usage

	var lastErr error
	var interceptionErr error

	// Sum the key attempts across all iterations and record once when the
	// interception completes.
	var totalKeyAttempts int
	if cp, ok := intercept.AsCentralizedPool(i.cred); ok {
		defer func() {
			cp.Pool.RecordAttempts(totalKeyAttempts)
		}()
	}

	isFirst := true
newStream:
	for {
		// TODO add outer loop span (https://github.com/coder/aibridge/issues/67)
		if err := streamCtx.Err(); err != nil {
			interceptionErr = xerrors.Errorf("stream exit: %w", err)
			break
		}

		// Per-iteration walker. An iteration is either an agentic
		// continuation (sending a tool result back in a new
		// stream) or a failover retry (previous key marked, try
		// the next one). A pool-less credential (BYOK, or pool-less
		// centralized such as Bedrock) has no walker and runs as a
		// single attempt.
		streamOpts := []option.RequestOption{i.withBody()}
		var currentPoolKey *keypool.Key
		if cp, isPool := intercept.AsCentralizedPool(i.cred); isPool {
			walker := cp.Pool.Walker()
			key, keyPoolErr := cp.NextKey(walker)
			if keyPoolErr != nil {
				// Pool exhausted in this iteration. Relay the error to the
				// client: as an SSE event if events have already been sent,
				// or by direct write otherwise.
				respErr := ResponseErrorFromKeyPool(keyPoolErr)
				interceptionErr = respErr
				if events.IsStreaming() {
					payload, mErr := i.marshal(respErr)
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
			streamOpts = append(streamOpts,
				option.WithAPIKey(key.Value()),
				// Disable SDK retries because the failover loop handles
				// retries via key rotation.
				option.WithMaxRetries(0),
			)
			totalKeyAttempts += walker.Attempts()
		}

		stream := i.newStream(streamCtx, svc, streamOpts...)

		var message anthropic.Message
		var lastToolName string

		pendingToolCalls := make(map[string]string)

		// iterationStarted is per-iteration (reset on every
		// newStream loop): true once the upstream call has
		// produced any events for this iteration. While false,
		// a key-specific failure can still fail over to the
		// next key. Distinct from events.IsStreaming(), which
		// is stream-wide and stays true once iteration 1 has
		// sent any event downstream.
		var iterationStarted bool

		for stream.Next() {
			iterationStarted = true
			event := stream.Current()
			if err := message.Accumulate(event); err != nil {
				logger.Warn(ctx, "failed to accumulate streaming events", slog.Error(err), slog.F("event", event), slog.F("msg", message.RawJSON()))
				lastErr = xerrors.Errorf("accumulate event: %w", err)
				break
			}

			// Tool-related handling.
			switch event.Type {
			case string(constant.ValueOf[constant.ContentBlockStart]()):
				if block, ok := event.AsContentBlockStart().ContentBlock.AsAny().(anthropic.ToolUseBlock); ok {
					lastToolName = block.Name

					if i.mcpProxy != nil && i.mcpProxy.GetTool(block.Name) != nil {
						pendingToolCalls[block.Name] = block.ID
						// Don't relay this event back, otherwise the client will try invoke the tool as well.
						continue
					}
				}
			case string(constant.ValueOf[constant.ContentBlockDelta]()):
				if len(pendingToolCalls) > 0 && i.mcpProxy != nil && i.mcpProxy.GetTool(lastToolName) != nil {
					// We're busy with a tool call, don't relay this event back.
					continue
				}
			case string(constant.ValueOf[constant.ContentBlockStop]()):
				// Reset the tool name
				isInjected := i.mcpProxy != nil && i.mcpProxy.GetTool(lastToolName) != nil
				lastToolName = ""

				if len(pendingToolCalls) > 0 && isInjected {
					// We're busy with a tool call, don't relay this event back.
					continue
				}
			case string(constant.ValueOf[constant.MessageStart]()):
				start := event.AsMessageStart()
				accumulateUsage(&cumulativeUsage, start.Message.Usage)

				_ = i.recorder.RecordTokenUsage(streamCtx, &recorder.TokenUsageRecord{
					InterceptionID:        i.ID().String(),
					MsgID:                 message.ID,
					Input:                 start.Message.Usage.InputTokens,
					Output:                start.Message.Usage.OutputTokens,
					CacheReadInputTokens:  start.Message.Usage.CacheReadInputTokens,
					CacheWriteInputTokens: start.Message.Usage.CacheCreationInputTokens,
					ExtraTokenTypes: map[string]int64{
						"web_search_requests":      start.Message.Usage.ServerToolUse.WebSearchRequests,
						"cache_ephemeral_1h_input": start.Message.Usage.CacheCreation.Ephemeral1hInputTokens,
						"cache_ephemeral_5m_input": start.Message.Usage.CacheCreation.Ephemeral5mInputTokens,
					},
				})

				if !isFirst {
					// Don't send message_start unless first message!
					// We're sending multiple messages back and forth with the API, but from the client's perspective
					// they're just expecting a single message.
					continue
				}
			case string(constant.ValueOf[constant.MessageDelta]()):
				delta := event.AsMessageDelta()
				accumulateUsage(&cumulativeUsage, delta.Usage)

				// Only output tokens should change in message_delta.
				_ = i.recorder.RecordTokenUsage(streamCtx, &recorder.TokenUsageRecord{
					InterceptionID: i.ID().String(),
					MsgID:          message.ID,
					Output:         delta.Usage.OutputTokens,
				})

				// Don't relay message_delta events which indicate injected tool use.
				if len(pendingToolCalls) > 0 && i.mcpProxy != nil && i.mcpProxy.GetTool(lastToolName) != nil {
					continue
				}

				// If currently calling a tool.
				if len(message.Content) > 0 && message.Content[len(message.Content)-1].Type == string(constant.ValueOf[constant.ToolUse]()) {
					toolName := message.Content[len(message.Content)-1].AsToolUse().Name
					if len(pendingToolCalls) > 0 && i.mcpProxy != nil && i.mcpProxy.GetTool(toolName) != nil {
						continue
					}
				}

				// We should be updating the event's usage to the calculated cumulative usage. However...
				// the SDK only accumulates output tokens on message_delta, since that's all that *should* change.
				//
				// Backstory: the API reports tokens during message_start AND message_delta. message_start reports the input
				// tokens and others, while the delta should only report changes to output tokens.
				// HOWEVER, when we invoke injected tools we're starting a whole new message (and subsequently receive
				// message_start and message_delta events), and the previous message_start has already been relayed, so in effect
				// we can't really modify anything other than output tokens here according to the SDK.
				// This will affect how the client reports token usage for input tokens, for example.
				// For our purposes, the server (aibridge) is authoritative anyway so it's not a big deal, but this is something to note.
				//
				// See https://github.com/anthropics/anthropic-sdk-go/blob/v1.12.0/message.go#L2619-L2622
				event.Usage.OutputTokens = cumulativeUsage.OutputTokens

			// Don't send message_stop until all tools have been called.
			case string(constant.ValueOf[constant.MessageStop]()):

				// Capture any thinking blocks that were returned.
				for _, t := range i.extractModelThoughts(&message) {
					_ = i.recorder.RecordModelThought(ctx, &recorder.ModelThoughtRecord{
						InterceptionID: i.ID().String(),
						Content:        t.Content,
						Metadata:       t.Metadata,
					})
				}

				// Process injected tools.
				if len(pendingToolCalls) > 0 {
					// Append the whole message from this stream as context since we'll be sending a new request with the tool results.
					var loopMessages []anthropic.MessageParam
					loopMessages = append(loopMessages, message.ToParam())

					for name, id := range pendingToolCalls {
						if i.mcpProxy == nil {
							continue
						}

						if i.mcpProxy.GetTool(name) == nil {
							// Not an MCP proxy call, don't do anything.
							continue
						}

						tool := i.mcpProxy.GetTool(name)
						if tool == nil {
							logger.Warn(ctx, "tool not found in manager", slog.F("tool_name", name))
							continue
						}

						var (
							input      json.RawMessage
							foundTool  bool
							foundTools int
						)
						for _, block := range message.Content {
							if variant, ok := block.AsAny().(anthropic.ToolUseBlock); ok {
								foundTools++
								if variant.Name == name {
									input = variant.Input
									foundTool = true
								}
							}
						}

						if !foundTool {
							logger.Warn(ctx, "failed to find tool input", slog.F("tool_name", name), slog.F("found_tools", foundTools))
							continue
						}

						res, err := tool.Call(streamCtx, input, i.tracer)

						_ = i.recorder.RecordToolUsage(streamCtx, &recorder.ToolUsageRecord{
							InterceptionID:  i.ID().String(),
							MsgID:           message.ID,
							ToolCallID:      id,
							ServerURL:       &tool.ServerURL,
							Tool:            tool.Name,
							Args:            input,
							Injected:        true,
							InvocationError: err,
						})

						if err != nil {
							// Always provide a tool_result even if the tool call failed
							loopMessages = append(loopMessages,
								anthropic.NewUserMessage(anthropic.NewToolResultBlock(id, fmt.Sprintf("Error calling tool: %v", err), true)),
							)
							continue
						}

						// Process tool result
						toolResult := anthropic.ContentBlockParamUnion{
							OfToolResult: &anthropic.ToolResultBlockParam{
								ToolUseID: id,
								IsError:   anthropic.Bool(false),
							},
						}

						var hasValidResult bool
						for _, content := range res.Content {
							switch cb := content.(type) {
							case mcplib.TextContent:
								toolResult.OfToolResult.Content = append(toolResult.OfToolResult.Content, anthropic.ToolResultBlockParamContentUnion{
									OfText: &anthropic.TextBlockParam{
										Text: cb.Text,
									},
								})
								hasValidResult = true
							case mcplib.EmbeddedResource:
								switch resource := cb.Resource.(type) {
								case mcplib.TextResourceContents:
									val := fmt.Sprintf("Binary resource (MIME: %s, URI: %s): %s",
										resource.MIMEType, resource.URI, resource.Text)
									toolResult.OfToolResult.Content = append(toolResult.OfToolResult.Content, anthropic.ToolResultBlockParamContentUnion{
										OfText: &anthropic.TextBlockParam{
											Text: val,
										},
									})
									hasValidResult = true
								case mcplib.BlobResourceContents:
									val := fmt.Sprintf("Binary resource (MIME: %s, URI: %s): %s",
										resource.MIMEType, resource.URI, resource.Blob)
									toolResult.OfToolResult.Content = append(toolResult.OfToolResult.Content, anthropic.ToolResultBlockParamContentUnion{
										OfText: &anthropic.TextBlockParam{
											Text: val,
										},
									})
									hasValidResult = true
								default:
									logger.Warn(ctx, "unknown embedded resource type", slog.F("type", fmt.Sprintf("%T", resource)))
									toolResult.OfToolResult.Content = append(toolResult.OfToolResult.Content, anthropic.ToolResultBlockParamContentUnion{
										OfText: &anthropic.TextBlockParam{
											Text: "Error: unknown embedded resource type",
										},
									})
									toolResult.OfToolResult.IsError = anthropic.Bool(true)
									hasValidResult = true
								}
							default:
								logger.Warn(ctx, "not handling non-text tool result", slog.F("type", fmt.Sprintf("%T", cb)))
								toolResult.OfToolResult.Content = append(toolResult.OfToolResult.Content, anthropic.ToolResultBlockParamContentUnion{
									OfText: &anthropic.TextBlockParam{
										Text: "Error: unsupported tool result type",
									},
								})
								toolResult.OfToolResult.IsError = anthropic.Bool(true)
								hasValidResult = true
							}
						}

						// If no content was processed, still add a tool_result
						if !hasValidResult {
							logger.Warn(ctx, "no tool result added", slog.F("content_len", len(res.Content)), slog.F("is_error", res.IsError))
							toolResult.OfToolResult.Content = append(toolResult.OfToolResult.Content, anthropic.ToolResultBlockParamContentUnion{
								OfText: &anthropic.TextBlockParam{
									Text: "Error: no valid tool result content",
								},
							})
							toolResult.OfToolResult.IsError = anthropic.Bool(true)
						}

						if len(toolResult.OfToolResult.Content) > 0 {
							loopMessages = append(loopMessages, anthropic.NewUserMessage(toolResult))
						}
					}

					// Sync the raw payload with updated messages so that withBody()
					// sends the updated payload on the next iteration.
					updatedPayload, syncErr := i.reqPayload.appendedMessages(loopMessages)
					if syncErr != nil {
						lastErr = xerrors.Errorf("sync payload for agentic loop: %w", syncErr)
						break
					}
					i.reqPayload = updatedPayload

					// Causes a new stream to be run with updated messages.
					isFirst = false
					// Commit the SSE stream before the next iteration so a
					// later IsStreaming check always takes the SSE branch
					// instead of racing with the Start goroutine.
					// sync.Once makes this safe.
					events.InitiateStream(w)
					continue newStream
				}

				// Find all the non-injected tools and track their uses.
				for _, block := range message.Content {
					if variant, ok := block.AsAny().(anthropic.ToolUseBlock); ok {
						if i.mcpProxy != nil && i.mcpProxy.GetTool(variant.Name) != nil {
							continue
						}

						_ = i.recorder.RecordToolUsage(streamCtx, &recorder.ToolUsageRecord{
							InterceptionID: i.ID().String(),
							MsgID:          message.ID,
							ToolCallID:     variant.ID,
							Tool:           variant.Name,
							Args:           variant.Input,
							Injected:       false,
						})
					}
				}
			}

			// Overwrite response identifier since proxy obscures injected tool call invocations.
			payload, err := i.marshalEvent(event)
			if err != nil {
				logger.Warn(ctx, "failed to marshal event", slog.Error(err), slog.F("event", event.RawJSON()))
				lastErr = xerrors.Errorf("marshal event: %w", err)
				break
			}
			if err := events.Send(streamCtx, payload); err != nil {
				if eventstream.IsUnrecoverableError(err) {
					logger.Debug(ctx, "processing terminated", slog.Error(err))
					break // Stop processing if client disconnected or context canceled.
				}
				logger.Warn(ctx, "failed to relay event", slog.Error(err))
				lastErr = xerrors.Errorf("relay event: %w", err)
				break
			}
		}

		if promptFound {
			_ = i.recorder.RecordPromptUsage(ctx, &recorder.PromptUsageRecord{
				InterceptionID: i.ID().String(),
				MsgID:          message.ID,
				Prompt:         prompt,
			})
			prompt = ""         //nolint:ineffassign // reset to prevent double-recording across newStream iterations
			promptFound = false //nolint:ineffassign // reset to prevent double-recording across newStream iterations
		}

		if iterationStarted {
			// Mid-stream error or logical error: events have
			// already streamed for this iteration, so the
			// error is relayed as an SSE event.
			streamErr := stream.Err()
			if respErr := i.mapStreamError(ctx, logger, streamErr, lastErr); respErr != nil {
				interceptionErr = respErr
				payload, err := i.marshal(respErr)
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
			if currentPoolKey != nil && i.markKeyOnError(ctx, currentPoolKey, stream.Err()) {
				continue newStream
			}
			// Non-key error: relay it. Use mapStreamError so that
			// unknown upstream errors (TCP reset, DNS failure, TLS
			// error, deadline exceeded) are wrapped in a generic
			// response instead of producing a silent HTTP 200.
			respErr := i.mapStreamError(ctx, logger, stream.Err(), lastErr)
			if respErr != nil {
				interceptionErr = respErr
				if events.IsStreaming() {
					// Prior iterations have streamed, so the SSE
					// connection is open: inject as an SSE event.
					payload, mErr := i.marshal(respErr)
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

		shutdownCtx, shutdownCancel := context.WithTimeout(ctx, time.Second*30)
		// Give the events stream 30 seconds (TODO: configurable) to gracefully shutdown.
		if err := events.Shutdown(shutdownCtx); err != nil {
			logger.Warn(ctx, "event stream shutdown", slog.Error(err))
		}
		shutdownCancel()

		// Cancel the stream context, we're now done.
		if interceptionErr != nil {
			streamCancel(interceptionErr)
		} else {
			streamCancel(xerrors.New("gracefully done"))
		}

		break
	}

	return interceptionErr
}

// mapStreamError converts a mid-stream upstream error or
// processing error into a relayable ResponseError. Returns nil
// when the error is unrecoverable, in which case nothing can be
// relayed back.
func (*StreamingInterception) mapStreamError(ctx context.Context, logger slog.Logger, streamErr, lastErr error) *ResponseError {
	if streamErr != nil {
		if eventstream.IsUnrecoverableError(streamErr) {
			logger.Debug(ctx, "stream terminated", slog.Error(streamErr))
			// We can't reflect an error back if there's a connection error or the request context was canceled.
			return nil
		}
		if antErr := responseErrorFromAPIError(streamErr); antErr != nil {
			logger.Warn(ctx, "anthropic stream error", slog.Error(streamErr))
			return antErr
		}
		logger.Warn(ctx, "unknown stream error", slog.Error(streamErr))
		// Unfortunately, the Anthropic SDK does not support parsing errors received in the stream
		// into known types (i.e. [shared.OverloadedError]).
		// See https://github.com/anthropics/anthropic-sdk-go/blob/v1.12.0/packages/ssestream/ssestream.go#L172-L174
		// All it does is wrap the payload in an error - which is all we can return, currently.
		return newResponseError(fmt.Sprintf("unknown stream error: %s", streamErr), string(constant.ValueOf[constant.Error]()), http.StatusBadGateway, 0)
	}
	if lastErr != nil {
		logger.Warn(ctx, "stream processing failed", slog.Error(lastErr))
		return newResponseError(fmt.Sprintf("processing error: %s", lastErr), string(constant.ValueOf[constant.Error]()), http.StatusBadGateway, 0)
	}
	return nil
}

func (i *StreamingInterception) marshalEvent(event anthropic.MessageStreamEventUnion) ([]byte, error) {
	sj, err := sjson.Set(event.RawJSON(), "message.id", i.ID().String())
	if err != nil {
		return nil, xerrors.Errorf("marshal event id failed: %w", err)
	}

	sj, err = sjson.Set(sj, "usage.output_tokens", event.Usage.OutputTokens)
	if err != nil {
		return nil, xerrors.Errorf("marshal event usage failed: %w", err)
	}

	return i.encodeForStream([]byte(sj), event.Type), nil
}

func (i *StreamingInterception) marshal(payload any) ([]byte, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, xerrors.Errorf("marshal payload: %w", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, xerrors.Errorf("unmarshal payload: %w", err)
	}

	eventType, ok := parsed["type"].(string)
	if !ok || strings.TrimSpace(eventType) == "" {
		return nil, xerrors.Errorf("could not determine type from payload %q", data)
	}

	return i.encodeForStream(data, eventType), nil
}

// https://docs.anthropic.com/en/docs/build-with-claude/streaming#basic-streaming-request
func (i *StreamingInterception) pingPayload() []byte {
	return i.encodeForStream([]byte(`{"type": "ping"}`), "ping")
}

func (*StreamingInterception) encodeForStream(payload []byte, typ string) []byte {
	// bytes.Buffer writes to in-memory storage and never return errors.
	var buf bytes.Buffer
	_, _ = buf.WriteString("event: ")
	_, _ = buf.WriteString(typ)
	_, _ = buf.WriteString("\n")
	_, _ = buf.WriteString("data: ")
	_, _ = buf.Write(payload)
	_, _ = buf.WriteString("\n\n")
	return buf.Bytes()
}

// newStream traces svc.NewStreaming() call.
func (i *StreamingInterception) newStream(ctx context.Context, svc anthropic.MessageService, opts ...option.RequestOption) *ssestream.Stream[anthropic.MessageStreamEventUnion] {
	_, span := i.tracer.Start(ctx, "Intercept.ProcessRequest.Upstream", trace.WithAttributes(tracing.InterceptionAttributesFromContext(ctx)...))
	defer span.End()

	return svc.NewStreaming(ctx, anthropic.MessageNewParams{}, opts...)
}
