package provider

import (
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

func (*OpenAI) Enabled() bool { return true }

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

	path := strings.TrimPrefix(r.URL.Path, p.RoutePrefix())
	switch path {
	case routeChatCompletions:
		var req chatcompletions.ChatCompletionNewParamsWrapper
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			return nil, xerrors.Errorf("unmarshal request body: %w", err)
		}

		if req.Stream {
			interceptor = chatcompletions.NewStreamingInterceptor(id, &req, cfg, cred, r.Header, tracer)
		} else {
			interceptor = chatcompletions.NewBlockingInterceptor(id, &req, cfg, cred, r.Header, tracer)
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
			interceptor = responses.NewStreamingInterceptor(id, reqPayload, cfg, cred, r.Header, tracer)
		} else {
			interceptor = responses.NewBlockingInterceptor(id, reqPayload, cfg, cred, r.Header, tracer)
		}

	default:
		span.SetStatus(codes.Error, "unknown route: "+r.URL.Path)
		return nil, ErrUnknownRoute
	}
	span.SetAttributes(interceptor.TraceAttributes(r)...)
	return interceptor, nil
}

// resolveCredential determines the upstream credential for a request. At this
// point the request contains only LLM provider headers. Any Coder-specific
// authentication has already been stripped. A BYOK token, if present, arrives
// in the Authorization header. Otherwise the request uses the provider's
// centralized key pool with failover, which must be configured.
func (p *OpenAI) resolveCredential(r *http.Request) (intercept.Credential, error) {
	if token := utils.ExtractBearerToken(r.Header.Get(intercept.AuthHeaderAuthorization)); token != "" {
		return intercept.BYOK{Secret: token, Header: intercept.AuthHeaderAuthorization}, nil
	}
	if p.cfg.KeyPool == nil {
		return nil, ErrNoCredential
	}
	return &intercept.CentralizedPool{Pool: p.cfg.KeyPool, Header: p.AuthHeader()}, nil
}

func (p *OpenAI) BaseURL() string {
	return p.cfg.BaseURL
}

func (*OpenAI) AuthHeader() string {
	return "Authorization"
}

func (p *OpenAI) KeyPool() *keypool.Pool {
	return p.cfg.KeyPool
}

func (p *OpenAI) KeyFailoverConfig(logger slog.Logger) keypool.KeyFailoverConfig {
	return keypool.KeyFailoverConfig{
		Pool:   p.cfg.KeyPool,
		Logger: logger,
		IsBYOK: func(r *http.Request) bool {
			return r.Header.Get("Authorization") != ""
		},
		InjectAuthKey: func(h *http.Header, key string) {
			h.Set("Authorization", "Bearer "+key)
		},
		BuildKeyPoolResponse: func(keyPoolErr *keypool.Error) *http.Response {
			return intercept.ResponseErrorFromKeyPool(keyPoolErr).ToResponse()
		},
	}
}

func (p *OpenAI) CircuitBreakerConfig() *config.CircuitBreaker {
	return p.circuitBreaker
}

func (p *OpenAI) APIDumpDir() string {
	return p.cfg.APIDumpDir
}
