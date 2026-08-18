package license_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/sloghuman"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/testutil"
)

// testAuthorizer satisfies Entitlements' expectation of a non-nil
// authorizer. The callers below never enable the workspace-capable
// licensing experiment, so it is never asked to authorize anything.
var testAuthorizer = rbac.NewCachingAuthorizer(prometheus.NewRegistry())

// premiumRuntimeHoursFixture returns a mock store primed with a Premium
// license carrying runtime hour claims (allocation 100, soft limit 80, hard
// limit 120) plus the store expectations every entitlements refresh consumes
// before usage is measured. Callers add expectations for the usage queries
// under test.
func premiumRuntimeHoursFixture(t *testing.T) (*dbmock.MockStore, *coderdenttest.LicenseOptions) {
	t.Helper()

	ctrl := gomock.NewController(t)
	mDB := dbmock.NewMockStore(ctrl)
	// Refreshes that observe publishing disabled read the enabled-since
	// marker to clear it if present.
	mDB.EXPECT().InTx(gomock.Any(), gomock.Any()).DoAndReturn(
		func(fn func(database.Store) error, _ *database.TxOptions) error {
			return fn(mDB)
		},
	).AnyTimes()
	mDB.EXPECT().TryAcquireLock(gomock.Any(), int64(database.LockIDUsagePublishingEnabledMarker)).Return(true, nil).AnyTimes()
	mDB.EXPECT().GetRuntimeConfig(gomock.Any(), license.UsagePublishingEnabledSinceKey).Return("", sql.ErrNoRows).AnyTimes()

	licenseOpts := (&coderdenttest.LicenseOptions{
		FeatureSet: codersdk.FeatureSetPremium,
		IssuedAt:   dbtime.Now().Add(-2 * time.Hour).Truncate(time.Second),
		NotBefore:  dbtime.Now().Add(-time.Hour).Truncate(time.Second),
		// GraceAt and ExpiresAt are far enough out that the license-expiry
		// warning cannot pollute the callers' warning assertions.
		GraceAt:   dbtime.Now().Add(time.Hour * 24 * 60).Truncate(time.Second),
		ExpiresAt: dbtime.Now().Add(time.Hour * 24 * 90).Truncate(time.Second),
		// The addon marks AI Bridge as explicitly entitled, suppressing
		// the unrelated "AI Governance add-on is required to use AI
		// Gateway" warning that Premium would otherwise produce.
	}).UserLimit(100).AIGovernanceAddon(100).AgentRuntimeHours(100, ptr.Ref[int64](80), ptr.Ref[int64](120))

	lic := database.License{
		ID:  1,
		JWT: coderdenttest.GenerateLicense(t, *licenseOpts),
		Exp: licenseOpts.ExpiresAt,
	}

	mDB.EXPECT().GetUnexpiredLicenses(gomock.Any()).Return([]database.License{lic}, nil)
	mDB.EXPECT().GetActiveUserCount(gomock.Any(), false).Return(int64(1), nil)
	mDB.EXPECT().GetActiveAISeatCount(gomock.Any()).Return(int64(0), nil)
	mDB.EXPECT().GetTemplatesWithFilter(gomock.Any(), gomock.Any()).Return([]database.Template{}, nil)

	return mDB, licenseOpts
}

