// Package credential defines shared upstream authentication metadata.
package credential

// Kind identifies how a request was authenticated.
// Values must match the credential_kind enum in coderd's database.
type Kind string

const (
	// KindCentralized identifies provider-managed credentials.
	KindCentralized Kind = "centralized"
	// KindBYOK identifies user-supplied provider credentials.
	KindBYOK Kind = "byok"

	// Auth header names shared by providers and request handlers.
	// AuthHeaderXAPIKey carries an API key.
	AuthHeaderXAPIKey = "X-Api-Key" //nolint:gosec // HTTP header name, not a credential.
	// AuthHeaderAuthorization carries an authorization credential.
	AuthHeaderAuthorization = "Authorization"

	// Hint placeholders for credentials with no static key value to mask.
	// Hints are persisted to aibridge_interceptions.credential_hint, a VARCHAR(15),
	// so every value here must be at most 15 characters.
	// HintFailoverKey is used before a centralized key is selected.
	HintFailoverKey = "<failover key>" //nolint:gosec // Placeholder, not a credential.
	// HintAWSChainKey identifies credentials resolved through the AWS default chain.
	HintAWSChainKey = "<aws chain>"
)
