package provider

import (
	"context"
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
	"github.com/coder/coder/v2/aibridge/tracing"
	"github.com/coder/coder/v2/aibridge/utils"
)

var _ Provider = &Anthropic{}

// Anthropic allows for interactions with the Anthropic API.
type Anthropic struct {
	cfg config.Anthropic
	// bedrock is nil for non-Bedrock providers.
	bedrock *messages.BedrockRuntime
	// claudePlatform is nil for non-Claude-Platform providers.
	claudePlatform *messages.ClaudePlatformAWSRuntime
}

const routeMessages = "/v1/messages" // https://docs.anthropic.com/en/api/messages

var anthropicOpenErrorResponse = func() []byte {
	return []byte(`{"type":"error","error":{"type":"overloaded_error","message":"circuit breaker is open"}}`)
}

var anthropicIsFailure = func(statusCode int) bool {
	// https://platform.claude.com/docs/en/api/errors
	if statusCode == 529 {
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
		bedrock = &messages.BedrockRuntime{Cfg: runtimeCfg, Creds: creds}
	}

	// Resolve the Claude Platform for AWS request options once. Like Bedrock,
	// this wires up the provider chain and signing middleware without making a
	// network call, so it is cheap to run at construction and is reused across
	// all requests.
	var claudePlatform *messages.ClaudePlatformAWSRuntime
	if claudePlatformCfg != nil {
		opts, err := buildClaudePlatformAWSOptions(ctx, *claudePlatformCfg)
		if err != nil {
			return nil, xerrors.Errorf("build claude platform for aws options: %w", err)
		}
		claudePlatform = &messages.ClaudePlatformAWSRuntime{Cfg: *claudePlatformCfg, Options: opts}
	}

	return &Anthropic{
		cfg:            cfg,
		bedrock:        bedrock,
		claudePlatform: claudePlatform,
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
		interceptor = messages.NewStreamingInterceptor(id, reqPayload, cfg, cred, p.bedrock, p.claudePlatform, r.Header, tracer)
	} else {
		interceptor = messages.NewBlockingInterceptor(id, reqPayload, cfg, cred, p.bedrock, p.claudePlatform, r.Header, tracer)
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
	if p.claudePlatform != nil {
		return intercept.ClaudePlatformAWS{
			APIKey:    p.claudePlatform.Cfg.APIKey,
			AccessKey: p.claudePlatform.Cfg.AccessKey,
		}, nil
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
