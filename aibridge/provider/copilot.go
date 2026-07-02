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
	"github.com/coder/coder/v2/aibridge/intercept/messages"
	"github.com/coder/coder/v2/aibridge/intercept/responses"
	"github.com/coder/coder/v2/aibridge/keypool"
	"github.com/coder/coder/v2/aibridge/tracing"
	"github.com/coder/coder/v2/aibridge/utils"
)

const (
	copilotBaseURL = "https://api.individual.githubcopilot.com"

	// Copilot exposes an OpenAI-compatible API, including for Anthropic models.
	routeCopilotChatCompletions = "/chat/completions"
	routeCopilotResponses       = "/responses"
	routeCopilotMessages        = "/v1/messages"
)

var copilotOpenErrorResponse = func() []byte {
	return []byte(`{"error":{"message":"circuit breaker is open","type":"server_error","code":"service_unavailable"}}`)
}

// Copilot implements the Provider interface for GitHub Copilot.
// Unlike other providers, Copilot uses per-user API keys that are passed through
// the request headers rather than configured statically.
type Copilot struct {
	cfg            config.Copilot
	circuitBreaker *config.CircuitBreaker
}

var _ Provider = &Copilot{}

func NewCopilot(cfg config.Copilot) *Copilot {
	if cfg.Name == "" {
		cfg.Name = config.ProviderCopilot
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = copilotBaseURL
	}
	if cfg.CircuitBreaker != nil {
		cfg.CircuitBreaker.OpenErrorResponse = copilotOpenErrorResponse
	}
	return &Copilot{
		cfg:            cfg,
		circuitBreaker: cfg.CircuitBreaker,
	}
}

func (*Copilot) Type() string {
	return config.ProviderCopilot
}

func (p *Copilot) Name() string {
	return p.cfg.Name
}

func (*Copilot) Enabled() bool { return true }

func (p *Copilot) BaseURL() string {
	return p.cfg.BaseURL
}

func (p *Copilot) RoutePrefix() string {
	return fmt.Sprintf("/%s", p.Name())
}

func (*Copilot) BridgedRoutes() []string {
	return []string{
		routeCopilotChatCompletions,
		routeCopilotResponses,
		routeCopilotMessages,
	}
}

func (*Copilot) PassthroughRoutes() []string {
	return []string{
		"/models",
		"/models/",
		"/agents/",
		"/mcp/",
		"/.well-known/",
	}
}

func (*Copilot) AuthHeader() string {
	return "Authorization"
}

// KeyPool returns nil. Copilot is always BYOK and has no key pool.
func (*Copilot) KeyPool() *keypool.Pool {
	return nil
}

// KeyFailoverConfig returns a config with a nil Pool, which makes
// the KeyFailoverTransport short-circuit. Copilot is always BYOK.
func (*Copilot) KeyFailoverConfig(_ slog.Logger) keypool.KeyFailoverConfig {
	return keypool.KeyFailoverConfig{}
}

func (p *Copilot) CircuitBreakerConfig() *config.CircuitBreaker {
	return p.circuitBreaker
}

func (p *Copilot) APIDumpDir() string {
	return p.cfg.APIDumpDir
}

func (p *Copilot) CreateInterceptor(_ http.ResponseWriter, r *http.Request, tracer trace.Tracer) (_ intercept.Interceptor, outErr error) {
	_, span := tracer.Start(r.Context(), "Intercept.CreateInterceptor")
	defer tracing.EndSpanErr(span, &outErr)

	// Extract the per-user Copilot key from the Authorization header.
	key := utils.ExtractBearerToken(r.Header.Get(intercept.AuthHeaderAuthorization))
	if key == "" {
		span.SetStatus(codes.Error, "missing authorization")
		return nil, xerrors.New("missing Copilot authorization: Authorization header not found or invalid")
	}

	id := uuid.New()

	// Copilot's API is OpenAI-compatible, so it reuses the OpenAI interceptors.
	// It is always BYOK: the per-user key arrives in the Authorization header.
	cfg := intercept.Config{
		ProviderName: p.Name(),
		BaseURL:      p.cfg.BaseURL,
		APIDumpDir:   p.cfg.APIDumpDir,
	}
	cred := intercept.BYOK{Secret: key, Header: intercept.AuthHeaderAuthorization}

	var interceptor intercept.Interceptor

	path := strings.TrimPrefix(r.URL.Path, p.RoutePrefix())
	switch path {
	case routeCopilotChatCompletions:
		var req chatcompletions.ChatCompletionNewParamsWrapper
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			return nil, xerrors.Errorf("unmarshal chat completions request body: %w", err)
		}

		if req.Stream {
			interceptor = chatcompletions.NewStreamingInterceptor(id, &req, cfg, cred, r.Header, tracer)
		} else {
			interceptor = chatcompletions.NewBlockingInterceptor(id, &req, cfg, cred, r.Header, tracer)
		}

	case routeCopilotResponses:
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

	case routeCopilotMessages:
		payload, err := io.ReadAll(r.Body)
		if err != nil {
			return nil, xerrors.Errorf("read body: %w", err)
		}
		reqPayload, err := messages.NewRequestPayload(payload)
		if err != nil {
			return nil, xerrors.Errorf("unmarshal request body: %w", err)
		}

		if reqPayload.Stream() {
			interceptor = messages.NewStreamingInterceptor(id, reqPayload, cfg, cred, nil, r.Header, tracer)
		} else {
			interceptor = messages.NewBlockingInterceptor(id, reqPayload, cfg, cred, nil, r.Header, tracer)
		}

	default:
		span.SetStatus(codes.Error, "unknown route: "+r.URL.Path)
		return nil, ErrUnknownRoute
	}

	span.SetAttributes(interceptor.TraceAttributes(r)...)
	return interceptor, nil
}