func TestEntitlements(t *testing.T) {
	t.Parallel()
	all := make(map[codersdk.FeatureName]bool)
	for _, n := range codersdk.FeatureNames {
		all[n] = true
	}

	empty := map[codersdk.FeatureName]bool{}

	t.Run("Defaults", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		entitlements, err := license.Entitlements(context.Background(), testutil.Logger(t), db, 1, 1, coderdenttest.Keys, all, testAuthorizer, nil)
		require.NoError(t, err)
		require.False(t, entitlements.HasLicense)
		require.False(t, entitlements.Trial)
		for _, featureName := range codersdk.FeatureNames {
			require.False(t, entitlements.Features[featureName].Enabled)
			require.Equal(t, codersdk.EntitlementNotEntitled, entitlements.Features[featureName].Entitlement)
		}
	})
	t.Run("Always return the current user count", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		entitlements, err := license.Entitlements(context.Background(), testutil.Logger(t), db, 1, 1, coderdenttest.Keys, all, testAuthorizer, nil)
		require.NoError(t, err)
		require.False(t, entitlements.HasLicense)
		require.False(t, entitlements.Trial)
		require.Equal(t, *entitlements.Features[codersdk.FeatureUserLimit].Actual, int64(0))
	})
	t.Run("SingleLicenseNothing", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		db.InsertLicense(context.Background(), database.InsertLicenseParams{
			JWT: coderdenttest.GenerateLicense(t, coderdenttest.LicenseOptions{}),
			Exp: dbtime.Now().Add(time.Hour),
		})
		entitlements, err := license.Entitlements(context.Background(), testutil.Logger(t), db, 1, 1, coderdenttest.Keys, empty, testAuthorizer, nil)
		require.NoError(t, err)
		require.True(t, entitlements.HasLicense)
		require.False(t, entitlements.Trial)
		for _, featureName := range codersdk.FeatureNames {
			require.False(t, entitlements.Features[featureName].Enabled)
			require.Equal(t, codersdk.EntitlementNotEntitled, entitlements.Features[featureName].Entitlement)
		}
	})
	t.Run("SingleLicenseAll", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		db.InsertLicense(context.Background(), database.InsertLicenseParams{
			JWT: coderdenttest.GenerateLicense(t, coderdenttest.LicenseOptions{
				Features: func() license.Features {
					f := make(license.Features)
					for _, name := range codersdk.FeatureNames {
						if name == codersdk.FeatureManagedAgentLimit {
							f[codersdk.FeatureManagedAgentLimit] = 100
							continue
						}
						if name == codersdk.FeatureAgentRuntimeHours {
							f[license.ClaimAgentRuntimeHoursAllocation] = 100
							continue
						}
						f[name] = 1
					}
					return f
				}(),
			}),
			Exp: dbtime.Now().Add(time.Hour),
		})
		entitlements, err := license.Entitlements(context.Background(), testutil.Logger(t), db, 1, 1, coderdenttest.Keys, empty, testAuthorizer, nil)
		require.NoError(t, err)
		require.True(t, entitlements.HasLicense)
		require.False(t, entitlements.Trial)
		for _, featureName := range codersdk.FeatureNames {
			require.Equal(t, codersdk.EntitlementEntitled, entitlements.Features[featureName].Entitlement, featureName)
		}
	})
	t.Run("SingleLicenseGrace", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		db.InsertLicense(context.Background(), database.InsertLicenseParams{
			JWT: coderdenttest.GenerateLicense(t, coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureUserLimit: 100,
					codersdk.FeatureAuditLog:  1,
				},

				NotBefore: dbtime.Now().Add(-time.Hour * 2),
				GraceAt:   dbtime.Now().Add(-time.Hour),
				ExpiresAt: dbtime.Now().Add(time.Hour),
			}),
			Exp: dbtime.Now().Add(time.Hour),
		})
		entitlements, err := license.Entitlements(context.Background(), testutil.Logger(t), db, 1, 1, coderdenttest.Keys, all, testAuthorizer, nil)
		require.NoError(t, err)
		require.True(t, entitlements.HasLicense)
		require.False(t, entitlements.Trial)

		require.Equal(t, codersdk.EntitlementGracePeriod, entitlements.Features[codersdk.FeatureAuditLog].Entitlement)
		require.Contains(
			t, entitlements.Warnings,
			fmt.Sprintf("%s is enabled but your license for this feature is expired.", codersdk.FeatureAuditLog.Humanize()),
		)
	})
	t.Run("Expiration warning", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		db.InsertLicense(context.Background(), database.InsertLicenseParams{
			JWT: coderdenttest.GenerateLicense(t, coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureUserLimit: 100,
					codersdk.FeatureAuditLog:  1,
				},

				GraceAt:   dbtime.Now().AddDate(0, 0, 2),
				ExpiresAt: dbtime.Now().AddDate(0, 0, 5),
			}),
			Exp: dbtime.Now().AddDate(0, 0, 5),
		})

		entitlements, err := license.Entitlements(context.Background(), testutil.Logger(t), db, 1, 1, coderdenttest.Keys, all, testAuthorizer, nil)

		require.NoError(t, err)
		require.True(t, entitlements.HasLicense)
		require.False(t, entitlements.Trial)

		require.Equal(t, codersdk.EntitlementEntitled, entitlements.Features[codersdk.FeatureAuditLog].Entitlement)
		require.Contains(
			t, entitlements.Warnings,
			"Your license expires in 2 days.",
		)
	})

	t.Run("Expiration warning for license expiring in 1 day", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		db.InsertLicense(context.Background(), database.InsertLicenseParams{
			JWT: coderdenttest.GenerateLicense(t, coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureUserLimit: 100,
					codersdk.FeatureAuditLog:  1,
				},

				GraceAt:   dbtime.Now().AddDate(0, 0, 1),
				ExpiresAt: dbtime.Now().AddDate(0, 0, 5),
			}),
			Exp: time.Now().AddDate(0, 0, 5),
		})

		entitlements, err := license.Entitlements(context.Background(), testutil.Logger(t), db, 1, 1, coderdenttest.Keys, all, testAuthorizer, nil)

		require.NoError(t, err)
		require.True(t, entitlements.HasLicense)
		require.False(t, entitlements.Trial)

		require.Equal(t, codersdk.EntitlementEntitled, entitlements.Features[codersdk.FeatureAuditLog].Entitlement)
		require.Contains(
			t, entitlements.Warnings,
			"Your license expires in 1 day.",
		)
	})

	t.Run("Expiration warning suppressed if new license covers gap", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)

		// Insert the expiring license
		graceDate := dbtime.Now().AddDate(0, 0, 1)
		_, err := db.InsertLicense(context.Background(), database.InsertLicenseParams{
			JWT: coderdenttest.GenerateLicense(t, coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureUserLimit:             100,
					codersdk.FeatureAuditLog:              1,
					codersdk.FeatureAIGovernanceUserLimit: 100,
				},
				FeatureSet: codersdk.FeatureSetPremium,
				GraceAt:    graceDate,
				ExpiresAt:  dbtime.Now().AddDate(0, 0, 5),
				Addons:     []codersdk.Addon{codersdk.AddonAIGovernance},
			}),
			Exp: time.Now().AddDate(0, 0, 5),
		})
		require.NoError(t, err)

		// Warning should be generated.
		entitlements, err := license.Entitlements(context.Background(), testutil.Logger(t), db, 1, 1, coderdenttest.Keys, all, testAuthorizer, nil)
		require.NoError(t, err)
		require.True(t, entitlements.HasLicense)
		require.False(t, entitlements.Trial)
		require.Equal(t, codersdk.EntitlementEntitled, entitlements.Features[codersdk.FeatureAuditLog].Entitlement)
		require.Len(t, entitlements.Warnings, 1)
		require.Contains(t, entitlements.Warnings, "Your license expires in 1 day.")

		// Insert the new, not-yet-valid license that starts BEFORE the expiring
		// license expires.
		_, err = db.InsertLicense(context.Background(), database.InsertLicenseParams{
			JWT: coderdenttest.GenerateLicense(t, coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureUserLimit:             100,
					codersdk.FeatureAuditLog:              1,
					codersdk.FeatureAIGovernanceUserLimit: 100,
				},
				FeatureSet: codersdk.FeatureSetPremium,
				NotBefore:  graceDate.Add(-time.Hour), // contiguous, and also in the future
				GraceAt:    dbtime.Now().AddDate(1, 0, 0),
				ExpiresAt:  dbtime.Now().AddDate(1, 0, 5),
				Addons:     []codersdk.Addon{codersdk.AddonAIGovernance},
			}),
			Exp: dbtime.Now().AddDate(1, 0, 5),
		})
		require.NoError(t, err)

		// Warning should be suppressed.
		entitlements, err = license.Entitlements(context.Background(), testutil.Logger(t), db, 1, 1, coderdenttest.Keys, all, testAuthorizer, nil)
		require.NoError(t, err)
		require.True(t, entitlements.HasLicense)
		require.False(t, entitlements.Trial)
		require.Equal(t, codersdk.EntitlementEntitled, entitlements.Features[codersdk.FeatureAuditLog].Entitlement)
		require.Len(t, entitlements.Warnings, 0) // suppressed
	})

	t.Run("Expiration warning not suppressed if new license has gap", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)

		// Insert the expiring license
		graceDate := dbtime.Now().AddDate(0, 0, 1)
		_, err := db.InsertLicense(context.Background(), database.InsertLicenseParams{
			JWT: coderdenttest.GenerateLicense(t, coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureUserLimit:             100,
					codersdk.FeatureAuditLog:              1,
					codersdk.FeatureAIGovernanceUserLimit: 100,
				},
				FeatureSet: codersdk.FeatureSetPremium,
				GraceAt:    graceDate,
				ExpiresAt:  dbtime.Now().AddDate(0, 0, 5),
				Addons:     []codersdk.Addon{codersdk.AddonAIGovernance},
			}),
			Exp: time.Now().AddDate(0, 0, 5),
		})
		require.NoError(t, err)

		// Should generate a warning.
		entitlements, err := license.Entitlements(context.Background(), testutil.Logger(t), db, 1, 1, coderdenttest.Keys, all, testAuthorizer, nil)
		require.NoError(t, err)
		require.True(t, entitlements.HasLicense)
		require.False(t, entitlements.Trial)
		require.Equal(t, codersdk.EntitlementEntitled, entitlements.Features[codersdk.FeatureAuditLog].Entitlement)
		require.Len(t, entitlements.Warnings, 1)
		require.Contains(t, entitlements.Warnings, "Your license expires in 1 day.")

		// Insert the new, not-yet-valid license that starts AFTER the expiring
		// license expires (e.g. there's a gap)
		_, err = db.InsertLicense(context.Background(), database.InsertLicenseParams{
			JWT: coderdenttest.GenerateLicense(t, coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureUserLimit:             100,
					codersdk.FeatureAuditLog:              1,
					codersdk.FeatureAIGovernanceUserLimit: 100,
				},
				FeatureSet: codersdk.FeatureSetPremium,
				NotBefore:  graceDate.Add(time.Minute), // gap of 1 second!
				GraceAt:    dbtime.Now().AddDate(1, 0, 0),
				ExpiresAt:  dbtime.Now().AddDate(1, 0, 5),
				Addons:     []codersdk.Addon{codersdk.AddonAIGovernance},
			}),
			Exp: dbtime.Now().AddDate(1, 0, 5),
		})
		require.NoError(t, err)

		// Warning should still be generated.
		entitlements, err = license.Entitlements(context.Background(), testutil.Logger(t), db, 1, 1, coderdenttest.Keys, all, testAuthorizer, nil)
		require.NoError(t, err)
		require.True(t, entitlements.HasLicense)
		require.False(t, entitlements.Trial)
		require.Equal(t, codersdk.EntitlementEntitled, entitlements.Features[codersdk.FeatureAuditLog].Entitlement)
		require.Len(t, entitlements.Warnings, 1)
		require.Contains(t, entitlements.Warnings, "Your license expires in 1 day.")
	})

	t.Run("Expiration warning for trials", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		db.InsertLicense(context.Background(), database.InsertLicenseParams{
			JWT: coderdenttest.GenerateLicense(t, coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureUserLimit: 100,
					codersdk.FeatureAuditLog:  1,
				},

				Trial:     true,
				GraceAt:   dbtime.Now().AddDate(0, 0, 8),
				ExpiresAt: dbtime.Now().AddDate(0, 0, 5),
			}),
			Exp: dbtime.Now().AddDate(0, 0, 5),
		})

		entitlements, err := license.Entitlements(context.Background(), testutil.Logger(t), db, 1, 1, coderdenttest.Keys, all, testAuthorizer, nil)

		require.NoError(t, err)
		require.True(t, entitlements.HasLicense)
		require.True(t, entitlements.Trial)

		require.Equal(t, codersdk.EntitlementEntitled, entitlements.Features[codersdk.FeatureAuditLog].Entitlement)
		require.NotContains( // it should not contain a warning since it is a trial license
			t, entitlements.Warnings,
			"Your license expires in 8 days.",
		)
	})

	t.Run("Expiration warning for non trials", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		db.InsertLicense(context.Background(), database.InsertLicenseParams{
			JWT: coderdenttest.GenerateLicense(t, coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureUserLimit: 100,
					codersdk.FeatureAuditLog:  1,
				},

				GraceAt:   dbtime.Now().AddDate(0, 0, 30),
				ExpiresAt: dbtime.Now().AddDate(0, 0, 5),
			}),
			Exp: dbtime.Now().AddDate(0, 0, 5),
		})

		entitlements, err := license.Entitlements(context.Background(), testutil.Logger(t), db, 1, 1, coderdenttest.Keys, all, testAuthorizer, nil)

		require.NoError(t, err)
		require.True(t, entitlements.HasLicense)
		require.False(t, entitlements.Trial)

		require.Equal(t, codersdk.EntitlementEntitled, entitlements.Features[codersdk.FeatureAuditLog].Entitlement)
		require.NotContains( // it should not contain a warning since it is a trial license
			t, entitlements.Warnings,
			"Your license expires in 30 days.",
		)
	})

	t.Run("SingleLicenseNotEntitled", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		db.InsertLicense(context.Background(), database.InsertLicenseParams{
			JWT: coderdenttest.GenerateLicense(t, coderdenttest.LicenseOptions{}),
			Exp: time.Now().Add(time.Hour),
		})
		entitlements, err := license.Entitlements(context.Background(), testutil.Logger(t), db, 1, 1, coderdenttest.Keys, all, testAuthorizer, nil)
		require.NoError(t, err)
		require.True(t, entitlements.HasLicense)
		require.False(t, entitlements.Trial)
		for _, featureName := range codersdk.FeatureNames {
			if featureName == codersdk.FeatureUserLimit ||
				featureName == codersdk.FeatureHighAvailability ||
				featureName == codersdk.FeatureMultipleExternalAuth ||
				featureName == codersdk.FeatureManagedAgentLimit ||
				featureName == codersdk.FeatureAgentRuntimeHours ||
				featureName == codersdk.FeatureAIGovernanceUserLimit ||
				featureName == codersdk.FeatureBoundary {
				// These fields don't generate warnings when not entitled unless
				// a limit is breached, or in the case of AI Governance features,
				// they require the AI Governance addon.
				continue
			}
			niceName := featureName.Humanize()
			// Ensures features that are not entitled are properly disabled.
			require.False(t, entitlements.Features[featureName].Enabled)
			require.Equal(t, codersdk.EntitlementNotEntitled, entitlements.Features[featureName].Entitlement)
			require.Contains(t, entitlements.Warnings, fmt.Sprintf("%s is enabled but your license is not entitled to this feature.", niceName))
		}
		// Agent runtime hours is enabled by `all` and not granted by this
		// license, which is exactly the state the warning suppression covers.
		require.NotContains(t, entitlements.Warnings, fmt.Sprintf(
			"%s is enabled but your license is not entitled to this feature.",
			codersdk.FeatureAgentRuntimeHours.Humanize(),
		))
	})
	t.Run("TooManyUsers", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		activeUser1, err := db.InsertUser(context.Background(), database.InsertUserParams{
			ID:        uuid.New(),
			Username:  "test1",
			Email:     "test1@coder.com",
			LoginType: database.LoginTypePassword,
			RBACRoles: []string{},
		})
		require.NoError(t, err)
		_, err = db.UpdateUserStatus(context.Background(), database.UpdateUserStatusParams{
			ID:        activeUser1.ID,
			Status:    database.UserStatusActive,
			UpdatedAt: dbtime.Now(),
		})
		require.NoError(t, err)
		activeUser2, err := db.InsertUser(context.Background(), database.InsertUserParams{
			ID:        uuid.New(),
			Username:  "test2",
			Email:     "test2@coder.com",
			LoginType: database.LoginTypePassword,
			RBACRoles: []string{},
		})
		require.NoError(t, err)
		_, err = db.UpdateUserStatus(context.Background(), database.UpdateUserStatusParams{
			ID:        activeUser2.ID,
			Status:    database.UserStatusActive,
			UpdatedAt: dbtime.Now(),
		})
		require.NoError(t, err)
		_, err = db.InsertUser(context.Background(), database.InsertUserParams{
			ID:        uuid.New(),
			Username:  "dormant-user",
			Email:     "dormant-user@coder.com",
			LoginType: database.LoginTypePassword,
			RBACRoles: []string{},
		})
		require.NoError(t, err)
		db.InsertLicense(context.Background(), database.InsertLicenseParams{
			JWT: coderdenttest.GenerateLicense(t, coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureUserLimit: 1,
				},
			}),
			Exp: time.Now().Add(time.Hour),
		})
		entitlements, err := license.Entitlements(context.Background(), testutil.Logger(t), db, 1, 1, coderdenttest.Keys, empty, testAuthorizer, nil)
		require.NoError(t, err)
		require.True(t, entitlements.HasLicense)
		require.Contains(t, entitlements.Warnings, "Your deployment has 2 active users but is only licensed for 1.")
	})
	t.Run("MaximizeUserLimit", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		db.InsertUser(context.Background(), database.InsertUserParams{})
		db.InsertUser(context.Background(), database.InsertUserParams{})
		db.InsertLicense(context.Background(), database.InsertLicenseParams{
			JWT: coderdenttest.GenerateLicense(t, coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureUserLimit: 10,
				},
				GraceAt: time.Now().Add(59 * 24 * time.Hour),
			}),
			Exp: time.Now().Add(60 * 24 * time.Hour),
		})
		db.InsertLicense(context.Background(), database.InsertLicenseParams{
			JWT: coderdenttest.GenerateLicense(t, coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureUserLimit: 1,
				},
				GraceAt: time.Now().Add(59 * 24 * time.Hour),
			}),
			Exp: time.Now().Add(60 * 24 * time.Hour),
		})
		entitlements, err := license.Entitlements(context.Background(), testutil.Logger(t), db, 1, 1, coderdenttest.Keys, empty, testAuthorizer, nil)
		require.NoError(t, err)
		require.True(t, entitlements.HasLicense)
		require.Empty(t, entitlements.Warnings)
	})
	t.Run("MultipleLicenseEnabled", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		// One trial
		db.InsertLicense(context.Background(), database.InsertLicenseParams{
			Exp: time.Now().Add(time.Hour),
			JWT: coderdenttest.GenerateLicense(t, coderdenttest.LicenseOptions{
				Trial: true,
			}),
		})
		// One not
		db.InsertLicense(context.Background(), database.InsertLicenseParams{
			Exp: time.Now().Add(time.Hour),
			JWT: coderdenttest.GenerateLicense(t, coderdenttest.LicenseOptions{
				Trial: false,
			}),
		})

		entitlements, err := license.Entitlements(context.Background(), testutil.Logger(t), db, 1, 1, coderdenttest.Keys, empty, testAuthorizer, nil)
		require.NoError(t, err)
		require.True(t, entitlements.HasLicense)
		require.False(t, entitlements.Trial)
	})

	t.Run("Enterprise", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		_, err := db.InsertLicense(context.Background(), database.InsertLicenseParams{
			Exp: time.Now().Add(time.Hour),
			JWT: coderdenttest.GenerateLicense(t, coderdenttest.LicenseOptions{
				FeatureSet: codersdk.FeatureSetEnterprise,
			}),
		})
		require.NoError(t, err)
		entitlements, err := license.Entitlements(context.Background(), testutil.Logger(t), db, 1, 1, coderdenttest.Keys, all, testAuthorizer, nil)
		require.NoError(t, err)
		require.True(t, entitlements.HasLicense)
		require.False(t, entitlements.Trial)

		// All enterprise features should be entitled
		enterpriseFeatures := codersdk.FeatureSetEnterprise.Features()
		for _, featureName := range codersdk.FeatureNames {
			if featureName == codersdk.FeatureUserLimit {
				continue
			}
			if featureName == codersdk.FeatureManagedAgentLimit {
				// Enterprise licenses don't get any agents by default.
				continue
			}
			if featureName.IsAddonFeature() {
				continue
			}
			if slices.Contains(enterpriseFeatures, featureName) {
				require.True(t, entitlements.Features[featureName].Enabled, featureName)
				require.Equal(t, codersdk.EntitlementEntitled, entitlements.Features[featureName].Entitlement)
			} else {
				require.False(t, entitlements.Features[featureName].Enabled, featureName)
				require.Equal(t, codersdk.EntitlementNotEntitled, entitlements.Features[featureName].Entitlement)
			}
		}
	})

	t.Run("Premium", func(t *testing.T) {
		t.Parallel()
		const userLimit = 1
		const expectedAgentLimit = 1000

		db, _ := dbtestutil.NewDB(t)
		licenseOptions := coderdenttest.LicenseOptions{
			NotBefore:  dbtime.Now().Add(-time.Hour * 2),
			GraceAt:    dbtime.Now().Add(time.Hour * 24),
			ExpiresAt:  dbtime.Now().Add(time.Hour * 24 * 2),
			FeatureSet: codersdk.FeatureSetPremium,
			Features: license.Features{
				codersdk.FeatureUserLimit: userLimit,
			},
		}
		_, err := db.InsertLicense(context.Background(), database.InsertLicenseParams{
			Exp: time.Now().Add(time.Hour),
			JWT: coderdenttest.GenerateLicense(t, licenseOptions),
		})
		require.NoError(t, err)
		entitlements, err := license.Entitlements(context.Background(), testutil.Logger(t), db, 1, 1, coderdenttest.Keys, all, testAuthorizer, nil)
		require.NoError(t, err)
		require.True(t, entitlements.HasLicense)
		require.False(t, entitlements.Trial)

		// All premium features should be entitled
		enterpriseFeatures := codersdk.FeatureSetPremium.Features()
		for _, featureName := range codersdk.FeatureNames {
			if featureName == codersdk.FeatureUserLimit {
				continue
			}
			if featureName == codersdk.FeatureManagedAgentLimit {
				agentEntitlement := entitlements.Features[featureName]
				require.True(t, agentEntitlement.Enabled)
				require.Equal(t, codersdk.EntitlementEntitled, agentEntitlement.Entitlement)
				require.EqualValues(t, expectedAgentLimit, *agentEntitlement.Limit)

				// This might be shocking, but there's a sound reason for this.
				// See license.go for more details.
				agentUsagePeriodIssuedAt := time.Date(2025, 7, 1, 0, 0, 0, 0, time.UTC)
				agentUsagePeriodStart := agentUsagePeriodIssuedAt
				agentUsagePeriodEnd := agentUsagePeriodStart.AddDate(100, 0, 0)
				require.Equal(t, agentUsagePeriodIssuedAt, agentEntitlement.UsagePeriod.IssuedAt)
				require.WithinDuration(t, agentUsagePeriodStart, agentEntitlement.UsagePeriod.Start, time.Second)
				require.WithinDuration(t, agentUsagePeriodEnd, agentEntitlement.UsagePeriod.End, time.Second)
				continue
			}
			if featureName == codersdk.FeatureAgentRuntimeHours {
				// Premium licenses without agent runtime hour claims are
				// grandfathered into a zero-hour allocation over the
				// license term, with usage still measured. See license.go
				// for more details.
				runtimeEntitlement := entitlements.Features[featureName]
				require.False(t, runtimeEntitlement.Enabled)
				require.Equal(t, codersdk.EntitlementEntitled, runtimeEntitlement.Entitlement)
				require.NotNil(t, runtimeEntitlement.Limit)
				require.EqualValues(t, 0, *runtimeEntitlement.Limit)
				require.NotNil(t, runtimeEntitlement.UsagePeriod)
				require.Equal(t, time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC), runtimeEntitlement.UsagePeriod.IssuedAt)
				require.WithinDuration(t, licenseOptions.NotBefore, runtimeEntitlement.UsagePeriod.Start, time.Second)
				require.WithinDuration(t, licenseOptions.ExpiresAt, runtimeEntitlement.UsagePeriod.End, time.Second)
				require.NotNil(t, runtimeEntitlement.Actual)
				require.EqualValues(t, 0, *runtimeEntitlement.Actual)
				continue
			}
			if featureName.IsAddonFeature() {
				continue
			}

			if slices.Contains(enterpriseFeatures, featureName) {
				require.True(t, entitlements.Features[featureName].Enabled, featureName)
				require.Equal(t, codersdk.EntitlementEntitled, entitlements.Features[featureName].Entitlement)
			} else {
				require.False(t, entitlements.Features[featureName].Enabled, featureName)
				require.Equal(t, codersdk.EntitlementNotEntitled, entitlements.Features[featureName].Entitlement)
			}
		}
	})

	t.Run("SetNone", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		_, err := db.InsertLicense(context.Background(), database.InsertLicenseParams{
			Exp: time.Now().Add(time.Hour),
			JWT: coderdenttest.GenerateLicense(t, coderdenttest.LicenseOptions{
				FeatureSet: "",
			}),
		})
		require.NoError(t, err)
		entitlements, err := license.Entitlements(context.Background(), testutil.Logger(t), db, 1, 1, coderdenttest.Keys, all, testAuthorizer, nil)
		require.NoError(t, err)
		require.True(t, entitlements.HasLicense)
		require.False(t, entitlements.Trial)

		for _, featureName := range codersdk.FeatureNames {
			require.False(t, entitlements.Features[featureName].Enabled, featureName)
			require.Equal(t, codersdk.EntitlementNotEntitled, entitlements.Features[featureName].Entitlement)
		}
	})

	// AllFeatures uses the deprecated 'AllFeatures' boolean.
	t.Run("AllFeatures", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		db.InsertLicense(context.Background(), database.InsertLicenseParams{
			Exp: time.Now().Add(time.Hour),
			JWT: coderdenttest.GenerateLicense(t, coderdenttest.LicenseOptions{
				AllFeatures: true,
			}),
		})
		entitlements, err := license.Entitlements(context.Background(), testutil.Logger(t), db, 1, 1, coderdenttest.Keys, all, testAuthorizer, nil)
		require.NoError(t, err)
		require.True(t, entitlements.HasLicense)
		require.False(t, entitlements.Trial)

		// All enterprise features should be entitled
		enterpriseFeatures := codersdk.FeatureSetEnterprise.Features()
		for _, featureName := range codersdk.FeatureNames {
			if featureName.UsesLimit() {
				continue
			}
			if featureName.IsAddonFeature() {
				continue
			}
			if slices.Contains(enterpriseFeatures, featureName) {
				require.True(t, entitlements.Features[featureName].Enabled, featureName)
				require.Equal(t, codersdk.EntitlementEntitled, entitlements.Features[featureName].Entitlement)
			} else {
				require.False(t, entitlements.Features[featureName].Enabled, featureName)
				require.Equal(t, codersdk.EntitlementNotEntitled, entitlements.Features[featureName].Entitlement)
			}
		}
	})

	t.Run("AllFeaturesAlwaysEnable", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		db.InsertLicense(context.Background(), database.InsertLicenseParams{
			Exp: dbtime.Now().Add(time.Hour),
			JWT: coderdenttest.GenerateLicense(t, coderdenttest.LicenseOptions{
				AllFeatures: true,
			}),
		})
		entitlements, err := license.Entitlements(context.Background(), testutil.Logger(t), db, 1, 1, coderdenttest.Keys, empty, testAuthorizer, nil)
		require.NoError(t, err)
		require.True(t, entitlements.HasLicense)
		require.False(t, entitlements.Trial)
		// All enterprise features should be entitled
		enterpriseFeatures := codersdk.FeatureSetEnterprise.Features()
		for _, featureName := range codersdk.FeatureNames {
			if featureName.UsesLimit() {
				continue
			}

			feature := entitlements.Features[featureName]
			if slices.Contains(enterpriseFeatures, featureName) {
				require.Equal(t, featureName.AlwaysEnable(), feature.Enabled)
				require.Equal(t, codersdk.EntitlementEntitled, feature.Entitlement)
			} else {
				require.False(t, entitlements.Features[featureName].Enabled, featureName)
				require.Equal(t, codersdk.EntitlementNotEntitled, entitlements.Features[featureName].Entitlement)
			}
		}
	})

	t.Run("AllFeaturesGrace", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		db.InsertLicense(context.Background(), database.InsertLicenseParams{
			Exp: dbtime.Now().Add(time.Hour),
			JWT: coderdenttest.GenerateLicense(t, coderdenttest.LicenseOptions{
				AllFeatures: true,
				NotBefore:   dbtime.Now().Add(-time.Hour * 2),
				GraceAt:     dbtime.Now().Add(-time.Hour),
				ExpiresAt:   dbtime.Now().Add(time.Hour),
			}),
		})
		entitlements, err := license.Entitlements(context.Background(), testutil.Logger(t), db, 1, 1, coderdenttest.Keys, all, testAuthorizer, nil)
		require.NoError(t, err)
		require.True(t, entitlements.HasLicense)
		require.False(t, entitlements.Trial)
		// All enterprise features should be entitled
		enterpriseFeatures := codersdk.FeatureSetEnterprise.Features()
		for _, featureName := range codersdk.FeatureNames {
			if featureName == codersdk.FeatureUserLimit {
				continue
			}
			if featureName.IsAddonFeature() {
				continue
			}
			if slices.Contains(enterpriseFeatures, featureName) {
				require.True(t, entitlements.Features[featureName].Enabled, featureName)
				require.Equal(t, codersdk.EntitlementGracePeriod, entitlements.Features[featureName].Entitlement)
			} else {
				require.False(t, entitlements.Features[featureName].Enabled, featureName)
				require.Equal(t, codersdk.EntitlementNotEntitled, entitlements.Features[featureName].Entitlement)
			}
		}
	})

	t.Run("MultipleReplicasNoLicense", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		entitlements, err := license.Entitlements(context.Background(), testutil.Logger(t), db, 2, 1, coderdenttest.Keys, all, testAuthorizer, nil)
		require.NoError(t, err)
		require.False(t, entitlements.HasLicense)
		require.Len(t, entitlements.Errors, 1)
		require.Equal(t, "You have multiple replicas but high availability is an Enterprise feature. You will be unable to connect to workspaces.", entitlements.Errors[0])
	})

	t.Run("MultipleReplicasNotEntitled", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		db.InsertLicense(context.Background(), database.InsertLicenseParams{
			Exp: time.Now().Add(time.Hour),
			JWT: coderdenttest.GenerateLicense(t, coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureAuditLog: 1,
				},
			}),
		})
		entitlements, err := license.Entitlements(context.Background(), testutil.Logger(t), db, 2, 1, coderdenttest.Keys, map[codersdk.FeatureName]bool{
			codersdk.FeatureHighAvailability: true,
		}, testAuthorizer, nil)
		require.NoError(t, err)
		require.True(t, entitlements.HasLicense)
		require.Len(t, entitlements.Errors, 1)
		require.Equal(t, "You have multiple replicas but your license is not entitled to high availability. You will be unable to connect to workspaces.", entitlements.Errors[0])
	})

	t.Run("MultipleReplicasGrace", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		db.InsertLicense(context.Background(), database.InsertLicenseParams{
			JWT: coderdenttest.GenerateLicense(t, coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureHighAvailability: 1,
				},
				NotBefore: dbtime.Now().Add(-time.Hour * 2),
				GraceAt:   time.Now().Add(-time.Hour),
				ExpiresAt: time.Now().Add(time.Hour),
			}),
			Exp: time.Now().Add(time.Hour),
		})
		entitlements, err := license.Entitlements(context.Background(), testutil.Logger(t), db, 2, 1, coderdenttest.Keys, map[codersdk.FeatureName]bool{
			codersdk.FeatureHighAvailability: true,
		}, testAuthorizer, nil)
		require.NoError(t, err)
		require.True(t, entitlements.HasLicense)
		require.Len(t, entitlements.Warnings, 1)
		require.Equal(t, "You have multiple replicas but your license for high availability is expired. Reduce to one replica or workspace connections will stop working.", entitlements.Warnings[0])
	})

	t.Run("MultipleGitAuthNoLicense", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		entitlements, err := license.Entitlements(context.Background(), testutil.Logger(t), db, 1, 2, coderdenttest.Keys, all, testAuthorizer, nil)
		require.NoError(t, err)
		require.False(t, entitlements.HasLicense)
		require.Len(t, entitlements.Errors, 1)
		require.Equal(t, "You have multiple External Auth Providers configured but this is an Enterprise feature. Reduce to one.", entitlements.Errors[0])
	})

	t.Run("MultipleGitAuthNotEntitled", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		db.InsertLicense(context.Background(), database.InsertLicenseParams{
			Exp: time.Now().Add(time.Hour),
			JWT: coderdenttest.GenerateLicense(t, coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureAuditLog: 1,
				},
			}),
		})
		entitlements, err := license.Entitlements(context.Background(), testutil.Logger(t), db, 1, 2, coderdenttest.Keys, map[codersdk.FeatureName]bool{
			codersdk.FeatureMultipleExternalAuth: true,
		}, testAuthorizer, nil)
		require.NoError(t, err)
		require.True(t, entitlements.HasLicense)
		require.Len(t, entitlements.Errors, 1)
		require.Equal(t, "You have multiple External Auth Providers configured but your license is limited at one.", entitlements.Errors[0])
	})

	t.Run("MultipleGitAuthGrace", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		db.InsertLicense(context.Background(), database.InsertLicenseParams{
			JWT: coderdenttest.GenerateLicense(t, coderdenttest.LicenseOptions{
				NotBefore: dbtime.Now().Add(-time.Hour * 2),
				GraceAt:   time.Now().Add(-time.Hour),
				ExpiresAt: time.Now().Add(time.Hour),
				Features: license.Features{
					codersdk.FeatureMultipleExternalAuth: 1,
				},
			}),
			Exp: time.Now().Add(time.Hour),
		})
		entitlements, err := license.Entitlements(context.Background(), testutil.Logger(t), db, 1, 2, coderdenttest.Keys, map[codersdk.FeatureName]bool{
			codersdk.FeatureMultipleExternalAuth: true,
		}, testAuthorizer, nil)
		require.NoError(t, err)
		require.True(t, entitlements.HasLicense)
		require.Len(t, entitlements.Warnings, 1)
		require.Equal(t, "You have multiple External Auth Providers configured but your license is expired. Reduce to one.", entitlements.Warnings[0])
	})

	t.Run("ManagedAgentLimitHasValue", func(t *testing.T) {
		t.Parallel()

		// Use a mock database for this test so I don't need to make real
		// workspace builds.
		ctrl := gomock.NewController(t)
		mDB := dbmock.NewMockStore(ctrl)
		// Refreshes that observe publishing disabled clear the enabled-since
		// marker in a locked transaction.
		mDB.EXPECT().InTx(gomock.Any(), gomock.Any()).DoAndReturn(
			func(fn func(database.Store) error, _ *database.TxOptions) error {
				return fn(mDB)
			},
		).AnyTimes()
		mDB.EXPECT().TryAcquireLock(gomock.Any(), int64(database.LockIDUsagePublishingEnabledMarker)).Return(true, nil).AnyTimes()
		mDB.EXPECT().GetRuntimeConfig(gomock.Any(), license.UsagePublishingEnabledSinceKey).Return("", sql.ErrNoRows).AnyTimes()

		licenseOpts := (&coderdenttest.LicenseOptions{
			FeatureSet: codersdk.FeatureSetPremium,
			IssuedAt:   dbtime.Now().Add(-2 * time.Hour).Truncate(time.Second),
			NotBefore:  dbtime.Now().Add(-time.Hour).Truncate(time.Second),
			GraceAt:    dbtime.Now().Add(time.Hour * 24 * 60).Truncate(time.Second), // 60 days to remove warning
			ExpiresAt:  dbtime.Now().Add(time.Hour * 24 * 90).Truncate(time.Second), // 90 days to remove warning
			Addons:     []codersdk.Addon{codersdk.AddonAIGovernance},
			Features: license.Features{
				codersdk.FeatureAIGovernanceUserLimit: 100,
			},
		}).
			UserLimit(100).
			ManagedAgentLimit(100)

		lic := database.License{
			ID:  1,
			JWT: coderdenttest.GenerateLicense(t, *licenseOpts),
			Exp: licenseOpts.ExpiresAt,
		}

		mDB.EXPECT().
			GetUnexpiredLicenses(gomock.Any()).
			Return([]database.License{lic}, nil)
		mDB.EXPECT().
			GetActiveUserCount(gomock.Any(), false).
			Return(int64(1), nil)
		mDB.EXPECT().
			GetActiveAISeatCount(gomock.Any()).
			Return(int64(27), nil)
		mDB.EXPECT().
			GetTotalUsageDCManagedAgentsV1(gomock.Any(), gomock.Cond(func(params database.GetTotalUsageDCManagedAgentsV1Params) bool {
				// gomock doesn't seem to compare times very nicely, so check
				// them manually.
				//
				// The query truncates these times to the date in UTC timezone,
				// but we still check that we're passing in the correct
				// timestamp in the first place.
				if !assert.WithinDuration(t, licenseOpts.NotBefore, params.StartDate, time.Second) {
					return false
				}
				if !assert.WithinDuration(t, licenseOpts.ExpiresAt, params.EndDate, time.Second) {
					return false
				}
				return true
			})).
			Return(int64(175), nil)
		// The premium grandfather default grants a zero-hour agent runtime
		// allocation, so that usage is queried too. It is not what this
		// test is about.
		mDB.EXPECT().
			GetTotalUsageHBAgentRuntimeV1(gomock.Any(), gomock.Any()).
			Return(int64(0), nil)
		mDB.EXPECT().
			GetTemplatesWithFilter(gomock.Any(), gomock.Any()).
			Return([]database.Template{}, nil)

		entitlements, err := license.Entitlements(context.Background(), testutil.Logger(t), mDB, 1, 0, coderdenttest.Keys, all, testAuthorizer, nil)
		require.NoError(t, err)
		require.True(t, entitlements.HasLicense)

		managedAgentLimit, ok := entitlements.Features[codersdk.FeatureManagedAgentLimit]
		require.True(t, ok)

		require.NotNil(t, managedAgentLimit.Limit)
		// The soft limit value (100) is used as the single Limit.
		require.EqualValues(t, 100, *managedAgentLimit.Limit)
		require.NotNil(t, managedAgentLimit.Actual)
		require.EqualValues(t, 175, *managedAgentLimit.Actual)

		aiGovernanceSeatLimit, ok := entitlements.Features[codersdk.FeatureAIGovernanceUserLimit]
		require.True(t, ok)
		require.NotNil(t, aiGovernanceSeatLimit.Actual)
		require.EqualValues(t, 27, *aiGovernanceSeatLimit.Actual)
		require.NotNil(t, aiGovernanceSeatLimit.Limit)
		require.EqualValues(t, 100, *aiGovernanceSeatLimit.Limit)

		// Usage exceeds the limit, so an exceeded warning should be present.
		require.Len(t, entitlements.Warnings, 1)
		require.Equal(t, codersdk.LicenseManagedAgentLimitExceededWarningText, entitlements.Warnings[0])
	})

	t.Run("AgentRuntimeHoursHasValue", func(t *testing.T) {
		t.Parallel()

		// Use a mock database so the production closure that reads
		// usage_events can be observed directly.
		mDB, licenseOpts := premiumRuntimeHoursFixture(t)

		// The Premium feature set grants a default managed agent limit, so
		// that usage is queried too. It is not what this test is about.
		mDB.EXPECT().
			GetTotalUsageDCManagedAgentsV1(gomock.Any(), gomock.Any()).
			Return(int64(0), nil)
		mDB.EXPECT().
			GetTotalUsageHBAgentRuntimeV1(gomock.Any(), gomock.Cond(func(params database.GetTotalUsageHBAgentRuntimeV1Params) bool {
				// gomock doesn't seem to compare times very nicely, so check
				// them manually. The bounds must be the usage period of the
				// winning license.
				if !assert.WithinDuration(t, licenseOpts.NotBefore, params.StartTime, time.Second) {
					return false
				}
				if !assert.WithinDuration(t, licenseOpts.ExpiresAt, params.EndTime, time.Second) {
					return false
				}
				return true
			})).
			// 90h30m of runtime floors to 90 hours.
			Return((90*time.Hour + 30*time.Minute).Milliseconds(), nil)

		entitlements, err := license.Entitlements(context.Background(), testutil.Logger(t), mDB, 1, 0, coderdenttest.Keys, all, testAuthorizer, nil)
		require.NoError(t, err)
		require.True(t, entitlements.HasLicense)
		require.Empty(t, entitlements.Errors)

		runtimeHours, ok := entitlements.Features[codersdk.FeatureAgentRuntimeHours]
		require.True(t, ok)
		require.NotNil(t, runtimeHours.Actual)
		require.EqualValues(t, 90, *runtimeHours.Actual)
		require.NotNil(t, runtimeHours.Limit)
		require.EqualValues(t, 100, *runtimeHours.Limit)

		// 90 hours is past the soft limit of 80 but below the allocation of
		// 100, so only the soft warning is emitted.
		require.Len(t, entitlements.Warnings, 1)
		require.Equal(t,
			fmt.Sprintf(codersdk.LicenseAgentRuntimeHoursSoftLimitWarningText, 90, 100, 80),
			entitlements.Warnings[0])
	})

	// An unlimited (-1) allocation still measures and publishes Actual, but
	// never emits a runtime hours warning regardless of usage.
	t.Run("AgentRuntimeHoursUnlimited", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		mDB := dbmock.NewMockStore(ctrl)
		// Refreshes that observe publishing disabled clear the enabled-since
		// marker in a locked transaction.
		mDB.EXPECT().InTx(gomock.Any(), gomock.Any()).DoAndReturn(
			func(fn func(database.Store) error, _ *database.TxOptions) error {
				return fn(mDB)
			},
		).AnyTimes()
		mDB.EXPECT().TryAcquireLock(gomock.Any(), int64(database.LockIDUsagePublishingEnabledMarker)).Return(true, nil).AnyTimes()
		mDB.EXPECT().GetRuntimeConfig(gomock.Any(), license.UsagePublishingEnabledSinceKey).Return("", sql.ErrNoRows).AnyTimes()

		licenseOpts := (&coderdenttest.LicenseOptions{
			FeatureSet: codersdk.FeatureSetPremium,
			IssuedAt:   dbtime.Now().Add(-2 * time.Hour).Truncate(time.Second),
			NotBefore:  dbtime.Now().Add(-time.Hour).Truncate(time.Second),
			GraceAt:    dbtime.Now().Add(time.Hour * 24 * 60).Truncate(time.Second), // 60 days to remove warning
			ExpiresAt:  dbtime.Now().Add(time.Hour * 24 * 90).Truncate(time.Second), // 90 days to remove warning
		}).UserLimit(100).AIGovernanceAddon(100).
			AgentRuntimeHours(license.AgentRuntimeHoursUnlimitedAllocation, nil, nil)

		lic := database.License{
			ID:  1,
			JWT: coderdenttest.GenerateLicense(t, *licenseOpts),
			Exp: licenseOpts.ExpiresAt,
		}

		mDB.EXPECT().GetUnexpiredLicenses(gomock.Any()).Return([]database.License{lic}, nil)
		mDB.EXPECT().GetActiveUserCount(gomock.Any(), false).Return(int64(1), nil)
		mDB.EXPECT().GetActiveAISeatCount(gomock.Any()).Return(int64(0), nil)
		mDB.EXPECT().GetTemplatesWithFilter(gomock.Any(), gomock.Any()).Return([]database.Template{}, nil)
		mDB.EXPECT().GetTotalUsageDCManagedAgentsV1(gomock.Any(), gomock.Any()).Return(int64(0), nil)
		mDB.EXPECT().
			GetTotalUsageHBAgentRuntimeV1(gomock.Any(), gomock.Any()).
			// Usage far beyond any plausible metered allocation.
			Return((1_000_000 * time.Hour).Milliseconds(), nil)

		entitlements, err := license.Entitlements(context.Background(), testutil.Logger(t), mDB, 1, 0, coderdenttest.Keys, all, testAuthorizer, nil)
		require.NoError(t, err)
		require.True(t, entitlements.HasLicense)
		require.Empty(t, entitlements.Errors)

		runtimeHours, ok := entitlements.Features[codersdk.FeatureAgentRuntimeHours]
		require.True(t, ok)
		require.True(t, runtimeHours.Enabled)
		require.Nil(t, runtimeHours.Limit)
		require.Nil(t, runtimeHours.SoftLimit)
		require.Nil(t, runtimeHours.HardLimit)
		require.NotNil(t, runtimeHours.UsagePeriod)
		require.NotNil(t, runtimeHours.Actual)
		require.EqualValues(t, 1_000_000, *runtimeHours.Actual)

		require.Empty(t, entitlements.Warnings)
	})

	t.Run("UsageQueryErrorsAreLoggedAndStable", func(t *testing.T) {
		t.Parallel()

		// Drive the real Entitlements closures with a mock database so
		// measureAgentRuntimeMs's failure path is exercised end to end: the cause
		// must land in the coderd log, which the stable payload texts point
		// at, and must not land on the unauthenticated entitlements payload.
		mDB, _ := premiumRuntimeHoursFixture(t)

		mDB.EXPECT().
			GetTotalUsageDCManagedAgentsV1(gomock.Any(), gomock.Any()).
			Return(int64(0), nil)
		mDB.EXPECT().
			GetTotalUsageHBAgentRuntimeV1(gomock.Any(), gomock.Any()).
			Return(int64(0), xerrors.New("kaboom runtime"))

		// The error-level logs are the behavior under test, so the default
		// failing test logger cannot be used.
		var logBuf bytes.Buffer
		logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).
			AppendSinks(sloghuman.Sink(&logBuf))

		entitlements, err := license.Entitlements(context.Background(), logger, mDB, 1, 0, coderdenttest.Keys, all, testAuthorizer, nil)
		require.NoError(t, err)
		require.True(t, entitlements.HasLicense)

		// The failure surfaces its stable text without the raw cause.
		require.Contains(t, entitlements.Errors, codersdk.LicenseAgentRuntimeUsageUnavailableErrorText)
		for _, entry := range append(entitlements.Errors, entitlements.Warnings...) {
			require.NotContains(t, entry, "kaboom")
		}

		logs := logBuf.String()
		require.Contains(t, logs, "get agent runtime for entitlements")
		require.Contains(t, logs, "kaboom runtime")
	})

	t.Run("UsageQueryCancelDoesNotLogError", func(t *testing.T) {
		t.Parallel()

		// A query failing while the refresh's own context is canceled,
		// e.g. during shutdown, aborts the whole entitlements refresh and
		// must not log a false query-failure alarm at error level.
		mDB, _ := premiumRuntimeHoursFixture(t)

		mDB.EXPECT().
			GetTotalUsageDCManagedAgentsV1(gomock.Any(), gomock.Any()).
			Return(int64(0), nil)
		mDB.EXPECT().
			GetTotalUsageHBAgentRuntimeV1(gomock.Any(), gomock.Any()).
			Return(int64(0), context.Canceled)

		var logBuf bytes.Buffer
		logger := testutil.Logger(t).AppendSinks(sloghuman.Sink(&logBuf))

		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := license.Entitlements(ctx, logger, mDB, 1, 0, coderdenttest.Keys, all, testAuthorizer, nil)
		require.ErrorContains(t, err, "get agent runtime")
		require.NotContains(t, logBuf.String(), "get agent runtime for entitlements")
	})

	t.Run("AIGovernanceSeatWarnings", func(t *testing.T) {
		t.Parallel()

		testCases := []struct {
			name            string
			limit           int64
			activeSeatCount int64
			expectedWarning string
		}{
			{
				name:            "At90Percent",
				limit:           100,
				activeSeatCount: 90,
				expectedWarning: fmt.Sprintf(codersdk.LicenseAIGovernance90PercentWarningText, 90),
			},
			{
				name:            "Below90Percent",
				limit:           100,
				activeSeatCount: 89,
			},
			{
				name:            "OverLimit",
				limit:           100,
				activeSeatCount: 110,
				expectedWarning: fmt.Sprintf(codersdk.LicenseAIGovernanceOverLimitWarningText, 110, 100, 10),
			},
			{
				name:            "AtLimit",
				limit:           100,
				activeSeatCount: 100,
				expectedWarning: fmt.Sprintf(codersdk.LicenseAIGovernance90PercentWarningText, 100),
			},
			{
				name:            "OverLimitRoundingDown",
				limit:           101,
				activeSeatCount: 106,
				expectedWarning: fmt.Sprintf(codersdk.LicenseAIGovernanceOverLimitWarningText, 106, 101, 5),
			},
			{
				name:            "TinyOverage",
				limit:           1000,
				activeSeatCount: 1001,
				expectedWarning: fmt.Sprintf(codersdk.LicenseAIGovernanceOverLimitWarningText, 1001, 1000, 1),
			},
			{
				name:            "ZeroLimitGuard",
				limit:           0,
				activeSeatCount: 5,
			},
		}

		for _, tc := range testCases {
			tc := tc
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				ctrl := gomock.NewController(t)
				mDB := dbmock.NewMockStore(ctrl)
				// Refreshes that observe publishing disabled read the enabled-since
				// marker to clear it if present.
				mDB.EXPECT().InTx(gomock.Any(), gomock.Any()).DoAndReturn(
					func(fn func(database.Store) error, _ *database.TxOptions) error {
						return fn(mDB)
					},
				).AnyTimes()
				mDB.EXPECT().TryAcquireLock(gomock.Any(), int64(database.LockIDUsagePublishingEnabledMarker)).Return(true, nil).AnyTimes()
				mDB.EXPECT().GetRuntimeConfig(gomock.Any(), license.UsagePublishingEnabledSinceKey).Return("", sql.ErrNoRows).AnyTimes()

				licenseOpts := (&coderdenttest.LicenseOptions{
					FeatureSet: codersdk.FeatureSetPremium,
					NotBefore:  dbtime.Now().Add(-time.Hour).Truncate(time.Second),
					GraceAt:    dbtime.Now().Add(time.Hour * 24 * 60).Truncate(time.Second),
					ExpiresAt:  dbtime.Now().Add(time.Hour * 24 * 90).Truncate(time.Second),
					Addons:     []codersdk.Addon{codersdk.AddonAIGovernance},
					Features: license.Features{
						codersdk.FeatureAIGovernanceUserLimit: tc.limit,
					},
				}).
					UserLimit(100)

				lic := database.License{
					ID:  1,
					JWT: coderdenttest.GenerateLicense(t, *licenseOpts),
					Exp: licenseOpts.ExpiresAt,
				}

				mDB.EXPECT().
					GetUnexpiredLicenses(gomock.Any()).
					Return([]database.License{lic}, nil)
				mDB.EXPECT().
					GetActiveUserCount(gomock.Any(), false).
					Return(int64(1), nil)
				mDB.EXPECT().
					GetActiveAISeatCount(gomock.Any()).
					Return(tc.activeSeatCount, nil)
				mDB.EXPECT().
					GetTotalUsageDCManagedAgentsV1(gomock.Any(), gomock.Any()).
					Return(int64(0), nil)
				// The premium grandfather default grants a zero-hour agent
				// runtime allocation, so that usage is queried too. It is
				// not what this test is about.
				mDB.EXPECT().
					GetTotalUsageHBAgentRuntimeV1(gomock.Any(), gomock.Any()).
					Return(int64(0), nil)
				mDB.EXPECT().
					GetTemplatesWithFilter(gomock.Any(), gomock.Any()).
					Return([]database.Template{}, nil)

				entitlements, err := license.Entitlements(context.Background(), testutil.Logger(t), mDB, 1, 0, coderdenttest.Keys, all, testAuthorizer, nil)
				require.NoError(t, err)
				require.True(t, entitlements.HasLicense)

				aiGovernanceSeatLimit, ok := entitlements.Features[codersdk.FeatureAIGovernanceUserLimit]
				require.True(t, ok)

				if tc.limit > 0 {
					require.NotNil(t, aiGovernanceSeatLimit.Actual)
					require.EqualValues(t, tc.activeSeatCount, *aiGovernanceSeatLimit.Actual)
					require.NotNil(t, aiGovernanceSeatLimit.Limit)
					require.EqualValues(t, tc.limit, *aiGovernanceSeatLimit.Limit)
				} else {
					require.Nil(t, aiGovernanceSeatLimit.Actual)
					require.Nil(t, aiGovernanceSeatLimit.Limit)
				}

				if tc.expectedWarning == "" {
					require.Len(t, entitlements.Warnings, 0)
				} else {
					require.Len(t, entitlements.Warnings, 1)
					require.Equal(t, tc.expectedWarning, entitlements.Warnings[0])
				}
			})
		}

		t.Run("GracePeriodOverLimit", func(t *testing.T) {
			t.Parallel()

			const (
				limit           int64 = 100
				activeSeatCount int64 = 127
			)

			ctrl := gomock.NewController(t)
			mDB := dbmock.NewMockStore(ctrl)
			// Refreshes that observe publishing disabled read the enabled-since
			// marker to clear it if present.
			mDB.EXPECT().InTx(gomock.Any(), gomock.Any()).DoAndReturn(
				func(fn func(database.Store) error, _ *database.TxOptions) error {
					return fn(mDB)
				},
			).AnyTimes()
			mDB.EXPECT().TryAcquireLock(gomock.Any(), int64(database.LockIDUsagePublishingEnabledMarker)).Return(true, nil).AnyTimes()
			mDB.EXPECT().GetRuntimeConfig(gomock.Any(), license.UsagePublishingEnabledSinceKey).Return("", sql.ErrNoRows).AnyTimes()

			licenseOpts := &coderdenttest.LicenseOptions{
				NotBefore: dbtime.Now().Add(-2 * time.Hour).Truncate(time.Second),
				GraceAt:   dbtime.Now().Add(-time.Hour).Truncate(time.Second),
				ExpiresAt: dbtime.Now().Add(24 * time.Hour).Truncate(time.Second),
				Addons:    []codersdk.Addon{codersdk.AddonAIGovernance},
				Features: license.Features{
					codersdk.FeatureAIGovernanceUserLimit: limit,
				},
			}

			lic := database.License{
				ID:  1,
				JWT: coderdenttest.GenerateLicense(t, *licenseOpts),
				Exp: licenseOpts.ExpiresAt,
			}

			mDB.EXPECT().
				GetUnexpiredLicenses(gomock.Any()).
				Return([]database.License{lic}, nil)
			mDB.EXPECT().
				GetActiveUserCount(gomock.Any(), false).
				Return(int64(1), nil)
			mDB.EXPECT().
				GetActiveAISeatCount(gomock.Any()).
				Return(activeSeatCount, nil)
			mDB.EXPECT().
				GetTemplatesWithFilter(gomock.Any(), gomock.Any()).
				Return([]database.Template{}, nil)

			enablements := map[codersdk.FeatureName]bool{
				codersdk.FeatureAIGovernanceUserLimit: true,
			}

			entitlements, err := license.Entitlements(context.Background(), testutil.Logger(t), mDB, 1, 0, coderdenttest.Keys, enablements, testAuthorizer, nil)
			require.NoError(t, err)
			require.True(t, entitlements.HasLicense)

			feature, ok := entitlements.Features[codersdk.FeatureAIGovernanceUserLimit]
			require.True(t, ok)
			require.Equal(t, codersdk.EntitlementGracePeriod, feature.Entitlement)

			require.Contains(t, entitlements.Warnings,
				fmt.Sprintf(
					"Your deployment has %d active AI Governance seats but the license with the limit %d is expired.",
					activeSeatCount, limit,
				),
			)
			require.Contains(t, entitlements.Warnings,
				fmt.Sprintf(codersdk.LicenseAIGovernanceOverLimitWarningText, activeSeatCount, limit, 27),
			)
		})

		t.Run("GracePeriod90Percent", func(t *testing.T) {
			t.Parallel()

			const (
				limit           int64 = 100
				activeSeatCount int64 = 95
			)

			ctrl := gomock.NewController(t)
			mDB := dbmock.NewMockStore(ctrl)
			// Refreshes that observe publishing disabled read the enabled-since
			// marker to clear it if present.
			mDB.EXPECT().InTx(gomock.Any(), gomock.Any()).DoAndReturn(
				func(fn func(database.Store) error, _ *database.TxOptions) error {
					return fn(mDB)
				},
			).AnyTimes()
			mDB.EXPECT().TryAcquireLock(gomock.Any(), int64(database.LockIDUsagePublishingEnabledMarker)).Return(true, nil).AnyTimes()
			mDB.EXPECT().GetRuntimeConfig(gomock.Any(), license.UsagePublishingEnabledSinceKey).Return("", sql.ErrNoRows).AnyTimes()

			licenseOpts := &coderdenttest.LicenseOptions{
				NotBefore: dbtime.Now().Add(-2 * time.Hour).Truncate(time.Second),
				GraceAt:   dbtime.Now().Add(-time.Hour).Truncate(time.Second),
				ExpiresAt: dbtime.Now().Add(24 * time.Hour).Truncate(time.Second),
				Addons:    []codersdk.Addon{codersdk.AddonAIGovernance},
				Features: license.Features{
					codersdk.FeatureAIGovernanceUserLimit: limit,
				},
			}

			lic := database.License{
				ID:  1,
				JWT: coderdenttest.GenerateLicense(t, *licenseOpts),
				Exp: licenseOpts.ExpiresAt,
			}

			mDB.EXPECT().
				GetUnexpiredLicenses(gomock.Any()).
				Return([]database.License{lic}, nil)
			mDB.EXPECT().
				GetActiveUserCount(gomock.Any(), false).
				Return(int64(1), nil)
			mDB.EXPECT().
				GetActiveAISeatCount(gomock.Any()).
				Return(activeSeatCount, nil)
			mDB.EXPECT().
				GetTemplatesWithFilter(gomock.Any(), gomock.Any()).
				Return([]database.Template{}, nil)

			enablements := map[codersdk.FeatureName]bool{
				codersdk.FeatureAIGovernanceUserLimit: true,
			}

			entitlements, err := license.Entitlements(context.Background(), testutil.Logger(t), mDB, 1, 0, coderdenttest.Keys, enablements, testAuthorizer, nil)
			require.NoError(t, err)
			require.True(t, entitlements.HasLicense)

			feature, ok := entitlements.Features[codersdk.FeatureAIGovernanceUserLimit]
			require.True(t, ok)
			require.Equal(t, codersdk.EntitlementGracePeriod, feature.Entitlement)

			expiryWarning := fmt.Sprintf(
				"Your deployment has %d active AI Governance seats but the license with the limit %d is expired.",
				activeSeatCount,
				limit,
			)
			require.Contains(t, entitlements.Warnings, expiryWarning)
			require.Contains(t, entitlements.Warnings,
				fmt.Sprintf(codersdk.LicenseAIGovernance90PercentWarningText, 95))
			for _, warning := range entitlements.Warnings {
				require.NotContains(t, warning, "over the limit")
			}
		})

		t.Run("NotEntitledSuppressed", func(t *testing.T) {
			t.Parallel()

			const activeSeatCount int64 = 42

			ctrl := gomock.NewController(t)
			mDB := dbmock.NewMockStore(ctrl)
			// Refreshes that observe publishing disabled read the enabled-since
			// marker to clear it if present.
			mDB.EXPECT().InTx(gomock.Any(), gomock.Any()).DoAndReturn(
				func(fn func(database.Store) error, _ *database.TxOptions) error {
					return fn(mDB)
				},
			).AnyTimes()
			mDB.EXPECT().TryAcquireLock(gomock.Any(), int64(database.LockIDUsagePublishingEnabledMarker)).Return(true, nil).AnyTimes()
			mDB.EXPECT().GetRuntimeConfig(gomock.Any(), license.UsagePublishingEnabledSinceKey).Return("", sql.ErrNoRows).AnyTimes()

			// Premium license without the AI Governance addon.
			licenseOpts := (&coderdenttest.LicenseOptions{
				FeatureSet: codersdk.FeatureSetPremium,
				NotBefore:  dbtime.Now().Add(-time.Hour).Truncate(time.Second),
				GraceAt:    dbtime.Now().Add(time.Hour * 24 * 60).Truncate(time.Second),
				ExpiresAt:  dbtime.Now().Add(time.Hour * 24 * 90).Truncate(time.Second),
			}).
				UserLimit(100)

			lic := database.License{
				ID:  1,
				JWT: coderdenttest.GenerateLicense(t, *licenseOpts),
				Exp: licenseOpts.ExpiresAt,
			}

			mDB.EXPECT().
				GetUnexpiredLicenses(gomock.Any()).
				Return([]database.License{lic}, nil)
			mDB.EXPECT().
				GetActiveUserCount(gomock.Any(), false).
				Return(int64(1), nil)
			mDB.EXPECT().
				GetActiveAISeatCount(gomock.Any()).
				Return(activeSeatCount, nil)
			mDB.EXPECT().
				GetTotalUsageDCManagedAgentsV1(gomock.Any(), gomock.Any()).
				Return(int64(0), nil)
			// The premium grandfather default grants a zero-hour agent
			// runtime allocation, so that usage is queried too. It is not
			// what this test is about.
			mDB.EXPECT().
				GetTotalUsageHBAgentRuntimeV1(gomock.Any(), gomock.Any()).
				Return(int64(0), nil)
			mDB.EXPECT().
				GetTemplatesWithFilter(gomock.Any(), gomock.Any()).
				Return([]database.Template{}, nil)

			entitlements, err := license.Entitlements(context.Background(), testutil.Logger(t), mDB, 1, 0, coderdenttest.Keys, all, testAuthorizer, nil)
			require.NoError(t, err)
			require.True(t, entitlements.HasLicense)

			// The not-entitled case should not produce errors about
			// AI Governance seat counts.
			for _, e := range entitlements.Errors {
				require.NotContains(t, e, "AI Governance seats")
			}
			for _, w := range entitlements.Warnings {
				require.NotContains(t, w, "AI Governance seats")
			}
		})
	})
}

