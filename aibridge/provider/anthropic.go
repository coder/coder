package provider

import (
	"context"
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
	"github.com/coder/coder/v2/aibridge/intercept/messages"
	"github.com/coder/coder/v2/aibridge/keypool"
	"github.com/coder/coder/v2/aibridge/recorder"
	"github.com/coder/coder/v2/aibridge/tracing"
	"github.com/coder/coder/v2/aibridge/utils"
)

var _ Provider = &Anthropic{}

// Anthropic allows for interactions with the Anthropic API.
type Anthropic struct {
	cfg config.Anthropic
	// auth carries the provider's AWS-backed authentication runtime. Both
	// fields are nil for a plain bearer-token Anthropic provider.
	auth messages.AuthRuntime
}

const routeMessages = "/v1/messages" // https://docs.anthropic.com/en/api/messages

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

func NewAnthropic(ctx context.Context, cfg config.Anthropic, bedrockCfg *config.AWSBedrock, claudePlatformCfg *config.AWSClaudePlatform) (*Anthropic, error) {
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
	if bedrockCfg != nil && claudePlatformCfg != nil {
		return nil, xerrors.New("bedrock and claude platform configuration are mutually exclusive")
	}

	// Resolve the AWS credentials provider once and bundle it with the config.
	// This performs no network call (the base identity and any AssumeRole
	// resolve lazily on first retrieval); it only wires up the provider chain,
	// so it is cheap to run at construction.
	var auth messages.AuthRuntime
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
		auth.Bedrock = &messages.BedrockRuntime{Cfg: runtimeCfg, Creds: creds}
	}

	if claudePlatformCfg != nil {
		runtimeCfg := *claudePlatformCfg
		// Unlike Bedrock, the region is never inferred from the AWS environment:
		// it selects the upstream host and the signing scope, so an implicit
		// value would silently route traffic to the wrong region.
		if err := runtimeCfg.Validate(); err != nil {
			return nil, xerrors.Errorf("claude platform config: %w", err)
		}

		runtime := &messages.ClaudePlatformRuntime{Cfg: runtimeCfg}
		if runtimeCfg.AuthMode == config.ClaudePlatformAuthModeIAM {
			creds, _, err := buildAWSCredentials(ctx, awsCredentialSpec{
				Region:          runtimeCfg.Region,
				AccessKey:       runtimeCfg.AccessKey,
				AccessKeySecret: runtimeCfg.AccessKeySecret,
				RoleARN:         runtimeCfg.RoleARN,
				ExternalID:      runtimeCfg.ExternalID,
			})
			if err != nil {
				return nil, xerrors.Errorf("build claude platform credentials: %w", err)
			}
			runtime.Creds = creds
		}
		auth.ClaudePlatform = runtime
	}

	return &Anthropic{
		cfg:  cfg,
		auth: auth,
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

func (*Anthropic) BridgedRoutes() []string {
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
	if path != routeMessages {
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
		BaseURL:          p.BaseURL(),
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
		interceptor = messages.NewStreamingInterceptor(id, reqPayload, cfg, cred, p.auth, r.Header, tracer)
	} else {
		interceptor = messages.NewBlockingInterceptor(id, reqPayload, cfg, cred, p.auth, r.Header, tracer)
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
// AWS-signed providers (Bedrock, and Claude Platform in IAM mode), which
// authenticate via request signing rather than a pool.
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
	if p.auth.Bedrock != nil {
		return intercept.AWSSigV4{AccessKey: p.auth.Bedrock.Cfg.AccessKey}, nil
	}
	if cp := p.auth.ClaudePlatform; cp != nil && cp.Cfg.AuthMode == config.ClaudePlatformAuthModeIAM {
		return intercept.AWSSigV4{AccessKey: cp.Cfg.AccessKey}, nil
	}
	return nil, ErrNoCredential
}

// BaseURL returns the provider's upstream base URL. Claude Platform for AWS
// derives it from the configured region unless explicitly overridden, so its
// passthrough routes reach the same host as bridged requests.
func (p *Anthropic) BaseURL() string {
	if p.auth.ClaudePlatform != nil {
		return p.auth.ClaudePlatform.Cfg.ResolvedBaseURL()
	}
	return p.cfg.BaseURL
}

// WrapPassthroughTransport authenticates passthrough routes for providers whose
// credential is not a static header value. Passthrough auth is otherwise
// key-pool driven, which leaves an IAM-mode Claude Platform provider sending
// unsigned requests to /v1/models and friends.
//
// It is installed beneath the key failover transport, so a BYOK or centralized
// key still wins; this only fills the gap where neither is present.
func (p *Anthropic) WrapPassthroughTransport(inner http.RoundTripper) http.RoundTripper {
	cp := p.auth.ClaudePlatform
	if cp == nil {
		return inner
	}
	return &claudePlatformPassthroughTransport{inner: inner, cfg: cp.Cfg, creds: cp.Creds}
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
