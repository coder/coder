package coderd_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/imulab/go-scim/pkg/v2/handlerutil"
	"github.com/imulab/go-scim/pkg/v2/spec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/notifications/notificationstest"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/cryptorand"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/legacyscim"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/testutil"
)

//nolint:revive
func makeScimUser(t testing.TB) legacyscim.SCIMUser {
	rstr, err := cryptorand.String(10)
	require.NoError(t, err)

	return legacyscim.SCIMUser{
		UserName: rstr,
		Name: struct {
			GivenName  string `json:"givenName"`
			FamilyName string `json:"familyName"`
		}{
			GivenName:  rstr,
			FamilyName: rstr,
		},
		Emails: []struct {
			Primary bool   `json:"primary"`
			Value   string `json:"value" format:"email"`
			Type    string `json:"type"`
			Display string `json:"display"`
		}{
			{Primary: true, Value: fmt.Sprintf("%s@coder.com", rstr)},
		},
		Active: ptr.Ref(true),
	}
}

func setScimAuth(key []byte) func(*http.Request) {
	return func(r *http.Request) {
		r.Header.Set("Authorization", string(key))
	}
}

// TestLegacyScim tests the legacy SCIM handler (imulab/go-scim based).
// This is a reduced set of integration tests verifying HTTP routing, auth,
// and core CRUD. Detailed handler logic is covered by the unit tests in
// enterprise/coderd/scim/scimusers_test.go.
//
//nolint:gocritic // SCIM authenticates via a special header and bypasses internal RBAC.
func TestLegacyScim(t *testing.T) {
	t.Parallel()

	t.Run("disabled", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		client, _ := coderdenttest.New(t, &coderdenttest.Options{
			SCIMAPIKey:    []byte("hi"),
			UseLegacySCIM: true,
			LicenseOptions: &coderdenttest.LicenseOptions{
				AccountID: "coolin",
				Features:  license.Features{codersdk.FeatureSCIM: 0},
			},
		})

		res, err := client.Request(ctx, "POST", "/scim/v2/Users", struct{}{})
		require.NoError(t, err)
		defer res.Body.Close()
		assert.Equal(t, http.StatusForbidden, res.StatusCode)
	})

	t.Run("noAuth", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		client, _ := coderdenttest.New(t, &coderdenttest.Options{
			SCIMAPIKey:    []byte("hi"),
			UseLegacySCIM: true,
			LicenseOptions: &coderdenttest.LicenseOptions{
				AccountID: "coolin",
				Features:  license.Features{codersdk.FeatureSCIM: 1},
			},
		})

		res, err := client.Request(ctx, "POST", "/scim/v2/Users", struct{}{})
		require.NoError(t, err)
		defer res.Body.Close()
		assert.Equal(t, http.StatusUnauthorized, res.StatusCode)
	})

	t.Run("postUser", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		scimAPIKey := []byte("hi")
		mockAudit := audit.NewMock()
		notifyEnq := &notificationstest.FakeEnqueuer{}
		client, _ := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				Auditor:               mockAudit,
				NotificationsEnqueuer: notifyEnq,
			},
			SCIMAPIKey:    scimAPIKey,
			UseLegacySCIM: true,
			AuditLogging:  true,
			LicenseOptions: &coderdenttest.LicenseOptions{
				AccountID: "coolin",
				Features: license.Features{
					codersdk.FeatureSCIM:                  1,
					codersdk.FeatureAuditLog:              1,
					codersdk.FeatureMultipleOrganizations: 1,
				},
			},
		})

		sUser := makeScimUser(t)
		res, err := client.Request(ctx, "POST", "/scim/v2/Users", sUser, setScimAuth(scimAPIKey))
		require.NoError(t, err)
		defer res.Body.Close()
		assert.Equal(t, http.StatusOK, res.StatusCode)

		var createdUser legacyscim.SCIMUser
		err = json.NewDecoder(res.Body).Decode(&createdUser)
		require.NoError(t, err)
		assert.NotEmpty(t, createdUser.ID)
		assert.Equal(t, sUser.UserName, createdUser.UserName)

		// Verify user exists.
		userRes, err := client.Users(ctx, codersdk.UsersRequest{Search: createdUser.UserName})
		require.NoError(t, err)
		require.Len(t, userRes.Users, 1)
		assert.Equal(t, codersdk.LoginTypeOIDC, userRes.Users[0].LoginType)

		// Verify audit log.
		require.True(t, len(mockAudit.AuditLogs()) > 0)

		// Verify no user admin notification (SCIM skips notifications).
		require.Empty(t, notifyEnq.Sent())
	})

	t.Run("Duplicate", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		scimAPIKey := []byte("hi")
		client, _ := coderdenttest.New(t, &coderdenttest.Options{
			SCIMAPIKey:    scimAPIKey,
			UseLegacySCIM: true,
			LicenseOptions: &coderdenttest.LicenseOptions{
				AccountID: "coolin",
				Features: license.Features{
					codersdk.FeatureSCIM:                  1,
					codersdk.FeatureMultipleOrganizations: 1,
				},
			},
		})

		sUser := makeScimUser(t)

		// Create same user 3 times.
		for i := 0; i < 3; i++ {
			res, err := client.Request(ctx, "POST", "/scim/v2/Users", sUser, setScimAuth(scimAPIKey))
			require.NoError(t, err)
			_ = res.Body.Close()
			assert.Equal(t, http.StatusOK, res.StatusCode)
		}

		// Only 1 user should exist.
		userRes, err := client.Users(ctx, codersdk.UsersRequest{Search: sUser.UserName})
		require.NoError(t, err)
		require.Len(t, userRes.Users, 1)
	})

	t.Run("patchUser", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		scimAPIKey := []byte("hi")
		mockAudit := audit.NewMock()
		client, _ := coderdenttest.New(t, &coderdenttest.Options{
			Options:       &coderdtest.Options{Auditor: mockAudit},
			SCIMAPIKey:    scimAPIKey,
			UseLegacySCIM: true,
			AuditLogging:  true,
			LicenseOptions: &coderdenttest.LicenseOptions{
				AccountID: "coolin",
				Features: license.Features{
					codersdk.FeatureSCIM:                  1,
					codersdk.FeatureAuditLog:              1,
					codersdk.FeatureMultipleOrganizations: 1,
				},
			},
		})

		// Create user first.
		sUser := makeScimUser(t)
		res, err := client.Request(ctx, "POST", "/scim/v2/Users", sUser, setScimAuth(scimAPIKey))
		require.NoError(t, err)
		defer res.Body.Close()
		require.Equal(t, http.StatusOK, res.StatusCode)

		var createdUser legacyscim.SCIMUser
		err = json.NewDecoder(res.Body).Decode(&createdUser)
		require.NoError(t, err)

		// Suspend via PATCH.
		mockAudit.ResetLogs()
		sUser.Active = ptr.Ref(false)
		res, err = client.Request(ctx, "PATCH", "/scim/v2/Users/"+createdUser.ID, sUser, setScimAuth(scimAPIKey))
		require.NoError(t, err)
		defer res.Body.Close()
		assert.Equal(t, http.StatusOK, res.StatusCode)

		// Verify suspended.
		userRes, err := client.User(ctx, createdUser.ID)
		require.NoError(t, err)
		assert.Equal(t, codersdk.UserStatusSuspended, userRes.Status)
	})

	t.Run("putUser", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		scimAPIKey := []byte("hi")
		mockAudit := audit.NewMock()
		client, _ := coderdenttest.New(t, &coderdenttest.Options{
			Options:       &coderdtest.Options{Auditor: mockAudit},
			SCIMAPIKey:    scimAPIKey,
			UseLegacySCIM: true,
			AuditLogging:  true,
			LicenseOptions: &coderdenttest.LicenseOptions{
				AccountID: "coolin",
				Features: license.Features{
					codersdk.FeatureSCIM:                  1,
					codersdk.FeatureAuditLog:              1,
					codersdk.FeatureMultipleOrganizations: 1,
				},
			},
		})

		// Create user first.
		sUser := makeScimUser(t)
		res, err := client.Request(ctx, "POST", "/scim/v2/Users", sUser, setScimAuth(scimAPIKey))
		require.NoError(t, err)
		defer res.Body.Close()
		require.Equal(t, http.StatusOK, res.StatusCode)

		var createdUser legacyscim.SCIMUser
		err = json.NewDecoder(res.Body).Decode(&createdUser)
		require.NoError(t, err)

		// Suspend via PUT.
		mockAudit.ResetLogs()
		sUser.Active = ptr.Ref(false)
		res, err = client.Request(ctx, "PUT", "/scim/v2/Users/"+createdUser.ID, sUser, setScimAuth(scimAPIKey))
		require.NoError(t, err)
		defer res.Body.Close()
		assert.Equal(t, http.StatusOK, res.StatusCode)

		// Verify suspended.
		userRes, err := client.User(ctx, createdUser.ID)
		require.NoError(t, err)
		assert.Equal(t, codersdk.UserStatusSuspended, userRes.Status)
	})
}

func TestLegacyScimError(t *testing.T) {
	t.Parallel()

	// Demonstrates that we cannot use the standard errors
	rw := httptest.NewRecorder()
	_ = handlerutil.WriteError(rw, spec.ErrNotFound)
	resp := rw.Result()
	defer resp.Body.Close()
	require.Equal(t, http.StatusInternalServerError, resp.StatusCode)

	// Our error wrapper works
	rw = httptest.NewRecorder()
	_ = handlerutil.WriteError(rw, legacyscim.NewHTTPError(http.StatusNotFound, spec.ErrNotFound.Type, xerrors.New("not found")))
	resp = rw.Result()
	defer resp.Body.Close()
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
}
