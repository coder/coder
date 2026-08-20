package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/aibridge/config"
	"github.com/coder/coder/v2/aibridge/intercept"
	"github.com/coder/coder/v2/aibridge/intercept/bedrocksig"
	"github.com/coder/coder/v2/aibridge/intercept/chatcompletions"
	"github.com/coder/coder/v2/aibridge/intercept/messages"
	"github.com/coder/coder/v2/aibridge/intercept/responses"
	"github.com/coder/coder/v2/aibridge/keypool"
	"github.com/coder/coder/v2/aibridge/recorder"
	"github.com/coder/coder/v2/aibridge/tracing"
	"github.com/coder/coder/v2/aibridge/utils"
)

// Bedrock Mantle OpenAI protocol routes. The Bedrock provider bridges these
// only under the mantle protocol; the invoke-model protocol serves messages
// only, enforced in CreateInterceptor.
const (
	routeBedrockChatCompletions = "/v1/chat/completions"
	routeBedrockResponses       = "/v1/responses"
)

var _ Provider = &Bedrock{}

// Bedrock implements the Provider interface for AWS Bedrock. It serves the
// native Anthropic Messages route under both the invoke-model and mantle
// protocols, and the OpenAI-shaped chat-completions and responses routes under
// mantle only. Authentication is via AWS SigV4 signing, not bearer keys.
type Bedrock struct {
	cfg config.Anthropic
	// runtime is always non-nil for this provider.
	runtime *messages.BedrockRuntime
}

// NewBedrock constructs a Bedrock provider. cfg supplies the shared
// provider-level fields (Name, BaseURL, APIDumpDir, SendActorHeaders,
// CircuitBreaker); bedrockCfg supplies the Bedrock-specific runtime config and
// resolves the AWS credentials provider once at construction.
func NewBedrock(ctx context.Context, cfg config.Anthropic, bedrockCfg config.AWSBedrock) (*Bedrock, error) {
	if cfg.Name == "" {
		cfg.Name = config.ProviderBedrock
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.anthropic.com/"
	}
	if cfg.CircuitBreaker != nil {
		cfg.CircuitBreaker.IsFailure = anthropicIsFailure
		cfg.CircuitBreaker.OpenErrorResponse = anthropicOpenErrorResponse
	}

	creds, resolvedRegion, err := buildBedrockCredentials(ctx, bedrockCfg)
	if err != nil {
		return nil, xerrors.Errorf("build bedrock credentials: %w", err)
	}
	runtimeCfg := bedrockCfg
	// resolvedRegion is bedrockCfg.Region if provided; otherwise, it is
	// resolved from the environment via awsconfig.LoadDefaultConfig.
	if runtimeCfg.Region == "" {
		runtimeCfg.Region = resolvedRegion
	}
	if err := runtimeCfg.Validate(); err != nil {
		return nil, xerrors.Errorf("bedrock config: %w", err)
	}

	return &Bedrock{
		cfg:     cfg,
		runtime: &messages.BedrockRuntime{Cfg: runtimeCfg, Creds: creds},
	}, nil
}

func (*Bedrock) Type() string {
	return config.ProviderBedrock
}

func (p *Bedrock) Name() string {
	return p.cfg.Name
}

func (*Bedrock) Enabled() bool { return true }

func (p *Bedrock) RoutePrefix() string {
	return fmt.Sprintf("/%s", p.Name())
}

// BridgedRoutes returns all three routes for mux registration. Both the
// invoke-model and mantle protocols serve the messages route; the OpenAI
// routes are guarded in CreateInterceptor so an invoke-model provider still
// answers messages but returns ErrUnknownRoute for the OpenAI routes.
func (*Bedrock) BridgedRoutes() []string {
	return []string{routeMessages, routeBedrockChatCompletions, routeBedrockResponses}
}

func (*Bedrock) PassthroughRoutes() []string {
	return []string{
		"/v1/models",
		"/v1/models/", // See https://pkg.go.dev/net/http#hdr-Trailing_slash_redirection-ServeMux.
		"/v1/messages/count_tokens",
		"/api/event_logging/",
	}
}

