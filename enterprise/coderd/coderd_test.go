package coderd_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/moby/moby/pkg/namesgenerator"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"

	"github.com/coder/coder/v2/agent"
	"github.com/coder/coder/v2/agent/agenttest"
	"github.com/coder/coder/v2/coderd/httpapi"
	agplprebuilds "github.com/coder/coder/v2/coderd/prebuilds"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/enterprise/coderd/prebuilds"
	"github.com/coder/coder/v2/tailnet/tailnettest"

	"github.com/coder/retry"
	"github.com/coder/serpent"

	agplaudit "github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/coderd/database/dbmem"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/enterprise/audit"
	"github.com/coder/coder/v2/enterprise/coderd"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/enterprise/dbcrypt"
	"github.com/coder/coder/v2/enterprise/replicasync"
	"github.com/coder/coder/v2/testutil"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m, testutil.GoleakOptions...)
}

func TestEntitlements(t *testing.T) {
	t.Parallel()
	t.Run("NoLicense", func(t *testing.T) {
		t.Parallel()
		adminClient, _, api, adminUser := coderdenttest.NewWithAPI(t, &coderdenttest.Options{
			DontAddLicense: true,
		})
		anotherClient, _ := coderdtest.CreateAnotherUser(t, adminClient, adminUser.OrganizationID)
		res, err := anotherClient.Entitlements(context.Background())
		require.NoError(t, err)
		require.False(t, res.HasLicense)
		require.Empty(t, res.Warnings)

		// Ensure the entitlements are the same reference
		require.Equal(t, fmt.Sprintf("%p", api.Entitlements), fmt.Sprintf("%p", api.AGPL.Entitlements))
	})
	t.Run("FullLicense", func(t *testing.T) {
		// PGCoordinator requires a real postgres
		if !dbtestutil.WillUsePostgres() {
			t.Skip("test only with postgres")
		}
		t.Parallel()
		adminClient, _ := coderdenttest.New(t, &coderdenttest.Options{
			AuditLogging:   true,
			DontAddLicense: true,
		})
		// Enable all features
		features := make(license.Features)
		for _, feature := range codersdk.FeatureNames {
			features[feature] = 1
		}
		features[codersdk.FeatureUserLimit] = 100
		coderdenttest.AddLicense(t, adminClient, coderdenttest.LicenseOptions{
			Features: features,
			GraceAt:  time.Now().Add(59 * 24 * time.Hour),
		})
		res, err := adminClient.Entitlements(context.Background()) //nolint:gocritic // adding another user would put us over user limit
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
		adminClient, adminUser := coderdenttest.New(t, &coderdenttest.Options{
			AuditLogging:   true,
			DontAddLicense: true,
		})
		anotherClient, _ := coderdtest.CreateAnotherUser(t, adminClient, adminUser.OrganizationID)
		license := coderdenttest.AddLicense(t, adminClient, coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureUserLimit: 100,
				codersdk.FeatureAuditLog:  1,
			},
		})
		res, err := anotherClient.Entitlements(context.Background())
		require.NoError(t, err)
		assert.True(t, res.HasLicense)
		al := res.Features[codersdk.FeatureAuditLog]
		assert.Equal(t, codersdk.EntitlementEntitled, al.Entitlement)
		assert.True(t, al.Enabled)

		err = adminClient.DeleteLicense(context.Background(), license.ID)
		require.NoError(t, err)

		res, err = anotherClient.Entitlements(context.Background())
		require.NoError(t, err)
		assert.False(t, res.HasLicense)
		al = res.Features[codersdk.FeatureAuditLog]
		assert.Equal(t, codersdk.EntitlementNotEntitled, al.Entitlement)
		assert.False(t, al.Enabled)
	})
	t.Run("Pubsub", func(t *testing.T) {
		t.Parallel()
		adminClient, _, api, adminUser := coderdenttest.NewWithAPI(t, &coderdenttest.Options{DontAddLicense: true})
		anotherClient, _ := coderdtest.CreateAnotherUser(t, adminClient, adminUser.OrganizationID)
		entitlements, err := anotherClient.Entitlements(context.Background())
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
			entitlements, err := anotherClient.Entitlements(context.Background())
			assert.NoError(t, err)
			return entitlements.HasLicense
		}, testutil.WaitShort, testutil.IntervalFast)
	})
	t.Run("Resync", func(t *testing.T) {
		t.Parallel()
		adminClient, _, api, adminUser := coderdenttest.NewWithAPI(t, &coderdenttest.Options{
			EntitlementsUpdateInterval: 25 * time.Millisecond,
			DontAddLicense:             true,
		})
		anotherClient, _ := coderdtest.CreateAnotherUser(t, adminClient, adminUser.OrganizationID)
		entitlements, err := anotherClient.Entitlements(context.Background())
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
			entitlements, err := anotherClient.Entitlements(context.Background())
			assert.NoError(t, err)
			return entitlements.HasLicense
		}, testutil.WaitShort, testutil.IntervalFast)
	})
}

