package aibridged

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/google/uuid"

	"github.com/coder/aibridge"
)

var _ aibridge.Provider = &AmpProvider{}

const (
	ProviderAmp      = "amp"
	ampRouteMessages = "/amp/v1/messages" // How aibridge identifies this route
)

type AmpConfig struct {
	BaseURL string
	Key     string
}

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
//   TODO(ssncferreira): should these include internal routes to amp?
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

func (p *AmpProvider) InjectAuthHeader(h *http.Header) {
	fmt.Println("########################## provider amp headers: ", h)
	// Amp already makes the request with X-Api-Key containing the authenticated user's API key
	//if h == nil || p.cfg.Key == "" {
	//	return
	//}
	//h.Set(p.AuthHeader(), p.cfg.Key)
}

// CreateInterceptor creates an interceptor for the request.
// Reuses Anthropic's interceptor since Amp uses the same API format.
func (p *AmpProvider) CreateInterceptor(w http.ResponseWriter, r *http.Request) (aibridge.Interceptor, error) {
	// Capture the API key from the incoming request
	apiKey := r.Header.Get("X-Api-Key")

	payload, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	id := uuid.New()

	switch r.URL.Path {
	case ampRouteMessages:
		var req aibridge.MessageNewParamsWrapper
		if err := json.Unmarshal(payload, &req); err != nil {
			return nil, fmt.Errorf("failed to unmarshal request: %w", err)
		}

		// Reuse Anthropic interceptors as Amp uses the same API format
		ampCfg := aibridge.AnthropicConfig{
			BaseURL: p.cfg.BaseURL,
			Key:     apiKey,
		}

		if req.Stream {
			return aibridge.NewAnthropicMessagesStreamingInterception(id, &req, ampCfg, nil), nil
		}

		return aibridge.NewAnthropicMessagesBlockingInterception(id, &req, ampCfg, nil), nil
	}

	return nil, aibridge.UnknownRoute
}
