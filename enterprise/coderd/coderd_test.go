package coderd_test

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	agplaudit "github.com/coder/coder/coderd/audit"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/enterprise/audit"
	"github.com/coder/coder/enterprise/coderd"
	"github.com/coder/coder/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/testutil"
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
	t.Run("FullLicense", func(t *testing.T) {
		t.Parallel()
		client := coderdenttest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		coderdenttest.AddLicense(t, client, coderdenttest.LicenseOptions{
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
		license := coderdenttest.AddLicense(t, client, coderdenttest.LicenseOptions{
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
		assert.False(t, res.HasLicense)
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
		coderdenttest.AddLicense(t, client, coderdenttest.LicenseOptions{
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
	t.Run("Pubsub", func(t *testing.T) {
		t.Parallel()
		client, _, api := coderdenttest.NewWithAPI(t, nil)
		entitlements, err := client.Entitlements(context.Background())
		require.NoError(t, err)
		require.False(t, entitlements.HasLicense)
		coderdtest.CreateFirstUser(t, client)
		_, err = api.Database.InsertLicense(context.Background(), database.InsertLicenseParams{
			UploadedAt: database.Now(),
			Exp:        database.Now().AddDate(1, 0, 0),
			JWT: coderdenttest.GenerateLicense(t, coderdenttest.LicenseOptions{
				AuditLog: true,
			}),
		})
		require.NoError(t, err)
		err = api.Pubsub.Publish(coderd.PubsubEventLicenses, []byte{})
		require.NoError(t, err)
		require.Eventually(t, func() bool {
			entitlements, err := client.Entitlements(context.Background())
			assert.NoError(t, err)
			return entitlements.HasLicense
		}, testutil.WaitShort, testutil.IntervalFast)
	})
	t.Run("Resync", func(t *testing.T) {
		t.Parallel()
		client, _, api := coderdenttest.NewWithAPI(t, &coderdenttest.Options{
			EntitlementsUpdateInterval: 25 * time.Millisecond,
		})
		entitlements, err := client.Entitlements(context.Background())
		require.NoError(t, err)
		require.False(t, entitlements.HasLicense)
		coderdtest.CreateFirstUser(t, client)
		// Valid
		_, err = api.Database.InsertLicense(context.Background(), database.InsertLicenseParams{
			UploadedAt: database.Now(),
			Exp:        database.Now().AddDate(1, 0, 0),
			JWT: coderdenttest.GenerateLicense(t, coderdenttest.LicenseOptions{
				AuditLog: true,
			}),
		})
		require.NoError(t, err)
		// Expired
		_, err = api.Database.InsertLicense(context.Background(), database.InsertLicenseParams{
			UploadedAt: database.Now(),
			Exp:        database.Now().AddDate(-1, 0, 0),
			JWT: coderdenttest.GenerateLicense(t, coderdenttest.LicenseOptions{
				ExpiresAt: database.Now().AddDate(-1, 0, 0),
			}),
		})
		require.NoError(t, err)
		// Invalid
		_, err = api.Database.InsertLicense(context.Background(), database.InsertLicenseParams{
			UploadedAt: database.Now(),
			Exp:        database.Now().AddDate(1, 0, 0),
			JWT:        "invalid",
		})
		require.NoError(t, err)
		require.Eventually(t, func() bool {
			entitlements, err := client.Entitlements(context.Background())
			assert.NoError(t, err)
			return entitlements.HasLicense
		}, testutil.WaitShort, testutil.IntervalFast)
	})
}

func TestAuditLogging(t *testing.T) {
	t.Parallel()
	t.Run("Enabled", func(t *testing.T) {
		t.Parallel()
		client, _, api := coderdenttest.NewWithAPI(t, nil)
		coderdtest.CreateFirstUser(t, client)
		coderdenttest.AddLicense(t, client, coderdenttest.LicenseOptions{
			AuditLog: true,
		})
		auditor := *api.AGPL.Auditor.Load()
		ea := audit.NewAuditor(audit.DefaultFilter)
		t.Logf("%T = %T", auditor, ea)
		assert.Equal(t, reflect.ValueOf(ea).Type(), reflect.ValueOf(auditor).Type())
	})
	t.Run("Disabled", func(t *testing.T) {
		t.Parallel()
		client, _, api := coderdenttest.NewWithAPI(t, nil)
		coderdtest.CreateFirstUser(t, client)
		auditor := *api.AGPL.Auditor.Load()
		ea := agplaudit.NewNop()
		t.Logf("%T = %T", auditor, ea)
		assert.Equal(t, reflect.ValueOf(ea).Type(), reflect.ValueOf(auditor).Type())
	})
}
