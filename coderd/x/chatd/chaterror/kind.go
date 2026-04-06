// Package chaterror classifies provider/runtime failures into stable,
// user-facing chat error payloads.
package chaterror

const (
	KindOverloaded     = "overloaded"
	KindRateLimit      = "rate_limit"
	KindTimeout        = "timeout"
	KindStartupTimeout = "startup_timeout"
	KindAuth           = "auth"
	KindConfig         = "config"
	KindGeneric        = "generic"
)
