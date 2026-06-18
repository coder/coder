package provider

import (
	"context"
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
	"github.com/coder/coder/v2/aibridge/tracing"
	"github.com/coder/coder/v2/aibridge/utils"
	"github.com/coder/quartz"
)

// anthropicForwardHeaders lists headers from incoming requests that should be
// forwarded to the Anthropic API.
// TODO(ssncferreira): remove as part of https://github.com/coder/aibridge/issues/192
var anthropicForwardHeaders = []string{
	"Anthropic-Beta",
}

var _ Provider = &Anthropic{}

// Anthropic allows for interactions with the Anthropic API.
type Anthropic struct {
	cfg        config.Anthropic
	bedrockCfg *config.AWSBedrock
	// bedrockCreds is the AWS credentials provider (including any assumed
	// role), resolved once at construction and shared across requests so
	// per-request retrieval is served from its cache rather than re-resolved.
	// nil when this provider is not Bedrock-backed.
	bedrockCreds aws.CredentialsProvider
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

func NewAnthropic(ctx context.Context, cfg config.Anthropic, bedrockCfg *config.AWSBedrock) (*Anthropic, error) {
	if cfg.Name == "" {
		cfg.Name = config.ProviderAnthropic
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.anthropic.com/"
	}
	// Resolve centralized key configuration into KeyPool.
	// Precedence:
	//   1. cfg.KeyPool (explicit, highest priority).
	//   2. cfg.Key (legacy single key).
	// After this block cfg.Key is empty so it can only carry a
	// BYOK X-Api-Key set per interception in CreateInterceptor.
	// TODO(ssncferreira): simplify auth field resolution per
	// https://github.com/coder/aibridge/issues/266.
	if cfg.KeyPool == nil && cfg.Key != "" {
		// keypool.New only fails on empty or duplicate keys,
		// neither possible with a single non-empty key.
		pool, err := keypool.New(cfg.Name, []string{cfg.Key}, quartz.NewReal(), nil)
		if err != nil {
			panic(fmt.Sprintf("anthropic provider: build single-key pool: %s", err))
		}
		cfg.KeyPool = pool
	}
	cfg.Key = ""
	if cfg.CircuitBreaker != nil {
		cfg.CircuitBreaker.IsFailure = anthropicIsFailure
		cfg.CircuitBreaker.OpenErrorResponse = anthropicOpenErrorResponse
	}

	// Resolve the AWS credentials provider once. This performs no network
	// call (the base identity and any AssumeRole resolve lazily on first
	// retrieval); it only wires up the provider chain, so it is cheap to run
	// at construction and on every provider reload.
	var bedrockCreds aws.CredentialsProvider
	if bedrockCfg != nil {
		var err error
		bedrockCreds, err = buildBedrockCredentials(ctx, *bedrockCfg)
		if err != nil {
			return nil, xerrors.Errorf("build bedrock credentials: %w", err)
		}
	}

	return &Anthropic{
		cfg:          cfg,
		bedrockCfg:   bedrockCfg,
		bedrockCreds: bedrockCreds,
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

	cfg := p.cfg
	cfg.ExtraHeaders = extractAnthropicHeaders(r)

	// At this point the request contains only LLM provider headers.
	// Any Coder-specific authentication has already been stripped.
	//
	// In centralized mode neither Authorization nor X-Api-Key is
	// present, so cfg keeps the KeyPool from provider construction
	// and the failover loop walks it.
	//
	// In BYOK mode the user's LLM credentials survive intact and
	// failover is disabled by clearing cfg.KeyPool. If X-Api-Key is
	// present the user has a personal API key, populate cfg.Key.
	// If Authorization is present the user authenticated directly
	// with the provider, populate cfg.BYOKBearerToken. When both
	// are present, X-Api-Key takes priority to match claude-code
	// behavior.
	//
	// TODO(ssncferreira): consolidate auth field handling per
	// https://github.com/coder/aibridge/issues/266.
	credKind := intercept.CredentialKindCentralized
	var credSecret string
	authHeaderName := p.AuthHeader()
	if apiKey := r.Header.Get("X-Api-Key"); apiKey != "" {
		cfg.Key = apiKey
		cfg.KeyPool = nil
		authHeaderName = "X-Api-Key"
		credKind = intercept.CredentialKindBYOK
		credSecret = apiKey
	} else if token := utils.ExtractBearerToken(r.Header.Get("Authorization")); token != "" {
		cfg.BYOKBearerToken = token
		cfg.KeyPool = nil
		authHeaderName = "Authorization"
		credKind = intercept.CredentialKindBYOK
		credSecret = token
	}
	// Centralized leaves credSecret empty: the hint is set by the
	// failover loop on each key attempt and persisted at
	// end-of-interception.
	cred := intercept.NewCredentialInfo(credKind, credSecret)

	// bedrockCreds was resolved once at construction; it is a shared,
	// rotating credentials cache. nil for non-Bedrock providers.
	var interceptor intercept.Interceptor
	if reqPayload.Stream() {
		interceptor = messages.NewStreamingInterceptor(id, reqPayload, p.Name(), cfg, p.bedrockCfg, p.bedrockCreds, r.Header, authHeaderName, tracer, cred)
	} else {
		interceptor = messages.NewBlockingInterceptor(id, reqPayload, p.Name(), cfg, p.bedrockCfg, p.bedrockCreds, r.Header, authHeaderName, tracer, cred)
	}
	span.SetAttributes(interceptor.TraceAttributes(r)...)
	return interceptor, nil
}

func (p *Anthropic) BaseURL() string {
	return p.cfg.BaseURL
}

func (*Anthropic) AuthHeader() string {
	return "X-Api-Key"
}

func (p *Anthropic) KeyPool() *keypool.Pool {
	return p.cfg.KeyPool
}

func (p *Anthropic) KeyFailoverConfig(logger slog.Logger) keypool.KeyFailoverConfig {
	return keypool.KeyFailoverConfig{
		Pool:   p.cfg.KeyPool,
		Logger: logger,
		IsBYOK: func(r *http.Request) bool {
			return r.Header.Get("X-Api-Key") != "" || r.Header.Get("Authorization") != ""
		},
		InjectAuthKey: func(h *http.Header, key string) {
			h.Set("X-Api-Key", key)
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

// extractAnthropicHeaders extracts headers required by the Anthropic API from
// the incoming request.
// TODO(ssncferreira): remove as part of https://github.com/coder/aibridge/issues/192
func extractAnthropicHeaders(r *http.Request) map[string]string {
	headers := make(map[string]string, len(anthropicForwardHeaders))
	for _, h := range anthropicForwardHeaders {
		if v := r.Header.Get(h); v != "" {
			headers[h] = v
		}
	}
	return headers
}
