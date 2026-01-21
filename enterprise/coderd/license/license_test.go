package license_test

import (
	"context"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
)

func TestEntitlements(t *testing.T) {
	t.Parallel()
	all := make(map[codersdk.FeatureName]bool)
	for _, n := range codersdk.FeatureNames {
		all[n] = true
	}

	premium := make(map[codersdk.FeatureName]bool)
	for _, n := range codersdk.FeatureSetPremium.Features() {
		premium[n] = !n.IsAddonFeature()
	}

	empty := map[codersdk.FeatureName]bool{}

	t.Run("Defaults", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		entitlements, err := license.Entitlements(context.Background(), db, 1, 1, coderdenttest.Keys, all)
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
		entitlements, err := license.Entitlements(context.Background(), db, 1, 1, coderdenttest.Keys, all)
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
		entitlements, err := license.Entitlements(context.Background(), db, 1, 1, coderdenttest.Keys, empty)
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
							f[codersdk.FeatureName("managed_agent_limit_soft")] = 100
							f[codersdk.FeatureName("managed_agent_limit_hard")] = 200
							continue
						}
						f[name] = 1
					}
					return f
				}(),
			}),
			Exp: dbtime.Now().Add(time.Hour),
		})
		entitlements, err := license.Entitlements(context.Background(), db, 1, 1, coderdenttest.Keys, empty)
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
		entitlements, err := license.Entitlements(context.Background(), db, 1, 1, coderdenttest.Keys, all)
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

		entitlements, err := license.Entitlements(context.Background(), db, 1, 1, coderdenttest.Keys, all)

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

		entitlements, err := license.Entitlements(context.Background(), db, 1, 1, coderdenttest.Keys, all)

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
					codersdk.FeatureUserLimit: 100,
					codersdk.FeatureAuditLog:  1,
				},
				FeatureSet: codersdk.FeatureSetPremium,
				GraceAt:    graceDate,
				ExpiresAt:  dbtime.Now().AddDate(0, 0, 5),
			}),
			Exp: time.Now().AddDate(0, 0, 5),
		})
		require.NoError(t, err)

		// Warning should be generated.
		entitlements, err := license.Entitlements(context.Background(), db, 1, 1, coderdenttest.Keys, premium)
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
					codersdk.FeatureUserLimit: 100,
					codersdk.FeatureAuditLog:  1,
				},

				FeatureSet: codersdk.FeatureSetPremium,
				NotBefore:  graceDate.Add(-time.Hour), // contiguous, and also in the future
				GraceAt:    dbtime.Now().AddDate(1, 0, 0),
				ExpiresAt:  dbtime.Now().AddDate(1, 0, 5),
			}),
			Exp: dbtime.Now().AddDate(1, 0, 5),
		})
		require.NoError(t, err)

		// Warning should be suppressed.
		entitlements, err = license.Entitlements(context.Background(), db, 1, 1, coderdenttest.Keys, premium)
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
					codersdk.FeatureAIGovernanceUserLimit: 1000,
					codersdk.FeatureManagedAgentLimit:     1000,
				},

				FeatureSet: codersdk.FeatureSetPremium,
				Addons:     []codersdk.Addon{codersdk.AddonAIGovernance},
				GraceAt:    graceDate,
				ExpiresAt:  dbtime.Now().AddDate(0, 0, 5),
			}),
			Exp: time.Now().AddDate(0, 0, 5),
		})
		require.NoError(t, err)

		// Should generate a warning.
		entitlements, err := license.Entitlements(context.Background(), db, 1, 1, coderdenttest.Keys, all)
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
					codersdk.FeatureUserLimit: 100,
					codersdk.FeatureAuditLog:  1,
				},

				FeatureSet: codersdk.FeatureSetPremium,
				NotBefore:  graceDate.Add(time.Minute), // gap of 1 second!
				GraceAt:    dbtime.Now().AddDate(1, 0, 0),
				ExpiresAt:  dbtime.Now().AddDate(1, 0, 5),
			}),
			Exp: dbtime.Now().AddDate(1, 0, 5),
		})
		require.NoError(t, err)

		// Warning should still be generated.
		entitlements, err = license.Entitlements(context.Background(), db, 1, 1, coderdenttest.Keys, all)
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

		entitlements, err := license.Entitlements(context.Background(), db, 1, 1, coderdenttest.Keys, all)

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

		entitlements, err := license.Entitlements(context.Background(), db, 1, 1, coderdenttest.Keys, all)

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
		entitlements, err := license.Entitlements(context.Background(), db, 1, 1, coderdenttest.Keys, all)
		require.NoError(t, err)
		require.True(t, entitlements.HasLicense)
		require.False(t, entitlements.Trial)
		for _, featureName := range codersdk.FeatureNames {
			if featureName == codersdk.FeatureUserLimit ||
				featureName == codersdk.FeatureHighAvailability ||
				featureName == codersdk.FeatureMultipleExternalAuth ||
				featureName == codersdk.FeatureManagedAgentLimit {
				// These fields don't generate warnings when not entitled unless
				// a limit is breached.
				continue
			}
			niceName := featureName.Humanize()
			// Ensures features that are not entitled are properly disabled.
			require.False(t, entitlements.Features[featureName].Enabled)
			require.Equal(t, codersdk.EntitlementNotEntitled, entitlements.Features[featureName].Entitlement)
			require.Contains(t, entitlements.Warnings, fmt.Sprintf("%s is enabled but your license is not entitled to this feature.", niceName))
		}
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
		entitlements, err := license.Entitlements(context.Background(), db, 1, 1, coderdenttest.Keys, empty)
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
		entitlements, err := license.Entitlements(context.Background(), db, 1, 1, coderdenttest.Keys, empty)
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

		entitlements, err := license.Entitlements(context.Background(), db, 1, 1, coderdenttest.Keys, empty)
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
		entitlements, err := license.Entitlements(context.Background(), db, 1, 1, coderdenttest.Keys, all)
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
		const expectedAgentSoftLimit = 1000
		const expectedAgentHardLimit = 1000

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
		entitlements, err := license.Entitlements(context.Background(), db, 1, 1, coderdenttest.Keys, all)
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
				require.EqualValues(t, expectedAgentSoftLimit, *agentEntitlement.SoftLimit)
				require.EqualValues(t, expectedAgentHardLimit, *agentEntitlement.Limit)

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
		entitlements, err := license.Entitlements(context.Background(), db, 1, 1, coderdenttest.Keys, all)
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
		entitlements, err := license.Entitlements(context.Background(), db, 1, 1, coderdenttest.Keys, all)
		require.NoError(t, err)
		require.True(t, entitlements.HasLicense)
		require.False(t, entitlements.Trial)

		// All enterprise features should be entitled
		enterpriseFeatures := codersdk.FeatureSetEnterprise.Features()
		for _, featureName := range codersdk.FeatureNames {
			if featureName.UsesLimit() {
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
		entitlements, err := license.Entitlements(context.Background(), db, 1, 1, coderdenttest.Keys, empty)
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
		entitlements, err := license.Entitlements(context.Background(), db, 1, 1, coderdenttest.Keys, all)
		require.NoError(t, err)
		require.True(t, entitlements.HasLicense)
		require.False(t, entitlements.Trial)
		// All enterprise features should be entitled
		enterpriseFeatures := codersdk.FeatureSetEnterprise.Features()
		for _, featureName := range codersdk.FeatureNames {
			if featureName == codersdk.FeatureUserLimit {
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
		entitlements, err := license.Entitlements(context.Background(), db, 2, 1, coderdenttest.Keys, all)
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
		entitlements, err := license.Entitlements(context.Background(), db, 2, 1, coderdenttest.Keys, map[codersdk.FeatureName]bool{
			codersdk.FeatureHighAvailability: true,
		})
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
				NotBefore: time.Now().Add(-time.Hour * 2),
				GraceAt:   time.Now().Add(-time.Hour),
				ExpiresAt: time.Now().Add(time.Hour),
			}),
			Exp: time.Now().Add(time.Hour),
		})
		entitlements, err := license.Entitlements(context.Background(), db, 2, 1, coderdenttest.Keys, map[codersdk.FeatureName]bool{
			codersdk.FeatureHighAvailability: true,
		})
		require.NoError(t, err)
		require.True(t, entitlements.HasLicense)
		require.Len(t, entitlements.Warnings, 1)
		require.Equal(t, "You have multiple replicas but your license for high availability is expired. Reduce to one replica or workspace connections will stop working.", entitlements.Warnings[0])
	})

	t.Run("MultipleGitAuthNoLicense", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		entitlements, err := license.Entitlements(context.Background(), db, 1, 2, coderdenttest.Keys, all)
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
		entitlements, err := license.Entitlements(context.Background(), db, 1, 2, coderdenttest.Keys, map[codersdk.FeatureName]bool{
			codersdk.FeatureMultipleExternalAuth: true,
		})
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
				NotBefore: time.Now().Add(-time.Hour * 2),
				GraceAt:   time.Now().Add(-time.Hour),
				ExpiresAt: time.Now().Add(time.Hour),
				Features: license.Features{
					codersdk.FeatureMultipleExternalAuth: 1,
				},
			}),
			Exp: time.Now().Add(time.Hour),
		})
		entitlements, err := license.Entitlements(context.Background(), db, 1, 2, coderdenttest.Keys, map[codersdk.FeatureName]bool{
			codersdk.FeatureMultipleExternalAuth: true,
		})
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

		licenseOpts := (&coderdenttest.LicenseOptions{
			FeatureSet: codersdk.FeatureSetPremium,
			IssuedAt:   dbtime.Now().Add(-2 * time.Hour).Truncate(time.Second),
			NotBefore:  dbtime.Now().Add(-time.Hour).Truncate(time.Second),
			GraceAt:    dbtime.Now().Add(time.Hour * 24 * 60).Truncate(time.Second), // 60 days to remove warning
			ExpiresAt:  dbtime.Now().Add(time.Hour * 24 * 90).Truncate(time.Second), // 90 days to remove warning
		}).
			AIGovernanceAddon(1000).
			UserLimit(100).
			ManagedAgentLimit(100, 200)

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
		mDB.EXPECT().
			GetTemplatesWithFilter(gomock.Any(), gomock.Any()).
			Return([]database.Template{}, nil)

		entitlements, err := license.Entitlements(context.Background(), mDB, 1, 0, coderdenttest.Keys, all)
		require.NoError(t, err)
		require.True(t, entitlements.HasLicense)

		managedAgentLimit, ok := entitlements.Features[codersdk.FeatureManagedAgentLimit]
		require.True(t, ok)

		require.NotNil(t, managedAgentLimit.SoftLimit)
		require.EqualValues(t, 100, *managedAgentLimit.SoftLimit)
		require.NotNil(t, managedAgentLimit.Limit)
		require.EqualValues(t, 200, *managedAgentLimit.Limit)
		require.NotNil(t, managedAgentLimit.Actual)
		require.EqualValues(t, 175, *managedAgentLimit.Actual)

		// Should've also populated a warning.
		require.Len(t, entitlements.Warnings, 1)
		require.Equal(t, "You are approaching the managed agent limit in your license. Please refer to the Deployment Licenses page for more information.", entitlements.Warnings[0])
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
	}

	legacyLicense := func() *coderdenttest.LicenseOptions {
		return (&coderdenttest.LicenseOptions{
			AccountType: "salesforce",
			AccountID:   "Alice",
			Trial:       false,
			// Use the legacy boolean
			AllFeatures: true,
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
		}).Valid(time.Now())
	}

	premiumLicense := func() *coderdenttest.LicenseOptions {
		return (&coderdenttest.LicenseOptions{
			AccountType:   "salesforce",
			AccountID:     "Charlie",
			DeploymentIDs: nil,
			Trial:         false,
			FeatureSet:    codersdk.FeatureSetPremium,
			AllFeatures:   true,
		}).Valid(time.Now())
	}

	testCases := []struct {
		Name        string
		Licenses    []*coderdenttest.LicenseOptions
		Enablements map[codersdk.FeatureName]bool
		Arguments   license.FeatureArguments

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
				enterpriseLicense().UserLimit(100).ManagedAgentLimit(100, 200),
			},
			Arguments: license.FeatureArguments{
				ManagedAgentCountFn: func(ctx context.Context, from time.Time, to time.Time) (int64, error) {
					// 175 will generate a warning as it's over 75% of the
					// difference between the soft and hard limit.
					return 174, nil
				},
			},
			AssertEntitlements: func(t *testing.T, entitlements codersdk.Entitlements) {
				assertNoErrors(t, entitlements)
				assertNoWarnings(t, entitlements)
				feature := entitlements.Features[codersdk.FeatureManagedAgentLimit]
				assert.Equal(t, codersdk.EntitlementEntitled, feature.Entitlement)
				assert.True(t, feature.Enabled)
				assert.Equal(t, int64(100), *feature.SoftLimit)
				assert.Equal(t, int64(200), *feature.Limit)
				assert.Equal(t, int64(174), *feature.Actual)
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
					ManagedAgentLimit(100, 100).
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
				assert.Equal(t, int64(100), *feature.SoftLimit)
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
					ManagedAgentLimit(100, 200).
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
				assert.Nil(t, feature.SoftLimit)
				assert.Nil(t, feature.Limit)
				assert.Nil(t, feature.Actual)
			},
		},
		{
			Name: "ManagedAgentLimitWarning/ApproachingLimit/DifferentSoftAndHardLimit",
			Licenses: []*coderdenttest.LicenseOptions{
				enterpriseLicense().
					UserLimit(100).
					ManagedAgentLimit(100, 200),
			},
			Arguments: license.FeatureArguments{
				ManagedAgentCountFn: func(ctx context.Context, from time.Time, to time.Time) (int64, error) {
					return 175, nil
				},
			},
			AssertEntitlements: func(t *testing.T, entitlements codersdk.Entitlements) {
				assert.Len(t, entitlements.Warnings, 1)
				assert.Equal(t, "You are approaching the managed agent limit in your license. Please refer to the Deployment Licenses page for more information.", entitlements.Warnings[0])
				assertNoErrors(t, entitlements)

				feature := entitlements.Features[codersdk.FeatureManagedAgentLimit]
				assert.Equal(t, codersdk.EntitlementEntitled, feature.Entitlement)
				assert.True(t, feature.Enabled)
				assert.Equal(t, int64(100), *feature.SoftLimit)
				assert.Equal(t, int64(200), *feature.Limit)
				assert.Equal(t, int64(175), *feature.Actual)
			},
		},
		{
			Name: "ManagedAgentLimitWarning/ApproachingLimit/EqualSoftAndHardLimit",
			Licenses: []*coderdenttest.LicenseOptions{
				enterpriseLicense().
					UserLimit(100).
					ManagedAgentLimit(100, 100),
			},
			Arguments: license.FeatureArguments{
				ManagedAgentCountFn: func(ctx context.Context, from time.Time, to time.Time) (int64, error) {
					return 75, nil
				},
			},
			AssertEntitlements: func(t *testing.T, entitlements codersdk.Entitlements) {
				assert.Len(t, entitlements.Warnings, 1)
				assert.Equal(t, "You are approaching the managed agent limit in your license. Please refer to the Deployment Licenses page for more information.", entitlements.Warnings[0])
				assertNoErrors(t, entitlements)

				feature := entitlements.Features[codersdk.FeatureManagedAgentLimit]
				assert.Equal(t, codersdk.EntitlementEntitled, feature.Entitlement)
				assert.True(t, feature.Enabled)
				assert.Equal(t, int64(100), *feature.SoftLimit)
				assert.Equal(t, int64(100), *feature.Limit)
				assert.Equal(t, int64(75), *feature.Actual)
			},
		},
		{
			Name: "ManagedAgentLimitWarning/BreachedLimit",
			Licenses: []*coderdenttest.LicenseOptions{
				enterpriseLicense().
					UserLimit(100).
					ManagedAgentLimit(100, 200),
			},
			Arguments: license.FeatureArguments{
				ManagedAgentCountFn: func(ctx context.Context, from time.Time, to time.Time) (int64, error) {
					return 200, nil
				},
			},
			AssertEntitlements: func(t *testing.T, entitlements codersdk.Entitlements) {
				assert.Len(t, entitlements.Warnings, 1)
				assert.Equal(t, "You have built more workspaces with managed agents than your license allows. Further managed agent builds will be blocked.", entitlements.Warnings[0])
				assertNoErrors(t, entitlements)

				feature := entitlements.Features[codersdk.FeatureManagedAgentLimit]
				assert.Equal(t, codersdk.EntitlementEntitled, feature.Entitlement)
				assert.True(t, feature.Enabled)
				assert.Equal(t, int64(100), *feature.SoftLimit)
				assert.Equal(t, int64(200), *feature.Limit)
				assert.Equal(t, int64(200), *feature.Actual)
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

			entitlements, err := license.LicensesEntitlements(context.Background(), time.Now(), generatedLicenses, tc.Enablements, coderdenttest.Keys, tc.Arguments)
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

func TestUsageLimitFeatures(t *testing.T) {
	t.Parallel()

	cases := []struct {
		sdkFeatureName       codersdk.FeatureName
		softLimitFeatureName codersdk.FeatureName
		hardLimitFeatureName codersdk.FeatureName
	}{
		{
			sdkFeatureName:       codersdk.FeatureManagedAgentLimit,
			softLimitFeatureName: codersdk.FeatureName("managed_agent_limit_soft"),
			hardLimitFeatureName: codersdk.FeatureName("managed_agent_limit_hard"),
		},
	}

	for _, c := range cases {
		t.Run(string(c.sdkFeatureName), func(t *testing.T) {
			t.Parallel()

			// Test for either a missing soft or hard limit feature value.
			t.Run("MissingGroupedFeature", func(t *testing.T) {
				t.Parallel()

				for _, feature := range []codersdk.FeatureName{
					c.softLimitFeatureName,
					c.hardLimitFeatureName,
				} {
					t.Run(string(feature), func(t *testing.T) {
						t.Parallel()

						lic := database.License{
							ID:         1,
							UploadedAt: time.Now(),
							Exp:        time.Now().Add(time.Hour),
							UUID:       uuid.New(),
							JWT: coderdenttest.GenerateLicense(t, coderdenttest.LicenseOptions{
								Features: license.Features{
									feature: 100,
								},
							}),
						}

						arguments := license.FeatureArguments{
							ManagedAgentCountFn: func(ctx context.Context, from time.Time, to time.Time) (int64, error) {
								return 0, nil
							},
						}
						entitlements, err := license.LicensesEntitlements(context.Background(), time.Now(), []database.License{lic}, map[codersdk.FeatureName]bool{}, coderdenttest.Keys, arguments)
						require.NoError(t, err)

						feature, ok := entitlements.Features[c.sdkFeatureName]
						require.True(t, ok, "feature %s not found", c.sdkFeatureName)
						require.Equal(t, codersdk.EntitlementNotEntitled, feature.Entitlement)

						require.Len(t, entitlements.Errors, 1)
						require.Equal(t, fmt.Sprintf("Invalid license (%v): feature %s has missing soft or hard limit values", lic.UUID, c.sdkFeatureName), entitlements.Errors[0])
					})
				}
			})

			t.Run("HardBelowSoft", func(t *testing.T) {
				t.Parallel()

				lic := database.License{
					ID:         1,
					UploadedAt: time.Now(),
					Exp:        time.Now().Add(time.Hour),
					UUID:       uuid.New(),
					JWT: coderdenttest.GenerateLicense(t, coderdenttest.LicenseOptions{
						Features: license.Features{
							c.softLimitFeatureName: 100,
							c.hardLimitFeatureName: 50,
						},
					}),
				}

				arguments := license.FeatureArguments{
					ManagedAgentCountFn: func(ctx context.Context, from time.Time, to time.Time) (int64, error) {
						return 0, nil
					},
				}
				entitlements, err := license.LicensesEntitlements(context.Background(), time.Now(), []database.License{lic}, map[codersdk.FeatureName]bool{}, coderdenttest.Keys, arguments)
				require.NoError(t, err)

				feature, ok := entitlements.Features[c.sdkFeatureName]
				require.True(t, ok, "feature %s not found", c.sdkFeatureName)
				require.Equal(t, codersdk.EntitlementNotEntitled, feature.Entitlement)

				require.Len(t, entitlements.Errors, 1)
				require.Equal(t, fmt.Sprintf("Invalid license (%v): feature %s has a hard limit less than the soft limit", lic.UUID, c.sdkFeatureName), entitlements.Errors[0])
			})

			// Ensures that these features are ranked by issued at, not by
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
						NotBefore: time.Now().Add(-time.Minute * 2),
						ExpiresAt: time.Now().Add(time.Hour * 2),
						Features: license.Features{
							c.softLimitFeatureName: 100,
							c.hardLimitFeatureName: 200,
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
							c.softLimitFeatureName: 50,
							c.hardLimitFeatureName: 100,
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

					feature, ok := entitlements.Features[c.sdkFeatureName]
					require.True(t, ok, "feature %s not found", c.sdkFeatureName)
					require.Equal(t, codersdk.EntitlementEntitled, feature.Entitlement)
					require.NotNil(t, feature.Limit)
					require.EqualValues(t, 100, *feature.Limit)
					require.NotNil(t, feature.SoftLimit)
					require.EqualValues(t, 50, *feature.SoftLimit)
					require.NotNil(t, feature.Actual)
					require.EqualValues(t, actualAgents, *feature.Actual)
					require.NotNil(t, feature.UsagePeriod)
					require.WithinDuration(t, lic2Iat, feature.UsagePeriod.IssuedAt, 2*time.Second)
					require.WithinDuration(t, lic2Nbf, feature.UsagePeriod.Start, 2*time.Second)
					require.WithinDuration(t, lic2Exp, feature.UsagePeriod.End, 2*time.Second)
				}
			})
		})
	}
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
		require.Nil(t, feature.SoftLimit)
		require.Nil(t, feature.Actual)
		require.Nil(t, feature.UsagePeriod)
	})

	// "Premium" licenses should receive a default managed agent limit of:
	// soft = 1000
	// hard = 1000
	t.Run("Premium", func(t *testing.T) {
		t.Parallel()

		const userLimit = 33
		const softLimit = 1000
		const hardLimit = 1000
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
		}

		entitlements, err := license.LicensesEntitlements(context.Background(), time.Now(), []database.License{lic}, map[codersdk.FeatureName]bool{}, coderdenttest.Keys, arguments)
		require.NoError(t, err)

		feature, ok := entitlements.Features[codersdk.FeatureManagedAgentLimit]
		require.True(t, ok, "feature %s not found", codersdk.FeatureManagedAgentLimit)
		require.Equal(t, codersdk.EntitlementEntitled, feature.Entitlement)
		require.NotNil(t, feature.Limit)
		require.EqualValues(t, hardLimit, *feature.Limit)
		require.NotNil(t, feature.SoftLimit)
		require.EqualValues(t, softLimit, *feature.SoftLimit)
		require.NotNil(t, feature.Actual)
		require.EqualValues(t, actualAgents, *feature.Actual)
		require.NotNil(t, feature.UsagePeriod)
		require.NotZero(t, feature.UsagePeriod.IssuedAt)
		require.NotZero(t, feature.UsagePeriod.Start)
		require.NotZero(t, feature.UsagePeriod.End)
	})

	// "Premium" licenses with an explicit managed agent limit should not
	// receive a default managed agent limit.
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
					codersdk.FeatureUserLimit:                        100,
					codersdk.FeatureName("managed_agent_limit_soft"): 100,
					codersdk.FeatureName("managed_agent_limit_hard"): 200,
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

		entitlements, err := license.LicensesEntitlements(context.Background(), time.Now(), []database.License{lic}, map[codersdk.FeatureName]bool{}, coderdenttest.Keys, arguments)
		require.NoError(t, err)

		feature, ok := entitlements.Features[codersdk.FeatureManagedAgentLimit]
		require.True(t, ok, "feature %s not found", codersdk.FeatureManagedAgentLimit)
		require.Equal(t, codersdk.EntitlementEntitled, feature.Entitlement)
		require.NotNil(t, feature.Limit)
		require.EqualValues(t, 200, *feature.Limit)
		require.NotNil(t, feature.SoftLimit)
		require.EqualValues(t, 100, *feature.SoftLimit)
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
					codersdk.FeatureUserLimit:                        100,
					codersdk.FeatureName("managed_agent_limit_soft"): 0,
					codersdk.FeatureName("managed_agent_limit_hard"): 0,
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

		entitlements, err := license.LicensesEntitlements(context.Background(), time.Now(), []database.License{lic}, map[codersdk.FeatureName]bool{}, coderdenttest.Keys, arguments)
		require.NoError(t, err)

		feature, ok := entitlements.Features[codersdk.FeatureManagedAgentLimit]
		require.True(t, ok, "feature %s not found", codersdk.FeatureManagedAgentLimit)
		require.Equal(t, codersdk.EntitlementEntitled, feature.Entitlement)
		require.False(t, feature.Enabled)
		require.NotNil(t, feature.Limit)
		require.EqualValues(t, 0, *feature.Limit)
		require.NotNil(t, feature.SoftLimit)
		require.EqualValues(t, 0, *feature.SoftLimit)
		require.NotNil(t, feature.Actual)
		require.EqualValues(t, actualAgents, *feature.Actual)
		require.NotNil(t, feature.UsagePeriod)
		require.NotZero(t, feature.UsagePeriod.IssuedAt)
		require.NotZero(t, feature.UsagePeriod.Start)
		require.NotZero(t, feature.UsagePeriod.End)
	})
}

