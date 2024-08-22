package entitlements_test

import (
	"testing"

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