func TestLicenseEntitlements(t *testing.T) {
	t.Parallel()

	// We must use actual 'time.Now()' in tests because the jwt library does
	// not accept a custom time function. The only way to change it is as a
	// package global, which does not work in t.Parallel().

	// This list comes from coderd.go on launch. This list is a bit arbitrary,
	// maybe some should be moved to "AlwaysEnabled" instead.
	defaultEnablements := map[codersdk.FeatureName]bool{
		codersdk.FeatureAuditLog:                   true,
		codersdk.FeatureConnectionLog:              true,
		codersdk.FeatureBrowserOnly:                true,
		codersdk.FeatureSCIM:                       true,
		codersdk.FeatureMultipleExternalAuth:       true,
		codersdk.FeatureTemplateRBAC:               true,
		codersdk.FeatureExternalTokenEncryption:    true,
		codersdk.FeatureExternalProvisionerDaemons: true,
		codersdk.FeatureAdvancedTemplateScheduling: true,
		codersdk.FeatureWorkspaceProxy:             true,
		codersdk.FeatureUserRoleManagement:         true,
		codersdk.FeatureAccessControl:              true,
		codersdk.FeatureControlSharedPorts:         true,
		codersdk.FeatureWorkspaceExternalAgent:     true,
		codersdk.FeatureAIBridge:                   true,
		codersdk.FeatureBoundary:                   true,
	}

	legacyLicense := func() *coderdenttest.LicenseOptions {
		return (&coderdenttest.LicenseOptions{
			AccountType: "salesforce",
			AccountID:   "Alice",
			Trial:       false,
			// Use the legacy boolean
			AllFeatures: true,
			Addons:      []codersdk.Addon{codersdk.AddonAIGovernance},
			Features: license.Features{
				codersdk.FeatureAIGovernanceUserLimit: 100,
			},
		}).Valid(time.Now())
	}

	enterpriseLicense := func() *coderdenttest.LicenseOptions {
		return (&coderdenttest.LicenseOptions{
			AccountType:   "salesforce",
			AccountID:     "Bob",
			DeploymentIDs: nil,
			Trial:         false,
			FeatureSet:    codersdk.FeatureSetEnterprise,
			AllFeatures:   true,
			Addons:        []codersdk.Addon{codersdk.AddonAIGovernance},
			Features: license.Features{
				codersdk.FeatureAIGovernanceUserLimit: 100,
			},
		}).Valid(time.Now())
	}

	// agentRuntimeHoursLicense builds an enterprise license carrying the
	// agent runtime hour claims. A nil softLimit omits the claim; any
	// non-nil value is minted verbatim so tests can construct zero or
	// nonsensical soft limits. A positive allocation also carries a hard
	// limit above the allocation (decodeAgentRuntimeHours ignores a lower
	// one).
	agentRuntimeHoursLicense := func(allocation int64, softLimit *int64) *coderdenttest.LicenseOptions {
		var hard *int64
		if allocation > 0 {
			hard = ptr.Ref(allocation + 20)
		}
		return enterpriseLicense().UserLimit(100).AgentRuntimeHours(allocation, softLimit, hard)
	}

	// hoursToMsFn reports whole hours of runtime as the milliseconds the usage
	// events actually record.
	hoursToMsFn := func(hours int64) license.AgentRuntimeMsFn {
		return func(_ context.Context, _, _ time.Time) (int64, error) {
			return (time.Duration(hours) * time.Hour).Milliseconds(), nil
		}
	}

	// Captured by AgentRuntimeHours/UsagePeriodBounds. Only that case
	// reads or writes these, so parallel siblings cannot race them.
	var agentRuntimeUsageQueryFrom, agentRuntimeUsageQueryTo time.Time
	var agentRuntimeUsageQueryCalled bool

	// grandfatherIssuedAt is the fixed UsagePeriod.IssuedAt carried by the
	// zero-hour agent runtime allocation that premium licenses without
	// agent runtime hour claims are grandfathered into; see license.go.
	grandfatherIssuedAt := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	// runtimeClaimIssuedAt mints claim-bearing licenses in the grandfather
	// precedence cases below, so the merged feature's UsagePeriod.IssuedAt
	// identifies which candidate won.
	runtimeClaimIssuedAt := dbtime.Now().Add(-2 * time.Hour).Truncate(time.Second)

	premiumLicense := func() *coderdenttest.LicenseOptions {
		return (&coderdenttest.LicenseOptions{
			AccountType:   "salesforce",
			AccountID:     "Charlie",
			DeploymentIDs: nil,
			Trial:         false,
			FeatureSet:    codersdk.FeatureSetPremium,
			AllFeatures:   true,
			Addons:        []codersdk.Addon{codersdk.AddonAIGovernance},
			Features: license.Features{
				codersdk.FeatureAIGovernanceUserLimit: 100,
			},
		}).Valid(time.Now())
	}

	testCases := []struct {
		Name        string
		Licenses    []*coderdenttest.LicenseOptions
		Enablements map[codersdk.FeatureName]bool
		Arguments   license.FeatureArguments
		// KeepNilAgentRuntimeMsFn skips the default AgentRuntimeMsFn
		// injection below so the nil dev-error path can be exercised.
		KeepNilAgentRuntimeMsFn bool
		// CancelContext cancels the context passed to LicensesEntitlements
		// before the call, exercising the usage-measurement abort policy.
		CancelContext bool

		ExpectedErrorContains string
		AssertEntitlements    func(t *testing.T, entitlements codersdk.Entitlements)
	}{
		{
			Name: "NoLicenses",
			AssertEntitlements: func(t *testing.T, entitlements codersdk.Entitlements) {
				assertNoErrors(t, entitlements)
				assertNoWarnings(t, entitlements)
				assert.False(t, entitlements.HasLicense)
				assert.False(t, entitlements.Trial)
			},
		},
		{
			Name: "MixedUsedCounts",
			Licenses: []*coderdenttest.LicenseOptions{
				legacyLicense().UserLimit(100),
				enterpriseLicense().UserLimit(500),
			},
			Enablements: defaultEnablements,
			Arguments: license.FeatureArguments{
				ActiveUserCount:   50,
				ReplicaCount:      0,
				ExternalAuthCount: 0,
			},
			AssertEntitlements: func(t *testing.T, entitlements codersdk.Entitlements) {
				assertEnterpriseFeatures(t, entitlements)
				assertNoErrors(t, entitlements)
				assertNoWarnings(t, entitlements)
				userFeature := entitlements.Features[codersdk.FeatureUserLimit]
				assert.Equalf(t, int64(500), *userFeature.Limit, "user limit")
				assert.Equalf(t, int64(50), *userFeature.Actual, "user count")
			},
		},
		{
			Name: "MixedUsedCountsWithExpired",
			Licenses: []*coderdenttest.LicenseOptions{
				// This license is ignored
				enterpriseLicense().UserLimit(500).Expired(time.Now()),
				enterpriseLicense().UserLimit(100),
			},
			Enablements: defaultEnablements,
			Arguments: license.FeatureArguments{
				ActiveUserCount:   200,
				ReplicaCount:      0,
				ExternalAuthCount: 0,
			},
			AssertEntitlements: func(t *testing.T, entitlements codersdk.Entitlements) {
				assertEnterpriseFeatures(t, entitlements)
				userFeature := entitlements.Features[codersdk.FeatureUserLimit]
				assert.Equalf(t, int64(100), *userFeature.Limit, "user limit")
				assert.Equalf(t, int64(200), *userFeature.Actual, "user count")

				require.Len(t, entitlements.Errors, 1, "invalid license error")
				require.Len(t, entitlements.Warnings, 1, "user count exceeds warning")
				require.Contains(t, entitlements.Errors[0], "Invalid license")
				require.Contains(t, entitlements.Warnings[0], "active users but is only licensed for")
			},
		},
		{
			// The new license does not have enough seats to cover the active user count.
			// The old license is in it's grace period.
			Name: "MixedUsedCountsWithGrace",
			Licenses: []*coderdenttest.LicenseOptions{
				enterpriseLicense().UserLimit(500).GracePeriod(time.Now()),
				enterpriseLicense().UserLimit(100),
			},
			Enablements: defaultEnablements,
			Arguments: license.FeatureArguments{
				ActiveUserCount:   200,
				ReplicaCount:      0,
				ExternalAuthCount: 0,
			},
			AssertEntitlements: func(t *testing.T, entitlements codersdk.Entitlements) {
				userFeature := entitlements.Features[codersdk.FeatureUserLimit]
				assert.Equalf(t, int64(500), *userFeature.Limit, "user limit")
				assert.Equalf(t, int64(200), *userFeature.Actual, "user count")
				assert.Equal(t, userFeature.Entitlement, codersdk.EntitlementGracePeriod)
			},
		},
		{
			// Legacy license uses the "AllFeatures" boolean
			Name: "LegacyLicense",
			Licenses: []*coderdenttest.LicenseOptions{
				legacyLicense().UserLimit(100),
			},
			Enablements: defaultEnablements,
			Arguments: license.FeatureArguments{
				ActiveUserCount:   50,
				ReplicaCount:      0,
				ExternalAuthCount: 0,
			},
			AssertEntitlements: func(t *testing.T, entitlements codersdk.Entitlements) {
				assertEnterpriseFeatures(t, entitlements)
				assertNoErrors(t, entitlements)
				assertNoWarnings(t, entitlements)
				userFeature := entitlements.Features[codersdk.FeatureUserLimit]
				assert.Equalf(t, int64(100), *userFeature.Limit, "user limit")
				assert.Equalf(t, int64(50), *userFeature.Actual, "user count")
			},
		},
		{
			Name: "EnterpriseDisabledMultiOrg",
			Licenses: []*coderdenttest.LicenseOptions{
				enterpriseLicense().UserLimit(100),
			},
			Enablements:           defaultEnablements,
			Arguments:             license.FeatureArguments{},
			ExpectedErrorContains: "",
			AssertEntitlements: func(t *testing.T, entitlements codersdk.Entitlements) {
				assert.False(t, entitlements.Features[codersdk.FeatureMultipleOrganizations].Enabled, "multi-org only enabled for premium")
				assert.False(t, entitlements.Features[codersdk.FeatureCustomRoles].Enabled, "custom-roles only enabled for premium")
			},
		},
		{
			Name: "PremiumEnabledMultiOrg",
			Licenses: []*coderdenttest.LicenseOptions{
				premiumLicense().UserLimit(100),
			},
			Enablements:           defaultEnablements,
			Arguments:             license.FeatureArguments{},
			ExpectedErrorContains: "",
			AssertEntitlements: func(t *testing.T, entitlements codersdk.Entitlements) {
				assert.True(t, entitlements.Features[codersdk.FeatureMultipleOrganizations].Enabled, "multi-org enabled for premium")
				assert.True(t, entitlements.Features[codersdk.FeatureCustomRoles].Enabled, "custom-roles enabled for premium")
			},
		},
		{
			Name: "CurrentAndFuture",
			Licenses: []*coderdenttest.LicenseOptions{
				enterpriseLicense().UserLimit(100),
				premiumLicense().UserLimit(200).FutureTerm(time.Now()),
			},
			Enablements: defaultEnablements,
			AssertEntitlements: func(t *testing.T, entitlements codersdk.Entitlements) {
				assertEnterpriseFeatures(t, entitlements)
				assertNoErrors(t, entitlements)
				assertNoWarnings(t, entitlements)
				userFeature := entitlements.Features[codersdk.FeatureUserLimit]
				assert.Equalf(t, int64(100), *userFeature.Limit, "user limit")
				assert.Equal(t, codersdk.EntitlementNotEntitled,
					entitlements.Features[codersdk.FeatureMultipleOrganizations].Entitlement)
				assert.Equal(t, codersdk.EntitlementNotEntitled,
					entitlements.Features[codersdk.FeatureCustomRoles].Entitlement)
			},
		},
		{
			Name: "ManagedAgentLimit",
			Licenses: []*coderdenttest.LicenseOptions{
				enterpriseLicense().UserLimit(100).ManagedAgentLimit(100),
			},
			Arguments: license.FeatureArguments{
				ManagedAgentCountFn: func(ctx context.Context, from time.Time, to time.Time) (int64, error) {
					// 74 is below the limit (soft=100), so no warning.
					return 74, nil
				},
			},
			AssertEntitlements: func(t *testing.T, entitlements codersdk.Entitlements) {
				assertNoErrors(t, entitlements)
				assertNoWarnings(t, entitlements)
				feature := entitlements.Features[codersdk.FeatureManagedAgentLimit]
				assert.Equal(t, codersdk.EntitlementEntitled, feature.Entitlement)
				assert.True(t, feature.Enabled)
				// Soft limit value is used as the single Limit.
				assert.Equal(t, int64(100), *feature.Limit)
				assert.Equal(t, int64(74), *feature.Actual)
			},
		},
		{
			Name: "ManagedAgentLimitWithGrace",
			Licenses: []*coderdenttest.LicenseOptions{
				// Add another license that is not entitled to managed agents to
				// suppress warnings for other features.
				enterpriseLicense().
					UserLimit(100).
					WithIssuedAt(time.Now().Add(-time.Hour * 2)),
				enterpriseLicense().
					UserLimit(100).
					ManagedAgentLimit(100).
					WithIssuedAt(time.Now().Add(-time.Hour * 1)).
					GracePeriod(time.Now()),
			},
			Arguments: license.FeatureArguments{
				ManagedAgentCountFn: func(ctx context.Context, from time.Time, to time.Time) (int64, error) {
					// When the soft and hard limit are equal, the warning is
					// triggered at 75% of the hard limit.
					return 74, nil
				},
			},
			AssertEntitlements: func(t *testing.T, entitlements codersdk.Entitlements) {
				assertNoErrors(t, entitlements)
				assertNoWarnings(t, entitlements)
				feature := entitlements.Features[codersdk.FeatureManagedAgentLimit]
				assert.Equal(t, codersdk.EntitlementGracePeriod, feature.Entitlement)
				assert.True(t, feature.Enabled)
				assert.Equal(t, int64(100), *feature.Limit)
				assert.Equal(t, int64(74), *feature.Actual)
			},
		},
		{
			Name: "ManagedAgentLimitWithExpired",
			Licenses: []*coderdenttest.LicenseOptions{
				// Add another license that is not entitled to managed agents to
				// suppress warnings for other features.
				enterpriseLicense().
					UserLimit(100).
					WithIssuedAt(time.Now().Add(-time.Hour * 2)),
				enterpriseLicense().
					UserLimit(100).
					ManagedAgentLimit(100).
					WithIssuedAt(time.Now().Add(-time.Hour * 1)).
					Expired(time.Now()),
			},
			Arguments: license.FeatureArguments{
				ManagedAgentCountFn: func(ctx context.Context, from time.Time, to time.Time) (int64, error) {
					return 10, nil
				},
			},
			AssertEntitlements: func(t *testing.T, entitlements codersdk.Entitlements) {
				feature := entitlements.Features[codersdk.FeatureManagedAgentLimit]
				assert.Equal(t, codersdk.EntitlementNotEntitled, feature.Entitlement)
				assert.False(t, feature.Enabled)
				assert.Nil(t, feature.Limit)
				assert.Nil(t, feature.Actual)
			},
		},
		{
			Name: "ManagedAgentLimitWarning/ExceededLimit",
			Licenses: []*coderdenttest.LicenseOptions{
				enterpriseLicense().
					UserLimit(100).
					ManagedAgentLimit(100),
			},
			Arguments: license.FeatureArguments{
				ManagedAgentCountFn: func(ctx context.Context, from time.Time, to time.Time) (int64, error) {
					return 150, nil
				},
			},
			AssertEntitlements: func(t *testing.T, entitlements codersdk.Entitlements) {
				assert.Len(t, entitlements.Warnings, 1)
				assert.Equal(t, codersdk.LicenseManagedAgentLimitExceededWarningText, entitlements.Warnings[0])
				assertNoErrors(t, entitlements)

				feature := entitlements.Features[codersdk.FeatureManagedAgentLimit]
				assert.Equal(t, codersdk.EntitlementEntitled, feature.Entitlement)
				assert.True(t, feature.Enabled)
				// Soft limit (100) is used as the single Limit.
				assert.Equal(t, int64(100), *feature.Limit)
				assert.Equal(t, int64(150), *feature.Actual)
			},
		},
		{
			// hoursToMsFn discards the period bounds, so a swapped or wrong
			// license period would still pass the other cases. Capture the
			// arguments here and require they match the feature's UsagePeriod.
			Name: "AgentRuntimeHours/UsagePeriodBounds",
			Licenses: []*coderdenttest.LicenseOptions{
				agentRuntimeHoursLicense(100, ptr.Ref[int64](80)),
			},
			Arguments: license.FeatureArguments{
				AgentRuntimeMsFn: func(_ context.Context, from, to time.Time) (int64, error) {
					agentRuntimeUsageQueryFrom = from
					agentRuntimeUsageQueryTo = to
					agentRuntimeUsageQueryCalled = true
					return 0, nil
				},
			},
			AssertEntitlements: func(t *testing.T, entitlements codersdk.Entitlements) {
				assertNoErrors(t, entitlements)
				require.True(t, agentRuntimeUsageQueryCalled)
				feature := entitlements.Features[codersdk.FeatureAgentRuntimeHours]
				require.NotNil(t, feature.UsagePeriod)
				assert.Equal(t, feature.UsagePeriod.Start, agentRuntimeUsageQueryFrom)
				assert.Equal(t, feature.UsagePeriod.End, agentRuntimeUsageQueryTo)
			},
		},
		{
			// The soft warning end to end: the remaining threshold
			// arithmetic is pinned by TestAppendAgentRuntimeHoursWarning.
			Name: "AgentRuntimeHours/AtSoftLimit",
			Licenses: []*coderdenttest.LicenseOptions{
				agentRuntimeHoursLicense(100, ptr.Ref[int64](80)),
			},
			Arguments: license.FeatureArguments{
				AgentRuntimeMsFn: hoursToMsFn(80),
			},
			AssertEntitlements: func(t *testing.T, entitlements codersdk.Entitlements) {
				assertNoErrors(t, entitlements)
				require.Len(t, entitlements.Warnings, 1)
				assert.Equal(t, fmt.Sprintf(codersdk.LicenseAgentRuntimeHoursSoftLimitWarningText, 80, 100, 80),
					entitlements.Warnings[0])
				feature := entitlements.Features[codersdk.FeatureAgentRuntimeHours]
				require.NotNil(t, feature.Actual)
				assert.Equal(t, int64(80), *feature.Actual)
				require.NotNil(t, feature.ActualMs)
				assert.Equal(t, (80 * time.Hour).Milliseconds(), *feature.ActualMs)
			},
		},
		{
			// At the allocation the soft warning is suppressed, so exactly one
			// warning is emitted rather than both.
			Name: "AgentRuntimeHours/AtAllocation",
			Licenses: []*coderdenttest.LicenseOptions{
				agentRuntimeHoursLicense(100, ptr.Ref[int64](80)),
			},
			Arguments: license.FeatureArguments{
				AgentRuntimeMsFn: hoursToMsFn(100),
			},
			AssertEntitlements: func(t *testing.T, entitlements codersdk.Entitlements) {
				assertNoErrors(t, entitlements)
				require.Len(t, entitlements.Warnings, 1)
				assert.Equal(t, fmt.Sprintf(codersdk.LicenseAgentRuntimeHoursAllocationReachedWarningText, 100, 100),
					entitlements.Warnings[0])
				assert.NotContains(t, entitlements.Warnings,
					fmt.Sprintf(codersdk.LicenseAgentRuntimeHoursSoftLimitWarningText, 100, 100, 80))
			},
		},
		{
			// A zero allocation carries no hour budget, so Enabled reports
			// false and the hour thresholds never warn, but Actual is still
			// reported. See decodeAgentRuntimeHours.
			Name: "AgentRuntimeHours/ZeroAllocation",
			Licenses: []*coderdenttest.LicenseOptions{
				agentRuntimeHoursLicense(0, nil),
			},
			Arguments: license.FeatureArguments{
				AgentRuntimeMsFn: hoursToMsFn(50),
			},
			AssertEntitlements: func(t *testing.T, entitlements codersdk.Entitlements) {
				assertNoErrors(t, entitlements)
				assertNoWarnings(t, entitlements)
				feature := entitlements.Features[codersdk.FeatureAgentRuntimeHours]
				assert.False(t, feature.Enabled)
				require.NotNil(t, feature.Limit)
				assert.Equal(t, int64(0), *feature.Limit)
				require.NotNil(t, feature.Actual)
				assert.Equal(t, int64(50), *feature.Actual)
			},
		},
		{
			// Partial hours are floored, so 99h59m59s does not reach the
			// 100 hour allocation. ActualMs still carries the exact
			// milliseconds so clients can render the fraction.
			Name: "AgentRuntimeHours/PartialHourFloored",
			Licenses: []*coderdenttest.LicenseOptions{
				agentRuntimeHoursLicense(100, ptr.Ref[int64](80)),
			},
			Arguments: license.FeatureArguments{
				AgentRuntimeMsFn: func(_ context.Context, _, _ time.Time) (int64, error) {
					return (100 * time.Hour).Milliseconds() - 1, nil
				},
			},
			AssertEntitlements: func(t *testing.T, entitlements codersdk.Entitlements) {
				assertNoErrors(t, entitlements)
				require.Len(t, entitlements.Warnings, 1)
				assert.Equal(t, fmt.Sprintf(codersdk.LicenseAgentRuntimeHoursSoftLimitWarningText, 99, 100, 80),
					entitlements.Warnings[0])
				feature := entitlements.Features[codersdk.FeatureAgentRuntimeHours]
				require.NotNil(t, feature.Actual)
				assert.Equal(t, int64(99), *feature.Actual)
				require.NotNil(t, feature.ActualMs)
				assert.Equal(t, (100*time.Hour).Milliseconds()-1, *feature.ActualMs)
			},
		},
		{
			// A fractional-hour runtime: Actual floors to whole hours
			// while ActualMs preserves the fraction (10.3 hours here).
			Name: "AgentRuntimeHours/FractionalHours",
			Licenses: []*coderdenttest.LicenseOptions{
				agentRuntimeHoursLicense(100, ptr.Ref[int64](80)),
			},
			Arguments: license.FeatureArguments{
				AgentRuntimeMsFn: func(_ context.Context, _, _ time.Time) (int64, error) {
					return (10*time.Hour + 18*time.Minute).Milliseconds(), nil
				},
			},
			AssertEntitlements: func(t *testing.T, entitlements codersdk.Entitlements) {
				assertNoErrors(t, entitlements)
				assertNoWarnings(t, entitlements)
				feature := entitlements.Features[codersdk.FeatureAgentRuntimeHours]
				require.NotNil(t, feature.Actual)
				assert.Equal(t, int64(10), *feature.Actual)
				require.NotNil(t, feature.ActualMs)
				assert.Equal(t, int64(37_080_000), *feature.ActualMs)
			},
		},
		{
			// Negative runtime is not producible by the production query,
			// but AgentRuntimeMsFn is a caller-supplied seam, so both
			// Actual and ActualMs clamp to 0.
			Name: "AgentRuntimeHours/NegativeRuntimeClamped",
			Licenses: []*coderdenttest.LicenseOptions{
				agentRuntimeHoursLicense(100, ptr.Ref[int64](80)),
			},
			Arguments: license.FeatureArguments{
				AgentRuntimeMsFn: func(_ context.Context, _, _ time.Time) (int64, error) {
					return -1, nil
				},
			},
			AssertEntitlements: func(t *testing.T, entitlements codersdk.Entitlements) {
				assertNoErrors(t, entitlements)
				assertNoWarnings(t, entitlements)
				feature := entitlements.Features[codersdk.FeatureAgentRuntimeHours]
				require.NotNil(t, feature.Actual)
				assert.Equal(t, int64(0), *feature.Actual)
				require.NotNil(t, feature.ActualMs)
				assert.Equal(t, int64(0), *feature.ActualMs)
			},
		},
		{
			// An enterprise license without the allocation claim does not
			// grant the feature, so usage is never queried and nothing
			// warns. Only premium licenses are grandfathered into a
			// zero-hour allocation.
			Name: "AgentRuntimeHours/NoClaimNoFeature",
			Licenses: []*coderdenttest.LicenseOptions{
				enterpriseLicense().UserLimit(100),
			},
			Arguments: license.FeatureArguments{
				AgentRuntimeMsFn: func(_ context.Context, _, _ time.Time) (int64, error) {
					// Poison value: if the runtime block ever ran without the
					// allocation claim, Actual would be set and the Nil
					// assertion below would fail on the subtest's t.
					return (9999 * time.Hour).Milliseconds(), nil
				},
			},
			AssertEntitlements: func(t *testing.T, entitlements codersdk.Entitlements) {
				assertNoErrors(t, entitlements)
				assertNoWarnings(t, entitlements)
				feature := entitlements.Features[codersdk.FeatureAgentRuntimeHours]
				assert.Nil(t, feature.Actual)
				assert.Nil(t, feature.UsagePeriod)
			},
		},
		{
			// A query failure is surfaced as a stable text in Errors and
			// leaves Actual unset without aborting the rest of the
			// entitlements.
			Name: "AgentRuntimeHours/QueryError",
			Licenses: []*coderdenttest.LicenseOptions{
				agentRuntimeHoursLicense(100, ptr.Ref[int64](80)),
			},
			Arguments: license.FeatureArguments{
				AgentRuntimeMsFn: func(_ context.Context, _, _ time.Time) (int64, error) {
					return 0, xerrors.New("kaboom")
				},
			},
			AssertEntitlements: func(t *testing.T, entitlements codersdk.Entitlements) {
				assertNoWarnings(t, entitlements)
				require.Len(t, entitlements.Errors, 1)
				assert.Equal(t, codersdk.LicenseAgentRuntimeUsageUnavailableErrorText, entitlements.Errors[0])
				// The raw error is logged rather than exposed on the
				// unauthenticated entitlements payload.
				assert.NotContains(t, entitlements.Errors[0], "kaboom")
				feature := entitlements.Features[codersdk.FeatureAgentRuntimeHours]
				assert.Nil(t, feature.Actual)
				// The rest of the entitlements are still computed.
				require.NotNil(t, feature.Limit)
				assert.Equal(t, int64(100), *feature.Limit)
			},
		},
		{
			// Forgetting to wire AgentRuntimeMsFn is a dev error: production
			// always provides both closures, so it fails the whole call
			// loudly instead of degrading into an operator-facing message.
			Name: "AgentRuntimeHours/NilRuntimeFnDevError",
			Licenses: []*coderdenttest.LicenseOptions{
				agentRuntimeHoursLicense(100, ptr.Ref[int64](80)),
			},
			KeepNilAgentRuntimeMsFn: true,
			ExpectedErrorContains:   "developer error: no closure provided to measure agent runtime usage",
		},
		{
			// A failure while the computation's own context is canceled
			// aborts the whole call rather than degrading to an
			// entitlements error.
			Name: "AgentRuntimeHours/ContextCanceled",
			Licenses: []*coderdenttest.LicenseOptions{
				agentRuntimeHoursLicense(100, ptr.Ref[int64](80)),
			},
			CancelContext: true,
			Arguments: license.FeatureArguments{
				AgentRuntimeMsFn: func(_ context.Context, _, _ time.Time) (int64, error) {
					return 0, context.Canceled
				},
			},
			ExpectedErrorContains: "get agent runtime",
		},
		{
			// Postgres raises the same SQLSTATE 57014 for statement_timeout
			// kills. With a live context that is a query failure, not a
			// shutdown: it must degrade into the stable diagnostic instead
			// of aborting every refresh (and coderd startup) on deployments
			// with an aggressive statement_timeout.
			Name: "AgentRuntimeHours/StatementTimeout",
			Licenses: []*coderdenttest.LicenseOptions{
				agentRuntimeHoursLicense(100, ptr.Ref[int64](80)),
			},
			Arguments: license.FeatureArguments{
				AgentRuntimeMsFn: func(_ context.Context, _, _ time.Time) (int64, error) {
					return 0, xerrors.Errorf("query: %w", &pq.Error{Code: "57014", Message: "canceling statement due to statement timeout"})
				},
			},
			AssertEntitlements: func(t *testing.T, entitlements codersdk.Entitlements) {
				assertNoWarnings(t, entitlements)
				require.Len(t, entitlements.Errors, 1)
				assert.Equal(t, codersdk.LicenseAgentRuntimeUsageUnavailableErrorText, entitlements.Errors[0])
				feature := entitlements.Features[codersdk.FeatureAgentRuntimeHours]
				assert.Nil(t, feature.Actual)
			},
		},
		{
			// A grace-period license still reports Actual and still warns at
			// its thresholds.
			Name: "AgentRuntimeHours/GracePeriod",
			Licenses: []*coderdenttest.LicenseOptions{
				agentRuntimeHoursLicense(100, ptr.Ref[int64](80)).GracePeriod(time.Now()),
			},
			Arguments: license.FeatureArguments{
				AgentRuntimeMsFn: hoursToMsFn(100),
			},
			AssertEntitlements: func(t *testing.T, entitlements codersdk.Entitlements) {
				assertNoErrors(t, entitlements)
				feature := entitlements.Features[codersdk.FeatureAgentRuntimeHours]
				assert.Equal(t, codersdk.EntitlementGracePeriod, feature.Entitlement)
				require.NotNil(t, feature.Actual)
				assert.Equal(t, int64(100), *feature.Actual)
				assert.Contains(t, entitlements.Warnings,
					fmt.Sprintf(codersdk.LicenseAgentRuntimeHoursAllocationReachedWarningText, 100, 100))
			},
		},
		{
			// A premium license without agent runtime hour claims is
			// grandfathered into a zero-hour allocation: granted disabled
			// with a zero limit over the license term, usage still
			// measured, and no deployment-wide warning even with nonzero
			// usage.
			Name: "AgentRuntimeHours/PremiumGrandfathered",
			Licenses: []*coderdenttest.LicenseOptions{
				premiumLicense().UserLimit(100),
			},
			Arguments: license.FeatureArguments{
				AgentRuntimeMsFn: hoursToMsFn(50),
			},
			AssertEntitlements: func(t *testing.T, entitlements codersdk.Entitlements) {
				assertNoErrors(t, entitlements)
				assertNoWarnings(t, entitlements)
				feature := entitlements.Features[codersdk.FeatureAgentRuntimeHours]
				assert.False(t, feature.Enabled)
				assert.Equal(t, codersdk.EntitlementEntitled, feature.Entitlement)
				require.NotNil(t, feature.Limit)
				assert.Equal(t, int64(0), *feature.Limit)
				assert.Nil(t, feature.SoftLimit)
				assert.Nil(t, feature.HardLimit)
				require.NotNil(t, feature.UsagePeriod)
				assert.Equal(t, grandfatherIssuedAt, feature.UsagePeriod.IssuedAt)
				// The usage period is the license term (premiumLicense is
				// valid from roughly now until 60 days out), not the
				// managed-agent default's fixed 100-year window.
				assert.WithinDuration(t, time.Now(), feature.UsagePeriod.Start, 5*time.Minute)
				assert.WithinDuration(t, time.Now().Add(60*24*time.Hour), feature.UsagePeriod.End, 5*time.Minute)
				require.NotNil(t, feature.Actual)
				assert.Equal(t, int64(50), *feature.Actual)
			},
		},
		{
			// A grace-period premium license grandfathers the same
			// zero-hour allocation with a grace entitlement.
			Name: "AgentRuntimeHours/PremiumGrandfatheredGracePeriod",
			Licenses: []*coderdenttest.LicenseOptions{
				premiumLicense().UserLimit(100).GracePeriod(time.Now()),
			},
			Arguments: license.FeatureArguments{
				AgentRuntimeMsFn: hoursToMsFn(50),
			},
			AssertEntitlements: func(t *testing.T, entitlements codersdk.Entitlements) {
				assertNoErrors(t, entitlements)
				feature := entitlements.Features[codersdk.FeatureAgentRuntimeHours]
				assert.False(t, feature.Enabled)
				assert.Equal(t, codersdk.EntitlementGracePeriod, feature.Entitlement)
				require.NotNil(t, feature.Limit)
				assert.Equal(t, int64(0), *feature.Limit)
				require.NotNil(t, feature.Actual)
				assert.Equal(t, int64(50), *feature.Actual)
			},
		},
		{
			// The grandfathered default carries a fixed early
			// UsagePeriod.IssuedAt, so a license actually carrying the
			// allocation claim wins the merge even when the claim-less
			// premium license is issued later.
			Name: "AgentRuntimeHours/GrandfatherLosesToAllocation",
			Licenses: []*coderdenttest.LicenseOptions{
				premiumLicense().UserLimit(100).WithIssuedAt(dbtime.Now().Add(-time.Hour)),
				agentRuntimeHoursLicense(20000, nil).WithIssuedAt(runtimeClaimIssuedAt),
			},
			Arguments: license.FeatureArguments{
				AgentRuntimeMsFn: hoursToMsFn(50),
			},
			AssertEntitlements: func(t *testing.T, entitlements codersdk.Entitlements) {
				assertNoErrors(t, entitlements)
				assertNoWarnings(t, entitlements)
				feature := entitlements.Features[codersdk.FeatureAgentRuntimeHours]
				assert.True(t, feature.Enabled)
				require.NotNil(t, feature.Limit)
				assert.Equal(t, int64(20000), *feature.Limit)
				require.NotNil(t, feature.UsagePeriod)
				assert.WithinDuration(t, runtimeClaimIssuedAt, feature.UsagePeriod.IssuedAt, time.Second)
			},
		},
		{
			// An unlimited allocation on any license outranks the
			// grandfathered zero-hour default.
			Name: "AgentRuntimeHours/GrandfatherLosesToUnlimited",
			Licenses: []*coderdenttest.LicenseOptions{
				premiumLicense().UserLimit(100),
				enterpriseLicense().UserLimit(100).AgentRuntimeHours(license.AgentRuntimeHoursUnlimitedAllocation, nil, nil),
			},
			Arguments: license.FeatureArguments{
				AgentRuntimeMsFn: hoursToMsFn(1_000_000),
			},
			AssertEntitlements: func(t *testing.T, entitlements codersdk.Entitlements) {
				assertNoErrors(t, entitlements)
				assertNoWarnings(t, entitlements)
				feature := entitlements.Features[codersdk.FeatureAgentRuntimeHours]
				assert.True(t, feature.Enabled)
				assert.Nil(t, feature.Limit)
				require.NotNil(t, feature.Actual)
				assert.Equal(t, int64(1_000_000), *feature.Actual)
			},
		},
		{
			// An explicit zero allocation and the grandfathered default
			// have identical semantics; the explicit claim's later issue
			// time wins the merge, which pins the Compare path.
			Name: "AgentRuntimeHours/GrandfatherLosesToExplicitZero",
			Licenses: []*coderdenttest.LicenseOptions{
				premiumLicense().UserLimit(100),
				agentRuntimeHoursLicense(0, nil).WithIssuedAt(runtimeClaimIssuedAt),
			},
			Arguments: license.FeatureArguments{
				AgentRuntimeMsFn: hoursToMsFn(50),
			},
			AssertEntitlements: func(t *testing.T, entitlements codersdk.Entitlements) {
				assertNoErrors(t, entitlements)
				assertNoWarnings(t, entitlements)
				feature := entitlements.Features[codersdk.FeatureAgentRuntimeHours]
				assert.False(t, feature.Enabled)
				require.NotNil(t, feature.Limit)
				assert.Equal(t, int64(0), *feature.Limit)
				require.NotNil(t, feature.UsagePeriod)
				assert.WithinDuration(t, runtimeClaimIssuedAt, feature.UsagePeriod.IssuedAt, time.Second)
				require.NotNil(t, feature.Actual)
				assert.Equal(t, int64(50), *feature.Actual)
			},
		},
		{
			Name: "ExternalTemplate",
			Licenses: []*coderdenttest.LicenseOptions{
				enterpriseLicense().UserLimit(100),
			},
			Arguments: license.FeatureArguments{
				ExternalTemplateCount: 1,
			},
			AssertEntitlements: func(t *testing.T, entitlements codersdk.Entitlements) {
				assert.Equal(t, codersdk.EntitlementEntitled, entitlements.Features[codersdk.FeatureWorkspaceExternalAgent].Entitlement)
				assert.True(t, entitlements.Features[codersdk.FeatureWorkspaceExternalAgent].Enabled)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			t.Parallel()

			generatedLicenses := make([]database.License, 0, len(tc.Licenses))
			for i, lo := range tc.Licenses {
				generatedLicenses = append(generatedLicenses, database.License{
					ID:         int32(i), // nolint:gosec
					UploadedAt: time.Now().Add(time.Hour * -1),
					JWT:        lo.Generate(t),
					Exp:        lo.GraceAt,
					UUID:       uuid.New(),
				})
			}

			// Default to 0 managed agent count.
			if tc.Arguments.ManagedAgentCountFn == nil {
				tc.Arguments.ManagedAgentCountFn = func(ctx context.Context, from time.Time, to time.Time) (int64, error) {
					return 0, nil
				}
			}
			// Default to 0 agent runtime.
			if tc.Arguments.AgentRuntimeMsFn == nil && !tc.KeepNilAgentRuntimeMsFn {
				tc.Arguments.AgentRuntimeMsFn = func(ctx context.Context, from time.Time, to time.Time) (int64, error) {
					return 0, nil
				}
			}

			ctx := context.Background()
			if tc.CancelContext {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			entitlements, err := license.LicensesEntitlements(ctx, time.Now(), generatedLicenses, tc.Enablements, coderdenttest.Keys, tc.Arguments)
			if tc.ExpectedErrorContains != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tc.ExpectedErrorContains)
			} else {
				require.NoError(t, err)
				tc.AssertEntitlements(t, entitlements)
			}
		})
	}
}

