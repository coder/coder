package aibridged

// ProviderStatus is the lifecycle state of a configured AI provider.
type ProviderStatus string

const (
	// ProviderStatusEnabled is a configured, valid provider included in
	// the active pool snapshot.
	ProviderStatusEnabled ProviderStatus = "enabled"
	// ProviderStatusDisabled is a configured provider intentionally
	// turned off by an operator.
	ProviderStatusDisabled ProviderStatus = "disabled"
	// ProviderStatusError is a configured provider that cannot be
	// constructed (missing keys, unsupported type, malformed settings).
	ProviderStatusError ProviderStatus = "error"
)

// ProviderOutcome is the classification of one ai_providers row.
// Observers see disabled and errored rows here that the pool snapshot
// (enabled only) excludes.
//
// Err is populated only when Status == ProviderStatusError. The
// construction-site error is already logged at the source; Err is kept
// on the outcome so test assertions can verify which row failed and
// why, without parsing logs.
type ProviderOutcome struct {
	Name   string
	Type   string
	Status ProviderStatus
	Err    error
}
