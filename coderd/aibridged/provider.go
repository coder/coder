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
	// ProviderStatusProxyExcluded means another enabled provider already
	// claimed its hostname. The provider is still routable via the direct
	// path (/api/v2/ai-gateway/{name}/...).
	ProviderStatusProxyExcluded ProviderStatus = "proxy_excluded"
)

// ProviderOutcome classifies one ai_providers row. Err is set when
// Status is Error or ProxyExcluded and is already logged at the call site.
type ProviderOutcome struct {
	Name   string
	Type   string
	Status ProviderStatus
	Err    error
}

// BaseURLHostname returns the normalized hostname from a provider base
// URL. Scheme-less inputs get https:// prepended.
func BaseURLHostname(baseURL string) string {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		return ""
	}
	parsed, err := url.Parse(baseURL)
	if err == nil && parsed.Hostname() == "" && !strings.Contains(baseURL, "://") {
		parsed, err = url.Parse("https://" + baseURL)
	}
	if err != nil {
		return ""
	}
	return strings.ToLower(parsed.Hostname())
}
