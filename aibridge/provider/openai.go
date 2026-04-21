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
	if cfg.Key == "" {
		cfg.Key = os.Getenv("OPENAI_API_KEY")
	}
	if cfg.APIDumpDir == "" {
		cfg.APIDumpDir = os.Getenv("BRIDGE_DUMP_DIR")
	}
	if cfg.MaxRetries == nil {
		if v := os.Getenv("OPENAI_MAX_RETRIES"); v != "" {
			if n, err := strconv.Atoi(v); err == nil {
				cfg.MaxRetries = &n
			}
		}
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
	// At this point the request contains only LLM provider headers. Any
	// Coder-specific authentication has already been stripped.
	//
	// In centralized mode Authorization is absent, so cfg keeps the
	// centralized key unchanged.
	//
	// In BYOK mode the user's credential is in Authorization. Replace
	// the centralized key with it so it is forwarded upstream.
	credKind := intercept.CredentialKindCentralized
	if token := utils.ExtractBearerToken(r.Header.Get("Authorization")); token != "" {
		cfg.Key = token
		credKind = intercept.CredentialKindBYOK
	}
	cred := intercept.NewCredentialInfo(credKind, cfg.Key)

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

	headers.Set(p.AuthHeader(), "Bearer "+p.cfg.Key)
}

func (p *OpenAI) CircuitBreakerConfig() *config.CircuitBreaker {
	return p.circuitBreaker
}

func (p *OpenAI) APIDumpDir() string {
	return p.cfg.APIDumpDir
}
