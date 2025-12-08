package aibridged

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/xerrors"

	"github.com/coder/aibridge"
)

var _ aibridge.Provider = &AmpProvider{}

const (
	ProviderAmp      = "amp"
	ampRouteMessages = "/amp/v1/messages"
)

type (
	AmpConfig = aibridge.ProviderConfig
)

type AmpProvider struct {
	cfg AmpConfig
}

func NewAmpProvider(cfg AmpConfig) *AmpProvider {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://ampcode.com/api/provider/anthropic"
	}
	return &AmpProvider{cfg: cfg}
}

func (p *AmpProvider) Name() string {
	return ProviderAmp
}

func (p *AmpProvider) BaseURL() string {
	return p.cfg.BaseURL
}

// BridgedRoutes returns routes that will be intercepted.
func (p *AmpProvider) BridgedRoutes() []string {
	return []string{ampRouteMessages}
}

// PassthroughRoutes returns routes that are proxied directly.
func (p *AmpProvider) PassthroughRoutes() []string {
	return []string{
		"/v1/models",
		"/v1/models/",
		"/v1/messages/count_tokens",
	}
}

func (p *AmpProvider) AuthHeader() string {
	return "X-Api-Key"
}

// InjectAuthHeader Amp already makes the request with X-Api-Key containing the authenticated user's API key
// One key per user instead of a global key.
func (p *AmpProvider) InjectAuthHeader(h *http.Header) {}

// CreateInterceptor creates an interceptor for the request.
// Reuses Anthropic's interceptor since Amp uses the same API format.
func (p *AmpProvider) CreateInterceptor(w http.ResponseWriter, r *http.Request, tracer trace.Tracer) (aibridge.Interceptor, error) {
	// Capture the API key from the incoming request
	apiKey := r.Header.Get("X-Api-Key")

	payload, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, xerrors.Errorf("read body: %w", err)
	}

	id := uuid.New()

	switch r.URL.Path {
	case ampRouteMessages:
		var req aibridge.MessageNewParamsWrapper
		if err := json.Unmarshal(payload, &req); err != nil {
			return nil, xerrors.Errorf("failed to unmarshal request: %w", err)
		}

		// Reuse Anthropic interceptors as Amp uses the same API format
		ampCfg := aibridge.AnthropicConfig{
			BaseURL: p.cfg.BaseURL,
			Key:     apiKey,
		}

		if req.Stream {
			return aibridge.NewAnthropicMessagesStreamingInterception(id, &req, ampCfg, nil, tracer), nil
		}

		return aibridge.NewAnthropicMessagesBlockingInterception(id, &req, ampCfg, nil, tracer), nil
	}

	return nil, aibridge.UnknownRoute
}
