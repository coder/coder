package license_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/database/databasefake"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/enterprise/coderd/license"
)

func TestEntitlements(t *testing.T) {
	t.Parallel()
	all := map[string]bool{
		codersdk.FeatureAuditLog:                   true,
		codersdk.FeatureBrowserOnly:                true,
		codersdk.FeatureSCIM:                       true,
		codersdk.FeatureHighAvailability:           true,
		codersdk.FeatureTemplateRBAC:               true,
		codersdk.FeatureMultipleGitAuth:            true,
		codersdk.FeatureExternalProvisionerDaemons: true,
		codersdk.FeatureAppearance:                 true,
	}

	t.Run("Defaults", func(t *testing.T) {
		t.Parallel()
		db := databasefake.New()
		entitlements, err := license.Entitlements(context.Background(), db, slog.Logger{}, 1, 1, coderdenttest.Keys, all)
		require.NoError(t, err)
		require.False(t, entitlements.HasLicense)
		require.False(t, entitlements.Trial)
		for _, featureName := range codersdk.FeatureNames {
			require.False(t, entitlements.Features[featureName].Enabled)
			require.Equal(t, codersdk.EntitlementNotEntitled, entitlements.Features[featureName].Entitlement)
		}
	})
	t.Run("SingleLicenseNothing", func(t *testing.T) {
		t.Parallel()
		db := databasefake.New()
		db.InsertLicense(context.Background(), database.InsertLicenseParams{
			JWT: coderdenttest.GenerateLicense(t, coderdenttest.LicenseOptions{}),
			Exp: time.Now().Add(time.Hour),
		})
		entitlements, err := license.Entitlements(context.Background(), db, slog.Logger{}, 1, 1, coderdenttest.Keys, map[string]bool{})
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
		db := databasefake.New()
		db.InsertLicense(context.Background(), database.InsertLicenseParams{
			JWT: coderdenttest.GenerateLicense(t, coderdenttest.LicenseOptions{
				UserLimit:                  100,
				AuditLog:                   true,
				BrowserOnly:                true,
				SCIM:                       true,
				HighAvailability:           true,
				TemplateRBAC:               true,
				MultipleGitAuth:            true,
				ExternalProvisionerDaemons: true,
				ServiceBanners:             true,
			}),
			Exp: time.Now().Add(time.Hour),
		})
		entitlements, err := license.Entitlements(context.Background(), db, slog.Logger{}, 1, 1, coderdenttest.Keys, map[string]bool{})
		require.NoError(t, err)
		require.True(t, entitlements.HasLicense)
		require.False(t, entitlements.Trial)
		for _, featureName := range codersdk.FeatureNames {
			require.Equal(t, codersdk.EntitlementEntitled, entitlements.Features[featureName].Entitlement, featureName)
		}
	})
	t.Run("SingleLicenseGrace", func(t *testing.T) {
		t.Parallel()
		db := databasefake.New()
		db.InsertLicense(context.Background(), database.InsertLicenseParams{
			JWT: coderdenttest.GenerateLicense(t, coderdenttest.LicenseOptions{
				UserLimit:                  100,
				AuditLog:                   true,
				BrowserOnly:                true,
				SCIM:                       true,
				HighAvailability:           true,
				TemplateRBAC:               true,
				ExternalProvisionerDaemons: true,
				ServiceBanners:             true,
				GraceAt:                    time.Now().Add(-time.Hour),
				ExpiresAt:                  time.Now().Add(time.Hour),
			}),
			Exp: time.Now().Add(time.Hour),
		})
		entitlements, err := license.Entitlements(context.Background(), db, slog.Logger{}, 1, 1, coderdenttest.Keys, all)
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
			if featureName == codersdk.FeatureMultipleGitAuth {
				continue
			}
			niceName := strings.Title(strings.ReplaceAll(featureName, "_", " "))
			require.Equal(t, codersdk.EntitlementGracePeriod, entitlements.Features[featureName].Entitlement)
			require.Contains(t, entitlements.Warnings, fmt.Sprintf("%s is enabled but your license for this feature is expired.", niceName))
		}
	})
	t.Run("SingleLicenseNotEntitled", func(t *testing.T) {
		t.Parallel()
		db := databasefake.New()
		db.InsertLicense(context.Background(), database.InsertLicenseParams{
			JWT: coderdenttest.GenerateLicense(t, coderdenttest.LicenseOptions{}),
			Exp: time.Now().Add(time.Hour),
		})
		entitlements, err := license.Entitlements(context.Background(), db, slog.Logger{}, 1, 1, coderdenttest.Keys, all)
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
			if featureName == codersdk.FeatureMultipleGitAuth {
				continue
			}
			niceName := strings.Title(strings.ReplaceAll(featureName, "_", " "))
			// Ensures features that are not entitled are properly disabled.
			require.False(t, entitlements.Features[featureName].Enabled)
			require.Equal(t, codersdk.EntitlementNotEntitled, entitlements.Features[featureName].Entitlement)
			require.Contains(t, entitlements.Warnings, fmt.Sprintf("%s is enabled but your license is not entitled to this feature.", niceName))
		}
	})
	t.Run("TooManyUsers", func(t *testing.T) {
		t.Parallel()
		db := databasefake.New()
		db.InsertUser(context.Background(), database.InsertUserParams{
			Username: "test1",
		})
		db.InsertUser(context.Background(), database.InsertUserParams{
			Username: "test2",
		})
		db.InsertLicense(context.Background(), database.InsertLicenseParams{
			JWT: coderdenttest.GenerateLicense(t, coderdenttest.LicenseOptions{
				UserLimit: 1,
			}),
			Exp: time.Now().Add(time.Hour),
		})
		entitlements, err := license.Entitlements(context.Background(), db, slog.Logger{}, 1, 1, coderdenttest.Keys, map[string]bool{})
		require.NoError(t, err)
		require.True(t, entitlements.HasLicense)
		require.Contains(t, entitlements.Warnings, "Your deployment has 2 active users but is only licensed for 1.")
	})
	t.Run("MaximizeUserLimit", func(t *testing.T) {
		t.Parallel()
		db := databasefake.New()
		db.InsertUser(context.Background(), database.InsertUserParams{})
		db.InsertUser(context.Background(), database.InsertUserParams{})
		db.InsertLicense(context.Background(), database.InsertLicenseParams{
			JWT: coderdenttest.GenerateLicense(t, coderdenttest.LicenseOptions{
				UserLimit: 10,
			}),
			Exp: time.Now().Add(time.Hour),
		})
		db.InsertLicense(context.Background(), database.InsertLicenseParams{
			JWT: coderdenttest.GenerateLicense(t, coderdenttest.LicenseOptions{
				UserLimit: 1,
			}),
			Exp: time.Now().Add(time.Hour),
		})
		entitlements, err := license.Entitlements(context.Background(), db, slog.Logger{}, 1, 1, coderdenttest.Keys, map[string]bool{})
		require.NoError(t, err)
		require.True(t, entitlements.HasLicense)
		require.Empty(t, entitlements.Warnings)
	})
	t.Run("MultipleLicenseEnabled", func(t *testing.T) {
		t.Parallel()
		db := databasefake.New()
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

		entitlements, err := license.Entitlements(context.Background(), db, slog.Logger{}, 1, 1, coderdenttest.Keys, map[string]bool{})
		require.NoError(t, err)
		require.True(t, entitlements.HasLicense)
		require.False(t, entitlements.Trial)
	})

	t.Run("AllFeatures", func(t *testing.T) {
		t.Parallel()
		db := databasefake.New()
		db.InsertLicense(context.Background(), database.InsertLicenseParams{
			Exp: time.Now().Add(time.Hour),
			JWT: coderdenttest.GenerateLicense(t, coderdenttest.LicenseOptions{
				AllFeatures: true,
			}),
		})
		entitlements, err := license.Entitlements(context.Background(), db, slog.Logger{}, 1, 1, coderdenttest.Keys, all)
		require.NoError(t, err)
		require.True(t, entitlements.HasLicense)
		require.False(t, entitlements.Trial)
		for _, featureName := range codersdk.FeatureNames {
			if featureName == codersdk.FeatureUserLimit {
				continue
			}
			require.True(t, entitlements.Features[featureName].Enabled)
			require.Equal(t, codersdk.EntitlementEntitled, entitlements.Features[featureName].Entitlement)
		}
	})

	t.Run("MultipleReplicasNoLicense", func(t *testing.T) {
		t.Parallel()
		db := databasefake.New()
		entitlements, err := license.Entitlements(context.Background(), db, slog.Logger{}, 2, 1, coderdenttest.Keys, all)
		require.NoError(t, err)
		require.False(t, entitlements.HasLicense)
		require.Len(t, entitlements.Errors, 1)
		require.Equal(t, "You have multiple replicas but high availability is an Enterprise feature. You will be unable to connect to workspaces.", entitlements.Errors[0])
	})

	t.Run("MultipleReplicasNotEntitled", func(t *testing.T) {
		t.Parallel()
		db := databasefake.New()
		db.InsertLicense(context.Background(), database.InsertLicenseParams{
			Exp: time.Now().Add(time.Hour),
			JWT: coderdenttest.GenerateLicense(t, coderdenttest.LicenseOptions{
				AuditLog: true,
			}),
		})
		entitlements, err := license.Entitlements(context.Background(), db, slog.Logger{}, 2, 1, coderdenttest.Keys, map[string]bool{
			codersdk.FeatureHighAvailability: true,
		})
		require.NoError(t, err)
		require.True(t, entitlements.HasLicense)
		require.Len(t, entitlements.Errors, 1)
		require.Equal(t, "You have multiple replicas but your license is not entitled to high availability. You will be unable to connect to workspaces.", entitlements.Errors[0])
	})

	t.Run("MultipleReplicasGrace", func(t *testing.T) {
		t.Parallel()
		db := databasefake.New()
		db.InsertLicense(context.Background(), database.InsertLicenseParams{
			JWT: coderdenttest.GenerateLicense(t, coderdenttest.LicenseOptions{
				HighAvailability: true,
				GraceAt:          time.Now().Add(-time.Hour),
				ExpiresAt:        time.Now().Add(time.Hour),
			}),
			Exp: time.Now().Add(time.Hour),
		})
		entitlements, err := license.Entitlements(context.Background(), db, slog.Logger{}, 2, 1, coderdenttest.Keys, map[string]bool{
			codersdk.FeatureHighAvailability: true,
		})
		require.NoError(t, err)
		require.True(t, entitlements.HasLicense)
		require.Len(t, entitlements.Warnings, 1)
		require.Equal(t, "You have multiple replicas but your license for high availability is expired. Reduce to one replica or workspace connections will stop working.", entitlements.Warnings[0])
	})

	t.Run("MultipleGitAuthNoLicense", func(t *testing.T) {
		t.Parallel()
		db := databasefake.New()
		entitlements, err := license.Entitlements(context.Background(), db, slog.Logger{}, 1, 2, coderdenttest.Keys, all)
		require.NoError(t, err)
		require.False(t, entitlements.HasLicense)
		require.Len(t, entitlements.Errors, 1)
		require.Equal(t, "You have multiple Git authorizations configured but this is an Enterprise feature. Reduce to one.", entitlements.Errors[0])
	})

	t.Run("MultipleGitAuthNotEntitled", func(t *testing.T) {
		t.Parallel()
		db := databasefake.New()
		db.InsertLicense(context.Background(), database.InsertLicenseParams{
			Exp: time.Now().Add(time.Hour),
			JWT: coderdenttest.GenerateLicense(t, coderdenttest.LicenseOptions{
				AuditLog: true,
			}),
		})
		entitlements, err := license.Entitlements(context.Background(), db, slog.Logger{}, 1, 2, coderdenttest.Keys, map[string]bool{
			codersdk.FeatureMultipleGitAuth: true,
		})
		require.NoError(t, err)
		require.True(t, entitlements.HasLicense)
		require.Len(t, entitlements.Errors, 1)
		require.Equal(t, "You have multiple Git authorizations configured but your license is limited at one.", entitlements.Errors[0])
	})

	t.Run("MultipleGitAuthGrace", func(t *testing.T) {
		t.Parallel()
		db := databasefake.New()
		db.InsertLicense(context.Background(), database.InsertLicenseParams{
			JWT: coderdenttest.GenerateLicense(t, coderdenttest.LicenseOptions{
				MultipleGitAuth: true,
				GraceAt:         time.Now().Add(-time.Hour),
				ExpiresAt:       time.Now().Add(time.Hour),
			}),
			Exp: time.Now().Add(time.Hour),
		})
		entitlements, err := license.Entitlements(context.Background(), db, slog.Logger{}, 1, 2, coderdenttest.Keys, map[string]bool{
			codersdk.FeatureMultipleGitAuth: true,
		})
		require.NoError(t, err)
		require.True(t, entitlements.HasLicense)
		require.Len(t, entitlements.Warnings, 1)
		require.Equal(t, "You have multiple Git authorizations configured but your license is expired. Reduce to one.", entitlements.Warnings[0])
	})
}
