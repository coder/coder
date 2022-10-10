package coderd_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/enterprise/coderd/license"
	"github.com/coder/coder/testutil"
)

func TestPostLicense(t *testing.T) {
	t.Parallel()

	t.Run("Success", func(t *testing.T) {
		t.Parallel()
		client := coderdenttest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		respLic := coderdenttest.AddLicense(t, client, coderdenttest.LicenseOptions{
			AccountType: license.AccountTypeSalesforce,
			AccountID:   "testing",
			AuditLog:    true,
		})
		assert.GreaterOrEqual(t, respLic.ID, int32(0))
		// just a couple spot checks for sanity
		assert.Equal(t, "testing", respLic.Claims["account_id"])
		features, ok := respLic.Claims["features"].(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, json.Number("1"), features[codersdk.FeatureAuditLog])
	})

	t.Run("Unauthorized", func(t *testing.T) {
		t.Parallel()
		client := coderdenttest.New(t, nil)
		_, err := client.AddLicense(context.Background(), codersdk.AddLicenseRequest{
			License: "content",
		})
		errResp := &codersdk.Error{}
		if xerrors.As(err, &errResp) {
			assert.Equal(t, 401, errResp.StatusCode())
		} else {
			t.Error("expected to get error status 401")
		}
	})

	t.Run("Corrupted", func(t *testing.T) {
		t.Parallel()
		client := coderdenttest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		coderdenttest.AddLicense(t, client, coderdenttest.LicenseOptions{})
		_, err := client.AddLicense(context.Background(), codersdk.AddLicenseRequest{
			License: "invalid",
		})
		errResp := &codersdk.Error{}
		if xerrors.As(err, &errResp) {
			assert.Equal(t, 400, errResp.StatusCode())
		} else {
			t.Error("expected to get error status 400")
		}
	})
}

func TestGetLicense(t *testing.T) {
	t.Parallel()
	t.Run("Success", func(t *testing.T) {
		t.Parallel()
		client := coderdenttest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		coderdenttest.AddLicense(t, client, coderdenttest.LicenseOptions{
			AccountID:   "testing",
			AuditLog:    true,
			SCIM:        true,
			BrowserOnly: true,
			RBACEnabled: true,
		})

		coderdenttest.AddLicense(t, client, coderdenttest.LicenseOptions{
			AccountID:   "testing2",
			AuditLog:    true,
			SCIM:        true,
			BrowserOnly: true,
			Trial:       true,
			UserLimit:   200,
			RBACEnabled: false,
		})

		licenses, err := client.Licenses(ctx)
		require.NoError(t, err)
		require.Len(t, licenses, 2)
		assert.Equal(t, int32(1), licenses[0].ID)
		assert.Equal(t, "testing", licenses[0].Claims["account_id"])
		assert.Equal(t, map[string]interface{}{
			codersdk.FeatureUserLimit:      json.Number("0"),
			codersdk.FeatureAuditLog:       json.Number("1"),
			codersdk.FeatureSCIM:           json.Number("1"),
			codersdk.FeatureBrowserOnly:    json.Number("1"),
			codersdk.FeatureWorkspaceQuota: json.Number("0"),
			codersdk.FeatureRBAC:           json.Number("1"),
		}, licenses[0].Claims["features"])
		assert.Equal(t, int32(2), licenses[1].ID)
		assert.Equal(t, "testing2", licenses[1].Claims["account_id"])
		assert.Equal(t, true, licenses[1].Claims["trial"])
		assert.Equal(t, map[string]interface{}{
			codersdk.FeatureUserLimit:      json.Number("200"),
			codersdk.FeatureAuditLog:       json.Number("1"),
			codersdk.FeatureSCIM:           json.Number("1"),
			codersdk.FeatureBrowserOnly:    json.Number("1"),
			codersdk.FeatureWorkspaceQuota: json.Number("0"),
			codersdk.FeatureRBAC:           json.Number("0"),
		}, licenses[1].Claims["features"])
	})
}

func TestDeleteLicense(t *testing.T) {
	t.Parallel()
	t.Run("Empty", func(t *testing.T) {
		t.Parallel()
		client := coderdenttest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		err := client.DeleteLicense(ctx, 1)
		errResp := &codersdk.Error{}
		if xerrors.As(err, &errResp) {
			assert.Equal(t, 404, errResp.StatusCode())
		} else {
			t.Error("expected to get error status 404")
		}
	})

	t.Run("BadID", func(t *testing.T) {
		t.Parallel()
		client := coderdenttest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		resp, err := client.Request(ctx, http.MethodDelete, "/api/v2/licenses/drivers", nil)
		require.NoError(t, err)
		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
		require.NoError(t, resp.Body.Close())
	})

	t.Run("Success", func(t *testing.T) {
		t.Parallel()
		client := coderdenttest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		coderdenttest.AddLicense(t, client, coderdenttest.LicenseOptions{
			AccountID: "testing",
			AuditLog:  true,
		})
		coderdenttest.AddLicense(t, client, coderdenttest.LicenseOptions{
			AccountID: "testing2",
			AuditLog:  true,
			UserLimit: 200,
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