func TestEntitlements_HeaderWarnings(t *testing.T) {
	t.Parallel()
	t.Run("ExistForAdmin", func(t *testing.T) {
		t.Parallel()
		adminClient, _ := coderdenttest.New(t, &coderdenttest.Options{
			AuditLogging: true,
			LicenseOptions: &coderdenttest.LicenseOptions{
				AllFeatures: false,
			},
		})
		//nolint:gocritic // This isn't actually bypassing any RBAC checks
		res, err := adminClient.Request(context.Background(), http.MethodGet, "/api/v2/users/me", nil)
		require.NoError(t, err)
		defer res.Body.Close()
		require.Equal(t, http.StatusOK, res.StatusCode)
		require.NotEmpty(t, res.Header.Values(codersdk.EntitlementsWarningHeader))
	})
	t.Run("NoneForNormalUser", func(t *testing.T) {
		t.Parallel()
		adminClient, adminUser := coderdenttest.New(t, &coderdenttest.Options{
			AuditLogging: true,
			LicenseOptions: &coderdenttest.LicenseOptions{
				AllFeatures: false,
			},
		})
		anotherClient, _ := coderdtest.CreateAnotherUser(t, adminClient, adminUser.OrganizationID)
		res, err := anotherClient.Request(context.Background(), http.MethodGet, "/api/v2/users/me", nil)
		require.NoError(t, err)
		defer res.Body.Close()
		require.Equal(t, http.StatusOK, res.StatusCode)
		require.Empty(t, res.Header.Values(codersdk.EntitlementsWarningHeader))
	})
}

