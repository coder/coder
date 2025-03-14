package coderd_test
import (
	"errors"
	"context"
	"net/http"
	"testing"
	"time"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/testutil"
)
func TestPostLicense(t *testing.T) {
	t.Parallel()
	t.Run("Success", func(t *testing.T) {
		t.Parallel()
		client, _ := coderdenttest.New(t, &coderdenttest.Options{DontAddLicense: true})
		respLic := coderdenttest.AddLicense(t, client, coderdenttest.LicenseOptions{
			AccountType: license.AccountTypeSalesforce,
			AccountID:   "testing",
			Features: license.Features{
				codersdk.FeatureAuditLog: 1,
			},
		})
		assert.GreaterOrEqual(t, respLic.ID, int32(0))
		// just a couple spot checks for sanity
		assert.Equal(t, "testing", respLic.Claims["account_id"])
		features, err := respLic.FeaturesClaims()
		require.NoError(t, err)
		assert.EqualValues(t, 1, features[codersdk.FeatureAuditLog])
	})
	t.Run("InvalidDeploymentID", func(t *testing.T) {
		t.Parallel()
		// The generated deployment will start out with a different deployment ID.
		client, _ := coderdenttest.New(t, &coderdenttest.Options{DontAddLicense: true})
		license := coderdenttest.GenerateLicense(t, coderdenttest.LicenseOptions{
			DeploymentIDs: []string{uuid.NewString()},
		})
		_, err := client.AddLicense(context.Background(), codersdk.AddLicenseRequest{
			License: license,
		})
		errResp := &codersdk.Error{}
		require.ErrorAs(t, err, &errResp)
		require.Equal(t, http.StatusBadRequest, errResp.StatusCode())
		require.Contains(t, errResp.Message, "License cannot be used on this deployment!")
	})
	t.Run("Unauthorized", func(t *testing.T) {
		t.Parallel()
		client, _ := coderdenttest.New(t, &coderdenttest.Options{DontAddLicense: true})
		client.SetSessionToken("")
		_, err := client.AddLicense(context.Background(), codersdk.AddLicenseRequest{
			License: "content",
		})
		errResp := &codersdk.Error{}
		if errors.As(err, &errResp) {
			assert.Equal(t, 401, errResp.StatusCode())
		} else {
			t.Error("expected to get error status 401")
		}
	})
	t.Run("Corrupted", func(t *testing.T) {
		t.Parallel()
		client, _ := coderdenttest.New(t, &coderdenttest.Options{DontAddLicense: true})
		coderdenttest.AddLicense(t, client, coderdenttest.LicenseOptions{})
		_, err := client.AddLicense(context.Background(), codersdk.AddLicenseRequest{
			License: "invalid",
		})
		errResp := &codersdk.Error{}
		if errors.As(err, &errResp) {
			assert.Equal(t, 400, errResp.StatusCode())
		} else {
			t.Error("expected to get error status 400")
		}
	})
	// Test a license that isn't yet valid, but will be in the future.  We should allow this so that
	// operators can upload a license ahead of time.
	t.Run("NotYet", func(t *testing.T) {
		t.Parallel()
		client, _ := coderdenttest.New(t, &coderdenttest.Options{DontAddLicense: true})
		respLic := coderdenttest.AddLicense(t, client, coderdenttest.LicenseOptions{
			AccountType: license.AccountTypeSalesforce,
			AccountID:   "testing",
			Features: license.Features{
				codersdk.FeatureAuditLog: 1,
			},
			NotBefore: time.Now().Add(time.Hour),
			GraceAt:   time.Now().Add(2 * time.Hour),
			ExpiresAt: time.Now().Add(3 * time.Hour),
		})
		assert.GreaterOrEqual(t, respLic.ID, int32(0))
		// just a couple spot checks for sanity
		assert.Equal(t, "testing", respLic.Claims["account_id"])
		features, err := respLic.FeaturesClaims()
		require.NoError(t, err)
		assert.EqualValues(t, 1, features[codersdk.FeatureAuditLog])
	})
	// Test we still reject a license that isn't valid yet, but has other issues (e.g. expired
	// before it starts).
	t.Run("NotEver", func(t *testing.T) {
		t.Parallel()
		client, _ := coderdenttest.New(t, &coderdenttest.Options{DontAddLicense: true})
		lic := coderdenttest.GenerateLicense(t, coderdenttest.LicenseOptions{
			AccountType: license.AccountTypeSalesforce,
			AccountID:   "testing",
			Features: license.Features{
				codersdk.FeatureAuditLog: 1,
			},
			NotBefore: time.Now().Add(time.Hour),
			GraceAt:   time.Now().Add(2 * time.Hour),
			ExpiresAt: time.Now().Add(-time.Hour),
		})
		_, err := client.AddLicense(context.Background(), codersdk.AddLicenseRequest{
			License: lic,
		})
		errResp := &codersdk.Error{}
		require.ErrorAs(t, err, &errResp)
		require.Equal(t, http.StatusBadRequest, errResp.StatusCode())
		require.Contains(t, errResp.Detail, license.ErrMultipleIssues.Error())
	})
}
func TestGetLicense(t *testing.T) {
	t.Parallel()
	t.Run("Success", func(t *testing.T) {
		t.Parallel()
		client, _ := coderdenttest.New(t, &coderdenttest.Options{DontAddLicense: true})
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()
		coderdenttest.AddLicense(t, client, coderdenttest.LicenseOptions{
			AccountID: "testing",
			Features: license.Features{
				codersdk.FeatureAuditLog:     1,
				codersdk.FeatureSCIM:         1,
				codersdk.FeatureBrowserOnly:  1,
				codersdk.FeatureTemplateRBAC: 1,
			},
		})
		coderdenttest.AddLicense(t, client, coderdenttest.LicenseOptions{
			AccountID: "testing2",
			Features: license.Features{
				codersdk.FeatureAuditLog:    1,
				codersdk.FeatureSCIM:        1,
				codersdk.FeatureBrowserOnly: 1,
				codersdk.FeatureUserLimit:   200,
			},
			Trial: true,
		})
		licenses, err := client.Licenses(ctx)
		require.NoError(t, err)
		require.Len(t, licenses, 2)
		assert.Equal(t, int32(1), licenses[0].ID)
		assert.Equal(t, "testing", licenses[0].Claims["account_id"])
		features, err := licenses[0].FeaturesClaims()
		require.NoError(t, err)
		assert.Equal(t, map[codersdk.FeatureName]int64{
			codersdk.FeatureAuditLog:     1,
			codersdk.FeatureSCIM:         1,
			codersdk.FeatureBrowserOnly:  1,
			codersdk.FeatureTemplateRBAC: 1,
		}, features)
		assert.Equal(t, int32(2), licenses[1].ID)
		assert.Equal(t, "testing2", licenses[1].Claims["account_id"])
		assert.Equal(t, true, licenses[1].Claims["trial"])
		features, err = licenses[1].FeaturesClaims()
		require.NoError(t, err)
		assert.Equal(t, map[codersdk.FeatureName]int64{
			codersdk.FeatureUserLimit:   200,
			codersdk.FeatureAuditLog:    1,
			codersdk.FeatureSCIM:        1,
			codersdk.FeatureBrowserOnly: 1,
		}, features)
	})
}
func TestDeleteLicense(t *testing.T) {
	t.Parallel()
	t.Run("Empty", func(t *testing.T) {
		t.Parallel()
		client, _ := coderdenttest.New(t, &coderdenttest.Options{DontAddLicense: true})
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()
		err := client.DeleteLicense(ctx, 1)
		errResp := &codersdk.Error{}
		if errors.As(err, &errResp) {
			assert.Equal(t, 404, errResp.StatusCode())
		} else {
			t.Error("expected to get error status 404")
		}
	})
	t.Run("BadID", func(t *testing.T) {
		t.Parallel()
		client, _ := coderdenttest.New(t, &coderdenttest.Options{DontAddLicense: true})
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()
		//nolint:gocritic // RBAC is irrelevant here.
		resp, err := client.Request(ctx, http.MethodDelete, "/api/v2/licenses/drivers", nil)
		require.NoError(t, err)
		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
		require.NoError(t, resp.Body.Close())
	})
	t.Run("Success", func(t *testing.T) {
		t.Parallel()
		client, _ := coderdenttest.New(t, &coderdenttest.Options{DontAddLicense: true})
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()
		coderdenttest.AddLicense(t, client, coderdenttest.LicenseOptions{
			AccountID: "testing",
			Features: license.Features{
				codersdk.FeatureAuditLog: 1,
			},
		})
		coderdenttest.AddLicense(t, client, coderdenttest.LicenseOptions{
			AccountID: "testing2",
			Features: license.Features{
				codersdk.FeatureAuditLog:  1,
				codersdk.FeatureUserLimit: 200,
			},
		})
		licenses, err := client.Licenses(ctx)
		require.NoError(t, err)
		assert.Len(t, licenses, 2)
		for _, l := range licenses {
			err = client.DeleteLicense(ctx, l.ID)
			require.NoError(t, err)
		}
		licenses, err = client.Licenses(ctx)
		require.NoError(t, err)
		assert.Len(t, licenses, 0)
	})
}
