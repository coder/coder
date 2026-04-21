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
	"github.com/coder/aibridge/config"
	aibcontext "github.com/coder/aibridge/context"
	"github.com/coder/aibridge/intercept"
	"github.com/coder/aibridge/intercept/eventstream"
	"github.com/coder/aibridge/mcp"
	"github.com/coder/aibridge/recorder"
	"github.com/coder/aibridge/tracing"
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
	providerName string,
	cfg config.OpenAI,
	clientHeaders http.Header,
	authHeaderName string,
	tracer trace.Tracer,
	cred intercept.CredentialInfo,
) *StreamingResponsesInterceptor {
	return &StreamingResponsesInterceptor{
		responsesInterceptionBase: responsesInterceptionBase{
			id:             id,
			providerName:   providerName,
			reqPayload:     reqPayload,
			cfg:            cfg,
			clientHeaders:  clientHeaders,
			authHeaderName: authHeaderName,
			tracer:         tracer,
			credential:     cred,
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
	r = r.WithContext(ctx) // Rewire context for SSE cancellation.

	if err := i.validateRequest(ctx, w); err != nil {
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

	prompt, promptFound, err := i.reqPayload.lastUserPrompt(ctx, i.logger)
	if err != nil {
		i.logger.Warn(ctx, "failed to get user prompt", slog.Error(err))
	}
	shouldLoop := true
	srv := i.newResponsesService()

	for shouldLoop {
		shouldLoop = false

		respCopy = responseCopier{}
		opts := i.requestOptions(&respCopy)

		// TODO(ssncferreira): inject actor headers directly in the client-header
		//   middleware instead of using SDK options.
		if actor := aibcontext.ActorFromContext(r.Context()); actor != nil && i.cfg.SendActorHeaders {
			opts = append(opts, intercept.ActorHeadersAsOpenAIOpts(actor)...)
		}
		stream := i.newStream(ctx, srv, opts)

		// func scope to defer steam.Close()
		err := func() error {
			defer stream.Close()

			if upstreamErr := stream.Err(); upstreamErr != nil {
				// events stream should never be initialized
				if events.IsStreaming() {
					i.logger.Warn(ctx, "event stream was initialized when no response was received from upstream")
					return upstreamErr
				}

				// no response received from upstream (eg. client/connection error), return custom error
				if !respCopy.responseReceived.Load() {
					i.sendCustomErr(ctx, w, http.StatusInternalServerError, upstreamErr)
					return upstreamErr
				}

				// forward received response as-is
				err := respCopy.forwardResp(w)
				return errors.Join(upstreamErr, err)
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
			shouldLoop, innerLoopErr = i.handleInnerAgenticLoop(ctx, pending, completedResponse)
			if innerLoopErr != nil {
				i.sendCustomErr(ctx, w, http.StatusInternalServerError, innerLoopErr)
				shouldLoop = false
			}

			// Record token usage for each inner loop iteration
			i.recordTokenUsage(ctx, completedResponse)
		}

		i.recordModelThoughts(ctx, completedResponse)
	}

	if promptFound {
		i.recordUserPrompt(ctx, firstResponseID, prompt)
	}
	i.recordNonInjectedToolUsage(ctx, completedResponse)

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