func TestEntitlements_Prebuilds(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name              string
		experimentEnabled bool
		featureEnabled    bool
		expectedEnabled   bool
	}{
		{
			name:              "Fully enabled",
			featureEnabled:    true,
			experimentEnabled: true,
			expectedEnabled:   true,
		},
		{
			name:              "Feature disabled",
			featureEnabled:    false,
			experimentEnabled: true,
			expectedEnabled:   false,
		},
		{
			name:              "Experiment disabled",
			featureEnabled:    true,
			experimentEnabled: false,
			expectedEnabled:   false,
		},
		{
			name:              "Fully disabled",
			featureEnabled:    false,
			experimentEnabled: false,
			expectedEnabled:   false,
		},
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var prebuildsEntitled int64
			if tc.featureEnabled {
				prebuildsEntitled = 1
			}

			_, _, api, _ := coderdenttest.NewWithAPI(t, &coderdenttest.Options{
				Options: &coderdtest.Options{
					DeploymentValues: coderdtest.DeploymentValues(t, func(values *codersdk.DeploymentValues) {
						if tc.experimentEnabled {
							values.Experiments = serpent.StringArray{string(codersdk.ExperimentWorkspacePrebuilds)}
						}
					}),
				},

				EntitlementsUpdateInterval: time.Second,
				LicenseOptions: &coderdenttest.LicenseOptions{
					Features: license.Features{
						codersdk.FeatureWorkspacePrebuilds: prebuildsEntitled,
					},
				},
			})

			// The entitlements will need to refresh before the reconciler is set.
			require.Eventually(t, func() bool {
				return api.AGPL.PrebuildsReconciler.Load() != nil
			}, testutil.WaitSuperLong, testutil.IntervalFast)

			reconciler := api.AGPL.PrebuildsReconciler.Load()
			claimer := api.AGPL.PrebuildsClaimer.Load()
			require.NotNil(t, reconciler)
			require.NotNil(t, claimer)

			if tc.expectedEnabled {
				require.IsType(t, &prebuilds.StoreReconciler{}, *reconciler)
				require.IsType(t, &prebuilds.EnterpriseClaimer{}, *claimer)
			} else {
				require.Equal(t, &agplprebuilds.DefaultReconciler, reconciler)
				require.Equal(t, &agplprebuilds.DefaultClaimer, claimer)
			}
		})
	}
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
		r := setupWorkspaceAgent(t, client, user, 0)
		conn, err := workspacesdk.New(client).DialAgent(ctx, r.sdkAgent.ID, nil) //nolint:gocritic // RBAC is not the purpose of this test
		require.NoError(t, err)
		defer conn.Close()
		connected := conn.AwaitReachable(ctx)
		require.True(t, connected)
		_ = r.agent.Close() // close first so we don't drop error logs from outdated build
		build := coderdtest.CreateWorkspaceBuild(t, client, r.workspace, database.WorkspaceTransitionStop)
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

func TestMultiReplica_EmptyRelayAddress(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	db, ps := dbtestutil.NewDB(t)
	logger := testutil.Logger(t)

	_, _ = coderdenttest.New(t, &coderdenttest.Options{
		EntitlementsUpdateInterval: 25 * time.Millisecond,
		ReplicaSyncUpdateInterval:  25 * time.Millisecond,
		Options: &coderdtest.Options{
			Logger:   &logger,
			Database: db,
			Pubsub:   ps,
		},
	})

	mgr, err := replicasync.New(ctx, logger, db, ps, &replicasync.Options{
		ID:             uuid.New(),
		RelayAddress:   "",
		RegionID:       999,
		UpdateInterval: testutil.IntervalFast,
	})
	require.NoError(t, err)
	defer mgr.Close()

	// Send a bunch of updates to see if the coderd will log errors.
	{
		ctx, cancel := context.WithTimeout(ctx, testutil.IntervalMedium)
		for r := retry.New(testutil.IntervalFast, testutil.IntervalFast); r.Wait(ctx); {
			require.NoError(t, mgr.PublishUpdate())
		}
		cancel()
	}
}

func TestMultiReplica_EmptyRelayAddress_DisabledDERP(t *testing.T) {
	t.Parallel()

	derpMap, _ := tailnettest.RunDERPAndSTUN(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		httpapi.Write(context.Background(), w, http.StatusOK, derpMap)
	}))
	t.Cleanup(srv.Close)

	ctx := testutil.Context(t, testutil.WaitLong)
	db, ps := dbtestutil.NewDB(t)
	logger := testutil.Logger(t)

	dv := coderdtest.DeploymentValues(t)
	dv.DERP.Server.Enable = serpent.Bool(false)
	dv.DERP.Config.URL = serpent.String(srv.URL)

	_, _ = coderdenttest.New(t, &coderdenttest.Options{
		EntitlementsUpdateInterval: 25 * time.Millisecond,
		ReplicaSyncUpdateInterval:  25 * time.Millisecond,
		Options: &coderdtest.Options{
			Logger:           &logger,
			Database:         db,
			Pubsub:           ps,
			DeploymentValues: dv,
		},
	})

	mgr, err := replicasync.New(ctx, logger, db, ps, &replicasync.Options{
		ID:             uuid.New(),
		RelayAddress:   "",
		RegionID:       999,
		UpdateInterval: testutil.IntervalFast,
	})
	require.NoError(t, err)
	defer mgr.Close()

	// Send a bunch of updates to see if the coderd will log errors.
	{
		ctx, cancel := context.WithTimeout(ctx, testutil.IntervalMedium)
		for r := retry.New(testutil.IntervalFast, testutil.IntervalFast); r.Wait(ctx); {
			require.NoError(t, mgr.PublishUpdate())
		}
		cancel()
	}
}