func TestAIBridgeSoftWarning(t *testing.T) {
	t.Parallel()

	aiBridgeEnabledEnablements := map[codersdk.FeatureName]bool{
		codersdk.FeatureAIBridge: true,
	}

	aiBridgeDisabledEnablements := map[codersdk.FeatureName]bool{
		codersdk.FeatureAIBridge: false,
	}

	aiBridgeWarningMessage := "The AI Governance add-on is required to use AI Gateway. Please reach out to your account team or sales@coder.com to learn more."

	// A Premium license grants a managed agent limit and a grandfathered
	// agent runtime allocation by default: a nil AgentRuntimeMsFn is a hard
	// developer error and a nil ManagedAgentCountFn degrades into an
	// entitlements error, so these subtests wire zero-usage closures.
	zeroUsageArgs := license.FeatureArguments{
		ManagedAgentCountFn: func(_ context.Context, _, _ time.Time) (int64, error) {
			return 0, nil
		},
		AgentRuntimeMsFn: func(_ context.Context, _, _ time.Time) (int64, error) {
			return 0, nil
		},
	}

	t.Run("NoAddon_AIBridgeOff", func(t *testing.T) {
		t.Parallel()
		// License without addon and AI Bridge disabled should NOT show warning.
		lo := (&coderdenttest.LicenseOptions{
			AccountType: "salesforce",
			AccountID:   "test",
			FeatureSet:  codersdk.FeatureSetPremium,
		}).Valid(time.Now())

		generatedLicenses := []database.License{
			{
				ID:         1,
				UploadedAt: time.Now().Add(time.Hour * -1),
				JWT:        lo.Generate(t),
				Exp:        lo.GraceAt,
				UUID:       uuid.New(),
			},
		}

		entitlements, err := license.LicensesEntitlements(context.Background(), time.Now(), generatedLicenses, aiBridgeDisabledEnablements, coderdenttest.Keys, zeroUsageArgs)
		require.NoError(t, err)

		aiBridgeFeature := entitlements.Features[codersdk.FeatureAIBridge]
		assert.False(t, aiBridgeFeature.Enabled)
		require.NotContains(t, entitlements.Warnings, aiBridgeWarningMessage)
	})

	t.Run("NoAddon_AIBridgeOn", func(t *testing.T) {
		t.Parallel()
		// License without addon and AI Bridge enabled SHOULD show warning.
		lo := (&coderdenttest.LicenseOptions{
			AccountType: "salesforce",
			AccountID:   "test",
			FeatureSet:  codersdk.FeatureSetPremium,
		}).Valid(time.Now())

		generatedLicenses := []database.License{
			{
				ID:         1,
				UploadedAt: time.Now().Add(time.Hour * -1),
				JWT:        lo.Generate(t),
				Exp:        lo.GraceAt,
				UUID:       uuid.New(),
			},
		}

		entitlements, err := license.LicensesEntitlements(context.Background(), time.Now(), generatedLicenses, aiBridgeEnabledEnablements, coderdenttest.Keys, zeroUsageArgs)
		require.NoError(t, err)

		aiBridgeFeature := entitlements.Features[codersdk.FeatureAIBridge]
		assert.True(t, aiBridgeFeature.Enabled)
		assert.Equal(t, codersdk.EntitlementEntitled, aiBridgeFeature.Entitlement)
		require.Contains(t, entitlements.Warnings, aiBridgeWarningMessage)
	})

	t.Run("Addon_AIBridgeOff", func(t *testing.T) {
		t.Parallel()
		// License with addon and AI Bridge disabled should NOT show warning.
		lo := (&coderdenttest.LicenseOptions{
			AccountType: "salesforce",
			AccountID:   "test",
			FeatureSet:  codersdk.FeatureSetPremium,
			Addons:      []codersdk.Addon{codersdk.AddonAIGovernance},
			Features: license.Features{
				codersdk.FeatureAIGovernanceUserLimit: 100,
			},
		}).Valid(time.Now())

		generatedLicenses := []database.License{
			{
				ID:         1,
				UploadedAt: time.Now().Add(time.Hour * -1),
				JWT:        lo.Generate(t),
				Exp:        lo.GraceAt,
				UUID:       uuid.New(),
			},
		}

		entitlements, err := license.LicensesEntitlements(context.Background(), time.Now(), generatedLicenses, aiBridgeDisabledEnablements, coderdenttest.Keys, zeroUsageArgs)
		require.NoError(t, err)

		aiBridgeFeature := entitlements.Features[codersdk.FeatureAIBridge]
		assert.False(t, aiBridgeFeature.Enabled)
		require.NotContains(t, entitlements.Warnings, aiBridgeWarningMessage)
	})

	t.Run("Addon_AIBridgeOn", func(t *testing.T) {
		t.Parallel()
		// License with addon and AI Bridge enabled should NOT show warning.
		lo := (&coderdenttest.LicenseOptions{
			AccountType: "salesforce",
			AccountID:   "test",
			FeatureSet:  codersdk.FeatureSetPremium,
			Addons:      []codersdk.Addon{codersdk.AddonAIGovernance},
			Features: license.Features{
				codersdk.FeatureAIGovernanceUserLimit: 100,
			},
		}).Valid(time.Now())

		generatedLicenses := []database.License{
			{
				ID:         1,
				UploadedAt: time.Now().Add(time.Hour * -1),
				JWT:        lo.Generate(t),
				Exp:        lo.GraceAt,
				UUID:       uuid.New(),
			},
		}

		entitlements, err := license.LicensesEntitlements(context.Background(), time.Now(), generatedLicenses, aiBridgeEnabledEnablements, coderdenttest.Keys, zeroUsageArgs)
		require.NoError(t, err)

		aiBridgeFeature := entitlements.Features[codersdk.FeatureAIBridge]
		assert.True(t, aiBridgeFeature.Enabled)
		assert.Equal(t, codersdk.EntitlementEntitled, aiBridgeFeature.Entitlement)
		require.NotContains(t, entitlements.Warnings, aiBridgeWarningMessage)
	})

	t.Run("NoLicense_AIBridgeOn", func(t *testing.T) {
		t.Parallel()
		// No license with AI Bridge enabled should NOT show the soft warning
		// (it will show the generic "not entitled" warning instead).
		entitlements, err := license.LicensesEntitlements(context.Background(), time.Now(), []database.License{}, aiBridgeEnabledEnablements, coderdenttest.Keys, zeroUsageArgs)
		require.NoError(t, err)

		aiBridgeFeature := entitlements.Features[codersdk.FeatureAIBridge]
		assert.Equal(t, codersdk.EntitlementNotEntitled, aiBridgeFeature.Entitlement)
		require.NotContains(t, entitlements.Warnings, aiBridgeWarningMessage)
	})
}

