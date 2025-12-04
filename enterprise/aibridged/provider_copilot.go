package aibridged

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/google/uuid"

	"github.com/coder/aibridge"
)

var _ aibridge.Provider = &CopilotProvider{}

const (
	ProviderCopilot             = "copilot"
	copilotRouteChatCompletions = "/copilot/v1/chat/completions"
)

type CopilotConfig struct {
	BaseURL string
	Key     string
}

type CopilotProvider struct {
	cfg CopilotConfig
}

func NewCopilotProvider(cfg CopilotConfig) *CopilotProvider {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.individual.githubcopilot.com"
	}
	return &CopilotProvider{cfg: cfg}
}

func (p *CopilotProvider) Name() string {
	return ProviderCopilot
}

func (p *CopilotProvider) BaseURL() string {
	return p.cfg.BaseURL
}

// BridgedRoutes returns routes that will be intercepted.
func (p *CopilotProvider) BridgedRoutes() []string {
	return []string{copilotRouteChatCompletions}
}

// PassthroughRoutes returns routes that are proxied directly.
func (p *CopilotProvider) PassthroughRoutes() []string {
	return []string{
		"/v1/models",
		"/v1/models/",   // See https://pkg.go.dev/net/http#hdr-Trailing_slash_redirection-ServeMux.
		"/v1/responses", // TODO: support Responses API.
	}
}

func (p *CopilotProvider) AuthHeader() string {
	return "Authorization"
}

func (p *CopilotProvider) InjectAuthHeader(h *http.Header) {
	if h == nil || p.cfg.Key == "" {
		return
	}
	h.Set(p.AuthHeader(), "Bearer "+p.cfg.Key)
}

// CreateInterceptor creates an interceptor for the request.
// Reuses OpenAI's interceptor since Copilot uses the same API format.
func (p *CopilotProvider) CreateInterceptor(w http.ResponseWriter, r *http.Request) (aibridge.Interceptor, error) {
	payload, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	id := uuid.New()

	switch r.URL.Path {
	case copilotRouteChatCompletions:
		var req aibridge.ChatCompletionNewParamsWrapper
		if err := json.Unmarshal(payload, &req); err != nil {
			return nil, fmt.Errorf("failed to unmarshal request: %w", err)
		}

		// Reuse OpenAI interceptors as Copilot uses the same API format
		if req.Stream {
			return aibridge.NewOpenAIStreamingChatInterception(id, &req, p.cfg.BaseURL, p.cfg.Key), nil
		}

		return aibridge.NewOpenAIBlockingChatInterception(id, &req, p.cfg.BaseURL, p.cfg.Key), nil
	}

	return nil, aibridge.UnknownRoute
}