func TestSCIMDisabled(t *testing.T) {
	t.Parallel()

	cli, _ := coderdenttest.New(t, &coderdenttest.Options{})

	checkPaths := []string{
		"/scim/v2",
		"/scim/v2/",
		"/scim/v2/users",
		"/scim/v2/Users",
		"/scim/v2/Users/",
		"/scim/v2/random/path/that/is/long",
		"/scim/v2/random/path/that/is/long.txt",
	}

	for _, p := range checkPaths {
		p := p
		t.Run(p, func(t *testing.T) {
			t.Parallel()

			u, err := cli.URL.Parse(p)
			require.NoError(t, err)

			req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, u.String(), nil)
			require.NoError(t, err)

			resp, err := http.DefaultClient.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()
			require.Equal(t, http.StatusNotFound, resp.StatusCode)

			var apiError codersdk.Response
			err = json.NewDecoder(resp.Body).Decode(&apiError)
			require.NoError(t, err)

			require.Contains(t, apiError.Message, "SCIM is disabled")
		})
	}
}

// testDBAuthzRole returns a context with a subject that has a role
// with permissions required for test setup.
func testDBAuthzRole(ctx context.Context) context.Context {
	return dbauthz.As(ctx, rbac.Subject{
		ID: uuid.Nil.String(),
		Roles: rbac.Roles([]rbac.Role{
			{
				Identifier:  rbac.RoleIdentifier{Name: "testing"},
				DisplayName: "Unit Tests",
				Site: rbac.Permissions(map[string][]policy.Action{
					rbac.ResourceWildcard.Type: {policy.WildcardSymbol},
				}),
				Org:  map[string][]rbac.Permission{},
				User: []rbac.Permission{},
			},
		}),
		Scope: rbac.ScopeAll,
	})
}

// restartableListener is a TCP listener that can have all of it's connections
// severed on demand.
type restartableListener struct {
	net.Listener
	mu    sync.Mutex
	conns []net.Conn
}

func (l *restartableListener) Accept() (net.Conn, error) {
	conn, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	l.mu.Lock()
	l.conns = append(l.conns, conn)
	l.mu.Unlock()
	return conn, nil
}

func (l *restartableListener) CloseConnections() {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, conn := range l.conns {
		_ = conn.Close()
	}
	l.conns = nil
}

type restartableTestServer struct {
	options *coderdenttest.Options
	rl      *restartableListener

	mu     sync.Mutex
	api    *coderd.API
	closer io.Closer
}

func newRestartableTestServer(t *testing.T, options *coderdenttest.Options) (*codersdk.Client, codersdk.CreateFirstUserResponse, *restartableTestServer) {
	t.Helper()
	if options == nil {
		options = &coderdenttest.Options{}
	}

	s := &restartableTestServer{
		options: options,
	}
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		api := s.api
		s.mu.Unlock()

		if api == nil {
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte("server is not started"))
			return
		}
		api.AGPL.RootHandler.ServeHTTP(w, r)
	}))
	s.rl = &restartableListener{Listener: srv.Listener}
	srv.Listener = s.rl
	srv.Start()
	t.Cleanup(srv.Close)

	u, err := url.Parse(srv.URL)
	require.NoError(t, err, "failed to parse server URL")
	s.options.AccessURL = u

	client, firstUser := s.startWithFirstUser(t)
	client.URL = u
	return client, firstUser, s
}

