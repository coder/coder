package provider

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
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
	// bedrockCfg is nil for non-Bedrock providers.
	bedrockCfg *config.AWSBedrock
	// awsCfg carries the region and the credentials provider that sign Bedrock
	// requests. It is meaningful only alongside bedrockCfg.
	awsCfg aws.Config
	// profiles resolves configured model identifiers on first use. It is shared
	// across providers so a reload does not discard resolutions.
	profiles *InferenceProfileCache
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

// NewAnthropic constructs a provider. It makes no network call: Bedrock
// credentials resolve lazily on first retrieval, and application inference
// profile ARNs resolve on the first request that needs them. Construction
// therefore cannot fail because AWS is slow or briefly unreachable, which would
// otherwise drop the provider from the reloaded snapshot.
func NewAnthropic(ctx context.Context, cfg config.Anthropic, bedrockCfg *config.AWSBedrock, profiles *InferenceProfileCache) (*Anthropic, error) {
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

	p := &Anthropic{cfg: cfg, profiles: profiles}
	if bedrockCfg == nil {
		return p, nil
	}
	if profiles == nil {
		return nil, xerrors.New("developer error: bedrock provider requires an inference profile cache")
	}

	// Resolve the AWS credentials provider once and bundle it with the config.
	// This performs no network call (the base identity and any AssumeRole
	// resolve lazily on first retrieval); it only wires up the provider chain,
	// so it is cheap to run at construction.
	awsCfg, err := buildBedrockCredentials(ctx, *bedrockCfg)
	if err != nil {
		return nil, xerrors.Errorf("build bedrock credentials: %w", err)
	}
	runtimeCfg := *bedrockCfg
	// awsCfg.Region is bedrockCfg.Region if provided;
	// otherwise, it is resolved from the environment via awsconfig.LoadDefaultConfig
	if runtimeCfg.Region == "" {
		runtimeCfg.Region = awsCfg.Region
	}
	if err := runtimeCfg.Validate(); err != nil {
		return nil, xerrors.Errorf("bedrock config: %w", err)
	}

	p.bedrockCfg = &runtimeCfg
	p.awsCfg = awsCfg
	return p, nil
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
		BaseURL:          p.cfg.BaseURL,
		APIDumpDir:       p.cfg.APIDumpDir,
		SendActorHeaders: p.cfg.SendActorHeaders,
	}
	cred, err := p.resolveCredential(r)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		return nil, xerrors.Errorf("resolve credential: %w", err)
	}

	var bedrock *messages.BedrockRuntime
	if p.bedrockCfg != nil {
		bedrock, err = p.bedrockRuntime(r.Context())
		if err != nil {
			span.SetStatus(codes.Error, err.Error())
			return nil, err
		}
	}

	var interceptor intercept.Interceptor
	if reqPayload.Stream() {
		interceptor = messages.NewStreamingInterceptor(id, reqPayload, cfg, cred, bedrock, r.Header, tracer)
	} else {
		interceptor = messages.NewBlockingInterceptor(id, reqPayload, cfg, cred, bedrock, r.Header, tracer)
	}
	span.SetAttributes(interceptor.TraceAttributes(r)...)
	return interceptor, nil
}

// bedrockRuntime pairs the configured Bedrock settings with the resolved model
// identities. Resolution only calls AWS for application inference profile ARNs,
// and only until the cache holds them, so deployments configured with plain
// model IDs never call AWS here and need no extra permission.
func (p *Anthropic) bedrockRuntime(ctx context.Context) (*messages.BedrockRuntime, error) {
	model, err := p.profiles.Resolve(ctx, p.awsCfg, p.bedrockCfg.Model)
	if err != nil {
		return nil, xerrors.Errorf("resolve model: %w", err)
	}
	smallFastModel, err := p.profiles.Resolve(ctx, p.awsCfg, p.bedrockCfg.SmallFastModel)
	if err != nil {
		return nil, xerrors.Errorf("resolve small fast model: %w", err)
	}
	return messages.NewBedrockRuntime(*p.bedrockCfg, p.awsCfg.Credentials, model, smallFastModel), nil
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
	if p.bedrockCfg != nil {
		return intercept.Bedrock{AccessKey: p.bedrockCfg.AccessKey}, nil
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