func (p *Bedrock) CreateInterceptor(_ http.ResponseWriter, r *http.Request, tracer trace.Tracer) (_ intercept.Interceptor, outErr error) {
	id := uuid.New()
	_, span := tracer.Start(r.Context(), "Intercept.CreateInterceptor")
	defer tracing.EndSpanErr(span, &outErr)

	path := strings.TrimPrefix(r.URL.Path, p.RoutePrefix())
	switch path {
	case routeMessages:
		return p.createMessagesInterceptor(id, r, tracer, span)
	case routeBedrockChatCompletions:
		// The OpenAI routes only make sense for mantle. InvokeModel keeps
		// messages-only behavior through this guard rather than BridgedRoutes,
		// because BridgedRoutes is also used for mux registration and an
		// invoke-model provider must still not 404 on messages.
		//
		if p.runtime.Cfg.ResolvedProtocol() != config.BedrockProtocolMantle {
			span.SetStatus(codes.Error, "unknown route: "+r.URL.Path)
			return nil, ErrUnknownRoute
		}
		return p.createChatCompletionsInterceptor(id, r, tracer, span)
	case routeBedrockResponses:
		if p.runtime.Cfg.ResolvedProtocol() != config.BedrockProtocolMantle {
			span.SetStatus(codes.Error, "unknown route: "+r.URL.Path)
			return nil, ErrUnknownRoute
		}
		return p.createResponsesInterceptor(id, r, tracer, span)
	default:
		span.SetStatus(codes.Error, "unknown route: "+r.URL.Path)
		return nil, ErrUnknownRoute
	}
}

// createMessagesInterceptor builds a messages interceptor wired to the full
// Bedrock runtime. The runtime carries the InvokeModel remap config and the
// SigV4 signing credentials.
func (p *Bedrock) createMessagesInterceptor(id uuid.UUID, r *http.Request, tracer trace.Tracer, span trace.Span) (intercept.Interceptor, error) {
	payload, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, xerrors.Errorf("read body: %w", err)
	}

	reqPayload, err := messages.NewRequestPayload(payload)
	if err != nil {
		return nil, xerrors.Errorf("unmarshal request body: %w", err)
	}

	cfg := intercept.Config{
		ProviderName:     p.Name(),
		BaseURL:          p.cfg.BaseURL,
		APIDumpDir:       p.cfg.APIDumpDir,
		SendActorHeaders: p.cfg.SendActorHeaders,
	}
	cred, err := p.resolveCredential(r)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		return nil, xerrors.Errorf("resolve credential: %w", err)
	}

	var interceptor intercept.Interceptor
	if reqPayload.Stream() {
		interceptor = messages.NewStreamingInterceptor(id, reqPayload, cfg, cred, p.runtime, r.Header, tracer)
	} else {
		interceptor = messages.NewBlockingInterceptor(id, reqPayload, cfg, cred, p.runtime, r.Header, tracer)
	}
	span.SetAttributes(interceptor.TraceAttributes(r)...)
	return interceptor, nil
}

// createChatCompletionsInterceptor builds a chatcompletions interceptor wired
// to the Bedrock mantle runtime. It mirrors the OpenAI provider's dispatch:
// decode ChatCompletionNewParamsWrapper from r.Body, then pick streaming vs
// blocking. The bedrock runtime carries the SigV4 signing middleware.
func (p *Bedrock) createChatCompletionsInterceptor(id uuid.UUID, r *http.Request, tracer trace.Tracer, span trace.Span) (intercept.Interceptor, error) {
	var req chatcompletions.ChatCompletionNewParamsWrapper
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return nil, xerrors.Errorf("unmarshal request body: %w", err)
	}

	cfg := p.bedrockInterceptConfig()
	cred, err := p.resolveCredential(r)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		return nil, xerrors.Errorf("resolve credential: %w", err)
	}

	var interceptor intercept.Interceptor
	if req.Stream {
		interceptor = chatcompletions.NewBedrockStreamingInterceptor(id, &req, cfg, cred, p.mantleConfig(), r.Header, tracer)
	} else {
		interceptor = chatcompletions.NewBedrockBlockingInterceptor(id, &req, cfg, cred, p.mantleConfig(), r.Header, tracer)
	}
	span.SetAttributes(interceptor.TraceAttributes(r)...)
	return interceptor, nil
}

