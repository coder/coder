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
	aibcontext "github.com/coder/coder/v2/aibridge/context"
	"github.com/coder/coder/v2/aibridge/intercept"
	"github.com/coder/coder/v2/aibridge/keypool"
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
	cfg intercept.Config,
	cred intercept.Credential,
	clientHeaders http.Header,
	tracer trace.Tracer,
) *BlockingResponsesInterceptor {
	return &BlockingResponsesInterceptor{
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

	// Sum the key attempts across all iterations and record once when the
	// interception completes.
	var totalKeyAttempts int
	if cp, ok := intercept.AsCentralizedPool(i.cred); ok {
		defer func() {
			cp.Pool.RecordAttempts(totalKeyAttempts)
		}()
	}

	for shouldLoop {
		srv := i.newResponsesService(ctx)
		respCopy = responseCopier{}

		opts := i.requestOptions(&respCopy)
		opts = append(opts, option.WithRequestTimeout(time.Second*600))

		// TODO(ssncferreira): inject actor headers directly in the client-header
		//   middleware instead of using SDK options.
		if actor := aibcontext.ActorFromContext(r.Context()); actor != nil && i.cfg.SendActorHeaders {
			opts = append(opts, intercept.ActorHeadersAsOpenAIOpts(actor)...)
		}

		var keyAttempts int
		response, keyAttempts, upstreamErr = i.newResponse(ctx, srv, opts)
		totalKeyAttempts += keyAttempts

		// The failover loop may return a keypool exhaustion
		// error. Render it here.
		if upstreamErr != nil {
			var keyPoolErr *keypool.Error
			if errors.As(upstreamErr, &keyPoolErr) {
				i.writeUpstreamError(w, intercept.ResponseErrorFromKeyPool(keyPoolErr))
				return xerrors.Errorf("key pool exhausted: %w", upstreamErr)
			}
		}

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

// newResponse routes by credential type, returning the upstream response, the
// number of key attempts made for this call, and any error. A centralized key
// pool fails over across keys, while BYOK authenticates with a single, fixed
// credential baked into srv, so it makes one attempt.
func (i *BlockingResponsesInterceptor) newResponse(ctx context.Context, srv responses.ResponseService, opts []option.RequestOption) (*responses.Response, int, error) {
	if cp, ok := intercept.AsCentralizedPool(i.cred); ok {
		return i.newResponseWithKeyFailover(ctx, srv, cp, opts)
	}
	response, err := i.newResponseWithKey(intercept.WithCredentialInfo(ctx, i.cred), srv, opts)
	return response, 0, err
}

// newResponseWithKey performs a single upstream call.
func (i *BlockingResponsesInterceptor) newResponseWithKey(ctx context.Context, srv responses.ResponseService, opts []option.RequestOption) (_ *responses.Response, outErr error) {
	_, span := i.tracer.Start(ctx, "Intercept.ProcessRequest.Upstream", trace.WithAttributes(tracing.InterceptionAttributesFromContext(ctx)...))
	defer tracing.EndSpanErr(span, &outErr)

	// The body is overridden by option.WithRequestBody(reqPayload) in requestOptions
	return srv.New(ctx, responses.ResponseNewParams{}, opts...)
}

// newResponseWithKeyFailover walks the centralized key pool, trying each key
// until one succeeds or the pool is exhausted. Keys are marked temporary on
// 429 and permanent on 401/403. Errors that aren't key-specific don't trigger
// failover and are returned to the caller. It returns the upstream response,
// the number of key attempts made for this call, and any error.
func (i *BlockingResponsesInterceptor) newResponseWithKeyFailover(ctx context.Context, srv responses.ResponseService, cp *intercept.CentralizedPool, opts []option.RequestOption) (*responses.Response, int, error) {
	walker := cp.Pool.Walker()
	for {
		key, keyPoolErr := cp.NextKey(walker)
		if keyPoolErr != nil {
			return nil, walker.Attempts(), keyPoolErr
		}

		ctx = intercept.WithCredentialInfo(ctx, i.cred)
		i.logger.Debug(ctx, "using centralized api key")
		requestOpts := append([]option.RequestOption{}, opts...)
		requestOpts = append(requestOpts,
			option.WithAPIKey(key.Value()),
			// Disable SDK retries because the failover loop
			// handles retries via key rotation.
			option.WithMaxRetries(0),
		)
		response, err := i.newResponseWithKey(ctx, srv, requestOpts)
		// Key-specific failure: try the next key.
		if i.markKeyOnError(ctx, key, err) {
			continue
		}
		// Either success (response, nil) or a non-key error
		// (nil, err): nothing to retry, return as-is.
		return response, walker.Attempts(), err
	}
}
