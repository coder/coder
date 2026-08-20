package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/aibridge/circuitbreaker"
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

var _ Provider = &Anthropic{}

// Anthropic allows for interactions with the Anthropic API.
type Anthropic struct {
	cfg config.Anthropic
	// bedrock is nil for non-Bedrock providers.
	bedrock *messages.BedrockRuntime
}

const routeMessages = "/v1/messages" // https://docs.anthropic.com/en/api/messages

// Bedrock Mantle OpenAI protocol routes. The Anthropic provider bridges these
// only when bedrock is configured with the mantle protocol; plain anthropic
// providers return ErrUnknownRoute for them, preserving existing behavior.
// Throwaway per AIGOV-532.
const (
	routeBedrockChatCompletions = "/v1/chat/completions"
	routeBedrockResponses       = "/v1/responses"
)

var anthropicOpenErrorResponse = func() []byte {
	return []byte(`{"type":"error","error":{"type":"overloaded_error","message":"circuit breaker is open"}}`)
}

// statusOverloaded is the non-standard HTTP status Anthropic returns when its
// API is overloaded. The net/http package does not define a constant for it.
// https://platform.claude.com/docs/en/api/errors
const statusOverloaded = 529

var anthropicIsFailure = func(statusCode int) bool {
	if statusCode == statusOverloaded {
		return true
	}
	return circuitbreaker.DefaultIsFailure(statusCode)
}

func NewAnthropic(ctx context.Context, cfg config.Anthropic, bedrockCfg *config.AWSBedrock) (*Anthropic, error) {
	if cfg.Name == "" {
		cfg.Name = config.ProviderAnthropic
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.anthropic.com/"
	}
	if cfg.CircuitBreaker != nil {
		cfg.CircuitBreaker.IsFailure = anthropicIsFailure
		cfg.CircuitBreaker.OpenErrorResponse = anthropicOpenErrorResponse
	}

	// Resolve the AWS credentials provider once and bundle it with the config.
	// This performs no network call (the base identity and any AssumeRole
	// resolve lazily on first retrieval); it only wires up the provider chain,
	// so it is cheap to run at construction.
	var bedrock *messages.BedrockRuntime
	if bedrockCfg != nil {
		creds, resolvedRegion, err := buildBedrockCredentials(ctx, *bedrockCfg)
		if err != nil {
			return nil, xerrors.Errorf("build bedrock credentials: %w", err)
		}
		runtimeCfg := *bedrockCfg
		// resolvedRegion is bedrockCfg.Region if provided;
		// otherwise, it is resolved from the environment via awsconfig.LoadDefaultConfig
		if runtimeCfg.Region == "" {
			runtimeCfg.Region = resolvedRegion
		}
		if err := runtimeCfg.Validate(); err != nil {
			return nil, xerrors.Errorf("bedrock config: %w", err)
		}
		bedrock = &messages.BedrockRuntime{Cfg: runtimeCfg, Creds: creds}
	}

	return &Anthropic{
		cfg:     cfg,
		bedrock: bedrock,
	}, nil
}

func (*Anthropic) Type() string {
	return config.ProviderAnthropic
}

func (p *Anthropic) Name() string {
	return p.cfg.Name
}

func (*Anthropic) Enabled() bool { return true }

func (p *Anthropic) RoutePrefix() string {
	return fmt.Sprintf("/%s", p.Name())
}

func (p *Anthropic) BridgedRoutes() []string {
	// Bedrock mantle bridges OpenAI routes in addition to the native
	// Anthropic Messages route. Plain anthropic providers (bedrock == nil)
	// or non-mantle protocols bridge only Messages.
	// Throwaway per AIGOV-532.
	if p.bedrock != nil && p.bedrock.Cfg.ResolvedProtocol() == config.BedrockProtocolMantle {
		return []string{routeMessages, routeBedrockChatCompletions, routeBedrockResponses}
	}
	return []string{routeMessages}
}

