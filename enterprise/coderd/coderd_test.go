package coderd_test

import (
	"bytes"
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	agplaudit "github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbmem"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/audit"
	"github.com/coder/coder/v2/enterprise/coderd"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/enterprise/dbcrypt"
	"github.com/coder/coder/v2/testutil"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestEntitlements(t *testing.T) {
	t.Parallel()
	t.Run("NoLicense", func(t *testing.T) {
		t.Parallel()
		client, _ := coderdenttest.New(t, &coderdenttest.Options{
			DontAddLicense: true,
		})
		res, err := client.Entitlements(context.Background())
		require.NoError(t, err)
		require.False(t, res.HasLicense)
		require.Empty(t, res.Warnings)
	})
	t.Run("FullLicense", func(t *testing.T) {
		t.Parallel()
		client, _ := coderdenttest.New(t, &coderdenttest.Options{
			AuditLogging:   true,
			DontAddLicense: true,
		})
		// Enable all features
		features := make(license.Features)
		for _, feature := range codersdk.FeatureNames {
			features[feature] = 1
		}
		features[codersdk.FeatureUserLimit] = 100
		coderdenttest.AddLicense(t, client, coderdenttest.LicenseOptions{
			Features: features,
			GraceAt:  time.Now().Add(59 * 24 * time.Hour),
		})
		res, err := client.Entitlements(context.Background())
		require.NoError(t, err)
		assert.True(t, res.HasLicense)
		ul := res.Features[codersdk.FeatureUserLimit]
		assert.Equal(t, codersdk.EntitlementEntitled, ul.Entitlement)
		if assert.NotNil(t, ul.Limit) {
			assert.Equal(t, int64(100), *ul.Limit)
		}
		if assert.NotNil(t, ul.Actual) {
			assert.Equal(t, int64(1), *ul.Actual)
		}
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
		client, _ := coderdenttest.New(t, &coderdenttest.Options{
			AuditLogging:   true,
			DontAddLicense: true,
		})
		license := coderdenttest.AddLicense(t, client, coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureUserLimit: 100,
				codersdk.FeatureAuditLog:  1,
			},
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
		assert.False(t, al.Enabled)
	})
	t.Run("Pubsub", func(t *testing.T) {
		t.Parallel()
		client, _, api, _ := coderdenttest.NewWithAPI(t, &coderdenttest.Options{DontAddLicense: true})
		entitlements, err := client.Entitlements(context.Background())
		require.NoError(t, err)
		require.False(t, entitlements.HasLicense)
		//nolint:gocritic // unit test
		ctx := testDBAuthzRole(context.Background())
		_, err = api.Database.InsertLicense(ctx, database.InsertLicenseParams{
			UploadedAt: dbtime.Now(),
			Exp:        dbtime.Now().AddDate(1, 0, 0),
			JWT: coderdenttest.GenerateLicense(t, coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureAuditLog: 1,
				},
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
		client, _, api, _ := coderdenttest.NewWithAPI(t, &coderdenttest.Options{
			EntitlementsUpdateInterval: 25 * time.Millisecond,
			DontAddLicense:             true,
		})
		entitlements, err := client.Entitlements(context.Background())
		require.NoError(t, err)
		require.False(t, entitlements.HasLicense)
		// Valid
		ctx := context.Background()
		//nolint:gocritic // unit test
		_, err = api.Database.InsertLicense(testDBAuthzRole(ctx), database.InsertLicenseParams{
			UploadedAt: dbtime.Now(),
			Exp:        dbtime.Now().AddDate(1, 0, 0),
			JWT: coderdenttest.GenerateLicense(t, coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureAuditLog: 1,
				},
			}),
		})
		require.NoError(t, err)
		// Expired
		//nolint:gocritic // unit test
		_, err = api.Database.InsertLicense(testDBAuthzRole(ctx), database.InsertLicenseParams{
			UploadedAt: dbtime.Now(),
			Exp:        dbtime.Now().AddDate(-1, 0, 0),
			JWT: coderdenttest.GenerateLicense(t, coderdenttest.LicenseOptions{
				ExpiresAt: dbtime.Now().AddDate(-1, 0, 0),
			}),
		})
		require.NoError(t, err)
		// Invalid
		//nolint:gocritic // unit test
		_, err = api.Database.InsertLicense(testDBAuthzRole(ctx), database.InsertLicenseParams{
			UploadedAt: dbtime.Now(),
			Exp:        dbtime.Now().AddDate(1, 0, 0),
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
		_, _, api, _ := coderdenttest.NewWithAPI(t, &coderdenttest.Options{
			AuditLogging: true,
			Options: &coderdtest.Options{
				Auditor: audit.NewAuditor(dbmem.New(), audit.DefaultFilter),
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureAuditLog: 1,
				},
			},
		})
		auditor := *api.AGPL.Auditor.Load()
		ea := audit.NewAuditor(dbmem.New(), audit.DefaultFilter)
		t.Logf("%T = %T", auditor, ea)
		assert.EqualValues(t, reflect.ValueOf(ea).Type(), reflect.ValueOf(auditor).Type())
	})
	t.Run("Disabled", func(t *testing.T) {
		t.Parallel()
		_, _, api, _ := coderdenttest.NewWithAPI(t, &coderdenttest.Options{DontAddLicense: true})
		auditor := *api.AGPL.Auditor.Load()
		ea := agplaudit.NewNop()
		t.Logf("%T = %T", auditor, ea)
		assert.Equal(t, reflect.ValueOf(ea).Type(), reflect.ValueOf(auditor).Type())
	})
	// The AGPL code runs with a fake auditor that doesn't represent the real implementation.
	// We do a simple test to ensure that basic flows function.
	t.Run("FullBuild", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		client, user := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				IncludeProvisionerDaemon: true,
			},
			DontAddLicense: true,
		})
		workspace, agent := setupWorkspaceAgent(t, client, user, 0)
		conn, err := client.DialWorkspaceAgent(ctx, agent.ID, nil)
		require.NoError(t, err)
		defer conn.Close()
		connected := conn.AwaitReachable(ctx)
		require.True(t, connected)
		build := coderdtest.CreateWorkspaceBuild(t, client, workspace, database.WorkspaceTransitionStop)
		coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, build.ID)
	})
}

