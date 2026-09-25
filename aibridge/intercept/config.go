package intercept

import "net/http"

// Config is the per-request configuration an interceptor needs to process
// an interception, independent of which provider produced it. Providers
// resolve it in CreateInterceptor and hand it to the API-format
// interceptor.
type Config struct {
	// ProviderName is the provider instance name, used for recording,
	// logging, and API dumps.
	ProviderName string
	// BaseURL is the upstream provider's API base URL.
	BaseURL string
	// HTTPClient overrides the Messages SDK client for provider-specific transport behavior.
	// Nil uses the SDK's default client.
	HTTPClient *http.Client
	// APIDumpDir is the directory for dumping API requests and responses,
	// or empty when API dumping is disabled.
	APIDumpDir string
	// SendActorHeaders reports whether actor identity headers should be
	// forwarded to the upstream provider.
	SendActorHeaders bool
}
