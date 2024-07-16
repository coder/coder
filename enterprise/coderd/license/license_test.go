package license_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/exp/slices"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmem"
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

	empty := map[codersdk.FeatureName]bool{}

	t.Run("Defaults", func(t *testing.T) {
		t.Parallel()
		db := dbmem.New()
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
		db := dbmem.New()
		entitlements, err := license.Entitlements(context.Background(), db, 1, 1, coderdenttest.Keys, all)
		require.NoError(t, err)
		require.False(t, entitlements.HasLicense)
		require.False(t, entitlements.Trial)
		require.Equal(t, *entitlements.Features[codersdk.FeatureUserLimit].Actual, int64(0))
	})
	t.Run("SingleLicenseNothing", func(t *testing.T) {
		t.Parallel()
		db := dbmem.New()
		db.InsertLicense(context.Background(), database.InsertLicenseParams{
			JWT: coderdenttest.GenerateLicense(t, coderdenttest.LicenseOptions{}),
			Exp: time.Now().Add(time.Hour),
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
		db := dbmem.New()
		db.InsertLicense(context.Background(), database.InsertLicenseParams{
			JWT: coderdenttest.GenerateLicense(t, coderdenttest.LicenseOptions{
				Features: func() license.Features {
					f := make(license.Features)
					for _, name := range codersdk.FeatureNames {
						f[name] = 1
					}
					return f
				}(),
			}),
			Exp: time.Now().Add(time.Hour),
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
		db := dbmem.New()
		db.InsertLicense(context.Background(), database.InsertLicenseParams{
			JWT: coderdenttest.GenerateLicense(t, coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureUserLimit: 100,
					codersdk.FeatureAuditLog:  1,
				},

				GraceAt:   time.Now().Add(-time.Hour),
				ExpiresAt: time.Now().Add(time.Hour),
			}),
			Exp: time.Now().Add(time.Hour),
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
		db := dbmem.New()
		db.InsertLicense(context.Background(), database.InsertLicenseParams{
			JWT: coderdenttest.GenerateLicense(t, coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureUserLimit: 100,
					codersdk.FeatureAuditLog:  1,
				},

				GraceAt:   time.Now().AddDate(0, 0, 2),
				ExpiresAt: time.Now().AddDate(0, 0, 5),
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
			"Your license expires in 2 days.",
		)
	})

	t.Run("Expiration warning for license expiring in 1 day", func(t *testing.T) {
		t.Parallel()
		db := dbmem.New()
		db.InsertLicense(context.Background(), database.InsertLicenseParams{
			JWT: coderdenttest.GenerateLicense(t, coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureUserLimit: 100,
					codersdk.FeatureAuditLog:  1,
				},

				GraceAt:   time.Now().AddDate(0, 0, 1),
				ExpiresAt: time.Now().AddDate(0, 0, 5),
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

	t.Run("Expiration warning for trials", func(t *testing.T) {
		t.Parallel()
		db := dbmem.New()
		db.InsertLicense(context.Background(), database.InsertLicenseParams{
			JWT: coderdenttest.GenerateLicense(t, coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureUserLimit: 100,
					codersdk.FeatureAuditLog:  1,
				},

				Trial:     true,
				GraceAt:   time.Now().AddDate(0, 0, 8),
				ExpiresAt: time.Now().AddDate(0, 0, 5),
			}),
			Exp: time.Now().AddDate(0, 0, 5),
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
		db := dbmem.New()
		db.InsertLicense(context.Background(), database.InsertLicenseParams{
			JWT: coderdenttest.GenerateLicense(t, coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureUserLimit: 100,
					codersdk.FeatureAuditLog:  1,
				},

				GraceAt:   time.Now().AddDate(0, 0, 30),
				ExpiresAt: time.Now().AddDate(0, 0, 5),
			}),
			Exp: time.Now().AddDate(0, 0, 5),
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
		db := dbmem.New()
		db.InsertLicense(context.Background(), database.InsertLicenseParams{
			JWT: coderdenttest.GenerateLicense(t, coderdenttest.LicenseOptions{}),
			Exp: time.Now().Add(time.Hour),
		})
		entitlements, err := license.Entitlements(context.Background(), db, 1, 1, coderdenttest.Keys, all)
		require.NoError(t, err)
		require.True(t, entitlements.HasLicense)
		require.False(t, entitlements.Trial)
		for _, featureName := range codersdk.FeatureNames {
			if featureName == codersdk.FeatureUserLimit {
				continue
			}
			if featureName == codersdk.FeatureHighAvailability {
				continue
			}
			if featureName == codersdk.FeatureMultipleExternalAuth {
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
		db := dbmem.New()
		activeUser1, err := db.InsertUser(context.Background(), database.InsertUserParams{
			ID:        uuid.New(),
			Username:  "test1",
			LoginType: database.LoginTypePassword,
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
			LoginType: database.LoginTypePassword,
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
			LoginType: database.LoginTypePassword,
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
		db := dbmem.New()
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
		db := dbmem.New()
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
		db := dbmem.New()
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
		db := dbmem.New()
		_, err := db.InsertLicense(context.Background(), database.InsertLicenseParams{
			Exp: time.Now().Add(time.Hour),
			JWT: coderdenttest.GenerateLicense(t, coderdenttest.LicenseOptions{
				FeatureSet: codersdk.FeatureSetPremium,
			}),
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
		db := dbmem.New()
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
		db := dbmem.New()
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
			if featureName == codersdk.FeatureUserLimit {
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
		db := dbmem.New()
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
			if featureName == codersdk.FeatureUserLimit {
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
		db := dbmem.New()
		db.InsertLicense(context.Background(), database.InsertLicenseParams{
			Exp: dbtime.Now().Add(time.Hour),
			JWT: coderdenttest.GenerateLicense(t, coderdenttest.LicenseOptions{
				AllFeatures: true,
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
		db := dbmem.New()
		entitlements, err := license.Entitlements(context.Background(), db, 2, 1, coderdenttest.Keys, all)
		require.NoError(t, err)
		require.False(t, entitlements.HasLicense)
		require.Len(t, entitlements.Errors, 1)
		require.Equal(t, "You have multiple replicas but high availability is an Enterprise feature. You will be unable to connect to workspaces.", entitlements.Errors[0])
	})

	t.Run("MultipleReplicasNotEntitled", func(t *testing.T) {
		t.Parallel()
		db := dbmem.New()
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
		db := dbmem.New()
		db.InsertLicense(context.Background(), database.InsertLicenseParams{
			JWT: coderdenttest.GenerateLicense(t, coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureHighAvailability: 1,
				},
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
		db := dbmem.New()
		entitlements, err := license.Entitlements(context.Background(), db, 1, 2, coderdenttest.Keys, all)
		require.NoError(t, err)
		require.False(t, entitlements.HasLicense)
		require.Len(t, entitlements.Errors, 1)
		require.Equal(t, "You have multiple External Auth Providers configured but this is an Enterprise feature. Reduce to one.", entitlements.Errors[0])
	})

	t.Run("MultipleGitAuthNotEntitled", func(t *testing.T) {
		t.Parallel()
		db := dbmem.New()
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
		db := dbmem.New()
		db.InsertLicense(context.Background(), database.InsertLicenseParams{
			JWT: coderdenttest.GenerateLicense(t, coderdenttest.LicenseOptions{
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
			AllFeatures:   false,
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
	}

	for _, tc := range testCases {
		tc := tc

		t.Run(tc.Name, func(t *testing.T) {
			t.Parallel()

			generatedLicenses := make([]database.License, 0, len(tc.Licenses))
			for i, lo := range tc.Licenses {
				generatedLicenses = append(generatedLicenses, database.License{
					ID:         int32(i),
					UploadedAt: time.Now().Add(time.Hour * -1),
					JWT:        lo.Generate(t),
					Exp:        lo.GraceAt,
					UUID:       uuid.New(),
				})
			}

			entitlements, err := license.LicensesEntitlements(time.Now(), generatedLicenses, tc.Enablements, coderdenttest.Keys, tc.Arguments)
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

func assertNoErrors(t *testing.T, entitlements codersdk.Entitlements) {
	assert.Empty(t, entitlements.Errors, "no errors")
}

func assertNoWarnings(t *testing.T, entitlements codersdk.Entitlements) {
	assert.Empty(t, entitlements.Warnings, "no warnings")
}

func assertEnterpriseFeatures(t *testing.T, entitlements codersdk.Entitlements) {
	for _, expected := range codersdk.FeatureSetEnterprise.Features() {
		f := entitlements.Features[expected]
		assert.Equalf(t, codersdk.EntitlementEntitled, f.Entitlement, "%s entitled", expected)
		assert.Equalf(t, true, f.Enabled, "%s enabled", expected)
	}
}