// createResponsesInterceptor builds a responses interceptor wired to the
// Bedrock mantle runtime. It mirrors the OpenAI provider's dispatch: read
// r.Body then parse via responses.NewRequestPayload, then pick streaming vs
// blocking.
func (p *Bedrock) createResponsesInterceptor(id uuid.UUID, r *http.Request, tracer trace.Tracer, span trace.Span) (intercept.Interceptor, error) {
	payload, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, xerrors.Errorf("read body: %w", err)
	}
	reqPayload, err := responses.NewRequestPayload(payload)
	if err != nil {
		return nil, xerrors.Errorf("unmarshal request body: %w", err)
	}

	cfg := p.bedrockInterceptConfig()
	cred, err := p.resolveCredential(r)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		return nil, xerrors.Errorf("resolve credential: %w", err)
	}

	var interceptor intercept.Interceptor
	if reqPayload.Stream() {
		interceptor = responses.NewBedrockStreamingInterceptor(id, reqPayload, cfg, cred, p.mantleConfig(), r.Header, tracer)
	} else {
		interceptor = responses.NewBedrockBlockingInterceptor(id, reqPayload, cfg, cred, p.mantleConfig(), r.Header, tracer)
	}
	span.SetAttributes(interceptor.TraceAttributes(r)...)
	return interceptor, nil
}

// mantleConfig narrows the provider's full Bedrock runtime to the fields the
// OpenAI-shaped interceptors use. The runtime is always non-nil for this
// provider.
func (p *Bedrock) mantleConfig() *bedrocksig.MantleConfig {
	return &bedrocksig.MantleConfig{
		BaseURL: p.runtime.Cfg.BaseURL,
		Region:  p.runtime.Cfg.Region,
		Creds:   p.runtime.Creds,
	}
}

// bedrockInterceptConfig builds the per-request intercept.Config for Bedrock
// mantle OpenAI routes. The effective upstream base URL is resolved per-model
// inside the interceptor via bedrocksig.BaseURLForModel, so cfg.BaseURL is the
// provider's base (unused for signing but kept for recording/dump).
func (p *Bedrock) bedrockInterceptConfig() intercept.Config {
	return intercept.Config{
		ProviderName:     p.Name(),
		BaseURL:          p.runtime.Cfg.BaseURL,
		APIDumpDir:       p.cfg.APIDumpDir,
		SendActorHeaders: p.cfg.SendActorHeaders,
	}
}

// resolveCredential determines the upstream credential for a request. Bedrock
// authenticates via AWS signing, so when no BYOK header is present it returns
// the Bedrock credential backed by the runtime's access key. BYOK
// X-Api-Key/Authorization headers are honored for users who bring their own
// key.
func (p *Bedrock) resolveCredential(r *http.Request) (intercept.Credential, error) {
	if apiKey := r.Header.Get(intercept.AuthHeaderXAPIKey); apiKey != "" {
		return intercept.BYOK{Secret: apiKey, Header: intercept.AuthHeaderXAPIKey}, nil
	}
	if token := utils.ExtractBearerToken(r.Header.Get(intercept.AuthHeaderAuthorization)); token != "" {
		return intercept.BYOK{Secret: token, Header: intercept.AuthHeaderAuthorization}, nil
	}
	return intercept.Bedrock{AccessKey: p.runtime.Cfg.AccessKey}, nil
}

func (p *Bedrock) BaseURL() string {
	return p.cfg.BaseURL
}

func (*Bedrock) AuthHeader() string {
	return intercept.AuthHeaderXAPIKey
}

// KeyPool returns nil. Bedrock authenticates via AWS signing, not a key pool.
func (*Bedrock) KeyPool() *keypool.Pool {
	return nil
}

func (*Bedrock) KeyFailoverConfig(_ slog.Logger) keypool.KeyFailoverConfig {
	// No key pool: the KeyFailoverTransport short-circuits. Bedrock uses AWS
	// signing rather than bearer-key failover.
	return keypool.KeyFailoverConfig{}
}

func (p *Bedrock) CircuitBreakerConfig() *config.CircuitBreaker {
	return p.cfg.CircuitBreaker
}

func (p *Bedrock) APIDumpDir() string {
	return p.cfg.APIDumpDir
}

// CategorizeError tries the Anthropic error categorizer first (the messages
// route) and falls back to the OpenAI categorizer (the chat-completions and
// responses routes), mirroring how copilot falls back across categorizers.
func (*Bedrock) CategorizeError(err error) *recorder.ErrorType {
	if t := categorizeAnthropicError(err); t != nil {
		return t
	}
	return categorizeOpenAIError(err)
}
