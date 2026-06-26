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

// scim2User is a minimal struct for decoding SCIM 2.0 user responses
// returned by the elimity-com/scim library.
type scim2User struct {
	ID       string `json:"id"`
	UserName string `json:"userName"`
	Active   bool   `json:"active"`
}

// scim2UserBody is the request body for SCIM 2.0 POST/PUT calls.
// Unlike the legacy handler, the elimity-com/scim library validates the
// "schemas" attribute against the core User schema URI and rejects bodies
// that omit it.
type scim2UserBody struct {
	Schemas  []string `json:"schemas"`
	UserName string   `json:"userName"`
	Name     struct {
		GivenName  string `json:"givenName"`
		FamilyName string `json:"familyName"`
	} `json:"name"`
	Emails []struct {
		Primary bool   `json:"primary"`
		Value   string `json:"value"`
	} `json:"emails"`
	Active *bool `json:"active,omitempty"`
}

func makeScim2User(t testing.TB) scim2UserBody {
	rstr, err := cryptorand.String(10)
	require.NoError(t, err)

	b := scim2UserBody{
		Schemas:  []string{"urn:ietf:params:scim:schemas:core:2.0:User"},
		UserName: rstr,
		Active:   ptr.Ref(true),
	}
	b.Name.GivenName = rstr
	b.Name.FamilyName = rstr
	b.Emails = []struct {
		Primary bool   `json:"primary"`
		Value   string `json:"value"`
	}{{Primary: true, Value: fmt.Sprintf("%s@coder.com", rstr)}}
	return b
}

