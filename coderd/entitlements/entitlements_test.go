package entitlements_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/entitlements"
	"github.com/coder/coder/v2/codersdk"
)

func TestUpdate(t *testing.T) {
	t.Parallel()

	set := entitlements.New()
	require.False(t, set.Enabled(codersdk.FeatureMultipleOrganizations))

	set.Update(func(entitlements *codersdk.Entitlements) {
		entitlements.Features[codersdk.FeatureMultipleOrganizations] = codersdk.Feature{
			Enabled:     true,
			Entitlement: codersdk.EntitlementEntitled,
		}
	})
	require.True(t, set.Enabled(codersdk.FeatureMultipleOrganizations))
}

func TestAllowRefresh(t *testing.T) {
	t.Parallel()

	now := time.Now()
	set := entitlements.New()
	set.Update(func(entitlements *codersdk.Entitlements) {
		entitlements.RefreshedAt = now
	})

	ok, wait := set.AllowRefresh(now)
	require.False(t, ok)
	require.InDelta(t, time.Minute.Seconds(), wait.Seconds(), 5)

	set.Update(func(entitlements *codersdk.Entitlements) {
		entitlements.RefreshedAt = now.Add(time.Minute * -2)
	})

	ok, wait = set.AllowRefresh(now)
	require.True(t, ok)
	require.Equal(t, time.Duration(0), wait)
}

func TestReplace(t *testing.T) {
	t.Parallel()

	set := entitlements.New()
	require.False(t, set.Enabled(codersdk.FeatureMultipleOrganizations))
	set.Replace(codersdk.Entitlements{
		Features: map[codersdk.FeatureName]codersdk.Feature{
			codersdk.FeatureMultipleOrganizations: {
				Enabled: true,
			},
		},
	})
	require.True(t, set.Enabled(codersdk.FeatureMultipleOrganizations))
}
