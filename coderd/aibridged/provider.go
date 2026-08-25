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

// BaseURLHostname returns the normalized hostname from an absolute HTTP(S)
// provider base URL. Invalid and scheme-less URLs return an empty string.
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
