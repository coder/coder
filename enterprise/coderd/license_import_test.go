package coderd_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
)

func TestImportLicenseFromFile(t *testing.T) {
	t.Parallel()

	t.Run("Success", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		ctx := context.Background()
		logger := slogtest.Make(t, nil)
		deploymentID := uuid.NewString()

		// Create a temporary license file
		licenseJWT := coderdenttest.GenerateLicense(t, coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureAuditLog: 1,
			},
		})
		tmpDir := t.TempDir()
		licenseFile := filepath.Join(tmpDir, "license.jwt")
		err := os.WriteFile(licenseFile, []byte(licenseJWT), 0o600)
		require.NoError(t, err)

		// Import the license
		err = coderd.ImportLicenseFromFile(ctx, db, coderdenttest.Keys, licenseFile, deploymentID, logger)
		require.NoError(t, err)

		// Verify the license was imported
		licenses, err := db.GetLicenses(ctx)
		require.NoError(t, err)
		require.Len(t, licenses, 1)
		require.Equal(t, licenseJWT, licenses[0].JWT)
	})

	t.Run("Idempotent", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		ctx := context.Background()
		logger := slogtest.Make(t, nil)
		deploymentID := uuid.NewString()

		// Create a temporary license file
		licenseJWT := coderdenttest.GenerateLicense(t, coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureAuditLog: 1,
			},
		})
		tmpDir := t.TempDir()
		licenseFile := filepath.Join(tmpDir, "license.jwt")
		err := os.WriteFile(licenseFile, []byte(licenseJWT), 0o600)
		require.NoError(t, err)

		// Import the license once
		err = coderd.ImportLicenseFromFile(ctx, db, coderdenttest.Keys, licenseFile, deploymentID, logger)
		require.NoError(t, err)

		// Try to import again - should skip
		err = coderd.ImportLicenseFromFile(ctx, db, coderdenttest.Keys, licenseFile, deploymentID, logger)
		require.NoError(t, err)

		// Verify only one license exists
		licenses, err := db.GetLicenses(ctx)
		require.NoError(t, err)
		require.Len(t, licenses, 1)
	})

	t.Run("EmptyFilePath", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		ctx := context.Background()
		logger := slogtest.Make(t, nil)
		deploymentID := uuid.NewString()

		// Import with empty file path
		err := coderd.ImportLicenseFromFile(ctx, db, coderdenttest.Keys, "", deploymentID, logger)
		require.NoError(t, err)

		// Verify no license was imported
		licenses, err := db.GetLicenses(ctx)
		require.NoError(t, err)
		require.Len(t, licenses, 0)
	})

	t.Run("FileDoesNotExist", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		ctx := context.Background()
		logger := slogtest.Make(t, nil)
		deploymentID := uuid.NewString()

		// Import from non-existent file
		err := coderd.ImportLicenseFromFile(ctx, db, coderdenttest.Keys, "/nonexistent/license.jwt", deploymentID, logger)
		require.NoError(t, err) // Should not error, just skip

		// Verify no license was imported
		licenses, err := db.GetLicenses(ctx)
		require.NoError(t, err)
		require.Len(t, licenses, 0)
	})

	t.Run("EmptyFile", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		ctx := context.Background()
		logger := slogtest.Make(t, nil)
		deploymentID := uuid.NewString()

		// Create an empty license file
		tmpDir := t.TempDir()
		licenseFile := filepath.Join(tmpDir, "license.jwt")
		err := os.WriteFile(licenseFile, []byte(""), 0o600)
		require.NoError(t, err)

		// Import from empty file
		err = coderd.ImportLicenseFromFile(ctx, db, coderdenttest.Keys, licenseFile, deploymentID, logger)
		require.NoError(t, err) // Should not error, just skip

		// Verify no license was imported
		licenses, err := db.GetLicenses(ctx)
		require.NoError(t, err)
		require.Len(t, licenses, 0)
	})

	t.Run("WhitespaceOnlyFile", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		ctx := context.Background()
		logger := slogtest.Make(t, nil)
		deploymentID := uuid.NewString()

		// Create a file with only whitespace
		tmpDir := t.TempDir()
		licenseFile := filepath.Join(tmpDir, "license.jwt")
		err := os.WriteFile(licenseFile, []byte("  \n\t  \n"), 0o600)
		require.NoError(t, err)

		// Import from whitespace-only file
		err = coderd.ImportLicenseFromFile(ctx, db, coderdenttest.Keys, licenseFile, deploymentID, logger)
		require.NoError(t, err) // Should not error, just skip

		// Verify no license was imported
		licenses, err := db.GetLicenses(ctx)
		require.NoError(t, err)
		require.Len(t, licenses, 0)
	})

	t.Run("InvalidLicenseJWT", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		ctx := context.Background()
		logger := slogtest.Make(t, nil)
		deploymentID := uuid.NewString()

		// Create a file with invalid JWT
		tmpDir := t.TempDir()
		licenseFile := filepath.Join(tmpDir, "license.jwt")
		err := os.WriteFile(licenseFile, []byte("invalid-jwt-token"), 0o600)
		require.NoError(t, err)

		// Import should fail
		err = coderd.ImportLicenseFromFile(ctx, db, coderdenttest.Keys, licenseFile, deploymentID, logger)
		require.Error(t, err)
		require.Contains(t, err.Error(), "parse license")
	})

	t.Run("DeploymentIDRestriction", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		ctx := context.Background()
		logger := slogtest.Make(t, nil)

		// Generate license for a specific deployment ID
		restrictedDeploymentID := uuid.NewString()
		licenseJWT := coderdenttest.GenerateLicense(t, coderdenttest.LicenseOptions{
			DeploymentIDs: []string{restrictedDeploymentID},
			Features: license.Features{
				codersdk.FeatureAuditLog: 1,
			},
		})

		tmpDir := t.TempDir()
		licenseFile := filepath.Join(tmpDir, "license.jwt")
		err := os.WriteFile(licenseFile, []byte(licenseJWT), 0o600)
		require.NoError(t, err)

		// Try to import with a different deployment ID
		differentDeploymentID := uuid.NewString()
		err = coderd.ImportLicenseFromFile(ctx, db, coderdenttest.Keys, licenseFile, differentDeploymentID, logger)
		require.Error(t, err)
		require.Contains(t, err.Error(), "license is locked to deployments")
		require.Contains(t, err.Error(), restrictedDeploymentID)
		require.Contains(t, err.Error(), differentDeploymentID)
	})

	t.Run("DeploymentIDMatch", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		ctx := context.Background()
		logger := slogtest.Make(t, nil)

		// Generate license for a specific deployment ID
		deploymentID := uuid.NewString()
		licenseJWT := coderdenttest.GenerateLicense(t, coderdenttest.LicenseOptions{
			DeploymentIDs: []string{deploymentID},
			Features: license.Features{
				codersdk.FeatureAuditLog: 1,
			},
		})

		tmpDir := t.TempDir()
		licenseFile := filepath.Join(tmpDir, "license.jwt")
		err := os.WriteFile(licenseFile, []byte(licenseJWT), 0o600)
		require.NoError(t, err)

		// Import with matching deployment ID should succeed
		err = coderd.ImportLicenseFromFile(ctx, db, coderdenttest.Keys, licenseFile, deploymentID, logger)
		require.NoError(t, err)

		// Verify the license was imported
		licenses, err := db.GetLicenses(ctx)
		require.NoError(t, err)
		require.Len(t, licenses, 1)
	})

	t.Run("ExpiredLicense", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		ctx := context.Background()
		logger := slogtest.Make(t, nil)
		deploymentID := uuid.NewString()

		// Generate an expired license
		licenseJWT := coderdenttest.GenerateLicense(t, coderdenttest.LicenseOptions{
			ExpiresAt: time.Now().Add(-24 * time.Hour), // Expired 1 day ago
			Features: license.Features{
				codersdk.FeatureAuditLog: 1,
			},
		})

		tmpDir := t.TempDir()
		licenseFile := filepath.Join(tmpDir, "license.jwt")
		err := os.WriteFile(licenseFile, []byte(licenseJWT), 0o600)
		require.NoError(t, err)

		// Import should fail for expired licenses
		err = coderd.ImportLicenseFromFile(ctx, db, coderdenttest.Keys, licenseFile, deploymentID, logger)
		require.Error(t, err)
		require.Contains(t, err.Error(), "parse license")

		// Verify no license was imported
		licenses, err := db.GetLicenses(ctx)
		require.NoError(t, err)
		require.Len(t, licenses, 0)
	})

	t.Run("LicenseWithUUID", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		ctx := context.Background()
		logger := slogtest.Make(t, nil)
		deploymentID := uuid.NewString()

		// Generate license (UUID is auto-generated in the license claims)
		licenseJWT := coderdenttest.GenerateLicense(t, coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureAuditLog: 1,
			},
		})

		// Parse the license to get its UUID
		claims, err := license.ParseClaims(licenseJWT, coderdenttest.Keys)
		require.NoError(t, err)
		expectedUUID, err := uuid.Parse(claims.ID)
		require.NoError(t, err)

		tmpDir := t.TempDir()
		licenseFile := filepath.Join(tmpDir, "license.jwt")
		err = os.WriteFile(licenseFile, []byte(licenseJWT), 0o600)
		require.NoError(t, err)

		// Import the license
		err = coderd.ImportLicenseFromFile(ctx, db, coderdenttest.Keys, licenseFile, deploymentID, logger)
		require.NoError(t, err)

		// Verify the license UUID matches
		licenses, err := db.GetLicenses(ctx)
		require.NoError(t, err)
		require.Len(t, licenses, 1)
		require.Equal(t, expectedUUID, licenses[0].UUID)
	})

	t.Run("SkipsWhenLicenseExists", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		ctx := context.Background()
		logger := slogtest.Make(t, nil)
		deploymentID := uuid.NewString()

		// Insert a license directly into the database
		existingLicense := coderdenttest.GenerateLicense(t, coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureAuditLog: 1,
			},
		})
		_, err := db.InsertLicense(ctx, database.InsertLicenseParams{
			JWT: existingLicense,
			Exp: time.Now().Add(24 * time.Hour),
		})
		require.NoError(t, err)

		// Create a different license file
		newLicenseJWT := coderdenttest.GenerateLicense(t, coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureMultipleOrganizations: 1,
			},
		})
		tmpDir := t.TempDir()
		licenseFile := filepath.Join(tmpDir, "license.jwt")
		err = os.WriteFile(licenseFile, []byte(newLicenseJWT), 0o600)
		require.NoError(t, err)

		// Try to import - should skip because license already exists
		err = coderd.ImportLicenseFromFile(ctx, db, coderdenttest.Keys, licenseFile, deploymentID, logger)
		require.NoError(t, err)

		// Verify only the original license exists (not the new one)
		licenses, err := db.GetLicenses(ctx)
		require.NoError(t, err)
		require.Len(t, licenses, 1)
		require.Equal(t, existingLicense, licenses[0].JWT)
	})
}