func TestAIGovernanceAddon(t *testing.T) {
	t.Parallel()

	empty := map[codersdk.FeatureName]bool{}

	t.Run("AIGovernanceAddon enables AI governance features when enablements are set", func(t *testing.T) {
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

		// Enable AI governance features in enablements.
		enablements := map[codersdk.FeatureName]bool{
			codersdk.FeatureAIBridge: true,
			codersdk.FeatureBoundary: true,
		}
		entitlements, err := license.Entitlements(context.Background(), db, 1, 1, coderdenttest.Keys, enablements)
		require.NoError(t, err)
		require.True(t, entitlements.HasLicense)

		// AI Bridge should be enabled and entitled.
		aibridgeFeature := entitlements.Features[codersdk.FeatureAIBridge]
		require.True(t, aibridgeFeature.Enabled, "AI Bridge should be enabled when addon is present and enablements are set")
		require.Equal(t, codersdk.EntitlementEntitled, aibridgeFeature.Entitlement, "AI Bridge should be entitled when addon is present")

		// Boundary should be enabled and entitled.
		boundaryFeature := entitlements.Features[codersdk.FeatureBoundary]
		require.True(t, boundaryFeature.Enabled, "Boundary should be enabled when addon is present and enablements are set")
		require.Equal(t, codersdk.EntitlementEntitled, boundaryFeature.Entitlement, "Boundary should be entitled when addon is present")
	})

	t.Run("AIGovernanceAddon not present disables AI governance features", func(t *testing.T) {
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
		entitlements, err := license.Entitlements(context.Background(), db, 1, 1, coderdenttest.Keys, enablements)
		require.NoError(t, err)
		require.True(t, entitlements.HasLicense)

		// AI Bridge should not be entitled.
		aibridgeFeature := entitlements.Features[codersdk.FeatureAIBridge]
		require.False(t, aibridgeFeature.Enabled, "AI Bridge should not be enabled when addon is absent")
		require.Equal(t, codersdk.EntitlementNotEntitled, aibridgeFeature.Entitlement, "AI Bridge should not be entitled when addon is absent")

		// Boundary should not be entitled.
		boundaryFeature := entitlements.Features[codersdk.FeatureBoundary]
		require.False(t, boundaryFeature.Enabled, "Boundary should not be enabled when addon is absent")
		require.Equal(t, codersdk.EntitlementNotEntitled, boundaryFeature.Entitlement, "Boundary should not be entitled when addon is absent")
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
		entitlements, err := license.Entitlements(context.Background(), db, 1, 1, coderdenttest.Keys, enablements)
		require.NoError(t, err)
		require.True(t, entitlements.HasLicense)

		// AI governance features should be enabled but in grace period.
		aibridgeFeature := entitlements.Features[codersdk.FeatureAIBridge]
		require.True(t, aibridgeFeature.Enabled, "AI Bridge should be enabled during grace period")
		require.Equal(t, codersdk.EntitlementGracePeriod, aibridgeFeature.Entitlement, "AI Bridge should be in grace period")

		boundaryFeature := entitlements.Features[codersdk.FeatureBoundary]
		require.True(t, boundaryFeature.Enabled, "Boundary should be enabled during grace period")
		require.Equal(t, codersdk.EntitlementGracePeriod, boundaryFeature.Entitlement, "Boundary should be in grace period")
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

		entitlements, err := license.Entitlements(context.Background(), db, 1, 1, coderdenttest.Keys, empty)
		require.NoError(t, err)
		require.True(t, entitlements.HasLicense)

		aibridgeFeature := entitlements.Features[codersdk.FeatureAIBridge]
		require.False(t, aibridgeFeature.Enabled, "AI Bridge should not be enabled without enablements")
		require.Equal(t, codersdk.EntitlementEntitled, aibridgeFeature.Entitlement, "AI Bridge should still be entitled")

		boundaryFeature := entitlements.Features[codersdk.FeatureBoundary]
		require.False(t, boundaryFeature.Enabled, "Boundary should not be enabled without enablements")
		require.Equal(t, codersdk.EntitlementEntitled, boundaryFeature.Entitlement, "Boundary should still be entitled")
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
		entitlements, err := license.Entitlements(context.Background(), db, 1, 1, coderdenttest.Keys, enablements)
		require.NoError(t, err)
		require.True(t, entitlements.HasLicense)

		// Should have validation errors for both missing dependencies.
		require.Len(t, entitlements.Errors, 2)
		require.Equal(t, "Feature AI Governance User Limit must be set when using the AI Governance addon.", entitlements.Errors[0])
		require.Equal(t, "Feature Managed Agent Limit must be set when using the AI Governance addon.", entitlements.Errors[1])

		// AI governance features should not be entitled when validation fails.
		aibridgeFeature := entitlements.Features[codersdk.FeatureAIBridge]
		require.False(t, aibridgeFeature.Enabled, "AI Bridge should not be enabled when addon validation fails")
		require.Equal(t, codersdk.EntitlementNotEntitled, aibridgeFeature.Entitlement, "AI Bridge should not be entitled when addon validation fails")

		boundaryFeature := entitlements.Features[codersdk.FeatureBoundary]
		require.False(t, boundaryFeature.Enabled, "Boundary should not be enabled when addon validation fails")
		require.Equal(t, codersdk.EntitlementNotEntitled, boundaryFeature.Entitlement, "Boundary should not be entitled when addon validation fails")
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
