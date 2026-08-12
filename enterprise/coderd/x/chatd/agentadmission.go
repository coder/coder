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

// Runtime-hour allocation does not enforce caps; the hard limit does.
func (u *agentCapacityUnlock) Unlocked() bool {
	f, ok := u.entitlements.Feature(codersdk.FeatureAgentRuntimeHours)
	if !ok || !f.Enabled {
		return false
	}
	if f.HardLimit == nil || f.Actual == nil {
		return true
	}
	return *f.Actual < *f.HardLimit
}
