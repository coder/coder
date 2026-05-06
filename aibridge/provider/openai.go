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
	"github.com/coder/coder/v2/aibridge/intercept/chatcompletions"
	"github.com/coder/coder/v2/aibridge/intercept/responses"
	"github.com/coder/coder/v2/aibridge/keypool"
	"github.com/coder/coder/v2/aibridge/tracing"
	"github.com/coder/coder/v2/aibridge/utils"
	"github.com/coder/quartz"
)

const (
	routeChatCompletions = "/chat/completions" // https://platform.openai.com/docs/api-reference/chat
	routeResponses       = "/responses"        // https://platform.openai.com/docs/api-reference/responses
)

var openAIOpenErrorResponse = func() []byte {
	return []byte(`{"error":{"message":"circuit breaker is open","type":"server_error","code":"service_unavailable"}}`)
}

// OpenAI allows for interactions with the OpenAI API.
type OpenAI struct {
	cfg            config.OpenAI
	circuitBreaker *config.CircuitBreaker
}

var _ Provider = &OpenAI{}

func NewOpenAI(cfg config.OpenAI) *OpenAI {
	if cfg.Name == "" {
		cfg.Name = config.ProviderOpenAI
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.openai.com/v1/"
	}
	// Resolve centralized key configuration into KeyPool.
	// Precedence:
	//   1. cfg.KeyPool (explicit, highest priority).
	//   2. cfg.Key (legacy single key).
	// After this block cfg.Key is empty so it can only carry a
	// BYOK Authorization Bearer set per interception in
	// CreateInterceptor.
	// TODO(ssncferreira): simplify auth field resolution per
	// https://github.com/coder/aibridge/issues/266.
	if cfg.KeyPool == nil && cfg.Key != "" {
		// keypool.New only fails on empty or duplicate keys,
		// neither possible with a single non-empty key.
		pool, err := keypool.New([]string{cfg.Key}, quartz.NewReal())
		if err != nil {
			panic(fmt.Sprintf("openai provider: build single-key pool: %s", err))
		}
		cfg.KeyPool = pool
	}
	cfg.Key = ""
	if cfg.CircuitBreaker != nil {
		cfg.CircuitBreaker.OpenErrorResponse = openAIOpenErrorResponse
	}

	return &OpenAI{
		cfg:            cfg,
		circuitBreaker: cfg.CircuitBreaker,
	}
}

func (*OpenAI) Type() string {
	return config.ProviderOpenAI
}

func (p *OpenAI) Name() string {
	return p.cfg.Name
}

func (p *OpenAI) RoutePrefix() string {
	// Route prefix includes version to match default OpenAI base URL.
	// More detailed explanation: https://github.com/coder/aibridge/pull/174#discussion_r2782320152
	return fmt.Sprintf("/%s/v1", p.Name())
}

func (*OpenAI) BridgedRoutes() []string {
	return []string{
		routeChatCompletions,
		routeResponses,
	}
}

// PassthroughRoutes define the routes which are not currently intercepted
// but must be passed through to the upstream.
// The /v1/completions legacy API is deprecated and will not be passed through.
// See https://platform.openai.com/docs/api-reference/completions.
func (*OpenAI) PassthroughRoutes() []string {
	return []string{
		// See https://pkg.go.dev/net/http#hdr-Trailing_slash_redirection-ServeMux.
		// but without non trailing slash route requests to `/v1/conversations` are going to catch all
		"/conversations",
		"/conversations/",
		"/models",
		"/models/",
		"/responses/", // Forwards other responses API endpoints, eg: https://platform.openai.com/docs/api-reference/responses/get
	}
}

