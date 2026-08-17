package coderd_test

import (
	"context"
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
	"github.com/coder/coder/v2/enterprise/trialer"
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

	t.Run("InvalidAgentRuntimeClaims", func(t *testing.T) {
		t.Parallel()
		client, _ := coderdenttest.New(t, &coderdenttest.Options{DontAddLicense: true})
		// A soft limit claim without an allocation claim rejects the whole
		// license.
		lic := coderdenttest.GenerateLicense(t, coderdenttest.LicenseOptions{
			Features: license.Features{
				license.ClaimAgentRuntimeHoursLimitSoft: 80,
			},
		})
		_, err := client.AddLicense(context.Background(), codersdk.AddLicenseRequest{
			License: lic,
		})
		errResp := &codersdk.Error{}
		require.ErrorAs(t, err, &errResp)
		require.Equal(t, http.StatusBadRequest, errResp.StatusCode())
		require.Contains(t, errResp.Message, "Invalid license")
	})

	t.Run("AgentRuntimeClaims", func(t *testing.T) {
		t.Parallel()
		client, _ := coderdenttest.New(t, &coderdenttest.Options{DontAddLicense: true})
		coderdenttest.AddLicense(t, client, coderdenttest.LicenseOptions{
			Features: license.Features{
				license.ClaimAgentRuntimeHoursAllocation: 100,
				license.ClaimAgentRuntimeHoursLimitSoft:  80,
				license.ClaimAgentRuntimeHoursLimitHard:  120,
			},
		})
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

	trialRequest := codersdk.CreateTrialLicenseRequest{
		Email:       "coder@coder.com",
		FirstName:   "Coder",
		LastName:    "McCoder",
		PhoneNumber: "+14155550100",
		JobTitle:    "Platform Engineer",
		CompanyName: "Coder",
		Country:     "United States",
		Developers:  "51 - 100",
	}

	// requesterCall records what the licensor stub was handed. It travels over a
	// channel because the stub runs on the request goroutine.
	type requesterCall struct {
		deploymentID string
		req          codersdk.CreateTrialLicenseRequest
	}

	t.Run("Success", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		// Generated outside the stub: GenerateLicense fails the test on error, which
		// is only safe on the test goroutine.
		lic := coderdenttest.GenerateLicense(t, coderdenttest.LicenseOptions{
			Trial:      true,
			FeatureSet: codersdk.FeatureSetPremium,
		})
		calls := make(chan requesterCall, 1)
		client, _, api, _ := coderdenttest.NewWithAPI(t, &coderdenttest.Options{
			DontAddLicense: true,
			TrialLicenseRequester: func(_ context.Context, deploymentID string, req codersdk.CreateTrialLicenseRequest) (string, error) {
				calls <- requesterCall{deploymentID: deploymentID, req: req}
				return lic, nil
			},
		})

		respLic, err := client.CreateTrialLicense(ctx, trialRequest)
		require.NoError(t, err)
		require.True(t, respLic.Trial())
		require.GreaterOrEqual(t, respLic.ID, int32(0))

		licenses, err := client.Licenses(ctx)
		require.NoError(t, err)
		require.Len(t, licenses, 1)

		// The handler refreshes entitlements itself, so a trial is usable without a
		// separate refresh call.
		//nolint:gocritic // Entitlements authz is not what's under test here.
		entitlements, err := client.Entitlements(ctx)
		require.NoError(t, err)
		require.True(t, entitlements.HasLicense)
		require.True(t, entitlements.Trial)

		require.Len(t, calls, 1)
		got := <-calls
		require.Equal(t, api.AGPL.DeploymentID, got.deploymentID)
		require.Equal(t, trialRequest, got.req)
	})

	t.Run("AlreadyLicensed", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		calls := make(chan requesterCall, 1)
		client, _ := coderdenttest.New(t, &coderdenttest.Options{
			TrialLicenseRequester: func(_ context.Context, deploymentID string, req codersdk.CreateTrialLicenseRequest) (string, error) {
				calls <- requesterCall{deploymentID: deploymentID, req: req}
				return "", nil
			},
		})

		//nolint:gocritic // Requesting a trial is owner-only; the Forbidden subtest covers non-owners.
		_, err := client.CreateTrialLicense(ctx, trialRequest)
		errResp := &codersdk.Error{}
		require.ErrorAs(t, err, &errResp)
		require.Equal(t, http.StatusConflict, errResp.StatusCode())
		require.Contains(t, errResp.Message, "already has a license")
		require.Empty(t, calls, "the licensor must not be contacted when a license is installed")
	})

	t.Run("NotConfigured", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		client, _ := coderdenttest.New(t, &coderdenttest.Options{DontAddLicense: true})

		//nolint:gocritic // Requesting a trial is owner-only; the Forbidden subtest covers non-owners.
		_, err := client.CreateTrialLicense(ctx, trialRequest)
		errResp := &codersdk.Error{}
		require.ErrorAs(t, err, &errResp)
		require.Equal(t, http.StatusNotImplemented, errResp.StatusCode())
	})

	t.Run("Forbidden", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		calls := make(chan requesterCall, 1)
		client, user := coderdenttest.New(t, &coderdenttest.Options{
			DontAddLicense: true,
			TrialLicenseRequester: func(_ context.Context, deploymentID string, req codersdk.CreateTrialLicenseRequest) (string, error) {
				calls <- requesterCall{deploymentID: deploymentID, req: req}
				return "", nil
			},
		})
		member, _ := coderdtest.CreateAnotherUser(t, client, user.OrganizationID)

		_, err := member.CreateTrialLicense(ctx, trialRequest)
		errResp := &codersdk.Error{}
		require.ErrorAs(t, err, &errResp)
		require.Equal(t, http.StatusForbidden, errResp.StatusCode())
		require.Empty(t, calls, "the licensor must not be contacted for an unauthorized request")
	})

	t.Run("MissingFields", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		client, _ := coderdenttest.New(t, &coderdenttest.Options{
			DontAddLicense: true,
			TrialLicenseRequester: func(context.Context, string, codersdk.CreateTrialLicenseRequest) (string, error) {
				return "", nil
			},
		})

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

	t.Run("InvalidEmail", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		client, _ := coderdenttest.New(t, &coderdenttest.Options{
			DontAddLicense: true,
			TrialLicenseRequester: func(context.Context, string, codersdk.CreateTrialLicenseRequest) (string, error) {
				return "", nil
			},
		})

		req := trialRequest
		req.Email = "not-an-email"
		//nolint:gocritic // Requesting a trial is owner-only; the Forbidden subtest covers non-owners.
		_, err := client.CreateTrialLicense(ctx, req)
		errResp := &codersdk.Error{}
		require.ErrorAs(t, err, &errResp)
		require.Equal(t, http.StatusBadRequest, errResp.StatusCode())
	})

	t.Run("LicensorRejects", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		const message = "this deployment already had a trial"
		client, _ := coderdenttest.New(t, &coderdenttest.Options{
			DontAddLicense: true,
			TrialLicenseRequester: func(context.Context, string, codersdk.CreateTrialLicenseRequest) (string, error) {
				return "", &trialer.LicensorError{Message: message}
			},
		})

		//nolint:gocritic // Requesting a trial is owner-only; the Forbidden subtest covers non-owners.
		_, err := client.CreateTrialLicense(ctx, trialRequest)
		errResp := &codersdk.Error{}
		require.ErrorAs(t, err, &errResp)
		require.Equal(t, http.StatusBadRequest, errResp.StatusCode())
		require.Equal(t, message, errResp.Detail)
	})

	t.Run("LicensorUnreachable", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		client, _ := coderdenttest.New(t, &coderdenttest.Options{
			DontAddLicense: true,
			TrialLicenseRequester: func(context.Context, string, codersdk.CreateTrialLicenseRequest) (string, error) {
				return "", xerrors.New("dial tcp: connection refused")
			},
		})

		//nolint:gocritic // Requesting a trial is owner-only; the Forbidden subtest covers non-owners.
		_, err := client.CreateTrialLicense(ctx, trialRequest)
		errResp := &codersdk.Error{}
		require.ErrorAs(t, err, &errResp)
		require.Equal(t, http.StatusBadGateway, errResp.StatusCode())
	})

	t.Run("InvalidLicenseFromLicensor", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		client, _ := coderdenttest.New(t, &coderdenttest.Options{
			DontAddLicense: true,
			TrialLicenseRequester: func(context.Context, string, codersdk.CreateTrialLicenseRequest) (string, error) {
				return "not-a-jwt", nil
			},
		})

		//nolint:gocritic // Requesting a trial is owner-only; the Forbidden subtest covers non-owners.
		_, err := client.CreateTrialLicense(ctx, trialRequest)
		errResp := &codersdk.Error{}
		require.ErrorAs(t, err, &errResp)
		require.Equal(t, http.StatusBadGateway, errResp.StatusCode())
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
