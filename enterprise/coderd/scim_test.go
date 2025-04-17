package coderd_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/golang-jwt/jwt/v4"
	"github.com/google/uuid"
	"github.com/imulab/go-scim/pkg/v2/handlerutil"
	"github.com/imulab/go-scim/pkg/v2/spec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/coderdtest/oidctest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/notifications/notificationstest"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/cryptorand"
	"github.com/coder/coder/v2/enterprise/coderd"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/enterprise/coderd/scim"
	"github.com/coder/coder/v2/testutil"
)

//nolint:revive
func makeScimUser(t testing.TB) coderd.SCIMUser {
	rstr, err := cryptorand.String(10)
	require.NoError(t, err)

	return coderd.SCIMUser{
		UserName: rstr,
		Name: struct {
			GivenName  string "json:\"givenName\""
			FamilyName string "json:\"familyName\""
		}{
			GivenName:  rstr,
			FamilyName: rstr,
		},
		Emails: []struct {
			Primary bool   "json:\"primary\""
			Value   string "json:\"value\" format:\"email\""
			Type    string "json:\"type\""
			Display string "json:\"display\""
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

func setScimAuthBearer(key []byte) func(*http.Request) {
	return func(r *http.Request) {
		// Do strange casing to ensure it's case-insensitive
		r.Header.Set("Authorization", "beAreR "+string(key))
	}
}

//nolint:gocritic // SCIM authenticates via a special header and bypasses internal RBAC.
func TestScim(t *testing.T) {
	t.Parallel()

	t.Run("postUser", func(t *testing.T) {
		t.Parallel()

		t.Run("disabled", func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
			defer cancel()

			client, _ := coderdenttest.New(t, &coderdenttest.Options{
				SCIMAPIKey: []byte("hi"),
				LicenseOptions: &coderdenttest.LicenseOptions{
					AccountID: "coolin",
					Features: license.Features{
						codersdk.FeatureSCIM: 0,
					},
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
					Features: license.Features{
						codersdk.FeatureSCIM: 1,
					},
				},
			})

			res, err := client.Request(ctx, "POST", "/scim/v2/Users", struct{}{})
			require.NoError(t, err)
			defer res.Body.Close()
			assert.Equal(t, http.StatusUnauthorized, res.StatusCode)
		})

		t.Run("OK", func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
			defer cancel()

			// given
			scimAPIKey := []byte("hi")
			mockAudit := audit.NewMock()
			db, ps := dbtestutil.NewDB(t)
			notifyEnq := &notificationstest.FakeEnqueuer{Store: db}
			client, _ := coderdenttest.New(t, &coderdenttest.Options{
				Options: &coderdtest.Options{
					Auditor:               mockAudit,
					NotificationsEnqueuer: notifyEnq,
					Database:              db,
					Pubsub:                ps,
				},
				SCIMAPIKey:   scimAPIKey,
				AuditLogging: true,
				LicenseOptions: &coderdenttest.LicenseOptions{
					AccountID: "coolin",
					Features: license.Features{
						codersdk.FeatureSCIM:     1,
						codersdk.FeatureAuditLog: 1,
					},
				},
			})
			mockAudit.ResetLogs()

			// verify scim is enabled
			res, err := client.Request(ctx, http.MethodGet, "/scim/v2/ServiceProviderConfig", nil)
			require.NoError(t, err)
			defer res.Body.Close()
			require.Equal(t, http.StatusOK, res.StatusCode)

			// when
			sUser := makeScimUser(t)
			res, err = client.Request(ctx, http.MethodPost, "/scim/v2/Users", sUser, setScimAuth(scimAPIKey))
			require.NoError(t, err)
			defer res.Body.Close()
			require.Equal(t, http.StatusOK, res.StatusCode)

			// then
			// Expect audit logs
			aLogs := mockAudit.AuditLogs()
			require.Len(t, aLogs, 1)
			af := map[string]string{}
			err = json.Unmarshal([]byte(aLogs[0].AdditionalFields), &af)
			require.NoError(t, err)
			assert.Equal(t, coderd.SCIMAuditAdditionalFields, af)
			assert.Equal(t, database.AuditActionCreate, aLogs[0].Action)

			// Expect users exposed over API
			userRes, err := client.Users(ctx, codersdk.UsersRequest{Search: sUser.Emails[0].Value})
			require.NoError(t, err)
			require.Len(t, userRes.Users, 1)
			assert.Equal(t, sUser.Emails[0].Value, userRes.Users[0].Email)
			assert.Equal(t, sUser.UserName, userRes.Users[0].Username)
			assert.Len(t, userRes.Users[0].OrganizationIDs, 1)

			// Expect zero notifications (SkipNotifications = true)
			require.Empty(t, notifyEnq.Sent())
		})

		t.Run("OK_Bearer", func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
			defer cancel()

			// given
			scimAPIKey := []byte("hi")
			mockAudit := audit.NewMock()
			db, ps := dbtestutil.NewDB(t)
			notifyEnq := &notificationstest.FakeEnqueuer{Store: db}
			client, _ := coderdenttest.New(t, &coderdenttest.Options{
				Options: &coderdtest.Options{
					Auditor:               mockAudit,
					NotificationsEnqueuer: notifyEnq,
					Database:              db,
					Pubsub:                ps,
				},
				SCIMAPIKey:   scimAPIKey,
				AuditLogging: true,
				LicenseOptions: &coderdenttest.LicenseOptions{
					AccountID: "coolin",
					Features: license.Features{
						codersdk.FeatureSCIM:     1,
						codersdk.FeatureAuditLog: 1,
					},
				},
			})
			mockAudit.ResetLogs()

			// when
			sUser := makeScimUser(t)
			res, err := client.Request(ctx, "POST", "/scim/v2/Users", sUser, setScimAuthBearer(scimAPIKey))
			require.NoError(t, err)
			defer res.Body.Close()
			require.Equal(t, http.StatusOK, res.StatusCode)

			// then
			// Expect audit logs
			aLogs := mockAudit.AuditLogs()
			require.Len(t, aLogs, 1)
			af := map[string]string{}
			err = json.Unmarshal([]byte(aLogs[0].AdditionalFields), &af)
			require.NoError(t, err)
			assert.Equal(t, coderd.SCIMAuditAdditionalFields, af)
			assert.Equal(t, database.AuditActionCreate, aLogs[0].Action)

			// Expect users exposed over API
			userRes, err := client.Users(ctx, codersdk.UsersRequest{Search: sUser.Emails[0].Value})
			require.NoError(t, err)
			require.Len(t, userRes.Users, 1)
			assert.Equal(t, sUser.Emails[0].Value, userRes.Users[0].Email)
			assert.Equal(t, sUser.UserName, userRes.Users[0].Username)
			assert.Len(t, userRes.Users[0].OrganizationIDs, 1)

			// Expect zero notifications (SkipNotifications = true)
			require.Empty(t, notifyEnq.Sent())
		})

		t.Run("OKNoDefault", func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
			defer cancel()

			// given
			scimAPIKey := []byte("hi")
			mockAudit := audit.NewMock()
			db, ps := dbtestutil.NewDB(t)
			notifyEnq := &notificationstest.FakeEnqueuer{Store: db}
			dv := coderdtest.DeploymentValues(t)
			dv.OIDC.OrganizationAssignDefault = false
			client, _ := coderdenttest.New(t, &coderdenttest.Options{
				Options: &coderdtest.Options{
					Auditor:               mockAudit,
					NotificationsEnqueuer: notifyEnq,
					DeploymentValues:      dv,
					Database:              db,
					Pubsub:                ps,
				},
				SCIMAPIKey:   scimAPIKey,
				AuditLogging: true,
				LicenseOptions: &coderdenttest.LicenseOptions{
					AccountID: "coolin",
					Features: license.Features{
						codersdk.FeatureSCIM:     1,
						codersdk.FeatureAuditLog: 1,
					},
				},
			})
			mockAudit.ResetLogs()

			// when
			sUser := makeScimUser(t)
			res, err := client.Request(ctx, "POST", "/scim/v2/Users", sUser, setScimAuth(scimAPIKey))
			require.NoError(t, err)
			defer res.Body.Close()
			require.Equal(t, http.StatusOK, res.StatusCode)

			// then
			// Expect audit logs
			aLogs := mockAudit.AuditLogs()
			require.Len(t, aLogs, 1)
			af := map[string]string{}
			err = json.Unmarshal([]byte(aLogs[0].AdditionalFields), &af)
			require.NoError(t, err)
			assert.Equal(t, coderd.SCIMAuditAdditionalFields, af)
			assert.Equal(t, database.AuditActionCreate, aLogs[0].Action)

			// Expect users exposed over API
			userRes, err := client.Users(ctx, codersdk.UsersRequest{Search: sUser.Emails[0].Value})
			require.NoError(t, err)
			require.Len(t, userRes.Users, 1)
			assert.Equal(t, sUser.Emails[0].Value, userRes.Users[0].Email)
			assert.Equal(t, sUser.UserName, userRes.Users[0].Username)
			assert.Len(t, userRes.Users[0].OrganizationIDs, 0)

			// Expect zero notifications (SkipNotifications = true)
			require.Empty(t, notifyEnq.Sent())
		})

		t.Run("Duplicate", func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
			defer cancel()

			scimAPIKey := []byte("hi")
			client, _ := coderdenttest.New(t, &coderdenttest.Options{
				SCIMAPIKey: scimAPIKey,
				LicenseOptions: &coderdenttest.LicenseOptions{
					AccountID: "coolin",
					Features: license.Features{
						codersdk.FeatureSCIM: 1,
					},
				},
			})

			sUser := makeScimUser(t)
			for i := 0; i < 3; i++ {
				res, err := client.Request(ctx, "POST", "/scim/v2/Users", sUser, setScimAuth(scimAPIKey))
				require.NoError(t, err)
				_ = res.Body.Close()
				assert.Equal(t, http.StatusOK, res.StatusCode)
			}

			userRes, err := client.Users(ctx, codersdk.UsersRequest{Search: sUser.Emails[0].Value})
			require.NoError(t, err)
			require.Len(t, userRes.Users, 1)

			assert.Equal(t, sUser.Emails[0].Value, userRes.Users[0].Email)
			assert.Equal(t, sUser.UserName, userRes.Users[0].Username)
		})

		t.Run("Unsuspend", func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
			defer cancel()

			scimAPIKey := []byte("hi")
			client, _ := coderdenttest.New(t, &coderdenttest.Options{
				SCIMAPIKey: scimAPIKey,
				LicenseOptions: &coderdenttest.LicenseOptions{
					AccountID: "coolin",
					Features: license.Features{
						codersdk.FeatureSCIM: 1,
					},
				},
			})

			sUser := makeScimUser(t)
			res, err := client.Request(ctx, "POST", "/scim/v2/Users", sUser, setScimAuth(scimAPIKey))
			require.NoError(t, err)
			defer res.Body.Close()
			assert.Equal(t, http.StatusOK, res.StatusCode)
			err = json.NewDecoder(res.Body).Decode(&sUser)
			require.NoError(t, err)

			sUser.Active = ptr.Ref(false)
			res, err = client.Request(ctx, "PATCH", "/scim/v2/Users/"+sUser.ID, sUser, setScimAuth(scimAPIKey))
			require.NoError(t, err)
			_, _ = io.Copy(io.Discard, res.Body)
			_ = res.Body.Close()
			assert.Equal(t, http.StatusOK, res.StatusCode)

			sUser.Active = ptr.Ref(true)
			res, err = client.Request(ctx, "POST", "/scim/v2/Users", sUser, setScimAuth(scimAPIKey))
			require.NoError(t, err)
			_, _ = io.Copy(io.Discard, res.Body)
			_ = res.Body.Close()
			assert.Equal(t, http.StatusOK, res.StatusCode)

			userRes, err := client.Users(ctx, codersdk.UsersRequest{Search: sUser.Emails[0].Value})
			require.NoError(t, err)
			require.Len(t, userRes.Users, 1)

			assert.Equal(t, sUser.Emails[0].Value, userRes.Users[0].Email)
			assert.Equal(t, sUser.UserName, userRes.Users[0].Username)
			assert.Equal(t, codersdk.UserStatusDormant, userRes.Users[0].Status)
		})

		t.Run("DomainStrips", func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
			defer cancel()

			scimAPIKey := []byte("hi")
			client, _ := coderdenttest.New(t, &coderdenttest.Options{
				SCIMAPIKey: scimAPIKey,
				LicenseOptions: &coderdenttest.LicenseOptions{
					AccountID: "coolin",
					Features: license.Features{
						codersdk.FeatureSCIM: 1,
					},
				},
			})

			sUser := makeScimUser(t)
			sUser.UserName = sUser.UserName + "@coder.com"
			res, err := client.Request(ctx, "POST", "/scim/v2/Users", sUser, setScimAuth(scimAPIKey))
			require.NoError(t, err)
			_, _ = io.Copy(io.Discard, res.Body)
			_ = res.Body.Close()
			assert.Equal(t, http.StatusOK, res.StatusCode)

			userRes, err := client.Users(ctx, codersdk.UsersRequest{Search: sUser.Emails[0].Value})
			require.NoError(t, err)
			require.Len(t, userRes.Users, 1)

			assert.Equal(t, sUser.Emails[0].Value, userRes.Users[0].Email)
			// Username should be the same as the given name. They all use the
			// same string before we modified it above.
			assert.Equal(t, sUser.Name.GivenName, userRes.Users[0].Username)
		})
	})

	t.Run("patchUser", func(t *testing.T) {
		t.Parallel()

		t.Run("disabled", func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
			defer cancel()

			client, _ := coderdenttest.New(t, &coderdenttest.Options{
				SCIMAPIKey: []byte("hi"),
				LicenseOptions: &coderdenttest.LicenseOptions{
					AccountID: "coolin",
					Features: license.Features{
						codersdk.FeatureSCIM: 0,
					},
				},
			})

			res, err := client.Request(ctx, "PATCH", "/scim/v2/Users/bob", struct{}{})
			require.NoError(t, err)
			_, _ = io.Copy(io.Discard, res.Body)
			_ = res.Body.Close()
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
					Features: license.Features{
						codersdk.FeatureSCIM: 1,
					},
				},
			})

			res, err := client.Request(ctx, "PATCH", "/scim/v2/Users/bob", struct{}{})
			require.NoError(t, err)
			_, _ = io.Copy(io.Discard, res.Body)
			_ = res.Body.Close()
			assert.Equal(t, http.StatusUnauthorized, res.StatusCode)
		})

		t.Run("OK", func(t *testing.T) {
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
						codersdk.FeatureSCIM:     1,
						codersdk.FeatureAuditLog: 1,
					},
				},
			})
			mockAudit.ResetLogs()

			sUser := makeScimUser(t)
			res, err := client.Request(ctx, "POST", "/scim/v2/Users", sUser, setScimAuth(scimAPIKey))
			require.NoError(t, err)
			defer res.Body.Close()
			assert.Equal(t, http.StatusOK, res.StatusCode)
			mockAudit.ResetLogs()

			err = json.NewDecoder(res.Body).Decode(&sUser)
			require.NoError(t, err)

			sUser.Active = ptr.Ref(false)

			res, err = client.Request(ctx, "PATCH", "/scim/v2/Users/"+sUser.ID, sUser, setScimAuth(scimAPIKey))
			require.NoError(t, err)
			_, _ = io.Copy(io.Discard, res.Body)
			_ = res.Body.Close()
			assert.Equal(t, http.StatusOK, res.StatusCode)

			aLogs := mockAudit.AuditLogs()
			require.Len(t, aLogs, 1)
			assert.Equal(t, database.AuditActionWrite, aLogs[0].Action)

			userRes, err := client.Users(ctx, codersdk.UsersRequest{Search: sUser.Emails[0].Value})
			require.NoError(t, err)
			require.Len(t, userRes.Users, 1)
			assert.Equal(t, codersdk.UserStatusSuspended, userRes.Users[0].Status)
		})

		// Create a user via SCIM, which starts as dormant.
		// Log in as the user, making them active.
		// Then patch the user again and the user should still be active.
		t.Run("ActiveIsActive", func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
			defer cancel()

			scimAPIKey := []byte("hi")

			mockAudit := audit.NewMock()
			fake := oidctest.NewFakeIDP(t, oidctest.WithServing())
			client, _ := coderdenttest.New(t, &coderdenttest.Options{
				Options: &coderdtest.Options{
					Auditor:    mockAudit,
					OIDCConfig: fake.OIDCConfig(t, []string{}),
				},
				SCIMAPIKey:   scimAPIKey,
				AuditLogging: true,
				LicenseOptions: &coderdenttest.LicenseOptions{
					AccountID: "coolin",
					Features: license.Features{
						codersdk.FeatureSCIM:     1,
						codersdk.FeatureAuditLog: 1,
					},
				},
			})
			mockAudit.ResetLogs()

			// User is dormant on create
			sUser := makeScimUser(t)
			res, err := client.Request(ctx, "POST", "/scim/v2/Users", sUser, setScimAuth(scimAPIKey))
			require.NoError(t, err)
			defer res.Body.Close()
			assert.Equal(t, http.StatusOK, res.StatusCode)

			err = json.NewDecoder(res.Body).Decode(&sUser)
			require.NoError(t, err)

			// Check the audit log
			aLogs := mockAudit.AuditLogs()
			require.Len(t, aLogs, 1)
			assert.Equal(t, database.AuditActionCreate, aLogs[0].Action)

			// Verify the user is dormant
			scimUser, err := client.User(ctx, sUser.UserName)
			require.NoError(t, err)
			require.Equal(t, codersdk.UserStatusDormant, scimUser.Status, "user starts as dormant")

			// Log in as the user, making them active
			//nolint:bodyclose
			scimUserClient, _ := fake.Login(t, client, jwt.MapClaims{
				"email": sUser.Emails[0].Value,
				"sub":   uuid.NewString(),
			})
			scimUser, err = scimUserClient.User(ctx, codersdk.Me)
			require.NoError(t, err)
			require.Equal(t, codersdk.UserStatusActive, scimUser.Status, "user should now be active")

			// Patch the user
			mockAudit.ResetLogs()
			res, err = client.Request(ctx, "PATCH", "/scim/v2/Users/"+sUser.ID, sUser, setScimAuth(scimAPIKey))
			require.NoError(t, err)
			_, _ = io.Copy(io.Discard, res.Body)
			_ = res.Body.Close()
			assert.Equal(t, http.StatusOK, res.StatusCode)

			// Should be no audit logs since there is no diff
			aLogs = mockAudit.AuditLogs()
			require.Len(t, aLogs, 0)

			// Verify the user is still active.
			scimUser, err = client.User(ctx, sUser.UserName)
			require.NoError(t, err)
			require.Equal(t, codersdk.UserStatusActive, scimUser.Status, "user is still active")
		})
	})

	t.Run("putUser", func(t *testing.T) {
		t.Parallel()

		t.Run("disabled", func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
			defer cancel()

			client, _ := coderdenttest.New(t, &coderdenttest.Options{
				SCIMAPIKey: []byte("hi"),
				LicenseOptions: &coderdenttest.LicenseOptions{
					AccountID: "coolin",
					Features: license.Features{
						codersdk.FeatureSCIM: 0,
					},
				},
			})

			res, err := client.Request(ctx, http.MethodPut, "/scim/v2/Users/bob", struct{}{})
			require.NoError(t, err)
			_, _ = io.Copy(io.Discard, res.Body)
			_ = res.Body.Close()
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
					Features: license.Features{
						codersdk.FeatureSCIM: 1,
					},
				},
			})

			res, err := client.Request(ctx, http.MethodPut, "/scim/v2/Users/bob", struct{}{})
			require.NoError(t, err)
			_, _ = io.Copy(io.Discard, res.Body)
			_ = res.Body.Close()
			assert.Equal(t, http.StatusUnauthorized, res.StatusCode)
		})

		t.Run("MissingActiveField", func(t *testing.T) {
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
						codersdk.FeatureSCIM:     1,
						codersdk.FeatureAuditLog: 1,
					},
				},
			})
			mockAudit.ResetLogs()

			sUser := makeScimUser(t)
			res, err := client.Request(ctx, "POST", "/scim/v2/Users", sUser, setScimAuth(scimAPIKey))
			require.NoError(t, err)
			defer res.Body.Close()
			assert.Equal(t, http.StatusOK, res.StatusCode)
			mockAudit.ResetLogs()

			err = json.NewDecoder(res.Body).Decode(&sUser)
			require.NoError(t, err)

			sUser.Active = nil

			res, err = client.Request(ctx, http.MethodPut, "/scim/v2/Users/"+sUser.ID, sUser, setScimAuth(scimAPIKey))
			require.NoError(t, err)
			defer res.Body.Close()
			assert.Equal(t, http.StatusBadRequest, res.StatusCode)

			data, err := io.ReadAll(res.Body)
			require.NoError(t, err)
			require.Contains(t, string(data), "active field is required")
			mockAudit.ResetLogs()
		})

		t.Run("ImmutabilityViolation", func(t *testing.T) {
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
						codersdk.FeatureSCIM:     1,
						codersdk.FeatureAuditLog: 1,
					},
				},
			})
			mockAudit.ResetLogs()

			sUser := makeScimUser(t)
			res, err := client.Request(ctx, "POST", "/scim/v2/Users", sUser, setScimAuth(scimAPIKey))
			require.NoError(t, err)
			defer res.Body.Close()
			assert.Equal(t, http.StatusOK, res.StatusCode)
			mockAudit.ResetLogs()

			err = json.NewDecoder(res.Body).Decode(&sUser)
			require.NoError(t, err)

			sUser.UserName += "changed"

			res, err = client.Request(ctx, http.MethodPut, "/scim/v2/Users/"+sUser.ID, sUser, setScimAuth(scimAPIKey))
			require.NoError(t, err)
			defer res.Body.Close()
			assert.Equal(t, http.StatusBadRequest, res.StatusCode)
			mockAudit.ResetLogs()

			data, err := io.ReadAll(res.Body)
			require.NoError(t, err)
			require.Contains(t, string(data), "mutability")
			require.NoError(t, err)
		})

		t.Run("OK", func(t *testing.T) {
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
						codersdk.FeatureSCIM:     1,
						codersdk.FeatureAuditLog: 1,
					},
				},
			})
			mockAudit.ResetLogs()

			sUser := makeScimUser(t)
			res, err := client.Request(ctx, "POST", "/scim/v2/Users", sUser, setScimAuth(scimAPIKey))
			require.NoError(t, err)
			defer res.Body.Close()
			assert.Equal(t, http.StatusOK, res.StatusCode)
			mockAudit.ResetLogs()

			err = json.NewDecoder(res.Body).Decode(&sUser)
			require.NoError(t, err)

			sUser.Active = ptr.Ref(false)

			res, err = client.Request(ctx, http.MethodPatch, "/scim/v2/Users/"+sUser.ID, sUser, setScimAuth(scimAPIKey))
			require.NoError(t, err)
			_, _ = io.Copy(io.Discard, res.Body)
			_ = res.Body.Close()
			assert.Equal(t, http.StatusOK, res.StatusCode)

			aLogs := mockAudit.AuditLogs()
			require.Len(t, aLogs, 1)
			assert.Equal(t, database.AuditActionWrite, aLogs[0].Action)

			userRes, err := client.Users(ctx, codersdk.UsersRequest{Search: sUser.Emails[0].Value})
			require.NoError(t, err)
			require.Len(t, userRes.Users, 1)
			assert.Equal(t, codersdk.UserStatusSuspended, userRes.Users[0].Status)
		})

		// Create a user via SCIM, which starts as dormant.
		// Log in as the user, making them active.
		// Then patch the user again and the user should still be active.
		t.Run("ActiveIsActive", func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
			defer cancel()

			scimAPIKey := []byte("hi")

			mockAudit := audit.NewMock()
			fake := oidctest.NewFakeIDP(t, oidctest.WithServing())
			client, _ := coderdenttest.New(t, &coderdenttest.Options{
				Options: &coderdtest.Options{
					Auditor:    mockAudit,
					OIDCConfig: fake.OIDCConfig(t, []string{}),
				},
				SCIMAPIKey:   scimAPIKey,
				AuditLogging: true,
				LicenseOptions: &coderdenttest.LicenseOptions{
					AccountID: "coolin",
					Features: license.Features{
						codersdk.FeatureSCIM:     1,
						codersdk.FeatureAuditLog: 1,
					},
				},
			})
			mockAudit.ResetLogs()

			// User is dormant on create
			sUser := makeScimUser(t)
			res, err := client.Request(ctx, http.MethodPost, "/scim/v2/Users", sUser, setScimAuth(scimAPIKey))
			require.NoError(t, err)
			defer res.Body.Close()
			assert.Equal(t, http.StatusOK, res.StatusCode)

			err = json.NewDecoder(res.Body).Decode(&sUser)
			require.NoError(t, err)

			// Check the audit log
			aLogs := mockAudit.AuditLogs()
			require.Len(t, aLogs, 1)
			assert.Equal(t, database.AuditActionCreate, aLogs[0].Action)

			// Verify the user is dormant
			scimUser, err := client.User(ctx, sUser.UserName)
			require.NoError(t, err)
			require.Equal(t, codersdk.UserStatusDormant, scimUser.Status, "user starts as dormant")

			// Log in as the user, making them active
			//nolint:bodyclose
			scimUserClient, _ := fake.Login(t, client, jwt.MapClaims{
				"email": sUser.Emails[0].Value,
				"sub":   uuid.NewString(),
			})
			scimUser, err = scimUserClient.User(ctx, codersdk.Me)
			require.NoError(t, err)
			require.Equal(t, codersdk.UserStatusActive, scimUser.Status, "user should now be active")

			// Patch the user
			mockAudit.ResetLogs()
			res, err = client.Request(ctx, http.MethodPut, "/scim/v2/Users/"+sUser.ID, sUser, setScimAuth(scimAPIKey))
			require.NoError(t, err)
			_, _ = io.Copy(io.Discard, res.Body)
			_ = res.Body.Close()
			assert.Equal(t, http.StatusOK, res.StatusCode)

			// Should be no audit logs since there is no diff
			aLogs = mockAudit.AuditLogs()
			require.Len(t, aLogs, 0)

			// Verify the user is still active.
			scimUser, err = client.User(ctx, sUser.UserName)
			require.NoError(t, err)
			require.Equal(t, codersdk.UserStatusActive, scimUser.Status, "user is still active")
		})
	})
}

func TestScimError(t *testing.T) {
	t.Parallel()

	// Demonstrates that we cannot use the standard errors
	rw := httptest.NewRecorder()
	_ = handlerutil.WriteError(rw, spec.ErrNotFound)
	resp := rw.Result()
	defer resp.Body.Close()
	require.Equal(t, http.StatusInternalServerError, resp.StatusCode)

	// Our error wrapper works
	rw = httptest.NewRecorder()
	_ = handlerutil.WriteError(rw, scim.NewHTTPError(http.StatusNotFound, spec.ErrNotFound.Type, xerrors.New("not found")))
	resp = rw.Result()
	defer resp.Body.Close()
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
}
