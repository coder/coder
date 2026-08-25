package aibridged

import (
	"net/url"
	"strings"
)

// ProviderStatus is the lifecycle state of a configured AI provider.
type ProviderStatus string

const (
	// ProviderStatusEnabled indicates the provider is configured and
	// valid, and is included in the active pool snapshot.
	ProviderStatusEnabled ProviderStatus = "enabled"
	// ProviderStatusDisabled indicates the provider is configured but
	// intentionally turned off by an operator.
	ProviderStatusDisabled ProviderStatus = "disabled"
	// ProviderStatusError indicates the provider is configured but
	// cannot be constructed (missing keys, unsupported type, malformed
	// settings).
	ProviderStatusError ProviderStatus = "error"
	// ProviderStatusProxyExcluded means another enabled provider
	// already claimed its hostname. The provider is still routable via
	// the direct path (/api/v2/ai-gateway/{name}/...).
	ProviderStatusProxyExcluded ProviderStatus = "proxy_excluded"
)

// ProviderOutcome classifies one ai_providers row, including
// disabled rows (503 sentinel), errored rows (excluded from pool),
// and proxy-excluded rows (excluded from proxy routing).
// Err is set when Status is Error or ProxyExcluded; the build error
// is already logged at the call site.
type ProviderOutcome struct {
	Name   string
	Type   string
	Status ProviderStatus
	Err    error
}

// BaseURLHostname returns the normalized hostname from a provider
// base URL. It is the canonical normalization used by the proxy
// classifier and the API status check. The base URL must be absolute
// with an http or https scheme and a hostname; every other input,
// including a scheme-less value such as "example.com/v1", returns an
// empty string so callers report the provider as misconfigured instead
// of routing traffic to a guessed host.
func BaseURLHostname(baseURL string) string {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		return ""
	}
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return ""
	}
	// url.Parse lowercases the scheme, so a direct comparison is enough.
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return ""
	}
	return strings.ToLower(parsed.Hostname())
}