func (*Anthropic) PassthroughRoutes() []string {
	return []string{
		"/v1/models",
		"/v1/models/", // See https://pkg.go.dev/net/http#hdr-Trailing_slash_redirection-ServeMux.
		"/v1/messages/count_tokens",
		"/api/event_logging/",
	}
}

func (p *Anthropic) CreateInterceptor(_ http.ResponseWriter, r *http.Request, tracer trace.Tracer) (_ intercept.Interceptor, outErr error) {
	id := uuid.New()
	_, span := tracer.Start(r.Context(), "Intercept.CreateInterceptor")
	defer tracing.EndSpanErr(span, &outErr)

	path := strings.TrimPrefix(r.URL.Path, p.RoutePrefix())
	switch path {
	case routeBedrockChatCompletions:
		return p.createChatCompletionsInterceptor(id, r, tracer, span)
	case routeBedrockResponses:
		return p.createResponsesInterceptor(id, r, tracer, span)
	case routeMessages:
		// Existing Anthropic Messages path, handled below.
	default:
		span.SetStatus(codes.Error, "unknown route: "+r.URL.Path)
		return nil, ErrUnknownRoute
	}
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
		interceptor = messages.NewStreamingInterceptor(id, reqPayload, cfg, cred, p.bedrock, r.Header, tracer)
	} else {
		interceptor = messages.NewBlockingInterceptor(id, reqPayload, cfg, cred, p.bedrock, r.Header, tracer)
	}
	span.SetAttributes(interceptor.TraceAttributes(r)...)
	return interceptor, nil
}

// resolveCredential determines the upstream credential for a request. At this
// point the request contains only LLM provider headers. Any Coder-specific
// authentication has already been stripped.
//
//   - X-Api-Key present: BYOK with a personal API key.
//   - Authorization present: BYOK with an access token.
//   - Neither present: centralized, using the provider's key pool with
//     failover.
//
// When both BYOK headers are present, X-Api-Key takes priority to match
// claude-code behavior. Centralized requests require a key pool, except for
// Bedrock providers, which authenticate via AWS signing rather than a pool.
func (p *Anthropic) resolveCredential(r *http.Request) (intercept.Credential, error) {
	if apiKey := r.Header.Get(intercept.AuthHeaderXAPIKey); apiKey != "" {
		return intercept.BYOK{Secret: apiKey, Header: intercept.AuthHeaderXAPIKey}, nil
	}
	if token := utils.ExtractBearerToken(r.Header.Get(intercept.AuthHeaderAuthorization)); token != "" {
		return intercept.BYOK{Secret: token, Header: intercept.AuthHeaderAuthorization}, nil
	}
	if p.cfg.KeyPool != nil {
		return &intercept.CentralizedPool{Pool: p.cfg.KeyPool, Header: p.AuthHeader()}, nil
	}
	if p.bedrock != nil {
		return intercept.Bedrock{AccessKey: p.bedrock.Cfg.AccessKey}, nil
	}
	return nil, ErrNoCredential
}

func (p *Anthropic) BaseURL() string {
	return p.cfg.BaseURL
}

func (*Anthropic) AuthHeader() string {
	return intercept.AuthHeaderXAPIKey
}

func (p *Anthropic) KeyPool() *keypool.Pool {
	return p.cfg.KeyPool
}

func (p *Anthropic) KeyFailoverConfig(logger slog.Logger) keypool.KeyFailoverConfig {
	return keypool.KeyFailoverConfig{
		Pool:   p.cfg.KeyPool,
		Logger: logger,
		IsBYOK: func(r *http.Request) bool {
			return r.Header.Get(intercept.AuthHeaderXAPIKey) != "" || r.Header.Get(intercept.AuthHeaderAuthorization) != ""
		},
		InjectAuthKey: func(h *http.Header, key string) {
			h.Set(intercept.AuthHeaderXAPIKey, key)
		},
		BuildKeyPoolResponse: func(keyPoolErr *keypool.Error) *http.Response {
			return messages.ResponseErrorFromKeyPool(keyPoolErr).ToResponse()
		},
	}
}

