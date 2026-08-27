package coderd_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database/dbtime"
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

	t.Run("UnusableAgentRuntimeClaims", func(t *testing.T) {
		t.Parallel()
		client, _ := coderdenttest.New(t, &coderdenttest.Options{DontAddLicense: true})
		// A soft limit claim without an allocation claim is unusable, but it
		// never rejects the whole license: the license stays valid, the
		// runtime hours feature is simply not granted, and the dropped claim
		// is surfaced as a warning. See decodeAgentRuntimeHours.
		lic := coderdenttest.GenerateLicense(t, coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureUserLimit:               100,
				license.ClaimAgentRuntimeHoursLimitSoft: 80,
			},
		})
		_, err := client.AddLicense(context.Background(), codersdk.AddLicenseRequest{
			License: lic,
		})
		require.NoError(t, err)
		// The claims round-trip through GET /api/v2/entitlements.
		//nolint:gocritic // This test asserts license state, not authz behavior.
		entitlements, err := client.Entitlements(context.Background())
		require.NoError(t, err)
		require.True(t, entitlements.HasLicense)
		require.Empty(t, entitlements.Errors)
		require.Contains(t, entitlements.Warnings,
			codersdk.LicenseAgentRuntimeHoursClaimsIgnoredWarningText)
		feature := entitlements.Features[codersdk.FeatureAgentRuntimeHours]
		require.Nil(t, feature.Limit)
		require.Nil(t, feature.UsagePeriod)
	})

	t.Run("AgentRuntimeClaims", func(t *testing.T) {
		t.Parallel()
		client, _ := coderdenttest.New(t, &coderdenttest.Options{DontAddLicense: true})
		coderdenttest.AddLicense(t, client,
			*(&coderdenttest.LicenseOptions{}).AgentRuntimeHours(100, new(int64(80)), new(int64(120))))
		// The claims round-trip through GET /api/v2/entitlements.
		//nolint:gocritic // This test asserts license state, not authz behavior.
		entitlements, err := client.Entitlements(context.Background())
		require.NoError(t, err)
		feature := entitlements.Features[codersdk.FeatureAgentRuntimeHours]
		require.Equal(t, codersdk.EntitlementEntitled, feature.Entitlement)
		require.True(t, feature.Enabled)
		require.NotNil(t, feature.Limit)
		require.EqualValues(t, 100, *feature.Limit)
		require.NotNil(t, feature.SoftLimit)
		require.EqualValues(t, 80, *feature.SoftLimit)
		require.NotNil(t, feature.HardLimit)
		require.EqualValues(t, 120, *feature.HardLimit)
		require.NotNil(t, feature.UsagePeriod)
		// Actual is read from usage_events, which has no runtime events in
		// this deployment. It is reported in whole hours, matching the unit
		// of the claims above, with the precise milliseconds in ActualMs.
		require.NotNil(t, feature.Actual)
		require.EqualValues(t, 0, *feature.Actual)
		require.NotNil(t, feature.ActualMs)
		require.EqualValues(t, 0, *feature.ActualMs)
		require.Empty(t, entitlements.Errors)
		// Zero usage is below both thresholds, so no runtime warning
		// fires. Unrelated warnings from this bare license are ignored.
		// The negatives are built from the exported constants so a reword
		// cannot silently disarm this guard.
		require.NotContains(t, entitlements.Warnings,
			fmt.Sprintf(codersdk.LicenseAgentRuntimeHoursSoftLimitWarningText, 0, 100, 80))
		require.NotContains(t, entitlements.Warnings,
			fmt.Sprintf(codersdk.LicenseAgentRuntimeHoursAllocationReachedWarningText, 0, 100))
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
			NotBefore: dbtime.Now().Add(time.Hour),
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
			NotBefore: dbtime.Now().Add(time.Hour),
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

func TestPostTrialLicense(t *testing.T) {
	t.Parallel()

	t.Run("Forbidden", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		client, user := coderdenttest.New(t, nil)
		member, _ := coderdtest.CreateAnotherUser(t, client, user.OrganizationID)

		_, err := member.CreateTrialLicense(ctx, codersdk.CreateTrialLicenseRequest{})
		errResp := &codersdk.Error{}
		require.ErrorAs(t, err, &errResp)
		require.Equal(t, http.StatusForbidden, errResp.StatusCode())
	})

	t.Run("MissingFields", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		client, _ := coderdenttest.New(t, nil)

		//nolint:gocritic // Requesting a trial is owner-only; the Forbidden subtest covers non-owners.
		_, err := client.CreateTrialLicense(ctx, codersdk.CreateTrialLicenseRequest{})
		errResp := &codersdk.Error{}
		require.ErrorAs(t, err, &errResp)
		require.Equal(t, http.StatusBadRequest, errResp.StatusCode())
		fields := make([]string, 0, len(errResp.Validations))
		for _, v := range errResp.Validations {
			fields = append(fields, v.Field)
		}
		require.Contains(t, fields, "email")
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
