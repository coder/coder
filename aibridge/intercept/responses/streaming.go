package responses

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/packages/ssestream"
	"github.com/openai/openai-go/v3/responses"
	oaiconst "github.com/openai/openai-go/v3/shared/constant"
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
	streamShutdownTimeout = time.Second * 30 // TODO: configurable
)

type StreamingResponsesInterceptor struct {
	responsesInterceptionBase
}

func NewStreamingInterceptor(
	id uuid.UUID,
	reqPayload RequestPayload,
	cfg intercept.Config,
	cred intercept.Credential,
	clientHeaders http.Header,
	tracer trace.Tracer,
) *StreamingResponsesInterceptor {
	return &StreamingResponsesInterceptor{
		responsesInterceptionBase: responsesInterceptionBase{
			id:            id,
			reqPayload:    reqPayload,
			cfg:           cfg,
			cred:          cred,
			clientHeaders: clientHeaders,
			tracer:        tracer,
		},
	}
}

func (i *StreamingResponsesInterceptor) Setup(logger slog.Logger, rec recorder.Recorder, mcpProxy mcp.ServerProxier) {
	i.responsesInterceptionBase.Setup(logger.Named("streaming"), rec, mcpProxy)
}

func (*StreamingResponsesInterceptor) Streaming() bool {
	return true
}

func (i *StreamingResponsesInterceptor) TraceAttributes(r *http.Request) []attribute.KeyValue {
	return i.responsesInterceptionBase.baseTraceAttributes(r, true)
}

