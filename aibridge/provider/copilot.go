package provider

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/xerrors"

	"github.com/coder/aibridge/config"
	"github.com/coder/aibridge/intercept"
	"github.com/coder/aibridge/intercept/chatcompletions"
	"github.com/coder/aibridge/intercept/responses"
	"github.com/coder/aibridge/tracing"
	"github.com/coder/aibridge/utils"
)

const (
	copilotBaseURL = "https://api.individual.githubcopilot.com"

	// Copilot exposes an OpenAI-compatible API, including for Anthropic models.
	routeCopilotChatCompletions = "/chat/completions"
	routeCopilotResponses       = "/responses"
)

var copilotOpenErrorResponse = func() []byte {
	return []byte(`{"error":{"message":"circuit breaker is open","type":"server_error","code":"service_unavailable"}}`)
}

// Headers that need to be forwarded to Copilot API.
// These were determined through manual testing as there is no reference
// of the headers in the official documentation.
// LiteLLM uses the same headers:
// https://docs.litellm.ai/docs/providers/github_copilot
var copilotForwardHeaders = []string{
	"Editor-Version",
	"Copilot-Integration-Id",
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
	if cfg.APIDumpDir == "" {
		cfg.APIDumpDir = os.Getenv("BRIDGE_DUMP_DIR")
	}
	if cfg.MaxRetries == nil {
		if v := os.Getenv("COPILOT_MAX_RETRIES"); v != "" {
			if n, err := strconv.Atoi(v); err == nil {
				cfg.MaxRetries = &n
			}
		}
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

// InjectAuthHeader is a no-op for Copilot.
// Copilot uses per-user tokens passed in the original Authorization header,
// rather than a global key configured at the provider level.
// The original Authorization header flows through untouched from the client.
func (*Copilot) InjectAuthHeader(_ *http.Header) {}

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
	key := utils.ExtractBearerToken(r.Header.Get("Authorization"))
	if key == "" {
		span.SetStatus(codes.Error, "missing authorization")
		return nil, xerrors.New("missing Copilot authorization: Authorization header not found or invalid")
	}

	id := uuid.New()

	// Build config for the interceptor using the per-request key.
	// Copilot's API is OpenAI-compatible, so it uses the OpenAI interceptors
	// that require a config.OpenAI.
	cfg := config.OpenAI{
		BaseURL:        p.cfg.BaseURL,
		Key:            key,
		APIDumpDir:     p.cfg.APIDumpDir,
		CircuitBreaker: p.cfg.CircuitBreaker,
		ExtraHeaders:   extractCopilotHeaders(r),
		MaxRetries:     p.cfg.MaxRetries,
	}

	cred := intercept.NewCredentialInfo(intercept.CredentialKindBYOK, key)

	var interceptor intercept.Interceptor

	path := strings.TrimPrefix(r.URL.Path, p.RoutePrefix())
	switch path {
	case routeCopilotChatCompletions:
		var req chatcompletions.ChatCompletionNewParamsWrapper
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			return nil, xerrors.Errorf("unmarshal chat completions request body: %w", err)
		}

		if req.Stream {
			interceptor = chatcompletions.NewStreamingInterceptor(id, &req, p.Name(), cfg, r.Header, p.AuthHeader(), tracer, cred)
		} else {
			interceptor = chatcompletions.NewBlockingInterceptor(id, &req, p.Name(), cfg, r.Header, p.AuthHeader(), tracer, cred)
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

// extractCopilotHeaders extracts headers required by the Copilot API from the
// incoming request. Copilot requires certain client headers to be forwarded.
func extractCopilotHeaders(r *http.Request) map[string]string {
	headers := make(map[string]string, len(copilotForwardHeaders))
	for _, h := range copilotForwardHeaders {
		if v := r.Header.Get(h); v != "" {
			headers[h] = v
		}
	}
	return headers
}