func TestUsageLimitFeatures(t *testing.T) {
	t.Parallel()

	// Ensures that usage limit features are ranked by issued at, not by
	// values.
	t.Run("IssuedAtRanking", func(t *testing.T) {
		t.Parallel()

		// Generate 2 real licenses both with managed agent limit
		// features. lic2 should trump lic1 even though it has a lower
		// limit, because it was issued later.
		lic1 := database.License{
			ID:         1,
			UploadedAt: time.Now(),
			Exp:        time.Now().Add(time.Hour),
			UUID:       uuid.New(),
			JWT: coderdenttest.GenerateLicense(t, coderdenttest.LicenseOptions{
				IssuedAt:  time.Now().Add(-time.Minute * 2),
				NotBefore: dbtime.Now().Add(-time.Minute * 2),
				ExpiresAt: time.Now().Add(time.Hour * 2),
				Features: license.Features{
					codersdk.FeatureManagedAgentLimit: 100,
				},
			}),
		}
		lic2Iat := time.Now().Add(-time.Minute * 1)
		lic2Nbf := lic2Iat.Add(-time.Minute)
		lic2Exp := lic2Iat.Add(time.Hour)
		lic2 := database.License{
			ID:         2,
			UploadedAt: time.Now(),
			Exp:        lic2Exp,
			UUID:       uuid.New(),
			JWT: coderdenttest.GenerateLicense(t, coderdenttest.LicenseOptions{
				IssuedAt:  lic2Iat,
				NotBefore: lic2Nbf,
				ExpiresAt: lic2Exp,
				Features: license.Features{
					codersdk.FeatureManagedAgentLimit: 50,
				},
			}),
		}

		const actualAgents = 10
		arguments := license.FeatureArguments{
			ActiveUserCount:   10,
			ReplicaCount:      0,
			ExternalAuthCount: 0,
			ManagedAgentCountFn: func(ctx context.Context, from time.Time, to time.Time) (int64, error) {
				return actualAgents, nil
			},
		}

		// Load the licenses in both orders to ensure the correct
		// behavior is observed no matter the order.
		for _, order := range [][]database.License{
			{lic1, lic2},
			{lic2, lic1},
		} {
			entitlements, err := license.LicensesEntitlements(context.Background(), time.Now(), order, map[codersdk.FeatureName]bool{}, coderdenttest.Keys, arguments)
			require.NoError(t, err)

			feature, ok := entitlements.Features[codersdk.FeatureManagedAgentLimit]
			require.True(t, ok, "feature %s not found", codersdk.FeatureManagedAgentLimit)
			require.Equal(t, codersdk.EntitlementEntitled, feature.Entitlement)
			require.NotNil(t, feature.Limit)
			require.EqualValues(t, 50, *feature.Limit)
			require.NotNil(t, feature.Actual)
			require.EqualValues(t, actualAgents, *feature.Actual)
			require.NotNil(t, feature.UsagePeriod)
			require.WithinDuration(t, lic2Iat, feature.UsagePeriod.IssuedAt, 2*time.Second)
			require.WithinDuration(t, lic2Nbf, feature.UsagePeriod.Start, 2*time.Second)
			require.WithinDuration(t, lic2Exp, feature.UsagePeriod.End, 2*time.Second)
		}
	})
}

