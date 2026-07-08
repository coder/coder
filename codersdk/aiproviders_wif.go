package codersdk

import "path/filepath"

// AIProviderSettingsTypeWIF is the _type discriminator value for
// AIProviderWIFSettings.
const AIProviderSettingsTypeWIF = "wif"

// AIProviderWIFSettingsVersion is the current schema version of
// AIProviderWIFSettings.
const AIProviderWIFSettingsVersion = 1

// AIProviderWIFSettings configures providers that authenticate via
// Anthropic Workload Identity Federation. The gateway exchanges an
// OIDC identity token for a short-lived Anthropic access token
// instead of using static API keys.
type AIProviderWIFSettings struct {
	// FederationRuleID is the tagged ID (fdrl_...) of the Anthropic
	// federation rule governing this exchange. Required.
	FederationRuleID string `json:"federation_rule_id"`
	// OrganizationID is the UUID of the Anthropic organization.
	// Required.
	OrganizationID string `json:"organization_id"`
	// IdentityTokenFile is the path to a file containing the OIDC
	// identity token (JWT). The file is re-read on every exchange.
	// Required.
	IdentityTokenFile string `json:"identity_token_file"`
	// ServiceAccountID is the svac_... tagged ID of the target service
	// account. Anthropic's WIF reference requires it for token exchange
	// under SERVICE_ACCOUNT-target federation rules; it is omitted from
	// the exchange request only for USER-target rules, where the
	// principal derives from the JWT claims.
	ServiceAccountID string `json:"service_account_id,omitempty"`
	// WorkspaceID is the wrkspc_... tagged ID or the literal "default".
	// Required when the federation rule is enabled for more than one
	// workspace; when omitted the server selects the rule's sole
	// enabled workspace.
	WorkspaceID string `json:"workspace_id,omitempty"`
}

// IsConfigured reports whether the minimum required WIF fields are set.
func (w AIProviderWIFSettings) IsConfigured() bool {
	return w.FederationRuleID != "" && w.OrganizationID != "" && w.IdentityTokenFile != ""
}

func (AIProviderWIFSettings) settingsType() string {
	return AIProviderSettingsTypeWIF
}

func (AIProviderWIFSettings) settingsVersion() int {
	return AIProviderWIFSettingsVersion
}

// WIFIdentityTokenFileAllowed reports whether a database-backed WIF
// provider may read identityTokenFile and exchange its contents against
// baseURL.
//
// The exchange posts the file contents as the OIDC assertion to the
// provider's base URL, and provider rows are writable through the HTTP
// API by Coder administrators who are not necessarily host
// administrators. Without a restriction, such an administrator could
// read any file visible to the server process and exfiltrate it to a
// base URL they control. Only operator-controlled deployment
// configuration can therefore bless a path:
//
//   - Files listed in CODER_AI_GATEWAY_WIF_ALLOWED_IDENTITY_TOKEN_FILES
//     are allowed with any base URL. The operator accepts that
//     administrators may direct these tokens to a custom HTTPS
//     endpoint.
//   - The (identity token file, base URL) pairs of env-configured WIF
//     providers (CODER_AI_GATEWAY_PROVIDER_<N>_WIF_*) are allowed
//     as-is, so env-seeded rows work without extra configuration while
//     repointing their base URL through the API is refused.
//
// Matching is lexical on cleaned absolute paths. Allowlisted paths are
// assumed to live on an operator-controlled filesystem, so symlink
// swaps are outside the threat model and symlinks are not resolved.
func (c AIBridgeConfig) WIFIdentityTokenFileAllowed(identityTokenFile, baseURL string) bool {
	if identityTokenFile == "" {
		return false
	}
	cleaned := filepath.Clean(identityTokenFile)
	if !filepath.IsAbs(cleaned) {
		return false
	}
	for _, allowed := range c.WIFAllowedIdentityTokenFiles.Value() {
		if allowed == "" {
			continue
		}
		cleanedAllowed := filepath.Clean(allowed)
		if filepath.IsAbs(cleanedAllowed) && cleanedAllowed == cleaned {
			return true
		}
	}
	for _, p := range c.Providers {
		if p.WIFIdentityTokenFile == "" {
			continue
		}
		if filepath.Clean(p.WIFIdentityTokenFile) == cleaned && p.BaseURL == baseURL {
			return true
		}
	}
	return false
}
