package coderd_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/enterprise/coderd/coderdenttest"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestEntitlements(t *testing.T) {
	t.Parallel()
	t.Run("NoLicense", func(t *testing.T) {
		t.Parallel()
		client := coderdenttest.New(t, nil)
		res, err := client.Entitlements(context.Background())
		require.NoError(t, err)
		require.False(t, res.HasLicense)
		require.Empty(t, res.Warnings)
	})
	t.Run("NoLicense", func(t *testing.T) {
		t.Parallel()
		client := coderdenttest.New(t, nil)
		res, err := client.Entitlements(context.Background())
		require.NoError(t, err)
		require.False(t, res.HasLicense)
		require.Empty(t, res.Warnings)
	})
	t.Run("FullLicense", func(t *testing.T) {
		t.Parallel()
		client := coderdenttest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		coderdenttest.AddLicense(t, client, coderdenttest.AddLicenseOptions{
			UserLimit: 100,
			AuditLog:  true,
		})
		res, err := client.Entitlements(context.Background())
		require.NoError(t, err)
		assert.True(t, res.HasLicense)
		ul := res.Features[codersdk.FeatureUserLimit]
		assert.Equal(t, codersdk.EntitlementEntitled, ul.Entitlement)
		assert.Equal(t, int64(100), *ul.Limit)
		assert.Equal(t, int64(1), *ul.Actual)
		assert.True(t, ul.Enabled)
		al := res.Features[codersdk.FeatureAuditLog]
		assert.Equal(t, codersdk.EntitlementEntitled, al.Entitlement)
		assert.True(t, al.Enabled)
		assert.Nil(t, al.Limit)
		assert.Nil(t, al.Actual)
		assert.Empty(t, res.Warnings)
	})
	t.Run("FullLicenseToNone", func(t *testing.T) {
		t.Parallel()
		client := coderdenttest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		license := coderdenttest.AddLicense(t, client, coderdenttest.AddLicenseOptions{
			UserLimit: 100,
			AuditLog:  true,
		})
		res, err := client.Entitlements(context.Background())
		require.NoError(t, err)
		assert.True(t, res.HasLicense)
		al := res.Features[codersdk.FeatureAuditLog]
		assert.Equal(t, codersdk.EntitlementEntitled, al.Entitlement)
		assert.True(t, al.Enabled)

		err = client.DeleteLicense(context.Background(), license.ID)
		require.NoError(t, err)

		res, err = client.Entitlements(context.Background())
		require.NoError(t, err)
		assert.True(t, res.HasLicense)
		al = res.Features[codersdk.FeatureAuditLog]
		assert.Equal(t, codersdk.EntitlementNotEntitled, al.Entitlement)
		assert.True(t, al.Enabled)
	})
	t.Run("Warnings", func(t *testing.T) {
		t.Parallel()
		client := coderdenttest.New(t, nil)
		first := coderdtest.CreateFirstUser(t, client)
		for i := 0; i < 4; i++ {
			coderdtest.CreateAnotherUser(t, client, first.OrganizationID)
		}
		coderdenttest.AddLicense(t, client, coderdenttest.AddLicenseOptions{
			UserLimit: 4,
			AuditLog:  true,
			GraceAt:   time.Now().Add(-time.Second),
		})
		res, err := client.Entitlements(context.Background())
		require.NoError(t, err)
		assert.True(t, res.HasLicense)
		ul := res.Features[codersdk.FeatureUserLimit]
		assert.Equal(t, codersdk.EntitlementGracePeriod, ul.Entitlement)
		assert.Equal(t, int64(4), *ul.Limit)
		assert.Equal(t, int64(5), *ul.Actual)
		assert.True(t, ul.Enabled)
		al := res.Features[codersdk.FeatureAuditLog]
		assert.Equal(t, codersdk.EntitlementGracePeriod, al.Entitlement)
		assert.True(t, al.Enabled)
		assert.Nil(t, al.Limit)
		assert.Nil(t, al.Actual)
		assert.Len(t, res.Warnings, 2)
		assert.Contains(t, res.Warnings,
			"Your deployment has 5 active users but is only licensed for 4.")
		assert.Contains(t, res.Warnings,
			"Audit logging is enabled but your license for this feature is expired.")
	})
}
