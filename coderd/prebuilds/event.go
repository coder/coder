package prebuilds

// PrebuildEventType represents the type of prebuild event.
type PrebuildEventType string

const (
	// PrebuildEventClaimSucceeded is recorded when a prebuild is
	// successfully claimed for a workspace creation request.
	PrebuildEventClaimSucceeded PrebuildEventType = "claim_succeeded"

	// PrebuildEventClaimMissed is recorded when a workspace creation
	// request matched a preset with prebuilds configured, but no
	// claimable prebuild was available, forcing a fallback to normal
	// workspace creation.
	PrebuildEventClaimMissed PrebuildEventType = "claim_missed"
)

// ValidPrebuildEventTypes is the set of allowed event types. Use this to
// validate before inserting.
var ValidPrebuildEventTypes = map[PrebuildEventType]struct{}{
	PrebuildEventClaimSucceeded: {},
	PrebuildEventClaimMissed:    {},
}