// TestScim exercises the SCIM 2.0 handler through real HTTP routes. It
// mirrors TestLegacyScim's structure (disabled/noAuth/post/patch/put) and
// adds coverage for behavior unique to the v2 implementation: discovery
// endpoints, 409 Conflict on duplicate active users, suspended-user
// reactivation, GET by id, and DELETE.
//
//nolint:gocritic // SCIM authenticates via a special header and bypasses internal RBAC.
func TestScim(t *testing.T) {
	t.Parallel()

	t.Run("disabled", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		client, _ := coderdenttest.New(t, &coderdenttest.Options{
			SCIMAPIKey: []byte("hi"),
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
			SCIMAPIKey: []byte("hi"),
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

	t.Run("discovery", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		scimAPIKey := []byte("hi")
		client, _ := coderdenttest.New(t, &coderdenttest.Options{
			SCIMAPIKey: scimAPIKey,
			LicenseOptions: &coderdenttest.LicenseOptions{
				AccountID: "coolin",
				Features:  license.Features{codersdk.FeatureSCIM: 1},
			},
		})

		for _, path := range []string{
			"/scim/v2/ServiceProviderConfig",
			"/scim/v2/ResourceTypes",
			"/scim/v2/Schemas",
		} {
			res, err := client.Request(ctx, "GET", path, nil, setScimAuth(scimAPIKey))
			require.NoError(t, err)
			_ = res.Body.Close()
			assert.Equal(t, http.StatusOK, res.StatusCode, "discovery endpoint %s", path)
		}
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
			SCIMAPIKey:   scimAPIKey,
			AuditLogging: true,
			LicenseOptions: &coderdenttest.LicenseOptions{
				AccountID: "coolin",
				Features: license.Features{
					codersdk.FeatureSCIM:                  1,
					codersdk.FeatureAuditLog:              1,
					codersdk.FeatureMultipleOrganizations: 1,
				},
			},
		})

		sUser := makeScim2User(t)
		res, err := client.Request(ctx, "POST", "/scim/v2/Users", sUser, setScimAuth(scimAPIKey))
		require.NoError(t, err)
		defer res.Body.Close()
		require.Equal(t, http.StatusCreated, res.StatusCode)

		var created scim2User
		require.NoError(t, json.NewDecoder(res.Body).Decode(&created))
		assert.NotEmpty(t, created.ID)
		assert.Equal(t, sUser.UserName, created.UserName)
		assert.True(t, created.Active)

		// Verify user exists.
		userRes, err := client.Users(ctx, codersdk.UsersRequest{Search: created.UserName})
		require.NoError(t, err)
		require.Len(t, userRes.Users, 1)
		assert.Equal(t, codersdk.LoginTypeOIDC, userRes.Users[0].LoginType)

		// Verify audit log.
		require.True(t, len(mockAudit.AuditLogs()) > 0)

		// Verify no user admin notification (SCIM skips notifications).
		require.Empty(t, notifyEnq.Sent())
	})

	t.Run("postUserConflict", func(t *testing.T) {
		// SCIM 2.0 returns 409 Conflict on duplicate active user, unlike the
		// legacy handler which returned 200 with the existing user.
		t.Parallel()
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		scimAPIKey := []byte("hi")
		client, _ := coderdenttest.New(t, &coderdenttest.Options{
			SCIMAPIKey: scimAPIKey,
			LicenseOptions: &coderdenttest.LicenseOptions{
				AccountID: "coolin",
				Features: license.Features{
					codersdk.FeatureSCIM:                  1,
					codersdk.FeatureMultipleOrganizations: 1,
				},
			},
		})

		sUser := makeScim2User(t)
		res, err := client.Request(ctx, "POST", "/scim/v2/Users", sUser, setScimAuth(scimAPIKey))
		require.NoError(t, err)
		_ = res.Body.Close()
		require.Equal(t, http.StatusCreated, res.StatusCode)

		res, err = client.Request(ctx, "POST", "/scim/v2/Users", sUser, setScimAuth(scimAPIKey))
		require.NoError(t, err)
		_ = res.Body.Close()
		assert.Equal(t, http.StatusConflict, res.StatusCode)

		userRes, err := client.Users(ctx, codersdk.UsersRequest{Search: sUser.UserName})
		require.NoError(t, err)
		require.Len(t, userRes.Users, 1)
	})

	t.Run("postUserReactivatesSuspended", func(t *testing.T) {
		// When the SCIM client deletes a user (which only suspends in Coder),
		// posting the same user again should reactivate the existing row.
		t.Parallel()
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		scimAPIKey := []byte("hi")
		client, _ := coderdenttest.New(t, &coderdenttest.Options{
			SCIMAPIKey: scimAPIKey,
			LicenseOptions: &coderdenttest.LicenseOptions{
				AccountID: "coolin",
				Features: license.Features{
					codersdk.FeatureSCIM:                  1,
					codersdk.FeatureMultipleOrganizations: 1,
				},
			},
		})

		sUser := makeScim2User(t)
		res, err := client.Request(ctx, "POST", "/scim/v2/Users", sUser, setScimAuth(scimAPIKey))
		require.NoError(t, err)
		var created scim2User
		require.NoError(t, json.NewDecoder(res.Body).Decode(&created))
		_ = res.Body.Close()
		require.Equal(t, http.StatusCreated, res.StatusCode)
		require.NotEmpty(t, created.ID)

		// Delete (suspends) the user.
		res, err = client.Request(ctx, "DELETE", "/scim/v2/Users/"+created.ID, nil, setScimAuth(scimAPIKey))
		require.NoError(t, err)
		_ = res.Body.Close()
		assert.Equal(t, http.StatusNoContent, res.StatusCode)

		userRes, err := client.User(ctx, created.ID)
		require.NoError(t, err)
		assert.Equal(t, codersdk.UserStatusSuspended, userRes.Status)

		// Re-create. The handler should reactivate the existing row.
		res, err = client.Request(ctx, "POST", "/scim/v2/Users", sUser, setScimAuth(scimAPIKey))
		require.NoError(t, err)
		var recreated scim2User
		require.NoError(t, json.NewDecoder(res.Body).Decode(&recreated))
		_ = res.Body.Close()
		require.Equal(t, http.StatusCreated, res.StatusCode)
		assert.Equal(t, created.ID, recreated.ID, "recreate should reactivate the existing row, not create a new one")
		assert.True(t, recreated.Active, "recreated user should be active in the SCIM response")

		// The DB user moves from suspended → dormant on reactivate; the SCIM
		// response reports both Active and Dormant as active=true.
		userRes, err = client.User(ctx, created.ID)
		require.NoError(t, err)
		assert.Equal(t, codersdk.UserStatusDormant, userRes.Status)
	})

	t.Run("getUser", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		scimAPIKey := []byte("hi")
		client, _ := coderdenttest.New(t, &coderdenttest.Options{
			SCIMAPIKey: scimAPIKey,
			LicenseOptions: &coderdenttest.LicenseOptions{
				AccountID: "coolin",
				Features: license.Features{
					codersdk.FeatureSCIM:                  1,
					codersdk.FeatureMultipleOrganizations: 1,
				},
			},
		})

		sUser := makeScim2User(t)
		res, err := client.Request(ctx, "POST", "/scim/v2/Users", sUser, setScimAuth(scimAPIKey))
		require.NoError(t, err)
		var created scim2User
		require.NoError(t, json.NewDecoder(res.Body).Decode(&created))
		_ = res.Body.Close()
		require.Equal(t, http.StatusCreated, res.StatusCode)

		res, err = client.Request(ctx, "GET", "/scim/v2/Users/"+created.ID, nil, setScimAuth(scimAPIKey))
		require.NoError(t, err)
		defer res.Body.Close()
		require.Equal(t, http.StatusOK, res.StatusCode)

		var got scim2User
		require.NoError(t, json.NewDecoder(res.Body).Decode(&got))
		assert.Equal(t, created.ID, got.ID)
		assert.Equal(t, sUser.UserName, got.UserName)
	})

	t.Run("patchUser", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		scimAPIKey := []byte("hi")
		mockAudit := audit.NewMock()
		client, _ := coderdenttest.New(t, &coderdenttest.Options{
			Options:      &coderdtest.Options{Auditor: mockAudit},
			SCIMAPIKey:   scimAPIKey,
			AuditLogging: true,
			LicenseOptions: &coderdenttest.LicenseOptions{
				AccountID: "coolin",
				Features: license.Features{
					codersdk.FeatureSCIM:                  1,
					codersdk.FeatureAuditLog:              1,
					codersdk.FeatureMultipleOrganizations: 1,
				},
			},
		})

		sUser := makeScim2User(t)
		res, err := client.Request(ctx, "POST", "/scim/v2/Users", sUser, setScimAuth(scimAPIKey))
		require.NoError(t, err)
		var created scim2User
		require.NoError(t, json.NewDecoder(res.Body).Decode(&created))
		_ = res.Body.Close()
		require.Equal(t, http.StatusCreated, res.StatusCode)

		// PATCH with replace op setting active=false.
		mockAudit.ResetLogs()
		patchBody := map[string]interface{}{
			"schemas": []string{"urn:ietf:params:scim:api:messages:2.0:PatchOp"},
			"Operations": []map[string]interface{}{
				{"op": "replace", "path": "active", "value": false},
			},
		}
		res, err = client.Request(ctx, "PATCH", "/scim/v2/Users/"+created.ID, patchBody, setScimAuth(scimAPIKey))
		require.NoError(t, err)
		_ = res.Body.Close()
		assert.Equal(t, http.StatusOK, res.StatusCode)

		userRes, err := client.User(ctx, created.ID)
		require.NoError(t, err)
		assert.Equal(t, codersdk.UserStatusSuspended, userRes.Status)
	})

	t.Run("putUser", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		scimAPIKey := []byte("hi")
		client, _ := coderdenttest.New(t, &coderdenttest.Options{
			SCIMAPIKey: scimAPIKey,
			LicenseOptions: &coderdenttest.LicenseOptions{
				AccountID: "coolin",
				Features: license.Features{
					codersdk.FeatureSCIM:                  1,
					codersdk.FeatureMultipleOrganizations: 1,
				},
			},
		})

		sUser := makeScim2User(t)
		res, err := client.Request(ctx, "POST", "/scim/v2/Users", sUser, setScimAuth(scimAPIKey))
		require.NoError(t, err)
		var created scim2User
		require.NoError(t, json.NewDecoder(res.Body).Decode(&created))
		_ = res.Body.Close()
		require.Equal(t, http.StatusCreated, res.StatusCode)

		// PUT with active=false.
		sUser.Active = ptr.Ref(false)
		res, err = client.Request(ctx, "PUT", "/scim/v2/Users/"+created.ID, sUser, setScimAuth(scimAPIKey))
		require.NoError(t, err)
		_ = res.Body.Close()
		assert.Equal(t, http.StatusOK, res.StatusCode)

		userRes, err := client.User(ctx, created.ID)
		require.NoError(t, err)
		assert.Equal(t, codersdk.UserStatusSuspended, userRes.Status)
	})

	t.Run("deleteUser", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		scimAPIKey := []byte("hi")
		client, _ := coderdenttest.New(t, &coderdenttest.Options{
			SCIMAPIKey: scimAPIKey,
			LicenseOptions: &coderdenttest.LicenseOptions{
				AccountID: "coolin",
				Features: license.Features{
					codersdk.FeatureSCIM:                  1,
					codersdk.FeatureMultipleOrganizations: 1,
				},
			},
		})

		sUser := makeScim2User(t)
		res, err := client.Request(ctx, "POST", "/scim/v2/Users", sUser, setScimAuth(scimAPIKey))
		require.NoError(t, err)
		var created scim2User
		require.NoError(t, json.NewDecoder(res.Body).Decode(&created))
		_ = res.Body.Close()
		require.Equal(t, http.StatusCreated, res.StatusCode)

		res, err = client.Request(ctx, "DELETE", "/scim/v2/Users/"+created.ID, nil, setScimAuth(scimAPIKey))
		require.NoError(t, err)
		_ = res.Body.Close()
		assert.Equal(t, http.StatusNoContent, res.StatusCode)

		// Coder does not hard-delete users. The user should remain but be suspended.
		userRes, err := client.User(ctx, created.ID)
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
