package codersdk

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