// TestOldStyleManagedAgentLicenses ensures backward compatibility with
// older licenses that encode the managed agent limit using separate
// "managed_agent_limit_soft" and "managed_agent_limit_hard" feature keys
// instead of the canonical "managed_agent_limit" key.
func TestOldStyleManagedAgentLicenses(t *testing.T) {
	t.Parallel()

	t.Run("SoftAndHard", func(t *testing.T) {
		t.Parallel()

		lic := database.License{
			ID:         1,
			UploadedAt: time.Now(),
			Exp:        time.Now().Add(time.Hour),
			UUID:       uuid.New(),
			JWT: coderdenttest.GenerateLicense(t, coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureName("managed_agent_limit_soft"): 100,
					codersdk.FeatureName("managed_agent_limit_hard"): 200,
				},
			}),
		}

		const actualAgents = 42
		arguments := license.FeatureArguments{
			ManagedAgentCountFn: func(_ context.Context, _, _ time.Time) (int64, error) {
				return actualAgents, nil
			},
		}

		entitlements, err := license.LicensesEntitlements(
			context.Background(), time.Now(), []database.License{lic},
			map[codersdk.FeatureName]bool{}, coderdenttest.Keys, arguments,
		)
		require.NoError(t, err)
		require.Empty(t, entitlements.Errors)

		feature := entitlements.Features[codersdk.FeatureManagedAgentLimit]
		require.Equal(t, codersdk.EntitlementEntitled, feature.Entitlement)
		require.True(t, feature.Enabled)
		require.NotNil(t, feature.Limit)
		// The soft limit should be used as the canonical limit.
		require.EqualValues(t, 100, *feature.Limit)
		require.NotNil(t, feature.Actual)
		require.EqualValues(t, actualAgents, *feature.Actual)
		require.NotNil(t, feature.UsagePeriod)
	})

	t.Run("OnlySoft", func(t *testing.T) {
		t.Parallel()

		lic := database.License{
			ID:         1,
			UploadedAt: time.Now(),
			Exp:        time.Now().Add(time.Hour),
			UUID:       uuid.New(),
			JWT: coderdenttest.GenerateLicense(t, coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureName("managed_agent_limit_soft"): 75,
				},
			}),
		}

		const actualAgents = 10
		arguments := license.FeatureArguments{
			ManagedAgentCountFn: func(_ context.Context, _, _ time.Time) (int64, error) {
				return actualAgents, nil
			},
		}

		entitlements, err := license.LicensesEntitlements(
			context.Background(), time.Now(), []database.License{lic},
			map[codersdk.FeatureName]bool{}, coderdenttest.Keys, arguments,
		)
		require.NoError(t, err)
		require.Empty(t, entitlements.Errors)

		feature := entitlements.Features[codersdk.FeatureManagedAgentLimit]
		require.Equal(t, codersdk.EntitlementEntitled, feature.Entitlement)
		require.True(t, feature.Enabled)
		require.NotNil(t, feature.Limit)
		require.EqualValues(t, 75, *feature.Limit)
	})

	// A license with only the hard limit key should silently ignore it,
	// leaving the feature unset (not entitled).
	t.Run("OnlyHard", func(t *testing.T) {
		t.Parallel()

		lic := database.License{
			ID:         1,
			UploadedAt: time.Now(),
			Exp:        time.Now().Add(time.Hour),
			UUID:       uuid.New(),
			JWT: coderdenttest.GenerateLicense(t, coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureName("managed_agent_limit_hard"): 200,
				},
			}),
		}

		arguments := license.FeatureArguments{
			ManagedAgentCountFn: func(_ context.Context, _, _ time.Time) (int64, error) {
				return 0, nil
			},
		}

		entitlements, err := license.LicensesEntitlements(
			context.Background(), time.Now(), []database.License{lic},
			map[codersdk.FeatureName]bool{}, coderdenttest.Keys, arguments,
		)
		require.NoError(t, err)
		require.Empty(t, entitlements.Errors)

		feature := entitlements.Features[codersdk.FeatureManagedAgentLimit]
		require.Equal(t, codersdk.EntitlementNotEntitled, feature.Entitlement)
	})

	// Old-style license with both soft and hard set to zero should
	// explicitly disable the feature (and override any Premium default).
	t.Run("ExplicitZero", func(t *testing.T) {
		t.Parallel()

		lic := database.License{
			ID:         1,
			UploadedAt: time.Now(),
			Exp:        time.Now().Add(time.Hour),
			UUID:       uuid.New(),
			JWT: coderdenttest.GenerateLicense(t, coderdenttest.LicenseOptions{
				FeatureSet: codersdk.FeatureSetPremium,
				Features: license.Features{
					codersdk.FeatureUserLimit:                        100,
					codersdk.FeatureName("managed_agent_limit_soft"): 0,
					codersdk.FeatureName("managed_agent_limit_hard"): 0,
				},
			}),
		}

		const actualAgents = 5
		arguments := license.FeatureArguments{
			ActiveUserCount: 10,
			ManagedAgentCountFn: func(_ context.Context, _, _ time.Time) (int64, error) {
				return actualAgents, nil
			},
			// The premium grandfather default grants a zero-hour agent
			// runtime allocation, so a runtime closure is required too.
			AgentRuntimeMsFn: func(_ context.Context, _, _ time.Time) (int64, error) {
				return 0, nil
			},
		}

		entitlements, err := license.LicensesEntitlements(
			context.Background(), time.Now(), []database.License{lic},
			map[codersdk.FeatureName]bool{}, coderdenttest.Keys, arguments,
		)
		require.NoError(t, err)

		feature := entitlements.Features[codersdk.FeatureManagedAgentLimit]
		require.Equal(t, codersdk.EntitlementEntitled, feature.Entitlement)
		require.False(t, feature.Enabled)
		require.NotNil(t, feature.Limit)
		require.EqualValues(t, 0, *feature.Limit)
		require.NotNil(t, feature.Actual)
		require.EqualValues(t, actualAgents, *feature.Actual)
	})
}

func TestManagedAgentLimitDefault(t *testing.T) {
	t.Parallel()

	// "Enterprise" licenses should not receive a default managed agent limit.
	t.Run("Enterprise", func(t *testing.T) {
		t.Parallel()

		lic := database.License{
			ID:         1,
			UploadedAt: time.Now(),
			Exp:        time.Now().Add(time.Hour),
			UUID:       uuid.New(),
			JWT: coderdenttest.GenerateLicense(t, coderdenttest.LicenseOptions{
				FeatureSet: codersdk.FeatureSetEnterprise,
				Features: license.Features{
					codersdk.FeatureUserLimit: 100,
				},
			}),
		}

		arguments := license.FeatureArguments{
			ActiveUserCount:   10,
			ReplicaCount:      0,
			ExternalAuthCount: 0,
			ManagedAgentCountFn: func(ctx context.Context, from time.Time, to time.Time) (int64, error) {
				return 0, nil
			},
		}
		entitlements, err := license.LicensesEntitlements(context.Background(), time.Now(), []database.License{lic}, map[codersdk.FeatureName]bool{}, coderdenttest.Keys, arguments)
		require.NoError(t, err)

		feature, ok := entitlements.Features[codersdk.FeatureManagedAgentLimit]
		require.True(t, ok, "feature %s not found", codersdk.FeatureManagedAgentLimit)
		require.Equal(t, codersdk.EntitlementNotEntitled, feature.Entitlement)
		require.Nil(t, feature.Limit)
		require.Nil(t, feature.Actual)
		require.Nil(t, feature.UsagePeriod)
	})

	// "Premium" licenses should receive a default managed agent limit of 1000.
	t.Run("Premium", func(t *testing.T) {
		t.Parallel()

		const userLimit = 33
		const defaultLimit = 1000
		lic := database.License{
			ID:         1,
			UploadedAt: time.Now(),
			Exp:        time.Now().Add(time.Hour),
			UUID:       uuid.New(),
			JWT: coderdenttest.GenerateLicense(t, coderdenttest.LicenseOptions{
				FeatureSet: codersdk.FeatureSetPremium,
				Features: license.Features{
					codersdk.FeatureUserLimit: userLimit,
				},
			}),
		}

		const actualAgents = 10
		arguments := license.FeatureArguments{
			ActiveUserCount:   10,
			ReplicaCount:      0,
			ExternalAuthCount: 0,
			ManagedAgentCountFn: func(ctx context.Context, from time.Time, to time.Time) (int64, error) {
				return actualAgents, nil
			},
			// The premium grandfather default grants a zero-hour agent
			// runtime allocation, so a runtime closure is required too.
			AgentRuntimeMsFn: func(_ context.Context, _, _ time.Time) (int64, error) {
				return 0, nil
			},
		}

		entitlements, err := license.LicensesEntitlements(context.Background(), time.Now(), []database.License{lic}, map[codersdk.FeatureName]bool{}, coderdenttest.Keys, arguments)
		require.NoError(t, err)

		feature, ok := entitlements.Features[codersdk.FeatureManagedAgentLimit]
		require.True(t, ok, "feature %s not found", codersdk.FeatureManagedAgentLimit)
		require.Equal(t, codersdk.EntitlementEntitled, feature.Entitlement)
		require.NotNil(t, feature.Limit)
		require.EqualValues(t, defaultLimit, *feature.Limit)
		require.NotNil(t, feature.Actual)
		require.EqualValues(t, actualAgents, *feature.Actual)
		require.NotNil(t, feature.UsagePeriod)
		require.NotZero(t, feature.UsagePeriod.IssuedAt)
		require.NotZero(t, feature.UsagePeriod.Start)
		require.NotZero(t, feature.UsagePeriod.End)
	})

	// "Premium" licenses with an explicit managed agent limit should use
	// that value instead of the default.
	t.Run("PremiumExplicitValues", func(t *testing.T) {
		t.Parallel()

		lic := database.License{
			ID:         1,
			UploadedAt: time.Now(),
			Exp:        time.Now().Add(time.Hour),
			UUID:       uuid.New(),
			JWT: coderdenttest.GenerateLicense(t, coderdenttest.LicenseOptions{
				FeatureSet: codersdk.FeatureSetPremium,
				Features: license.Features{
					codersdk.FeatureUserLimit:         100,
					codersdk.FeatureManagedAgentLimit: 100,
				},
			}),
		}

		const actualAgents = 10
		arguments := license.FeatureArguments{
			ActiveUserCount:   10,
			ReplicaCount:      0,
			ExternalAuthCount: 0,
			ManagedAgentCountFn: func(ctx context.Context, from time.Time, to time.Time) (int64, error) {
				return actualAgents, nil
			},
			// The premium grandfather default grants a zero-hour agent
			// runtime allocation, so a runtime closure is required too.
			AgentRuntimeMsFn: func(_ context.Context, _, _ time.Time) (int64, error) {
				return 0, nil
			},
		}

		entitlements, err := license.LicensesEntitlements(context.Background(), time.Now(), []database.License{lic}, map[codersdk.FeatureName]bool{}, coderdenttest.Keys, arguments)
		require.NoError(t, err)

		feature, ok := entitlements.Features[codersdk.FeatureManagedAgentLimit]
		require.True(t, ok, "feature %s not found", codersdk.FeatureManagedAgentLimit)
		require.Equal(t, codersdk.EntitlementEntitled, feature.Entitlement)
		require.NotNil(t, feature.Limit)
		require.EqualValues(t, 100, *feature.Limit)
		require.NotNil(t, feature.Actual)
		require.EqualValues(t, actualAgents, *feature.Actual)
		require.NotNil(t, feature.UsagePeriod)
		require.NotZero(t, feature.UsagePeriod.IssuedAt)
		require.NotZero(t, feature.UsagePeriod.Start)
		require.NotZero(t, feature.UsagePeriod.End)
	})

	// "Premium" licenses with an explicit 0 count should be entitled to 0
	// agents and should not receive a default managed agent limit.
	t.Run("PremiumExplicitZero", func(t *testing.T) {
		t.Parallel()

		lic := database.License{
			ID:         1,
			UploadedAt: time.Now(),
			Exp:        time.Now().Add(time.Hour),
			UUID:       uuid.New(),
			JWT: coderdenttest.GenerateLicense(t, coderdenttest.LicenseOptions{
				FeatureSet: codersdk.FeatureSetPremium,
				Features: license.Features{
					codersdk.FeatureUserLimit:         100,
					codersdk.FeatureManagedAgentLimit: 0,
				},
			}),
		}

		const actualAgents = 10
		arguments := license.FeatureArguments{
			ActiveUserCount:   10,
			ReplicaCount:      0,
			ExternalAuthCount: 0,
			ManagedAgentCountFn: func(ctx context.Context, from time.Time, to time.Time) (int64, error) {
				return actualAgents, nil
			},
			// The premium grandfather default grants a zero-hour agent
			// runtime allocation, so a runtime closure is required too.
			AgentRuntimeMsFn: func(_ context.Context, _, _ time.Time) (int64, error) {
				return 0, nil
			},
		}

		entitlements, err := license.LicensesEntitlements(context.Background(), time.Now(), []database.License{lic}, map[codersdk.FeatureName]bool{}, coderdenttest.Keys, arguments)
		require.NoError(t, err)

		feature, ok := entitlements.Features[codersdk.FeatureManagedAgentLimit]
		require.True(t, ok, "feature %s not found", codersdk.FeatureManagedAgentLimit)
		require.Equal(t, codersdk.EntitlementEntitled, feature.Entitlement)
		require.False(t, feature.Enabled)
		require.NotNil(t, feature.Limit)
		require.EqualValues(t, 0, *feature.Limit)
		require.NotNil(t, feature.Actual)
		require.EqualValues(t, actualAgents, *feature.Actual)
		require.NotNil(t, feature.UsagePeriod)
		require.NotZero(t, feature.UsagePeriod.IssuedAt)
		require.NotZero(t, feature.UsagePeriod.Start)
		require.NotZero(t, feature.UsagePeriod.End)
	})
}

// TestAgentRuntimeHoursLicenses ensures licenses carrying the agent runtime
// hour claims (allocation, soft limit, hard limit) surface as the single
// agent_runtime_hours feature.
func TestAgentRuntimeHoursLicenses(t *testing.T) {
	t.Parallel()

	// These cases exercise claim decoding rather than usage accounting, so
	// they report no runtime. A nil AgentRuntimeMsFn fails the whole
	// LicensesEntitlements call as a developer error when the feature is
	// present, so the closure must always be supplied.
	noRuntime := func() license.FeatureArguments {
		return license.FeatureArguments{
			AgentRuntimeMsFn: func(_ context.Context, _, _ time.Time) (int64, error) {
				return 0, nil
			},
		}
	}

	t.Run("AllClaims", func(t *testing.T) {
		t.Parallel()

		licIat := time.Now().Add(-time.Minute)
		licNbf := licIat.Add(-time.Minute)
		licExp := licIat.Add(time.Hour)
		lic := database.License{
			ID:         1,
			UploadedAt: time.Now(),
			Exp:        licExp,
			UUID:       uuid.New(),
			JWT: coderdenttest.GenerateLicense(t, coderdenttest.LicenseOptions{
				IssuedAt:  licIat,
				NotBefore: licNbf,
				ExpiresAt: licExp,
				Features: license.Features{
					license.ClaimAgentRuntimeHoursAllocation: 100,
					license.ClaimAgentRuntimeHoursLimitSoft:  80,
					license.ClaimAgentRuntimeHoursLimitHard:  120,
				},
			}),
		}

		entitlements, err := license.LicensesEntitlements(
			context.Background(), time.Now(), []database.License{lic},
			map[codersdk.FeatureName]bool{}, coderdenttest.Keys, noRuntime(),
		)
		require.NoError(t, err)
		require.Empty(t, entitlements.Errors)

		feature, ok := entitlements.Features[codersdk.FeatureAgentRuntimeHours]
		require.True(t, ok, "feature %s not found", codersdk.FeatureAgentRuntimeHours)
		require.Equal(t, codersdk.EntitlementEntitled, feature.Entitlement)
		require.True(t, feature.Enabled)
		require.NotNil(t, feature.Limit)
		require.EqualValues(t, 100, *feature.Limit)
		require.NotNil(t, feature.SoftLimit)
		require.EqualValues(t, 80, *feature.SoftLimit)
		require.NotNil(t, feature.HardLimit)
		require.EqualValues(t, 120, *feature.HardLimit)
		// Actual is populated from usage, which is zero for this license.
		require.NotNil(t, feature.Actual)
		require.EqualValues(t, 0, *feature.Actual)
		require.NotNil(t, feature.ActualMs)
		require.EqualValues(t, 0, *feature.ActualMs)
		require.NotNil(t, feature.UsagePeriod)
		require.WithinDuration(t, licIat, feature.UsagePeriod.IssuedAt, 2*time.Second)
		require.WithinDuration(t, licNbf, feature.UsagePeriod.Start, 2*time.Second)
		require.WithinDuration(t, licExp, feature.UsagePeriod.End, 2*time.Second)

		// The feature round-trips into the entitlements JSON served by
		// GET /api/v2/entitlements with all four fields.
		data, err := json.Marshal(entitlements)
		require.NoError(t, err)
		var raw struct {
			Features map[codersdk.FeatureName]map[string]any `json:"features"`
		}
		require.NoError(t, json.Unmarshal(data, &raw))
		rawFeature := raw.Features[codersdk.FeatureAgentRuntimeHours]
		require.EqualValues(t, 100, rawFeature["limit"])
		require.EqualValues(t, 80, rawFeature["soft_limit"])
		require.EqualValues(t, 120, rawFeature["hard_limit"])
		require.EqualValues(t, 0, rawFeature["actual_ms"])
		require.Contains(t, rawFeature, "usage_period")
	})

	t.Run("GracePeriod", func(t *testing.T) {
		t.Parallel()

		now := time.Now()
		opts := coderdenttest.LicenseOptions{
			Features: license.Features{
				license.ClaimAgentRuntimeHoursAllocation: 100,
				license.ClaimAgentRuntimeHoursLimitSoft:  80,
				license.ClaimAgentRuntimeHoursLimitHard:  120,
			},
		}
		opts.GracePeriod(now)
		lic := database.License{
			ID:         1,
			UploadedAt: now,
			Exp:        now.Add(time.Hour * 24),
			UUID:       uuid.New(),
			JWT:        coderdenttest.GenerateLicense(t, opts),
		}

		entitlements, err := license.LicensesEntitlements(
			context.Background(), now, []database.License{lic},
			map[codersdk.FeatureName]bool{}, coderdenttest.Keys, noRuntime(),
		)
		require.NoError(t, err)
		require.Empty(t, entitlements.Errors)

		feature := entitlements.Features[codersdk.FeatureAgentRuntimeHours]
		require.Equal(t, codersdk.EntitlementGracePeriod, feature.Entitlement)
		require.True(t, feature.Enabled)
		require.NotNil(t, feature.Limit)
		require.EqualValues(t, 100, *feature.Limit)
		require.NotNil(t, feature.SoftLimit)
		require.EqualValues(t, 80, *feature.SoftLimit)
		require.NotNil(t, feature.HardLimit)
		require.EqualValues(t, 120, *feature.HardLimit)
		require.NotNil(t, feature.UsagePeriod)
	})

	t.Run("AllocationOnly", func(t *testing.T) {
		t.Parallel()

		lic := database.License{
			ID:         1,
			UploadedAt: time.Now(),
			Exp:        time.Now().Add(time.Hour),
			UUID:       uuid.New(),
			JWT: coderdenttest.GenerateLicense(t, coderdenttest.LicenseOptions{
				Features: license.Features{
					license.ClaimAgentRuntimeHoursAllocation: 100,
				},
			}),
		}

		entitlements, err := license.LicensesEntitlements(
			context.Background(), time.Now(), []database.License{lic},
			map[codersdk.FeatureName]bool{}, coderdenttest.Keys, noRuntime(),
		)
		require.NoError(t, err)
		require.Empty(t, entitlements.Errors)

		feature := entitlements.Features[codersdk.FeatureAgentRuntimeHours]
		require.Equal(t, codersdk.EntitlementEntitled, feature.Entitlement)
		require.True(t, feature.Enabled)
		require.NotNil(t, feature.Limit)
		require.EqualValues(t, 100, *feature.Limit)
		require.Nil(t, feature.SoftLimit)
		require.Nil(t, feature.HardLimit)
		require.NotNil(t, feature.UsagePeriod)
	})

	// A license with an explicit zero allocation is entitled but disabled,
	// mirroring the managed agent limit behavior.
	t.Run("ExplicitZero", func(t *testing.T) {
		t.Parallel()

		lic := database.License{
			ID:         1,
			UploadedAt: time.Now(),
			Exp:        time.Now().Add(time.Hour),
			UUID:       uuid.New(),
			JWT: coderdenttest.GenerateLicense(t, coderdenttest.LicenseOptions{
				Features: license.Features{
					license.ClaimAgentRuntimeHoursAllocation: 0,
				},
			}),
		}

		entitlements, err := license.LicensesEntitlements(
			context.Background(), time.Now(), []database.License{lic},
			map[codersdk.FeatureName]bool{}, coderdenttest.Keys, noRuntime(),
		)
		require.NoError(t, err)
		require.Empty(t, entitlements.Errors)

		feature := entitlements.Features[codersdk.FeatureAgentRuntimeHours]
		require.Equal(t, codersdk.EntitlementEntitled, feature.Entitlement)
		require.False(t, feature.Enabled)
		require.NotNil(t, feature.Limit)
		require.EqualValues(t, 0, *feature.Limit)
		require.NotNil(t, feature.UsagePeriod)
	})

	// An unlimited (-1) allocation grants the feature enabled with no Limit,
	// which the API serves as an omitted "limit" field, the shape the UI
	// already renders as "Unlimited".
	t.Run("UnlimitedAllocation", func(t *testing.T) {
		t.Parallel()

		lic := database.License{
			ID:         1,
			UploadedAt: time.Now(),
			Exp:        time.Now().Add(time.Hour),
			UUID:       uuid.New(),
			JWT: coderdenttest.GenerateLicense(t, coderdenttest.LicenseOptions{
				Features: license.Features{
					license.ClaimAgentRuntimeHoursAllocation: license.AgentRuntimeHoursUnlimitedAllocation,
				},
			}),
		}

		entitlements, err := license.LicensesEntitlements(
			context.Background(), time.Now(), []database.License{lic},
			map[codersdk.FeatureName]bool{}, coderdenttest.Keys, noRuntime(),
		)
		require.NoError(t, err)
		require.Empty(t, entitlements.Errors)
		require.NotContains(t, entitlements.Warnings,
			codersdk.LicenseAgentRuntimeHoursClaimsIgnoredWarningText)

		feature := entitlements.Features[codersdk.FeatureAgentRuntimeHours]
		require.Equal(t, codersdk.EntitlementEntitled, feature.Entitlement)
		require.True(t, feature.Enabled)
		require.Nil(t, feature.Limit)
		require.Nil(t, feature.SoftLimit)
		require.Nil(t, feature.HardLimit)
		require.NotNil(t, feature.UsagePeriod)

		// The entitlements JSON served by GET /api/v2/entitlements omits
		// "limit" entirely for the unlimited feature.
		data, err := json.Marshal(entitlements)
		require.NoError(t, err)
		var raw struct {
			Features map[codersdk.FeatureName]map[string]any `json:"features"`
		}
		require.NoError(t, json.Unmarshal(data, &raw))
		rawFeature := raw.Features[codersdk.FeatureAgentRuntimeHours]
		require.Equal(t, true, rawFeature["enabled"])
		require.NotContains(t, rawFeature, "limit")
		require.Contains(t, rawFeature, "usage_period")
	})

	// The license with the newest issued-at claim wins, even if another
	// license was loaded first or has a larger allocation. The soft and hard
	// limits come from the winning license.
	// Mirrors TestUsageLimitFeatures/IssuedAtRanking.
	t.Run("IssuedAtRanking", func(t *testing.T) {
		t.Parallel()

		lic1 := database.License{
			ID:         1,
			UploadedAt: time.Now(),
			Exp:        time.Now().Add(time.Hour),
			UUID:       uuid.New(),
			JWT: coderdenttest.GenerateLicense(t, coderdenttest.LicenseOptions{
				IssuedAt:  time.Now().Add(-time.Minute * 2),
				NotBefore: time.Now().Add(-time.Minute * 2),
				ExpiresAt: time.Now().Add(time.Hour * 2),
				Features: license.Features{
					license.ClaimAgentRuntimeHoursAllocation: 100,
					license.ClaimAgentRuntimeHoursLimitSoft:  80,
					license.ClaimAgentRuntimeHoursLimitHard:  120,
				},
			}),
		}
		lic2Iat := time.Now().Add(-time.Minute * 1)
		lic2Nbf := lic2Iat.Add(-time.Minute)
		lic2Exp := lic2Iat.Add(time.Hour)
		lic2 := database.License{
			ID:         2,
			UploadedAt: time.Now(),
			Exp:        lic2Exp,
			UUID:       uuid.New(),
			JWT: coderdenttest.GenerateLicense(t, coderdenttest.LicenseOptions{
				IssuedAt:  lic2Iat,
				NotBefore: lic2Nbf,
				ExpiresAt: lic2Exp,
				Features: license.Features{
					license.ClaimAgentRuntimeHoursAllocation: 50,
					license.ClaimAgentRuntimeHoursLimitSoft:  40,
					license.ClaimAgentRuntimeHoursLimitHard:  60,
				},
			}),
		}

		// Load the licenses in both orders to ensure the correct
		// behavior is observed no matter the order.
		for _, order := range [][]database.License{
			{lic1, lic2},
			{lic2, lic1},
		} {
			entitlements, err := license.LicensesEntitlements(context.Background(), time.Now(), order, map[codersdk.FeatureName]bool{}, coderdenttest.Keys, noRuntime())
			require.NoError(t, err)

			feature, ok := entitlements.Features[codersdk.FeatureAgentRuntimeHours]
			require.True(t, ok, "feature %s not found", codersdk.FeatureAgentRuntimeHours)
			require.Equal(t, codersdk.EntitlementEntitled, feature.Entitlement)
			require.NotNil(t, feature.Limit)
			require.EqualValues(t, 50, *feature.Limit)
			require.NotNil(t, feature.SoftLimit)
			require.EqualValues(t, 40, *feature.SoftLimit)
			require.NotNil(t, feature.HardLimit)
			require.EqualValues(t, 60, *feature.HardLimit)
			require.NotNil(t, feature.UsagePeriod)
			require.WithinDuration(t, lic2Iat, feature.UsagePeriod.IssuedAt, 2*time.Second)
			require.WithinDuration(t, lic2Nbf, feature.UsagePeriod.Start, 2*time.Second)
			require.WithinDuration(t, lic2Exp, feature.UsagePeriod.End, 2*time.Second)
		}
	})

	// When an unlimited and a metered license are minted with identical
	// issued-at and expiry claims, the unlimited grant must win the tie,
	// regardless of load order.
	t.Run("UnlimitedOutranksMeteredOnTie", func(t *testing.T) {
		t.Parallel()

		// JWT NumericDate claims have second granularity, so truncate to
		// keep the round-tripped issued-at values identical.
		iat := time.Now().Add(-time.Minute).Truncate(time.Second)
		nbf := iat
		exp := iat.Add(time.Hour).Truncate(time.Second)
		unlimited := database.License{
			ID:         1,
			UploadedAt: time.Now(),
			Exp:        exp,
			UUID:       uuid.New(),
			JWT: coderdenttest.GenerateLicense(t, coderdenttest.LicenseOptions{
				IssuedAt:  iat,
				NotBefore: nbf,
				ExpiresAt: exp,
				Features: license.Features{
					license.ClaimAgentRuntimeHoursAllocation: license.AgentRuntimeHoursUnlimitedAllocation,
				},
			}),
		}
		metered := database.License{
			ID:         2,
			UploadedAt: time.Now(),
			Exp:        exp,
			UUID:       uuid.New(),
			JWT: coderdenttest.GenerateLicense(t, coderdenttest.LicenseOptions{
				IssuedAt:  iat,
				NotBefore: nbf,
				ExpiresAt: exp,
				Features: license.Features{
					license.ClaimAgentRuntimeHoursAllocation: 100,
					license.ClaimAgentRuntimeHoursLimitSoft:  80,
					license.ClaimAgentRuntimeHoursLimitHard:  120,
				},
			}),
		}

		for _, order := range [][]database.License{
			{unlimited, metered},
			{metered, unlimited},
		} {
			entitlements, err := license.LicensesEntitlements(context.Background(), time.Now(), order, map[codersdk.FeatureName]bool{}, coderdenttest.Keys, noRuntime())
			require.NoError(t, err)

			feature, ok := entitlements.Features[codersdk.FeatureAgentRuntimeHours]
			require.True(t, ok, "feature %s not found", codersdk.FeatureAgentRuntimeHours)
			require.Equal(t, codersdk.EntitlementEntitled, feature.Entitlement)
			require.True(t, feature.Enabled)
			require.Nil(t, feature.Limit)
			require.Nil(t, feature.SoftLimit)
			require.Nil(t, feature.HardLimit)
			require.NotNil(t, feature.UsagePeriod)
		}
	})

	// A newer license without soft/hard limits must fully replace an older
	// license that carried them; the limits must not merge across licenses.
	t.Run("SoftHardRideAlongWithWinner", func(t *testing.T) {
		t.Parallel()

		lic1 := database.License{
			ID:         1,
			UploadedAt: time.Now(),
			Exp:        time.Now().Add(time.Hour),
			UUID:       uuid.New(),
			JWT: coderdenttest.GenerateLicense(t, coderdenttest.LicenseOptions{
				IssuedAt:  time.Now().Add(-time.Minute * 2),
				NotBefore: time.Now().Add(-time.Minute * 2),
				ExpiresAt: time.Now().Add(time.Hour * 2),
				Features: license.Features{
					license.ClaimAgentRuntimeHoursAllocation: 100,
					license.ClaimAgentRuntimeHoursLimitSoft:  80,
					license.ClaimAgentRuntimeHoursLimitHard:  120,
				},
			}),
		}
		lic2Iat := time.Now().Add(-time.Minute * 1)
		lic2 := database.License{
			ID:         2,
			UploadedAt: time.Now(),
			Exp:        lic2Iat.Add(time.Hour),
			UUID:       uuid.New(),
			JWT: coderdenttest.GenerateLicense(t, coderdenttest.LicenseOptions{
				IssuedAt:  lic2Iat,
				NotBefore: lic2Iat.Add(-time.Minute),
				ExpiresAt: lic2Iat.Add(time.Hour),
				Features: license.Features{
					license.ClaimAgentRuntimeHoursAllocation: 50,
				},
			}),
		}

		for _, order := range [][]database.License{
			{lic1, lic2},
			{lic2, lic1},
		} {
			entitlements, err := license.LicensesEntitlements(context.Background(), time.Now(), order, map[codersdk.FeatureName]bool{}, coderdenttest.Keys, noRuntime())
			require.NoError(t, err)

			feature := entitlements.Features[codersdk.FeatureAgentRuntimeHours]
			require.Equal(t, codersdk.EntitlementEntitled, feature.Entitlement)
			require.NotNil(t, feature.Limit)
			require.EqualValues(t, 50, *feature.Limit)
			require.Nil(t, feature.SoftLimit)
			require.Nil(t, feature.HardLimit)
		}
	})

	// The feature name itself is not a valid claim; the allocation must come
	// from the dedicated claim.
	t.Run("DirectFeatureNameClaimIgnored", func(t *testing.T) {
		t.Parallel()

		lic := database.License{
			ID:         1,
			UploadedAt: time.Now(),
			Exp:        time.Now().Add(time.Hour),
			UUID:       uuid.New(),
			JWT: coderdenttest.GenerateLicense(t, coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureAgentRuntimeHours: 100,
				},
			}),
		}

		entitlements, err := license.LicensesEntitlements(
			context.Background(), time.Now(), []database.License{lic},
			map[codersdk.FeatureName]bool{}, coderdenttest.Keys, noRuntime(),
		)
		require.NoError(t, err)
		require.Empty(t, entitlements.Errors)

		feature := entitlements.Features[codersdk.FeatureAgentRuntimeHours]
		require.Equal(t, codersdk.EntitlementNotEntitled, feature.Entitlement)
		require.Nil(t, feature.Limit)
	})

	// The rollout guarantee for old deployments is that none of the three
	// claim names is itself a feature name, so an old server ignores them as
	// unknown claims. Pin the invariant so a future feature registration
	// cannot break it silently.
	t.Run("ClaimNamesAreNotFeatureNames", func(t *testing.T) {
		t.Parallel()

		for _, claim := range []string{
			license.ClaimAgentRuntimeHoursAllocation,
			license.ClaimAgentRuntimeHoursLimitSoft,
			license.ClaimAgentRuntimeHoursLimitHard,
		} {
			require.NotContains(t, codersdk.FeatureNamesMap, codersdk.FeatureName(claim))
		}
	})

	// Ensures licenses carrying claims for features this server version does
	// not know about do not break entitlement computation. This is exactly
	// what old deployments see when a license carries the agent runtime hour
	// claims, since none of the three claim names is a feature name.
	t.Run("UnknownClaimsCompatibility", func(t *testing.T) {
		t.Parallel()

		lic := database.License{
			ID:         1,
			UploadedAt: time.Now(),
			Exp:        time.Now().Add(time.Hour),
			UUID:       uuid.New(),
			JWT: coderdenttest.GenerateLicense(t, coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureUserLimit:                         100,
					codersdk.FeatureName("future_feature_allocation"): 100,
					codersdk.FeatureName("future_feature_limit_soft"): 80,
					codersdk.FeatureName("future_feature_limit_hard"): 120,
					codersdk.FeatureName("future_boolean_feature"):    1,
				},
			}),
		}

		entitlements, err := license.LicensesEntitlements(
			context.Background(), time.Now(), []database.License{lic},
			map[codersdk.FeatureName]bool{}, coderdenttest.Keys, noRuntime(),
		)
		require.NoError(t, err)
		require.Empty(t, entitlements.Errors)
		require.True(t, entitlements.HasLicense)

		// The unknown claims are silently ignored and the known claim is
		// still entitled.
		require.NotContains(t, entitlements.Features, codersdk.FeatureName("future_feature_allocation"))
		require.NotContains(t, entitlements.Features, codersdk.FeatureName("future_boolean_feature"))
		userLimit := entitlements.Features[codersdk.FeatureUserLimit]
		require.NotNil(t, userLimit.Limit)
		require.EqualValues(t, 100, *userLimit.Limit)
	})
}