func (i *StreamingResponsesInterceptor) ProcessRequest(w http.ResponseWriter, r *http.Request) (outErr error) {
	ctx, span := i.tracer.Start(r.Context(), "Intercept.ProcessRequest", trace.WithAttributes(tracing.InterceptionAttributesFromContext(r.Context())...))
	defer tracing.EndSpanErr(span, &outErr)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	credCtx := intercept.WithCredentialInfo(ctx, i.cred)

	r = r.WithContext(ctx) // Rewire context for SSE cancellation.

	if err := i.validateRequest(credCtx, w); err != nil {
		return err
	}

	i.injectTools()

	events := eventstream.NewEventStream(ctx, i.logger.Named("sse-sender"), nil, quartz.NewReal())
	go events.Start(w, r)
	defer func() {
		shutdownCtx, shutdownCancel := context.WithTimeout(ctx, streamShutdownTimeout)
		defer shutdownCancel()
		_ = events.Shutdown(shutdownCtx)
	}()

	var respCopy responseCopier
	var firstResponseID string
	var completedResponse *responses.Response
	var innerLoopErr error
	var streamErr error

	prompt, promptFound, err := i.reqPayload.lastUserPrompt(credCtx, i.logger)
	if err != nil {
		i.logger.Warn(credCtx, "failed to get user prompt", slog.Error(err))
	}
	shouldLoop := true
	srv := i.newResponsesService(credCtx)

	// Sum the key attempts across all iterations and record once when the
	// interception completes.
	var totalKeyAttempts int
	if cp, ok := intercept.AsCentralizedPool(i.cred); ok {
		defer func() {
			cp.Pool.RecordAttempts(totalKeyAttempts)
		}()
	}

	for shouldLoop {
		shouldLoop = false

		// A pool credential advances its failover walker. An iteration is an
		// agentic continuation or a failover retry after the previous key was
		// marked. BYOK has no pool and runs as a single attempt.
		var walker *keypool.Walker
		cp, isPool := intercept.AsCentralizedPool(i.cred)
		if isPool {
			walker = cp.Pool.Walker()
		}

		// Failover sub-loop: try keys until a stream starts
		// successfully or we hit a non-recoverable error.
		var stream *ssestream.Stream[responses.ResponseStreamEventUnion]
		var startErr error
		for {
			respCopy = responseCopier{}
			opts := i.requestOptions(&respCopy)

			// TODO(ssncferreira): inject actor headers directly in the client-header
			//   middleware instead of using SDK options.
			if actor := aibcontext.ActorFromContext(r.Context()); actor != nil && i.cfg.SendActorHeaders {
				opts = append(opts, intercept.ActorHeadersAsOpenAIOpts(actor)...)
			}

			var currentPoolKey *keypool.Key
			if isPool {
				key, keyPoolErr := cp.NextKey(walker)
				if keyPoolErr != nil {
					// Pool exhausted: write the error directly. In
					// agentic mode the inner loop buffers events
					// instead of streaming them downstream, so the
					// SSE connection has not been opened yet.
					totalKeyAttempts += walker.Attempts()
					i.writeUpstreamError(w, intercept.ResponseErrorFromKeyPool(keyPoolErr))
					return xerrors.Errorf("key pool exhausted: %w", keyPoolErr)
				}
				currentPoolKey = key
				opts = append(opts,
					option.WithAPIKey(key.Value()),
					// Disable SDK retries because the failover
					// loop handles retries via key rotation.
					option.WithMaxRetries(0),
				)
				// Re-attribute this iteration's logs to the selected key.
				credCtx = intercept.WithCredentialInfo(ctx, i.cred)
				i.logger.Debug(credCtx, "using centralized api key")
			}

			stream = i.newStream(credCtx, srv, opts)
			if upstreamErr := stream.Err(); upstreamErr != nil {
				// Pre-stream failure of this attempt. For
				// centralized requests, mark the key and
				// retry with the next one.
				if currentPoolKey != nil && i.markKeyOnError(credCtx, currentPoolKey, upstreamErr) {
					stream.Close()
					continue
				}
				// Non-key error: stop trying and let the
				// existing handling below report it.
				startErr = upstreamErr
				break
			}
			// Stream started successfully: commit to this key.
			break
		}

		if isPool {
			totalKeyAttempts += walker.Attempts()
		}

		// func scope to defer steam.Close()
		err := func() error {
			defer stream.Close()

			if startErr != nil {
				// events stream should never be initialized
				if events.IsStreaming() {
					i.logger.Warn(credCtx, "event stream was initialized when no response was received from upstream")
					return startErr
				}

				// no response received from upstream (eg. client/connection error), return custom error
				if !respCopy.responseReceived.Load() {
					i.sendCustomErr(credCtx, w, http.StatusInternalServerError, startErr)
					return startErr
				}

				// forward received response as-is
				err := respCopy.forwardResp(w)
				return errors.Join(startErr, err)
			}

			for stream.Next() {
				ev := stream.Current()

				// Not every event has response.id set (eg: fixtures/openai/responses/streaming/simple.txtar).
				// First event should be of 'response.created' type and have response.id set.
				// Set responseID to the first response.id that is set.
				if firstResponseID == "" && ev.Response.ID != "" {
					firstResponseID = ev.Response.ID
				}

				// Capture the response from the response.completed event.
				// Only response.completed event type have 'usage' field set.
				if ev.Type == string(oaiconst.ValueOf[oaiconst.ResponseCompleted]()) {
					completedEvent := ev.AsResponseCompleted()
					completedResponse = &completedEvent.Response
				}

				// If no MCP proxy is provided then no tools are injected.
				// Inner loop will never iterate more than once, so events can be forwarded as soon as received.
				//
				// Otherwise inner loop could iterate. Only last response should be forwarded.
				// This is needed to keep consistency between response.id and response.previous_response_id fields.
				if i.mcpProxy == nil {
					if err := events.Send(ctx, respCopy.buff.readDelta()); err != nil {
						err = xerrors.Errorf("failed to relay chunk: %w", err)
						return err
					}
				}
			}

			streamErr = stream.Err()
			return nil
		}()
		if err != nil {
			return err
		}

		if i.mcpProxy != nil && completedResponse != nil {
			pending := i.getPendingInjectedToolCalls(completedResponse)
			shouldLoop, innerLoopErr = i.handleInnerAgenticLoop(credCtx, pending, completedResponse)
			if innerLoopErr != nil {
				i.sendCustomErr(credCtx, w, http.StatusInternalServerError, innerLoopErr)
				shouldLoop = false
			}

			// Record token usage for each inner loop iteration
			i.recordTokenUsage(credCtx, completedResponse)
		}

		i.recordModelThoughts(credCtx, completedResponse)
	}

	if promptFound {
		i.recordUserPrompt(credCtx, firstResponseID, prompt)
	}
	i.recordNonInjectedToolUsage(credCtx, completedResponse)

	// On innerLoop error custom error has been already sent,
	// exit without emptying respCopy buffer.
	if innerLoopErr != nil {
		return innerLoopErr
	}

	b, err := respCopy.readAll()
	if err != nil {
		return xerrors.Errorf("failed to read response body: %w", err)
	}

	err = events.Send(ctx, b)
	return errors.Join(err, streamErr)
}

func (i *StreamingResponsesInterceptor) newStream(ctx context.Context, srv responses.ResponseService, opts []option.RequestOption) *ssestream.Stream[responses.ResponseStreamEventUnion] {
	ctx, span := i.tracer.Start(ctx, "Intercept.ProcessRequest.Upstream", trace.WithAttributes(tracing.InterceptionAttributesFromContext(ctx)...))
	defer span.End()

	// The body is overridden by option.WithRequestBody(reqPayload) in requestOptions
	return srv.NewStreaming(ctx, responses.ResponseNewParams{}, opts...)
}
