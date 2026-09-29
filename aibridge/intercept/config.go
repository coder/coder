package intercept

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
	// APIDumpDir is the directory for dumping API requests and responses,
	// or empty when API dumping is disabled.
	APIDumpDir string
	// ActorHeaderNames selects the destinations for id and username.
	// Nil turns forwarding off; an empty map enables cleanup without injection.
	// Attributes without a destination are not forwarded.
	ActorHeaderNames map[string]string
}