func (p *Anthropic) CircuitBreakerConfig() *config.CircuitBreaker {
	return p.cfg.CircuitBreaker
}

func (p *Anthropic) APIDumpDir() string {
	return p.cfg.APIDumpDir
}

func (*Anthropic) CategorizeError(err error) *recorder.ErrorType {
	return categorizeAnthropicError(err)
}

// categorizeAnthropicError categorizes a terminal error from an Anthropic
// (messages) provider. It returns nil when err is not an Anthropic-shaped error.
func categorizeAnthropicError(err error) *recorder.ErrorType {
	var status int
	var envErr *messages.ResponseError
	switch {
	case errors.As(err, &envErr):
		status = envErr.StatusCode
	default:
		apiErr := messages.ResponseErrorFromAPIError(err)
		if apiErr == nil {
			return nil
		}
		status = apiErr.StatusCode
	}
	t := recorder.ErrorTypeFromStatus(status)
	if status == statusOverloaded {
		t = recorder.ErrorTypeOverloaded
	}
	return &t
}

// createChatCompletionsInterceptor builds a chatcompletions interceptor wired
// to the Bedrock mantle runtime. It mirrors the OpenAI provider's dispatch:
// decode ChatCompletionNewParamsWrapper from r.Body, then pick streaming vs
// blocking. The bedrock runtime carries the SigV4 signing middleware.
// Throwaway per AIGOV-532.
func (p *Anthropic) createChatCompletionsInterceptor(id uuid.UUID, r *http.Request, tracer trace.Tracer, span trace.Span) (intercept.Interceptor, error) {
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
		interceptor = chatcompletions.NewStreamingInterceptor(id, &req, cfg, cred, p.mantleConfig(), r.Header, tracer)
	} else {
		interceptor = chatcompletions.NewBlockingInterceptor(id, &req, cfg, cred, p.mantleConfig(), r.Header, tracer)
	}
	span.SetAttributes(interceptor.TraceAttributes(r)...)
	return interceptor, nil
}

// createResponsesInterceptor builds a responses interceptor wired to the
// Bedrock mantle runtime. It mirrors the OpenAI provider's dispatch: read r.Body
// then parse via responses.NewRequestPayload, then pick streaming vs blocking.
// Throwaway per AIGOV-532.
func (p *Anthropic) createResponsesInterceptor(id uuid.UUID, r *http.Request, tracer trace.Tracer, span trace.Span) (intercept.Interceptor, error) {
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
		interceptor = responses.NewStreamingInterceptor(id, reqPayload, cfg, cred, p.mantleConfig(), r.Header, tracer)
	} else {
		interceptor = responses.NewBlockingInterceptor(id, reqPayload, cfg, cred, p.mantleConfig(), r.Header, tracer)
	}
	span.SetAttributes(interceptor.TraceAttributes(r)...)
	return interceptor, nil
}

// mantleConfig narrows the provider's full Bedrock runtime to the fields the
// OpenAI-shaped interceptors use. Returns nil when bedrock is not configured.
// Throwaway per AIGOV-532.
func (p *Anthropic) mantleConfig() *bedrocksig.MantleConfig {
	if p.bedrock == nil {
		return nil
	}
	return &bedrocksig.MantleConfig{
		BaseURL: p.bedrock.Cfg.BaseURL,
		Region:  p.bedrock.Cfg.Region,
		Creds:   p.bedrock.Creds,
	}
}

// bedrockInterceptConfig builds the per-request intercept.Config for Bedrock
// mantle OpenAI routes. The effective upstream base URL is resolved per-model
// inside the interceptor via bedrocksig.BaseURLForModel, so cfg.BaseURL is the
// provider's base (unused for signing but kept for recording/dump).
// Throwaway per AIGOV-532.
func (p *Anthropic) bedrockInterceptConfig() intercept.Config {
	return intercept.Config{
		ProviderName:     p.Name(),
		BaseURL:          p.bedrock.Cfg.BaseURL,
		APIDumpDir:       p.cfg.APIDumpDir,
		SendActorHeaders: p.cfg.SendActorHeaders,
	}
}
