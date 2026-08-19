package chatd

import (
	"github.com/coder/coder/v2/coderd/entitlements"
	osschatd "github.com/coder/coder/v2/coderd/x/chatd"
	"github.com/coder/coder/v2/codersdk"
)

// NewAgentCapacityUnlock returns an unlock that tracks entitlement changes.
func NewAgentCapacityUnlock(set *entitlements.Set) osschatd.AgentCapacityUnlock {
	return &agentCapacityUnlock{entitlements: set}
}

type agentCapacityUnlock struct {
	entitlements *entitlements.Set
}

// The Agent Hours allocation is advisory; the hard limit restores concurrency caps.
func (u *agentCapacityUnlock) Unlocked() bool {
	f, ok := u.entitlements.Feature(codersdk.FeatureAgentRuntimeHours)
	if !ok || !f.Enabled {
		return false
	}
	if f.HardLimit == nil || f.Actual == nil {
		// Missing hard limits or usage measurements leave concurrency uncapped.
		return true
	}
	return *f.Actual < *f.HardLimit
}
