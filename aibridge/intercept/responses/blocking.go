package responses

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/responses"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/aibridge/config"
	aibcontext "github.com/coder/coder/v2/aibridge/context"
	"github.com/coder/coder/v2/aibridge/intercept"
	"github.com/coder/coder/v2/aibridge/mcp"
	"github.com/coder/coder/v2/aibridge/recorder"
	"github.com/coder/coder/v2/aibridge/tracing"
)

type BlockingResponsesInterceptor struct {
	responsesInterceptionBase
}

func NewBlockingInterceptor(
	id uuid.UUID,
	reqPayload RequestPayload,
	providerName string,
	cfg config.OpenAI,
	clientHeaders http.Header,
	authHeaderName string,
	tracer trace.Tracer,
	cred intercept.CredentialInfo,
) *BlockingResponsesInterceptor {
	return &BlockingResponsesInterceptor{
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

func (i *BlockingResponsesInterceptor) Setup(logger slog.Logger, rec recorder.Recorder, mcpProxy mcp.ServerProxier) {
	i.responsesInterceptionBase.Setup(logger.Named("blocking"), rec, mcpProxy)
}

func (*BlockingResponsesInterceptor) Streaming() bool {
	return false
}

func (i *BlockingResponsesInterceptor) TraceAttributes(r *http.Request) []attribute.KeyValue {
	return i.responsesInterceptionBase.baseTraceAttributes(r, false)
}

func (i *BlockingResponsesInterceptor) ProcessRequest(w http.ResponseWriter, r *http.Request) (outErr error) {
	ctx, span := i.tracer.Start(r.Context(), "Intercept.ProcessRequest", trace.WithAttributes(tracing.InterceptionAttributesFromContext(r.Context())...))
	defer tracing.EndSpanErr(span, &outErr)

	if err := i.validateRequest(ctx, w); err != nil {
		return err
	}

	i.injectTools()

	var (
		response        *responses.Response
		upstreamErr     error
		respCopy        responseCopier
		firstResponseID string
	)

	prompt, promptFound, err := i.reqPayload.lastUserPrompt(ctx, i.logger)
	if err != nil {
		i.logger.Warn(ctx, "failed to get user prompt", slog.Error(err))
	}
	shouldLoop := true

	for shouldLoop {
		srv := i.newResponsesService()
		respCopy = responseCopier{}

		opts := i.requestOptions(&respCopy)
		opts = append(opts, option.WithRequestTimeout(time.Second*600))

		// TODO(ssncferreira): inject actor headers directly in the client-header
		//   middleware instead of using SDK options.
		if actor := aibcontext.ActorFromContext(r.Context()); actor != nil && i.cfg.SendActorHeaders {
			opts = append(opts, intercept.ActorHeadersAsOpenAIOpts(actor)...)
		}

		response, upstreamErr = i.newResponse(ctx, srv, opts)

		if upstreamErr != nil || response == nil {
			break
		}

		if firstResponseID == "" {
			firstResponseID = response.ID
		}

		i.recordTokenUsage(ctx, response)
		i.recordModelThoughts(ctx, response)

		// Check if there any injected tools to invoke.
		pending := i.getPendingInjectedToolCalls(response)
		shouldLoop, err = i.handleInnerAgenticLoop(ctx, pending, response)
		if err != nil {
			i.sendCustomErr(ctx, w, http.StatusInternalServerError, err)
			shouldLoop = false
		}
	}

	if promptFound {
		i.recordUserPrompt(ctx, firstResponseID, prompt)
	}
	i.recordNonInjectedToolUsage(ctx, response)

	if upstreamErr != nil && !respCopy.responseReceived.Load() {
		// no response received from upstream, return custom error
		i.sendCustomErr(ctx, w, http.StatusInternalServerError, upstreamErr)
		return xerrors.Errorf("failed to connect to upstream: %w", upstreamErr)
	}

	err = respCopy.forwardResp(w)
	return errors.Join(upstreamErr, err)
}

func (i *BlockingResponsesInterceptor) newResponse(ctx context.Context, srv responses.ResponseService, opts []option.RequestOption) (_ *responses.Response, outErr error) {
	ctx, span := i.tracer.Start(ctx, "Intercept.ProcessRequest.Upstream", trace.WithAttributes(tracing.InterceptionAttributesFromContext(ctx)...))
	defer tracing.EndSpanErr(span, &outErr)

	// The body is overridden by option.WithRequestBody(reqPayload) in requestOptions
	return srv.New(ctx, responses.ResponseNewParams{}, opts...)
}
