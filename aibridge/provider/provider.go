package provider

import (
	"net/http"

	"go.opentelemetry.io/otel/trace"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/aibridge/config"
	"github.com/coder/coder/v2/aibridge/intercept"
	"github.com/coder/coder/v2/aibridge/keypool"
)

var ErrUnknownRoute = xerrors.New("unknown route")

// ErrNoCredential is returned when a request resolves to centralized
// authentication but the provider has no centralized keys configured (and the
// request is not BYOK), so it cannot be authenticated.
var ErrNoCredential = xerrors.New("no credential: request is not BYOK and the provider has no centralized keys")

// Provider defines routes (bridged and passed through) for given provider.
// Bridged routes are processed by dedicated interceptors.
//
// All routes have following pattern:
//   - https://coder.host.com/api/v2 + /ai-gateway      + /{provider.RoutePrefix()}  + /{bridged or passthrough route}
//     {host}                          {ai-gateway root}   {provider prefix}            {provider route}
//
// {host} + {ai-gateway root} + {provider prefix} form the base URL used in tools/clients using AI Gateway (e.g. Claude/Codex).
//
// When request is bridged, interceptor created based on route processes the request.
// When request is passed through the {host} + {ai-gateway root} + {provider prefix} URL part
// is replaced by provider's base URL and request is forwarded.
// This mirrors behavior in bridged routes and SDKs used by interceptors.
//
// Example:
//
//   - OpenAI chat completions
//     AI Gateway base URL (set in Codex): "https://host.coder.com/api/v2/ai-gateway/openai/v1"
//     Upstream base URl (set in coder config): http://api.openai.com/v1
//     Request: Codex -> https://host.coder.com/api/v2/ai-gateway/openai/v1/chat/completions -> AI Gateway -> http://api.openai.com/v1/chat/completions
//     url change: 'https://host.coder.com/api/v2/ai-gateway/openai/v1' -> 'http://api.openai.com/v1' | '/chat/completions' suffix remains the same
//
//   - Anthropic messages
//     AI Gateway base URL (set in Codex): "https://host.coder.com/api/v2/ai-gateway/anthropic"
//     Upstream base URl (set in coder config): http://api.anthropic.com
//     Request: Codex -> https://host.coder.com/api/v2/ai-gateway/anthropic/v1/messages -> AI Gateway -> http://api.anthropic.com/v1/messages
//     url change: 'https://host.coder.com/api/v2/ai-gateway/anthropic' -> 'http://api.anthropic.com' | '/v1/messages' suffix remains the same
//
// !Note!
// OpenAI and Anthropic use different route patterns.
// OpenAI includes the version '/v1' in the base url while Anthropic does not.
// More details/examples: https://github.com/coder/aibridge/pull/174#discussion_r2782320152
type Provider interface {
	// Type returns the provider type: "copilot", "openai", or "anthropic".
	// Multiple provider instances can share the same type.
	Type() string
	// Name returns the provider instance name.
	// Defaults to Type() when not explicitly configured.
	Name() string
	// Enabled reports whether the provider should serve requests.
	Enabled() bool
	// BaseURL defines the base URL endpoint for this provider's API.
	BaseURL() string

	// CreateInterceptor starts a new [Interceptor] which is responsible for intercepting requests,
	// communicating with the upstream provider and formulating a response to be sent to the requesting client.
	CreateInterceptor(http.ResponseWriter, *http.Request, trace.Tracer) (intercept.Interceptor, error)

	// RoutePrefix returns a prefix on which the provider's bridged and passthroguh routes will be registered.
	// Must be unique across providers to avoid conflicts.
	RoutePrefix() string

	// BridgedRoutes returns a slice of [http.ServeMux]-compatible routes which will have special handling.
	// See https://pkg.go.dev/net/http#hdr-Patterns-ServeMux.
	BridgedRoutes() []string
	// PassthroughRoutes returns a slice of whitelisted [http.ServeMux]-compatible* routes which are
	// not currently intercepted and must be handled by the upstream directly.
	//
	// * only path routes can be specified, not ones containing HTTP methods. (i.e. GET /route).
	// By default, these passthrough routes will accept any HTTP method.
	PassthroughRoutes() []string

	// AuthHeader returns the name of the header which the provider expects to find its authentication
	// token in.
	AuthHeader() string
	// KeyFailoverConfig returns the per-provider configuration for
	// automatic key failover on passthrough routes.
	KeyFailoverConfig(logger slog.Logger) keypool.KeyFailoverConfig

	// KeyPool returns the provider's key pool for centralized keys, or nil
	// when the provider is BYOK only.
	KeyPool() *keypool.Pool

	// CircuitBreakerConfig returns the circuit breaker configuration for the provider.
	CircuitBreakerConfig() *config.CircuitBreaker

	// APIDumpDir returns the directory path for dumping API requests and responses.
	// Empty string is returned when API dumping is not enabled.
	APIDumpDir() string
}