func (s *restartableTestServer) Stop(t *testing.T) {
	t.Helper()

	s.mu.Lock()
	closer := s.closer
	s.closer = nil
	api := s.api
	s.api = nil
	s.mu.Unlock()

	if closer != nil {
		err := closer.Close()
		require.NoError(t, err)
	}
	if api != nil {
		err := api.Close()
		require.NoError(t, err)
	}

	s.rl.CloseConnections()
}

func (s *restartableTestServer) Start(t *testing.T) {
	t.Helper()
	_, _ = s.startWithFirstUser(t)
}

func (s *restartableTestServer) startWithFirstUser(t *testing.T) (client *codersdk.Client, firstUser codersdk.CreateFirstUserResponse) {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closer != nil || s.api != nil {
		t.Fatal("server already started, close must be called first")
	}
	// This creates it's own TCP listener unfortunately, but it's not being
	// used in this test.
	client, s.closer, s.api, firstUser = coderdenttest.NewWithAPI(t, s.options)

	// Never add the first user or license on subsequent restarts.
	s.options.DontAddFirstUser = true
	s.options.DontAddLicense = true

	return client, firstUser
}

// Test_CoordinatorRollingRestart tests that two peers can maintain a connection
// without forgetting about each other when a HA coordinator does a rolling
// restart.
//
// We had a few issues with this in the past:
//  1. We didn't allow clients to maintain their peer ID after a reconnect,
//     which resulted in the other peer thinking the client was a new peer.
//     (This is fixed and independently tested in AGPL code)
//  2. HA coordinators would delete all peers (via FK constraints) when they
//     were closed, which meant tunnels would be deleted and peers would be
//     notified that the other peer was permanently gone.
//     (This is fixed and independently tested above)
//
// This test uses a real server and real clients.
func TestConn_CoordinatorRollingRestart(t *testing.T) {
	t.Parallel()

	if !dbtestutil.WillUsePostgres() {
		t.Skip("test only with postgres")
	}

	// Although DERP will have connection issues until the connection is
	// reestablished, any open connections should be maintained.
	//
	// Direct connections should be able to transmit packets throughout the
	// restart without issue.
	//nolint:paralleltest // Outdated rule
	for _, direct := range []bool{true, false} {
		name := "DERP"
		if direct {
			name = "Direct"
		}

		t.Run(name, func(t *testing.T) {
			t.Parallel()

			store, ps := dbtestutil.NewDB(t)
			dv := coderdtest.DeploymentValues(t, func(dv *codersdk.DeploymentValues) {
				dv.DERP.Config.BlockDirect = serpent.Bool(!direct)
			})
			logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)

			// Create two restartable test servers with the same database.
			client1, user, s1 := newRestartableTestServer(t, &coderdenttest.Options{
				DontAddFirstUser: false,
				DontAddLicense:   false,
				Options: &coderdtest.Options{
					Logger:                   ptr.Ref(logger.Named("server1")),
					Database:                 store,
					Pubsub:                   ps,
					DeploymentValues:         dv,
					IncludeProvisionerDaemon: true,
				},
				LicenseOptions: &coderdenttest.LicenseOptions{
					Features: license.Features{
						codersdk.FeatureHighAvailability: 1,
					},
				},
			})
			client2, _, s2 := newRestartableTestServer(t, &coderdenttest.Options{
				DontAddFirstUser: true,
				DontAddLicense:   true,
				Options: &coderdtest.Options{
					Logger:           ptr.Ref(logger.Named("server2")),
					Database:         store,
					Pubsub:           ps,
					DeploymentValues: dv,
				},
			})
			client2.SetSessionToken(client1.SessionToken())

			workspace := dbfake.WorkspaceBuild(t, store, database.WorkspaceTable{
				OrganizationID: user.OrganizationID,
				OwnerID:        user.UserID,
			}).WithAgent().Do()

			// Agent connects via the first coordinator.
			_ = agenttest.New(t, client1.URL, workspace.AgentToken, func(o *agent.Options) {
				o.Logger = logger.Named("agent1")
			})
			resources := coderdtest.NewWorkspaceAgentWaiter(t, client1, workspace.Workspace.ID).Wait()

			agentID := uuid.Nil
			for _, r := range resources {
				for _, a := range r.Agents {
					agentID = a.ID
					break
				}
			}
			require.NotEqual(t, uuid.Nil, agentID)

			// Client connects via the second coordinator.
			ctx := testutil.Context(t, testutil.WaitSuperLong)
			workspaceClient2 := workspacesdk.New(client2)
			conn, err := workspaceClient2.DialAgent(ctx, agentID, &workspacesdk.DialAgentOptions{
				Logger: logger.Named("client"),
			})
			require.NoError(t, err)
			defer conn.Close()

			require.Eventually(t, func() bool {
				_, p2p, _, err := conn.Ping(ctx)
				assert.NoError(t, err)
				return p2p == direct
			}, testutil.WaitShort, testutil.IntervalFast)

			// Open a TCP server and connection to it through the tunnel that
			// should be maintained throughout the restart.
			tcpServerAddr := tcpEchoServer(t)
			tcpConn, err := conn.DialContext(ctx, "tcp", tcpServerAddr)
			require.NoError(t, err)
			defer tcpConn.Close()
			writeReadEcho(t, ctx, tcpConn)

			// Stop the first server.
			logger.Info(ctx, "test: stopping server 1")
			s1.Stop(t)

			// Pings should fail on DERP but succeed on direct connections.
			pingCtx, pingCancel := context.WithTimeout(ctx, 2*time.Second) //nolint:gocritic // it's going to hang and timeout for DERP, so this needs to be short
			defer pingCancel()
			_, p2p, _, err := conn.Ping(pingCtx)
			if direct {
				require.NoError(t, err)
				require.True(t, p2p, "expected direct connection")
			} else {
				require.ErrorIs(t, err, context.DeadlineExceeded)
			}

			// The existing TCP connection should still be working if we're
			// using direct connections.
			if direct {
				writeReadEcho(t, ctx, tcpConn)
			}

			// Start the first server again.
			logger.Info(ctx, "test: starting server 1")
			s1.Start(t)

			// Restart the second server.
			logger.Info(ctx, "test: stopping server 2")
			s2.Stop(t)
			logger.Info(ctx, "test: starting server 2")
			s2.Start(t)

			// Pings should eventually succeed on both DERP and direct
			// connections.
			require.True(t, conn.AwaitReachable(ctx))
			_, p2p, _, err = conn.Ping(ctx)
			require.NoError(t, err)
			require.Equal(t, direct, p2p, "mismatched p2p state")

			// The existing TCP connection should still be working.
			writeReadEcho(t, ctx, tcpConn)
		})
	}
}

func tcpEchoServer(t *testing.T) string {
	tcpListener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = tcpListener.Close()
	})
	go func() {
		for {
			conn, err := tcpListener.Accept()
			if err != nil {
				return
			}
			t.Cleanup(func() {
				_ = conn.Close()
			})
			go func() {
				defer conn.Close()
				_, _ = io.Copy(conn, conn)
			}()
		}
	}()
	return tcpListener.Addr().String()
}

// nolint:revive // t takes precedence.
func writeReadEcho(t *testing.T, ctx context.Context, conn net.Conn) {
	msg := namesgenerator.GetRandomName(0)

	deadline, ok := ctx.Deadline()
	if ok {
		_ = conn.SetWriteDeadline(deadline)
		defer conn.SetWriteDeadline(time.Time{})
		_ = conn.SetReadDeadline(deadline)
		defer conn.SetReadDeadline(time.Time{})
	}

	// Write a message
	_, err := conn.Write([]byte(msg))
	require.NoError(t, err)

	// Read the message back
	buf := make([]byte, 1024)
	n, err := conn.Read(buf)
	require.NoError(t, err)
	require.Equal(t, msg, string(buf[:n]))
}