// TestAgentRuntimeHoursClaimTolerance pins decodeAgentRuntimeHours's
// tolerate-and-warn contract; see that function's doc for the rationale.
func TestAgentRuntimeHoursClaimTolerance(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		features license.Features

		// expectFeature is nil when the feature must be absent.
		expectFeature *codersdk.Feature
		// expectClaimsIgnored is true when at least one present claim is
		// dropped, which must surface the claims-ignored warning: tolerating
		// a claim and signaling nothing would make an incorrectly issued license
		// undetectable from the deployment.
		expectClaimsIgnored bool
	}{
		{
			name: "AllClaims",
			features: license.Features{
				license.ClaimAgentRuntimeHoursAllocation: 100,
				license.ClaimAgentRuntimeHoursLimitSoft:  80,
				license.ClaimAgentRuntimeHoursLimitHard:  120,
			},
			expectFeature: &codersdk.Feature{
				Enabled:   true,
				Limit:     ptr.Ref[int64](100),
				SoftLimit: ptr.Ref[int64](80),
				HardLimit: ptr.Ref[int64](120),
			},
		},
		{
			name: "AllocationOnly",
			features: license.Features{
				license.ClaimAgentRuntimeHoursAllocation: 100,
			},
			expectFeature: &codersdk.Feature{
				Enabled: true,
				Limit:   ptr.Ref[int64](100),
			},
		},
		{
			// A zero soft limit is valid (0 <= soft < allocation) and warns
			// from the start of the usage period. Omitting the claim is the
			// way to express "no soft limit".
			name: "ZeroSoft",
			features: license.Features{
				license.ClaimAgentRuntimeHoursAllocation: 100,
				license.ClaimAgentRuntimeHoursLimitSoft:  0,
			},
			expectFeature: &codersdk.Feature{
				Enabled:   true,
				Limit:     ptr.Ref[int64](100),
				SoftLimit: ptr.Ref[int64](0),
			},
		},
		{
			name: "NegativeSoft",
			features: license.Features{
				license.ClaimAgentRuntimeHoursAllocation: 100,
				license.ClaimAgentRuntimeHoursLimitSoft:  -1,
			},
			expectFeature: &codersdk.Feature{
				Enabled: true,
				Limit:   ptr.Ref[int64](100),
			},
			expectClaimsIgnored: true,
		},
		{
			// A soft limit at or above the allocation could never fire
			// before the allocation warning supersedes it.
			name: "SoftEqualsAllocation",
			features: license.Features{
				license.ClaimAgentRuntimeHoursAllocation: 100,
				license.ClaimAgentRuntimeHoursLimitSoft:  100,
			},
			expectFeature: &codersdk.Feature{
				Enabled: true,
				Limit:   ptr.Ref[int64](100),
			},
			expectClaimsIgnored: true,
		},
		{
			name: "SoftAboveAllocation",
			features: license.Features{
				license.ClaimAgentRuntimeHoursAllocation: 100,
				license.ClaimAgentRuntimeHoursLimitSoft:  150,
			},
			expectFeature: &codersdk.Feature{
				Enabled: true,
				Limit:   ptr.Ref[int64](100),
			},
			expectClaimsIgnored: true,
		},
		{
			name: "HardEqualsAllocation",
			features: license.Features{
				license.ClaimAgentRuntimeHoursAllocation: 100,
				license.ClaimAgentRuntimeHoursLimitHard:  100,
			},
			expectFeature: &codersdk.Feature{
				Enabled:   true,
				Limit:     ptr.Ref[int64](100),
				HardLimit: ptr.Ref[int64](100),
			},
		},
		{
			name: "HardBelowAllocation",
			features: license.Features{
				license.ClaimAgentRuntimeHoursAllocation: 100,
				license.ClaimAgentRuntimeHoursLimitHard:  99,
			},
			expectFeature: &codersdk.Feature{
				Enabled: true,
				Limit:   ptr.Ref[int64](100),
			},
			expectClaimsIgnored: true,
		},
		{
			name: "ZeroAllocation",
			features: license.Features{
				license.ClaimAgentRuntimeHoursAllocation: 0,
			},
			expectFeature: &codersdk.Feature{
				Enabled: false,
				Limit:   ptr.Ref[int64](0),
			},
		},
		{
			// A zero allocation has no hour budget, so threshold claims
			// alongside it are dropped, with the warning.
			name: "ZeroAllocationWithLimits",
			features: license.Features{
				license.ClaimAgentRuntimeHoursAllocation: 0,
				license.ClaimAgentRuntimeHoursLimitSoft:  80,
				license.ClaimAgentRuntimeHoursLimitHard:  1000,
			},
			expectFeature: &codersdk.Feature{
				Enabled: false,
				Limit:   ptr.Ref[int64](0),
			},
			expectClaimsIgnored: true,
		},
		{
			// An unlimited allocation grants the feature with no Limit and
			// no warning: -1 is the canonical unlimited encoding, not an
			// issuance mistake.
			name: "UnlimitedAllocation",
			features: license.Features{
				license.ClaimAgentRuntimeHoursAllocation: license.AgentRuntimeHoursUnlimitedAllocation,
			},
			expectFeature: &codersdk.Feature{
				Enabled: true,
			},
		},
		{
			// Threshold claims alongside an unlimited allocation have
			// nothing to threshold against; the grant survives but the
			// issuance mistake must stay visible via the warning.
			name: "UnlimitedWithSoft",
			features: license.Features{
				license.ClaimAgentRuntimeHoursAllocation: license.AgentRuntimeHoursUnlimitedAllocation,
				license.ClaimAgentRuntimeHoursLimitSoft:  80,
			},
			expectFeature: &codersdk.Feature{
				Enabled: true,
			},
			expectClaimsIgnored: true,
		},
		{
			name: "UnlimitedWithHard",
			features: license.Features{
				license.ClaimAgentRuntimeHoursAllocation: license.AgentRuntimeHoursUnlimitedAllocation,
				license.ClaimAgentRuntimeHoursLimitHard:  120,
			},
			expectFeature: &codersdk.Feature{
				Enabled: true,
			},
			expectClaimsIgnored: true,
		},
		{
			// Only exactly -1 is the unlimited sentinel; any other negative
			// allocation stays unusable.
			name: "NegativeAllocation",
			features: license.Features{
				license.ClaimAgentRuntimeHoursAllocation: -2,
			},
			expectClaimsIgnored: true,
		},
		{
			name: "SoftWithoutAllocation",
			features: license.Features{
				license.ClaimAgentRuntimeHoursLimitSoft: 80,
			},
			expectClaimsIgnored: true,
		},
		{
			name: "HardWithoutAllocation",
			features: license.Features{
				license.ClaimAgentRuntimeHoursLimitHard: 120,
			},
			expectClaimsIgnored: true,
		},
		{
			// The feature name itself is never a valid claim: the
			// allocation must come from the dedicated claim. It is the
			// shape every other metered feature uses, so a license minting
			// it is the most plausible issuer mistake and must warn
			// rather than being dropped silently.
			name: "FeatureNameAsClaim",
			features: license.Features{
				codersdk.FeatureAgentRuntimeHours: 100,
			},
			expectClaimsIgnored: true,
		},
		{
			// The feature name claim is dropped (with the warning) even
			// when a usable allocation claim grants the feature.
			name: "FeatureNameAlongsideAllocation",
			features: license.Features{
				codersdk.FeatureAgentRuntimeHours:        50,
				license.ClaimAgentRuntimeHoursAllocation: 100,
			},
			expectFeature: &codersdk.Feature{
				Enabled: true,
				Limit:   ptr.Ref[int64](100),
			},
			expectClaimsIgnored: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			features := license.Features{
				codersdk.FeatureUserLimit: 100,
			}
			maps.Copy(features, tc.features)
			lic := database.License{
				ID:         1,
				UploadedAt: time.Now(),
				Exp:        time.Now().Add(time.Hour),
				UUID:       uuid.New(),
				JWT: coderdenttest.GenerateLicense(t, coderdenttest.LicenseOptions{
					Features: features,
				}),
			}

			var logBuf bytes.Buffer
			entitlements, err := license.LicensesEntitlements(
				context.Background(), time.Now(), []database.License{lic},
				map[codersdk.FeatureName]bool{}, coderdenttest.Keys, license.FeatureArguments{
					Logger: slog.Make(sloghuman.Sink(&logBuf)),
					AgentRuntimeMsFn: func(_ context.Context, _, _ time.Time) (int64, error) {
						return 0, nil
					},
				},
			)
			require.NoError(t, err)

			// The license as a whole survives: unrelated paid features are
			// unaffected by an unusable runtime hour claim.
			require.Empty(t, entitlements.Errors)
			require.True(t, entitlements.HasLicense)
			userLimit := entitlements.Features[codersdk.FeatureUserLimit]
			require.NotNil(t, userLimit.Limit)
			require.EqualValues(t, 100, *userLimit.Limit)

			// Dropped claims are tolerated but never silent: the operator
			// sees the stable warning, and the log names the license and
			// the dropped claims for support.
			if tc.expectClaimsIgnored {
				require.Contains(t, entitlements.Warnings,
					codersdk.LicenseAgentRuntimeHoursClaimsIgnoredWarningText)
				logs := logBuf.String()
				require.Contains(t, logs, "ignored unusable Coder Agent runtime hour claims in license")
				require.Contains(t, logs, lic.UUID.String())
			} else {
				require.NotContains(t, entitlements.Warnings,
					codersdk.LicenseAgentRuntimeHoursClaimsIgnoredWarningText)
				require.Empty(t, logBuf.String())
			}

			// Every known feature name has a default entry in the map, so
			// "the license does not grant the feature" surfaces as the
			// default: no limit, no usage period, not enabled.
			feature := entitlements.Features[codersdk.FeatureAgentRuntimeHours]
			if tc.expectFeature == nil {
				require.Nil(t, feature.Limit, "feature must not be granted")
				require.Nil(t, feature.UsagePeriod, "feature must not be granted")
				require.False(t, feature.Enabled)
				return
			}
			require.NotNil(t, feature.UsagePeriod, "feature must be granted")
			require.Equal(t, tc.expectFeature.Enabled, feature.Enabled)
			require.Equal(t, tc.expectFeature.Limit, feature.Limit)
			require.Equal(t, tc.expectFeature.SoftLimit, feature.SoftLimit)
			require.Equal(t, tc.expectFeature.HardLimit, feature.HardLimit)
		})
	}

	t.Run("WarningDeduplicatedAcrossLicenses", func(t *testing.T) {
		t.Parallel()

		// Two licenses with unusable claims must publish the stable warning
		// once, or the banner would stack identical texts, while the log
		// names each affected license so the operator can tell which ones
		// need re-issuing.
		newLicense := func(id int32) database.License {
			return database.License{
				ID:         id,
				UploadedAt: time.Now(),
				Exp:        time.Now().Add(time.Hour),
				UUID:       uuid.New(),
				JWT: coderdenttest.GenerateLicense(t, coderdenttest.LicenseOptions{
					Features: license.Features{
						codersdk.FeatureUserLimit: 100,
						// A threshold without an allocation is unusable.
						license.ClaimAgentRuntimeHoursLimitSoft: 80,
					},
				}),
			}
		}
		licenses := []database.License{newLicense(1), newLicense(2)}

		var logBuf bytes.Buffer
		entitlements, err := license.LicensesEntitlements(
			context.Background(), time.Now(), licenses,
			map[codersdk.FeatureName]bool{}, coderdenttest.Keys, license.FeatureArguments{
				Logger: slog.Make(sloghuman.Sink(&logBuf)),
				AgentRuntimeMsFn: func(_ context.Context, _, _ time.Time) (int64, error) {
					return 0, nil
				},
			},
		)
		require.NoError(t, err)

		warningCount := 0
		for _, warning := range entitlements.Warnings {
			if warning == codersdk.LicenseAgentRuntimeHoursClaimsIgnoredWarningText {
				warningCount++
			}
		}
		require.Equal(t, 1, warningCount, "the claims-ignored warning must appear exactly once")

		logs := logBuf.String()
		for _, lic := range licenses {
			require.Contains(t, logs, lic.UUID.String())
		}
	})
}