func (p *OpenAI) CreateInterceptor(_ http.ResponseWriter, r *http.Request, tracer trace.Tracer) (_ intercept.Interceptor, outErr error) {
	id := uuid.New()

	_, span := tracer.Start(r.Context(), "Intercept.CreateInterceptor")
	defer tracing.EndSpanErr(span, &outErr)

	var interceptor intercept.Interceptor

	cfg := p.cfg
	// At this point the request contains only LLM provider headers.
	// Any Coder-specific authentication has already been stripped.
	//
	// In centralized mode Authorization is absent, so cfg keeps the
	// KeyPool from provider construction and the failover loop walks
	// it.
	//
	// In BYOK mode the user's credential is in Authorization,
	// populate cfg.Key and clear cfg.KeyPool so failover is disabled.
	//
	// TODO(ssncferreira): consolidate auth field handling per
	// https://github.com/coder/aibridge/issues/266.
	credKind := intercept.CredentialKindCentralized
	var credSecret string
	if token := utils.ExtractBearerToken(r.Header.Get("Authorization")); token != "" {
		cfg.Key = token
		cfg.KeyPool = nil
		credKind = intercept.CredentialKindBYOK
		credSecret = token
	} else if cfg.KeyPool != nil {
		// Centralized: use the first key as a placeholder hint.
		// TODO(ssncferreira): record the actually-used key in
		// the interception record to reflect failover.
		if k, err := cfg.KeyPool.Walker().Next(); err == nil {
			credSecret = k.Value()
		}
	}
	cred := intercept.NewCredentialInfo(credKind, credSecret)

	path := strings.TrimPrefix(r.URL.Path, p.RoutePrefix())
	switch path {
	case routeChatCompletions:
		var req chatcompletions.ChatCompletionNewParamsWrapper
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			return nil, xerrors.Errorf("unmarshal request body: %w", err)
		}

		if req.Stream {
			interceptor = chatcompletions.NewStreamingInterceptor(id, &req, p.Name(), cfg, r.Header, p.AuthHeader(), tracer, cred)
		} else {
			interceptor = chatcompletions.NewBlockingInterceptor(id, &req, p.Name(), cfg, r.Header, p.AuthHeader(), tracer, cred)
		}

	case routeResponses:
		payload, err := io.ReadAll(r.Body)
		if err != nil {
			return nil, xerrors.Errorf("read body: %w", err)
		}
		reqPayload, err := responses.NewRequestPayload(payload)
		if err != nil {
			return nil, xerrors.Errorf("unmarshal request body: %w", err)
		}
		if reqPayload.Stream() {
			interceptor = responses.NewStreamingInterceptor(id, reqPayload, p.Name(), cfg, r.Header, p.AuthHeader(), tracer, cred)
		} else {
			interceptor = responses.NewBlockingInterceptor(id, reqPayload, p.Name(), cfg, r.Header, p.AuthHeader(), tracer, cred)
		}

	default:
		span.SetStatus(codes.Error, "unknown route: "+r.URL.Path)
		return nil, ErrUnknownRoute
	}
	span.SetAttributes(interceptor.TraceAttributes(r)...)
	return interceptor, nil
}

func (p *OpenAI) BaseURL() string {
	return p.cfg.BaseURL
}

func (*OpenAI) AuthHeader() string {
	return "Authorization"
}

func (p *OpenAI) InjectAuthHeader(headers *http.Header) {
	if headers == nil {
		headers = &http.Header{}
	}

	// BYOK: if the request already carries user-supplied credentials,
	// do not overwrite them with the centralized key.
	if headers.Get("Authorization") != "" {
		return
	}

	// Centralized: pull a single key from the pool. No failover
	// or exhaustion handling here.
	// TODO(ssncferreira): replace with RoundTripper-based auth
	// in the upstack passthrough PR.
	if p.cfg.KeyPool == nil {
		return
	}
	if key, err := p.cfg.KeyPool.Walker().Next(); err == nil {
		headers.Set(p.AuthHeader(), "Bearer "+key.Value())
	}
}

func (p *OpenAI) KeyFailoverConfig(logger slog.Logger) keypool.KeyFailoverConfig {
	name := p.Name()
	return keypool.KeyFailoverConfig{
		Pool: p.cfg.KeyPool,
		IsBYOK: func(r *http.Request) bool {
			return r.Header.Get("Authorization") != ""
		},
		InjectAuthKey: func(h *http.Header, key string) {
			h.Set("Authorization", "Bearer "+key)
		},
		MarkKey: func(ctx context.Context, key *keypool.Key, resp *http.Response) bool {
			return keypool.MarkKeyOnStatus(ctx, key, resp, logger, name)
		},
		BuildExhaustedResponse: func(err error) *http.Response {
			return chatcompletions.MapExhaustionError(err).ToResponse()
		},
	}
}

func (p *OpenAI) CircuitBreakerConfig() *config.CircuitBreaker {
	return p.circuitBreaker
}

func (p *OpenAI) APIDumpDir() string {
	return p.cfg.APIDumpDir
}
