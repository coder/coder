package aibridged

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
)

// ProviderOutcome classifies one ai_providers row, including disabled
// rows (which the pool keeps as 503 stubs) and errored rows (which the
// pool excludes). Err is populated only when Status == ProviderStatusError;
// the build error is already logged at the call site.
type ProviderOutcome struct {
	Name   string
	Type   string
	Status ProviderStatus
	Err    error
}
