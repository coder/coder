package coderd_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/enterprise/coderd/usage/tallymansdk"
	"github.com/coder/coder/v2/testutil"
)

func TestPostUsageEmbeddableDashboard(t *testing.T) {
	t.Parallel()

	t.Run("NoLicense", func(t *testing.T) {
		t.Parallel()

		client, _ := coderdenttest.New(t, &coderdenttest.Options{
			DontAddLicense: true,
		})

		ctx := testutil.Context(t, testutil.WaitShort)
		//nolint:gocritic // owner user is required to read licenses
		_, err := client.GetUsageEmbeddableDashboard(ctx, codersdk.GetUsageEmbeddableDashboardRequest{
			Dashboard: codersdk.UsageEmbeddableDashboardTypeUsage,
		})

		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		assert.Equal(t, http.StatusNotFound, apiErr.StatusCode())
		assert.Contains(t, apiErr.Message, "No license supports usage publishing")
	})

	t.Run("LicenseWithoutPublishing", func(t *testing.T) {
		t.Parallel()

		client, _ := coderdenttest.New(t, &coderdenttest.Options{})
		// Default license has PublishUsageData: false

		ctx := testutil.Context(t, testutil.WaitShort)
		//nolint:gocritic // owner user is required to read licenses
		_, err := client.GetUsageEmbeddableDashboard(ctx, codersdk.GetUsageEmbeddableDashboardRequest{
			Dashboard: codersdk.UsageEmbeddableDashboardTypeUsage,
		})

		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		assert.Equal(t, http.StatusNotFound, apiErr.StatusCode())
		assert.Contains(t, apiErr.Message, "No license supports usage publishing")
	})

	t.Run("UnauthorizedUser", func(t *testing.T) {
		t.Parallel()

		adminClient, adminUser := coderdenttest.New(t, &coderdenttest.Options{})
		coderdenttest.AddLicense(t, adminClient, coderdenttest.LicenseOptions{
			AccountType:      license.AccountTypeSalesforce,
			AccountID:        "test-account",
			PublishUsageData: true,
		})

		// Create a regular user (non-admin)
		memberClient, _ := coderdtest.CreateAnotherUser(t, adminClient, adminUser.OrganizationID)

		ctx := testutil.Context(t, testutil.WaitShort)
		_, err := memberClient.GetUsageEmbeddableDashboard(ctx, codersdk.GetUsageEmbeddableDashboardRequest{
			Dashboard: codersdk.UsageEmbeddableDashboardTypeUsage,
		})

		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		assert.Equal(t, http.StatusForbidden, apiErr.StatusCode())
	})

	t.Run("InvalidDashboardType", func(t *testing.T) {
		t.Parallel()

		client, _ := coderdenttest.New(t, &coderdenttest.Options{})
		coderdenttest.AddLicense(t, client, coderdenttest.LicenseOptions{
			AccountType:      license.AccountTypeSalesforce,
			AccountID:        "test-account",
			PublishUsageData: true,
		})

		ctx := testutil.Context(t, testutil.WaitShort)
		//nolint:gocritic // owner user is required to read licenses
		_, err := client.GetUsageEmbeddableDashboard(ctx, codersdk.GetUsageEmbeddableDashboardRequest{
			Dashboard: "invalid-type",
		})

		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		assert.Equal(t, http.StatusBadRequest, apiErr.StatusCode())
		assert.Contains(t, apiErr.Message, "Invalid dashboard type")
	})

	t.Run("Success", func(t *testing.T) {
		t.Parallel()

		// Create a fake Tallyman server
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Verify the request is correct
			assert.Equal(t, "/api/v1/dashboards/embed", r.URL.Path)
			assert.Equal(t, http.MethodPost, r.Method)

			// Verify authentication headers
			assert.NotEmpty(t, r.Header.Get("Coder-License-Key"), "missing Coder-License-Key header")
			assert.NotEmpty(t, r.Header.Get("Coder-Deployment-ID"), "missing Coder-Deployment-ID header")

			// Return a valid response
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			err := json.NewEncoder(w).Encode(tallymansdk.RetrieveEmbeddableDashboardResponse{
				DashboardURL: "https://app.metronome.com/embed/dashboard/test123",
			})
			assert.NoError(t, err)
		}))
		defer srv.Close()

		tallymanURL, err := url.Parse(srv.URL)
		require.NoError(t, err)

		client, _ := coderdenttest.New(t, &coderdenttest.Options{
			TallymanURL: tallymanURL,
		})
		coderdenttest.AddLicense(t, client, coderdenttest.LicenseOptions{
			AccountType:      license.AccountTypeSalesforce,
			AccountID:        "test-account",
			PublishUsageData: true,
		})

		ctx := testutil.Context(t, testutil.WaitShort)
		//nolint:gocritic // owner user is required to read licenses
		resp, err := client.GetUsageEmbeddableDashboard(ctx, codersdk.GetUsageEmbeddableDashboardRequest{
			Dashboard: codersdk.UsageEmbeddableDashboardTypeUsage,
		})

		require.NoError(t, err)
		assert.Equal(t, "https://app.metronome.com/embed/dashboard/test123", resp.DashboardURL)
	})

	t.Run("WithColorOverrides", func(t *testing.T) {
		t.Parallel()

		// Create a fake Tallyman server that validates the request
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var req tallymansdk.RetrieveEmbeddableDashboardRequest
			err := json.NewDecoder(r.Body).Decode(&req)
			assert.NoError(t, err)

			// Verify color overrides were passed through
			assert.Len(t, req.ColorOverrides, 2)
			assert.Equal(t, "Primary_medium", req.ColorOverrides[0].Name)
			assert.Equal(t, "#FF5733", req.ColorOverrides[0].Value)

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			err = json.NewEncoder(w).Encode(tallymansdk.RetrieveEmbeddableDashboardResponse{
				DashboardURL: "https://dashboard.metronome.com/embed/test",
			})
			assert.NoError(t, err)
		}))
		defer srv.Close()

		tallymanURL, err := url.Parse(srv.URL)
		require.NoError(t, err)

		client, _ := coderdenttest.New(t, &coderdenttest.Options{
			TallymanURL: tallymanURL,
		})
		coderdenttest.AddLicense(t, client, coderdenttest.LicenseOptions{
			AccountType:      license.AccountTypeSalesforce,
			AccountID:        "test-account",
			PublishUsageData: true,
		})

		ctx := testutil.Context(t, testutil.WaitShort)
		//nolint:gocritic // owner user is required to read licenses
		resp, err := client.GetUsageEmbeddableDashboard(ctx, codersdk.GetUsageEmbeddableDashboardRequest{
			Dashboard: codersdk.UsageEmbeddableDashboardTypeUsage,
			ColorOverrides: []codersdk.DashboardColorOverride{
				{Name: "Primary_medium", Value: "#FF5733"},
				{Name: "UsageLine_0", Value: "#33FF57"},
			},
		})

		require.NoError(t, err)
		assert.Equal(t, "https://dashboard.metronome.com/embed/test", resp.DashboardURL)
	})

	t.Run("InvalidColorNameRejected", func(t *testing.T) {
		t.Parallel()

		client, _ := coderdenttest.New(t, &coderdenttest.Options{})
		coderdenttest.AddLicense(t, client, coderdenttest.LicenseOptions{
			AccountType:      license.AccountTypeSalesforce,
			AccountID:        "test-account",
			PublishUsageData: true,
		})

		ctx := testutil.Context(t, testutil.WaitShort)
		//nolint:gocritic // owner user is required to read licenses
		_, err := client.GetUsageEmbeddableDashboard(ctx, codersdk.GetUsageEmbeddableDashboardRequest{
			Dashboard: codersdk.UsageEmbeddableDashboardTypeUsage,
			ColorOverrides: []codersdk.DashboardColorOverride{
				{Name: "invalid_color_name", Value: "#FF5733"},
			},
		})

		require.Error(t, err)
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		assert.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
		assert.Contains(t, sdkErr.Message, "Invalid color name")
	})

	t.Run("UntrustedHostRejected", func(t *testing.T) {
		t.Parallel()

		// Create a fake Tallyman server that returns an untrusted host
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			err := json.NewEncoder(w).Encode(tallymansdk.RetrieveEmbeddableDashboardResponse{
				DashboardURL: "https://evil.com/steal-data",
			})
			assert.NoError(t, err)
		}))
		defer srv.Close()

		tallymanURL, err := url.Parse(srv.URL)
		require.NoError(t, err)

		client, _ := coderdenttest.New(t, &coderdenttest.Options{
			TallymanURL: tallymanURL,
		})
		coderdenttest.AddLicense(t, client, coderdenttest.LicenseOptions{
			AccountType:      license.AccountTypeSalesforce,
			AccountID:        "test-account",
			PublishUsageData: true,
		})

		ctx := testutil.Context(t, testutil.WaitShort)
		_, err = client.GetUsageEmbeddableDashboard(ctx, codersdk.GetUsageEmbeddableDashboardRequest{
			Dashboard: codersdk.UsageEmbeddableDashboardTypeUsage,
		})

		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		assert.Equal(t, http.StatusBadGateway, apiErr.StatusCode())
		assert.Contains(t, apiErr.Message, "Dashboard URL host is not trusted")
	})

	t.Run("TallymanServerError", func(t *testing.T) {
		t.Parallel()

		// Create a fake Tallyman server that returns an error
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			_, err := w.Write([]byte("Internal server error"))
			assert.NoError(t, err)
		}))
		defer srv.Close()

		tallymanURL, err := url.Parse(srv.URL)
		require.NoError(t, err)

		client, _ := coderdenttest.New(t, &coderdenttest.Options{
			TallymanURL: tallymanURL,
		})
		coderdenttest.AddLicense(t, client, coderdenttest.LicenseOptions{
			AccountType:      license.AccountTypeSalesforce,
			AccountID:        "test-account",
			PublishUsageData: true,
		})

		ctx := testutil.Context(t, testutil.WaitShort)
		_, err = client.GetUsageEmbeddableDashboard(ctx, codersdk.GetUsageEmbeddableDashboardRequest{
			Dashboard: codersdk.UsageEmbeddableDashboardTypeUsage,
		})

		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		assert.Equal(t, http.StatusInternalServerError, apiErr.StatusCode())
		assert.Contains(t, apiErr.Message, "Failed to retrieve dashboard URL")
	})
}

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

	t.Run("InvalidAccountID", func(t *testing.T) {
		t.Parallel()
		// The generated deployment will start out with a different deployment ID.
		client, _ := coderdenttest.New(t, &coderdenttest.Options{DontAddLicense: true})
		license := coderdenttest.GenerateLicense(t, coderdenttest.LicenseOptions{
			AllowEmpty: true,
			AccountID:  "",
		})
		_, err := client.AddLicense(context.Background(), codersdk.AddLicenseRequest{
			License: license,
		})
		errResp := &codersdk.Error{}
		require.ErrorAs(t, err, &errResp)
		require.Equal(t, http.StatusBadRequest, errResp.StatusCode())
		require.Contains(t, errResp.Message, "Invalid license")
	})

	t.Run("InvalidAccountType", func(t *testing.T) {
		t.Parallel()
		// The generated deployment will start out with a different deployment ID.
		client, _ := coderdenttest.New(t, &coderdenttest.Options{DontAddLicense: true})
		license := coderdenttest.GenerateLicense(t, coderdenttest.LicenseOptions{
			AllowEmpty:  true,
			AccountType: "",
		})
		_, err := client.AddLicense(context.Background(), codersdk.AddLicenseRequest{
			License: license,
		})
		errResp := &codersdk.Error{}
		require.ErrorAs(t, err, &errResp)
		require.Equal(t, http.StatusBadRequest, errResp.StatusCode())
		require.Contains(t, errResp.Message, "Invalid license")
	})

	t.Run("InvalidLicenseExpires", func(t *testing.T) {
		t.Parallel()
		// The generated deployment will start out with a different deployment ID.
		client, _ := coderdenttest.New(t, &coderdenttest.Options{DontAddLicense: true})
		license := coderdenttest.GenerateLicense(t, coderdenttest.LicenseOptions{
			GraceAt: time.Unix(99999999999, 0),
		})
		_, err := client.AddLicense(context.Background(), codersdk.AddLicenseRequest{
			License: license,
		})
		errResp := &codersdk.Error{}
		require.ErrorAs(t, err, &errResp)
		require.Equal(t, http.StatusBadRequest, errResp.StatusCode())
		require.Contains(t, errResp.Message, "Invalid license")
	})

	t.Run("Unauthorized", func(t *testing.T) {
		t.Parallel()
		client, _ := coderdenttest.New(t, &coderdenttest.Options{DontAddLicense: true})
		client.SetSessionToken("")
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
		client, _ := coderdenttest.New(t, &coderdenttest.Options{DontAddLicense: true})
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
		if xerrors.As(err, &errResp) {
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
