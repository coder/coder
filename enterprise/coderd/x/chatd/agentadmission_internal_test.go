package chatd

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/entitlements"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
)

func TestAgentCapacityUnlock(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		feature  *codersdk.Feature
		unlocked bool
	}{
		{
			name: "EnabledWithoutUsage",
			feature: &codersdk.Feature{
				Entitlement: codersdk.EntitlementEntitled,
				Enabled:     true,
			},
			unlocked: true,
		},
		{
			name: "RemainingHours",
			feature: &codersdk.Feature{
				Entitlement: codersdk.EntitlementEntitled,
				Enabled:     true,
				Limit:       ptr.Ref(int64(100)),
				Actual:      ptr.Ref(int64(40)),
			},
			unlocked: true,
		},
		{
			name: "AtAllocationWithoutHardLimit",
			feature: &codersdk.Feature{
				Entitlement: codersdk.EntitlementEntitled,
				Enabled:     true,
				Limit:       ptr.Ref(int64(100)),
				Actual:      ptr.Ref(int64(100)),
			},
			unlocked: true,
		},
		{
			name: "OverAllocationWithoutHardLimit",
			feature: &codersdk.Feature{
				Entitlement: codersdk.EntitlementEntitled,
				Enabled:     true,
				Limit:       ptr.Ref(int64(100)),
				Actual:      ptr.Ref(int64(150)),
			},
			unlocked: true,
		},
		{
			name: "BelowHardLimit",
			feature: &codersdk.Feature{
				Entitlement: codersdk.EntitlementEntitled,
				Enabled:     true,
				Limit:       ptr.Ref(int64(100)),
				HardLimit:   ptr.Ref(int64(120)),
				Actual:      ptr.Ref(int64(119)),
			},
			unlocked: true,
		},
		{
			name: "AtHardLimit",
			feature: &codersdk.Feature{
				Entitlement: codersdk.EntitlementEntitled,
				Enabled:     true,
				Limit:       ptr.Ref(int64(100)),
				HardLimit:   ptr.Ref(int64(120)),
				Actual:      ptr.Ref(int64(120)),
			},
		},
		{
			name: "AboveHardLimit",
			feature: &codersdk.Feature{
				Entitlement: codersdk.EntitlementEntitled,
				Enabled:     true,
				Limit:       ptr.Ref(int64(100)),
				HardLimit:   ptr.Ref(int64(120)),
				Actual:      ptr.Ref(int64(121)),
			},
		},
		{
			name: "HardLimitEqualsAllocation",
			feature: &codersdk.Feature{
				Entitlement: codersdk.EntitlementEntitled,
				Enabled:     true,
				Limit:       ptr.Ref(int64(100)),
				HardLimit:   ptr.Ref(int64(100)),
				Actual:      ptr.Ref(int64(100)),
			},
		},
		{
			// A zero-allocation license grants the feature disabled with a
			// zero limit. Premium licenses without agent_runtime_hours_*
			// claims are grandfathered into the same shape, so this case
			// pins that both stay capped.
			name: "DisabledZeroAllocation",
			feature: &codersdk.Feature{
				Entitlement: codersdk.EntitlementEntitled,
				Enabled:     false,
				Limit:       ptr.Ref(int64(0)),
			},
		},
		{name: "NoFeature"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			set := entitlements.New()
			if tc.feature != nil {
				set.Modify(func(ents *codersdk.Entitlements) {
					ents.Features[codersdk.FeatureAgentRuntimeHours] = *tc.feature
				})
			}
			require.Equal(t, tc.unlocked, NewAgentCapacityUnlock(set).Unlocked())
		})
	}
}

func TestAgentCapacityUnlockTracksEntitlementUpdates(t *testing.T) {
	t.Parallel()

	set := entitlements.New()
	unlock := NewAgentCapacityUnlock(set)
	require.False(t, unlock.Unlocked())

	set.Modify(func(ents *codersdk.Entitlements) {
		ents.Features[codersdk.FeatureAgentRuntimeHours] = codersdk.Feature{
			Entitlement: codersdk.EntitlementEntitled,
			Enabled:     true,
			HardLimit:   ptr.Ref(int64(120)),
			Actual:      ptr.Ref(int64(119)),
		}
	})
	require.True(t, unlock.Unlocked())

	set.Modify(func(ents *codersdk.Entitlements) {
		feature := ents.Features[codersdk.FeatureAgentRuntimeHours]
		feature.Actual = ptr.Ref(int64(120))
		ents.Features[codersdk.FeatureAgentRuntimeHours] = feature
	})
	require.False(t, unlock.Unlocked())
}