func TestAIGovernanceAddon(t *testing.T) {
	t.Parallel()

	empty := map[codersdk.FeatureName]bool{}

	t.Run("AIGovernanceAddon enables AI Governance features when enablements are set", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		db.InsertLicense(context.Background(), database.InsertLicenseParams{
			JWT: coderdenttest.GenerateLicense(t, coderdenttest.LicenseOptions{
				FeatureSet: codersdk.FeatureSetPremium,
				Features: license.Features{
					codersdk.FeatureAIGovernanceUserLimit: 1000,
					codersdk.FeatureManagedAgentLimit:     1000,
				},
				Addons: []codersdk.Addon{codersdk.AddonAIGovernance},
			}),
			Exp: dbtime.Now().Add(time.Hour),
		})

		// Enable AI Governance features in enablements.
		enablements := map[codersdk.FeatureName]bool{
			codersdk.FeatureAIBridge: true,
			codersdk.FeatureBoundary: true,
		}
		entitlements, err := license.Entitlements(context.Background(), testutil.Logger(t), db, 1, 1, coderdenttest.Keys, enablements, testAuthorizer, nil)
		require.NoError(t, err)
		require.True(t, entitlements.HasLicense)

		// AI Bridge should be enabled without warning when addon is present.
		aibridgeFeature := entitlements.Features[codersdk.FeatureAIBridge]
		require.True(t, aibridgeFeature.Enabled, "AI Bridge should be enabled when addon is present and enablements are set")
		aiBridgeWarningMessage := "The AI Governance add-on is required to use AI Gateway. Please reach out to your account team or sales@coder.com to learn more."
		require.NotContains(t, entitlements.Warnings, aiBridgeWarningMessage, "AI Bridge warning should not appear when AI Governance addon is present")

		// require.Equal(t, codersdk.EntitlementEntitled, aibridgeFeature.Entitlement, "AI Bridge should be entitled when addon is present")

		// TODO: Readd this test once Boundary is enforced as an add-on license.
		// boundaryFeature := entitlements.Features[codersdk.FeatureBoundary]
		// require.True(t, boundaryFeature.Enabled, "Boundary should be enabled when addon is present and enablements are set")
		// require.Equal(t, codersdk.EntitlementEntitled, boundaryFeature.Entitlement, "Boundary should be entitled when addon is present")
	})

	t.Run("AIGovernanceAddon not present disables AI Governance features", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		db.InsertLicense(context.Background(), database.InsertLicenseParams{
			JWT: coderdenttest.GenerateLicense(t, coderdenttest.LicenseOptions{
				FeatureSet: codersdk.FeatureSetPremium,
			}),
			Exp: dbtime.Now().Add(time.Hour),
		})

		enablements := map[codersdk.FeatureName]bool{
			codersdk.FeatureAIBridge: true,
			codersdk.FeatureBoundary: true,
		}
		entitlements, err := license.Entitlements(context.Background(), testutil.Logger(t), db, 1, 1, coderdenttest.Keys, enablements, testAuthorizer, nil)
		require.NoError(t, err)
		require.True(t, entitlements.HasLicense)

		// TODO: Readd this test once AI Bridge is enforced as an add-on license.
		// AI Bridge should not be entitled.
		// aibridgeFeature := entitlements.Features[codersdk.FeatureAIBridge]
		// require.False(t, aibridgeFeature.Enabled, "AI Bridge should not be enabled when addon is absent")
		// require.Equal(t, codersdk.EntitlementNotEntitled, aibridgeFeature.Entitlement, "AI Bridge should not be entitled when addon is absent")

		// TODO: Readd this test once Boundary is enforced as an add-on license.
		// boundaryFeature := entitlements.Features[codersdk.FeatureBoundary]
		// require.False(t, boundaryFeature.Enabled, "Boundary should not be enabled when addon is absent")
		// require.Equal(t, codersdk.EntitlementNotEntitled, boundaryFeature.Entitlement, "Boundary should not be entitled when addon is absent")
	})

	t.Run("AIGovernanceAddon respects grace period entitlement", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		db.InsertLicense(context.Background(), database.InsertLicenseParams{
			JWT: coderdenttest.GenerateLicense(t, coderdenttest.LicenseOptions{
				FeatureSet: codersdk.FeatureSetPremium,
				Features: license.Features{
					codersdk.FeatureAIGovernanceUserLimit: 1000,
					codersdk.FeatureManagedAgentLimit:     1000,
				},
				Addons:    []codersdk.Addon{codersdk.AddonAIGovernance},
				NotBefore: dbtime.Now().Add(-time.Hour * 2),
				GraceAt:   dbtime.Now().Add(-time.Hour),
				ExpiresAt: dbtime.Now().Add(time.Hour),
			}),
			Exp: dbtime.Now().Add(time.Hour),
		})

		enablements := map[codersdk.FeatureName]bool{
			codersdk.FeatureAIBridge: true,
			codersdk.FeatureBoundary: true,
		}
		entitlements, err := license.Entitlements(context.Background(), testutil.Logger(t), db, 1, 1, coderdenttest.Keys, enablements, testAuthorizer, nil)
		require.NoError(t, err)
		require.True(t, entitlements.HasLicense)

		// TODO: Readd this test once AI Bridge is enforced as an add-on license.
		// AI Governance features should be enabled but in grace period.
		// aibridgeFeature := entitlements.Features[codersdk.FeatureAIBridge]
		// require.True(t, aibridgeFeature.Enabled, "AI Bridge should be enabled during grace period")
		// require.Equal(t, codersdk.EntitlementGracePeriod, aibridgeFeature.Entitlement, "AI Bridge should be in grace period")

		// TODO: Readd this test once Boundary is enforced as an add-on license.
		// boundaryFeature := entitlements.Features[codersdk.FeatureBoundary]
		// require.True(t, boundaryFeature.Enabled, "Boundary should be enabled during grace period")
		// require.Equal(t, codersdk.EntitlementGracePeriod, boundaryFeature.Entitlement, "Boundary should be in grace period")
	})

	t.Run("AIGovernanceAddon requires enablements to enable features", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		db.InsertLicense(context.Background(), database.InsertLicenseParams{
			JWT: coderdenttest.GenerateLicense(t, coderdenttest.LicenseOptions{
				FeatureSet: codersdk.FeatureSetPremium,
				Features: license.Features{
					codersdk.FeatureAIGovernanceUserLimit: 1000,
					codersdk.FeatureManagedAgentLimit:     1000,
				},
				Addons: []codersdk.Addon{codersdk.AddonAIGovernance},
			}),
			Exp: dbtime.Now().Add(time.Hour),
		})

		entitlements, err := license.Entitlements(context.Background(), testutil.Logger(t), db, 1, 1, coderdenttest.Keys, empty, testAuthorizer, nil)
		require.NoError(t, err)
		require.True(t, entitlements.HasLicense)

		// TODO: Readd this test once AI Bridge is enforced as an add-on license.
		// aibridgeFeature := entitlements.Features[codersdk.FeatureAIBridge]
		// require.False(t, aibridgeFeature.Enabled, "AI Bridge should not be enabled without enablements")
		// require.Equal(t, codersdk.EntitlementEntitled, aibridgeFeature.Entitlement, "AI Bridge should still be entitled")

		// TODO: Readd this test once Boundary is enforced as an add-on license.
		// boundaryFeature := entitlements.Features[codersdk.FeatureBoundary]
		// require.False(t, boundaryFeature.Enabled, "Boundary should not be enabled without enablements")
		// require.Equal(t, codersdk.EntitlementEntitled, boundaryFeature.Entitlement, "Boundary should still be entitled")
	})

	t.Run("AIGovernanceAddon missing dependencies", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		// Use Enterprise so ManagedAgentLimit doesn't get default value, and
		// don't set either dependency.
		db.InsertLicense(context.Background(), database.InsertLicenseParams{
			JWT: coderdenttest.GenerateLicense(t, coderdenttest.LicenseOptions{
				FeatureSet: codersdk.FeatureSetEnterprise,
				Features:   license.Features{},
				Addons:     []codersdk.Addon{codersdk.AddonAIGovernance},
			}),
			Exp: dbtime.Now().Add(time.Hour),
		})

		enablements := map[codersdk.FeatureName]bool{
			codersdk.FeatureAIBridge: true,
			codersdk.FeatureBoundary: true,
		}
		entitlements, err := license.Entitlements(context.Background(), testutil.Logger(t), db, 1, 1, coderdenttest.Keys, enablements, testAuthorizer, nil)
		require.NoError(t, err)
		require.True(t, entitlements.HasLicense)

		// Should have validation error for missing AI Governance User Limit.
		require.Len(t, entitlements.Errors, 1)
		require.Equal(t, "Feature AI Governance User Limit must be set when using the AI Governance addon.", entitlements.Errors[0])

		// TODO: Readd this test once AI Bridge is enforced as an add-on license.
		// AI Governance features should not be entitled when validation fails.
		// aibridgeFeature := entitlements.Features[codersdk.FeatureAIBridge]
		// require.False(t, aibridgeFeature.Enabled, "AI Bridge should not be enabled when addon validation fails")
		// require.Equal(t, codersdk.EntitlementNotEntitled, aibridgeFeature.Entitlement, "AI Bridge should not be entitled when addon validation fails")

		// TODO: Readd this test once Boundary is enforced as an add-on license.
		// boundaryFeature := entitlements.Features[codersdk.FeatureBoundary]
		// require.False(t, boundaryFeature.Enabled, "Boundary should not be enabled when addon validation fails")
		// require.Equal(t, codersdk.EntitlementNotEntitled, boundaryFeature.Entitlement, "Boundary should not be entitled when addon validation fails")
	})
}

func TestUsagePublishingStatus(t *testing.T) {
	t.Parallel()

	empty := map[codersdk.FeatureName]bool{}

	// Seeds a usage event in the given state. A zero publishedAt means the
	// event has not been published. A publishedAt with a failureMessage is a
	// permanent rejection.
	seedEvent := func(ctx context.Context, t *testing.T, db database.Store, id string, createdAt, publishedAt time.Time, failureMessage string) {
		t.Helper()
		err := db.InsertUsageEvent(ctx, database.InsertUsageEventParams{
			ID:        id,
			EventType: "dc_managed_agents_v1",
			EventData: []byte(`{"count": 1}`),
			CreatedAt: createdAt,
			// Stuckness is measured against insertion time, so tests that
			// seed stuck events need the insertion time backdated too.
			InsertedAt: createdAt,
		})
		require.NoError(t, err)
		if !publishedAt.IsZero() || failureMessage != "" {
			err = db.UpdateUsageEventsPostPublish(ctx, database.UpdateUsageEventsPostPublishParams{
				Now:             publishedAt,
				IDs:             []string{id},
				FailureMessages: []string{failureMessage},
				SetPublishedAts: []bool{!publishedAt.IsZero()},
			})
			require.NoError(t, err)
		}
	}

	insertPublishingLicense := func(ctx context.Context, t *testing.T, db database.Store, opts coderdenttest.LicenseOptions) {
		t.Helper()
		opts.PublishUsageData = true
		if opts.NotBefore.IsZero() {
			opts.NotBefore = time.Now().Add(-90 * 24 * time.Hour)
		}
		_, err := db.InsertLicense(ctx, database.InsertLicenseParams{
			JWT: coderdenttest.GenerateLicense(t, opts),
			Exp: dbtime.Now().Add(time.Hour),
		})
		require.NoError(t, err)
		// Backdate the enabled-since marker so seeded failures are not
		// clamped into the post-enablement grace period, as if publishing
		// had been enabled and observed for the whole license term.
		marker, err := json.Marshal(map[string]time.Time{
			"enabled_since": opts.NotBefore.UTC(),
			"last_seen":     time.Now().UTC(),
		})
		require.NoError(t, err)
		//nolint:gocritic // Unit test.
		err = db.UpsertRuntimeConfig(dbauthz.AsSystemRestricted(ctx), database.UpsertRuntimeConfigParams{
			Key:   license.UsagePublishingEnabledSinceKey,
			Value: string(marker),
		})
		require.NoError(t, err)
	}

	t.Run("Healthy", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()
		db, _ := dbtestutil.NewDB(t)
		insertPublishingLicense(ctx, t, db, coderdenttest.LicenseOptions{})
		now := time.Now()
		seedEvent(ctx, t, db, "1", now.Add(-2*time.Hour), now.Add(-time.Hour), "")

		entitlements, err := license.Entitlements(ctx, testutil.Logger(t), db, 1, 1, coderdenttest.Keys, empty, testAuthorizer, nil)
		require.NoError(t, err)
		require.NotContains(t, entitlements.Warnings, codersdk.LicenseUsagePublishingFailingWarningText)
		require.True(t, entitlements.UsagePublishing.PublishingEnabled)
		require.NotNil(t, entitlements.UsagePublishing.LastPublishedAt)
		require.WithinDuration(t, now.Add(-time.Hour), *entitlements.UsagePublishing.LastPublishedAt, time.Second)
		require.Nil(t, entitlements.UsagePublishing.FailingSince)
	})

	t.Run("FailingStuckEventsThenRecovers", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()
		db, _ := dbtestutil.NewDB(t)
		insertPublishingLicense(ctx, t, db, coderdenttest.LicenseOptions{})
		now := time.Now()
		seedEvent(ctx, t, db, "1", now.Add(-25*time.Hour), time.Time{}, "")

		entitlements, err := license.Entitlements(ctx, testutil.Logger(t), db, 1, 1, coderdenttest.Keys, empty, testAuthorizer, nil)
		require.NoError(t, err)
		require.Contains(t, entitlements.Warnings, codersdk.LicenseUsagePublishingFailingWarningText)
		require.True(t, entitlements.UsagePublishing.PublishingEnabled)
		require.Nil(t, entitlements.UsagePublishing.LastPublishedAt)
		require.NotNil(t, entitlements.UsagePublishing.FailingSince)
		require.WithinDuration(t, now.Add(-25*time.Hour), *entitlements.UsagePublishing.FailingSince, time.Second)

		// Simulate the outage recovering: the event gets published and the
		// next entitlements refresh clears the warning.
		err = db.UpdateUsageEventsPostPublish(ctx, database.UpdateUsageEventsPostPublishParams{
			Now:             now,
			IDs:             []string{"1"},
			FailureMessages: []string{""},
			SetPublishedAts: []bool{true},
		})
		require.NoError(t, err)

		entitlements, err = license.Entitlements(ctx, testutil.Logger(t), db, 1, 1, coderdenttest.Keys, empty, testAuthorizer, nil)
		require.NoError(t, err)
		require.NotContains(t, entitlements.Warnings, codersdk.LicenseUsagePublishingFailingWarningText)
		require.Nil(t, entitlements.UsagePublishing.FailingSince)
		require.NotNil(t, entitlements.UsagePublishing.LastPublishedAt)
	})

	t.Run("FailingPermanentRejection", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()
		db, _ := dbtestutil.NewDB(t)
		insertPublishingLicense(ctx, t, db, coderdenttest.LicenseOptions{})
		now := time.Now()
		seedEvent(ctx, t, db, "1", now.Add(-2*time.Hour), now.Add(-time.Hour), "permanently rejected")

		entitlements, err := license.Entitlements(ctx, testutil.Logger(t), db, 1, 1, coderdenttest.Keys, empty, testAuthorizer, nil)
		require.NoError(t, err)
		require.Contains(t, entitlements.Warnings, codersdk.LicenseUsagePublishingFailingWarningText)
		require.True(t, entitlements.UsagePublishing.PublishingEnabled)
		// Permanent rejections don't count as successful publishes.
		require.Nil(t, entitlements.UsagePublishing.LastPublishedAt)
		require.NotNil(t, entitlements.UsagePublishing.FailingSince)
		require.WithinDuration(t, now.Add(-time.Hour), *entitlements.UsagePublishing.FailingSince, time.Second)
	})

	t.Run("AirGappedClaimFalseNeverWarns", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()
		db, _ := dbtestutil.NewDB(t)
		_, err := db.InsertLicense(ctx, database.InsertLicenseParams{
			JWT: coderdenttest.GenerateLicense(t, coderdenttest.LicenseOptions{
				PublishUsageData: false,
			}),
			Exp: dbtime.Now().Add(time.Hour),
		})
		require.NoError(t, err)
		// Even with events that would otherwise be considered stuck.
		now := time.Now()
		seedEvent(ctx, t, db, "1", now.Add(-25*time.Hour), time.Time{}, "")

		entitlements, err := license.Entitlements(ctx, testutil.Logger(t), db, 1, 1, coderdenttest.Keys, empty, testAuthorizer, nil)
		require.NoError(t, err)
		require.NotContains(t, entitlements.Warnings, codersdk.LicenseUsagePublishingFailingWarningText)
		require.False(t, entitlements.UsagePublishing.PublishingEnabled)
		require.Nil(t, entitlements.UsagePublishing.LastPublishedAt)
		require.Nil(t, entitlements.UsagePublishing.FailingSince)
	})

	t.Run("StaleRefreshDoesNotMoveMarkerBackward", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()
		db, _ := dbtestutil.NewDB(t)
		insertPublishingLicense(ctx, t, db, coderdenttest.LicenseOptions{})

		// Simulate a concurrent replica having just stamped a fresher
		// observation: last_seen ahead of this refresh's now. The stalled
		// refresh must not move it backward or reset enabled_since.
		enabledSince := time.Now().Add(-90 * 24 * time.Hour).UTC()
		lastSeen := time.Now().Add(time.Hour).UTC()
		marker, err := json.Marshal(map[string]time.Time{
			"enabled_since": enabledSince,
			"last_seen":     lastSeen,
		})
		require.NoError(t, err)
		//nolint:gocritic // Unit test.
		err = db.UpsertRuntimeConfig(dbauthz.AsSystemRestricted(ctx), database.UpsertRuntimeConfigParams{
			Key:   license.UsagePublishingEnabledSinceKey,
			Value: string(marker),
		})
		require.NoError(t, err)

		_, err = license.Entitlements(ctx, testutil.Logger(t), db, 1, 1, coderdenttest.Keys, empty, testAuthorizer, nil)
		require.NoError(t, err)

		//nolint:gocritic // Unit test.
		raw, err := db.GetRuntimeConfig(dbauthz.AsSystemRestricted(ctx), license.UsagePublishingEnabledSinceKey)
		require.NoError(t, err)
		var got map[string]time.Time
		require.NoError(t, json.Unmarshal([]byte(raw), &got))
		require.True(t, got["enabled_since"].Equal(enabledSince), "enabled_since must be unchanged")
		require.True(t, got["last_seen"].Equal(lastSeen), "last_seen must not move backward")
	})

	t.Run("NoLicenseNeverWarns", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()
		db, _ := dbtestutil.NewDB(t)
		now := time.Now()
		seedEvent(ctx, t, db, "1", now.Add(-25*time.Hour), time.Time{}, "")

		entitlements, err := license.Entitlements(ctx, testutil.Logger(t), db, 1, 1, coderdenttest.Keys, empty, testAuthorizer, nil)
		require.NoError(t, err)
		require.NotContains(t, entitlements.Warnings, codersdk.LicenseUsagePublishingFailingWarningText)
		require.False(t, entitlements.UsagePublishing.PublishingEnabled)
		require.Nil(t, entitlements.UsagePublishing.LastPublishedAt)
		require.Nil(t, entitlements.UsagePublishing.FailingSince)
	})

	t.Run("NonSalesforceAccountDisabled", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()
		db, _ := dbtestutil.NewDB(t)
		_, err := db.InsertLicense(ctx, database.InsertLicenseParams{
			JWT: coderdenttest.GenerateLicense(t, coderdenttest.LicenseOptions{
				AccountType:      "developer",
				PublishUsageData: true,
			}),
			Exp: dbtime.Now().Add(time.Hour),
		})
		require.NoError(t, err)
		now := time.Now()
		seedEvent(ctx, t, db, "1", now.Add(-25*time.Hour), time.Time{}, "")

		entitlements, err := license.Entitlements(ctx, testutil.Logger(t), db, 1, 1, coderdenttest.Keys, empty, testAuthorizer, nil)
		require.NoError(t, err)
		require.NotContains(t, entitlements.Warnings, codersdk.LicenseUsagePublishingFailingWarningText)
		require.False(t, entitlements.UsagePublishing.PublishingEnabled)
	})

	t.Run("EventsBeforeLicenseStartIgnored", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()
		db, _ := dbtestutil.NewDB(t)
		now := time.Now()
		insertPublishingLicense(ctx, t, db, coderdenttest.LicenseOptions{
			NotBefore: now.Add(-time.Hour),
			GraceAt:   now.Add(time.Hour),
			ExpiresAt: now.Add(time.Hour),
		})
		// Stuck event from before the license term started.
		seedEvent(ctx, t, db, "1", now.Add(-25*time.Hour), time.Time{}, "")

		entitlements, err := license.Entitlements(ctx, testutil.Logger(t), db, 1, 1, coderdenttest.Keys, empty, testAuthorizer, nil)
		require.NoError(t, err)
		require.NotContains(t, entitlements.Warnings, codersdk.LicenseUsagePublishingFailingWarningText)
		require.True(t, entitlements.UsagePublishing.PublishingEnabled)
		require.Nil(t, entitlements.UsagePublishing.FailingSince)
	})

	t.Run("EventsOlderThanPublishWindowIgnored", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()
		db, _ := dbtestutil.NewDB(t)
		now := time.Now()
		insertPublishingLicense(ctx, t, db, coderdenttest.LicenseOptions{
			NotBefore: now.Add(-90 * 24 * time.Hour),
			GraceAt:   now.Add(time.Hour),
			ExpiresAt: now.Add(time.Hour),
		})
		// The publisher never publishes events older than 30 days, so they
		// must not trigger a permanently-stuck warning.
		seedEvent(ctx, t, db, "1", now.Add(-31*24*time.Hour), time.Time{}, "")

		entitlements, err := license.Entitlements(ctx, testutil.Logger(t), db, 1, 1, coderdenttest.Keys, empty, testAuthorizer, nil)
		require.NoError(t, err)
		require.NotContains(t, entitlements.Warnings, codersdk.LicenseUsagePublishingFailingWarningText)
		require.True(t, entitlements.UsagePublishing.PublishingEnabled)
		require.Nil(t, entitlements.UsagePublishing.FailingSince)
	})

	t.Run("LastPublishedAtIgnoresRejections", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()
		db, _ := dbtestutil.NewDB(t)
		insertPublishingLicense(ctx, t, db, coderdenttest.LicenseOptions{})
		now := time.Now()
		// A successful publish followed by an old permanent rejection
		// (outside the failure threshold, so no warning).
		seedEvent(ctx, t, db, "1", now.Add(-73*time.Hour), now.Add(-72*time.Hour), "")
		seedEvent(ctx, t, db, "2", now.Add(-49*time.Hour), now.Add(-48*time.Hour), "permanently rejected")

		entitlements, err := license.Entitlements(ctx, testutil.Logger(t), db, 1, 1, coderdenttest.Keys, empty, testAuthorizer, nil)
		require.NoError(t, err)
		require.NotContains(t, entitlements.Warnings, codersdk.LicenseUsagePublishingFailingWarningText)
		require.True(t, entitlements.UsagePublishing.PublishingEnabled)
		require.NotNil(t, entitlements.UsagePublishing.LastPublishedAt)
		require.WithinDuration(t, now.Add(-72*time.Hour), *entitlements.UsagePublishing.LastPublishedAt, time.Second)
		require.Nil(t, entitlements.UsagePublishing.FailingSince)
	})
}

func assertNoErrors(t *testing.T, entitlements codersdk.Entitlements) {
	t.Helper()
	assert.Empty(t, entitlements.Errors, "no errors")
}

func assertNoWarnings(t *testing.T, entitlements codersdk.Entitlements) {
	t.Helper()
	assert.Empty(t, entitlements.Warnings, "no warnings")
}

func assertEnterpriseFeatures(t *testing.T, entitlements codersdk.Entitlements) {
	t.Helper()
	for _, expected := range codersdk.FeatureSetEnterprise.Features() {
		f := entitlements.Features[expected]
		assert.Equalf(t, codersdk.EntitlementEntitled, f.Entitlement, "%s entitled", expected)
		assert.Equalf(t, true, f.Enabled, "%s enabled", expected)
	}
}