func TestExternalTokenEncryption(t *testing.T) {
	t.Parallel()

	t.Run("Enabled", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)
		db, ps := dbtestutil.NewDB(t)
		ciphers, err := dbcrypt.NewCiphers(bytes.Repeat([]byte("a"), 32))
		require.NoError(t, err)
		client, _ := coderdenttest.New(t, &coderdenttest.Options{
			EntitlementsUpdateInterval: 25 * time.Millisecond,
			ExternalTokenEncryption:    ciphers,
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureExternalTokenEncryption: 1,
				},
			},
			Options: &coderdtest.Options{
				Database: db,
				Pubsub:   ps,
			},
		})
		keys, err := db.GetDBCryptKeys(ctx)
		require.NoError(t, err)
		require.Len(t, keys, 1)
		require.Equal(t, ciphers[0].HexDigest(), keys[0].ActiveKeyDigest.String)

		require.Eventually(t, func() bool {
			entitlements, err := client.Entitlements(context.Background())
			assert.NoError(t, err)
			feature := entitlements.Features[codersdk.FeatureExternalTokenEncryption]
			entitled := feature.Entitlement == codersdk.EntitlementEntitled
			var warningExists bool
			for _, warning := range entitlements.Warnings {
				if strings.Contains(warning, codersdk.FeatureExternalTokenEncryption.Humanize()) {
					warningExists = true
					break
				}
			}
			t.Logf("feature: %+v, warnings: %+v, errors: %+v", feature, entitlements.Warnings, entitlements.Errors)
			return feature.Enabled && entitled && !warningExists
		}, testutil.WaitShort, testutil.IntervalFast)
	})

	t.Run("Disabled", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)
		db, ps := dbtestutil.NewDB(t)
		ciphers, err := dbcrypt.NewCiphers()
		require.NoError(t, err)
		client, _ := coderdenttest.New(t, &coderdenttest.Options{
			DontAddLicense:             true,
			EntitlementsUpdateInterval: 25 * time.Millisecond,
			ExternalTokenEncryption:    ciphers,
			Options: &coderdtest.Options{
				Database: db,
				Pubsub:   ps,
			},
		})
		keys, err := db.GetDBCryptKeys(ctx)
		require.NoError(t, err)
		require.Empty(t, keys)

		require.Eventually(t, func() bool {
			entitlements, err := client.Entitlements(context.Background())
			assert.NoError(t, err)
			feature := entitlements.Features[codersdk.FeatureExternalTokenEncryption]
			entitled := feature.Entitlement == codersdk.EntitlementEntitled
			var warningExists bool
			for _, warning := range entitlements.Warnings {
				if strings.Contains(warning, codersdk.FeatureExternalTokenEncryption.Humanize()) {
					warningExists = true
					break
				}
			}
			t.Logf("feature: %+v, warnings: %+v, errors: %+v", feature, entitlements.Warnings, entitlements.Errors)
			return !feature.Enabled && !entitled && !warningExists
		}, testutil.WaitShort, testutil.IntervalFast)
	})

	t.Run("PreviouslyEnabledButMissingFromLicense", func(t *testing.T) {
		// If this test fails, it potentially means that a customer who has
		// actively been using this feature is now unable _start coderd_
		// because of a licensing issue. This should never happen.
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)
		db, ps := dbtestutil.NewDB(t)
		ciphers, err := dbcrypt.NewCiphers(bytes.Repeat([]byte("a"), 32))
		require.NoError(t, err)

		dbc, err := dbcrypt.New(ctx, db, ciphers...) // should insert key
		require.NoError(t, err)

		keys, err := dbc.GetDBCryptKeys(ctx)
		require.NoError(t, err)
		require.Len(t, keys, 1)

		client, _ := coderdenttest.New(t, &coderdenttest.Options{
			DontAddLicense:             true,
			EntitlementsUpdateInterval: 25 * time.Millisecond,
			ExternalTokenEncryption:    ciphers,
			Options: &coderdtest.Options{
				Database: db,
				Pubsub:   ps,
			},
		})

		require.Eventually(t, func() bool {
			entitlements, err := client.Entitlements(context.Background())
			assert.NoError(t, err)
			feature := entitlements.Features[codersdk.FeatureExternalTokenEncryption]
			entitled := feature.Entitlement == codersdk.EntitlementEntitled
			var warningExists bool
			for _, warning := range entitlements.Warnings {
				if strings.Contains(warning, codersdk.FeatureExternalTokenEncryption.Humanize()) {
					warningExists = true
					break
				}
			}
			t.Logf("feature: %+v, warnings: %+v, errors: %+v", feature, entitlements.Warnings, entitlements.Errors)
			return feature.Enabled && !entitled && warningExists
		}, testutil.WaitShort, testutil.IntervalFast)
	})
}

// testDBAuthzRole returns a context with a subject that has a role
// with permissions required for test setup.
func testDBAuthzRole(ctx context.Context) context.Context {
	return dbauthz.As(ctx, rbac.Subject{
		ID: uuid.Nil.String(),
		Roles: rbac.Roles([]rbac.Role{
			{
				Name:        "testing",
				DisplayName: "Unit Tests",
				Site: rbac.Permissions(map[string][]rbac.Action{
					rbac.ResourceWildcard.Type: {rbac.WildcardSymbol},
				}),
				Org:  map[string][]rbac.Permission{},
				User: []rbac.Permission{},
			},
		}),
		Scope: rbac.ScopeAll,
	})
}
