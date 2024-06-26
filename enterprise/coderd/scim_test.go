package coderd_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/cryptorand"
	"github.com/coder/coder/v2/enterprise/coderd"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
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
		Active: true,
	}
}

func setScimAuth(key []byte) func(*http.Request) {
	return func(r *http.Request) {
		r.Header.Set("Authorization", string(key))
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
			assert.Equal(t, http.StatusNotFound, res.StatusCode)
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
			assert.Equal(t, http.StatusInternalServerError, res.StatusCode)
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
			require.Equal(t, http.StatusOK, res.StatusCode)

			aLogs := mockAudit.AuditLogs()
			require.Len(t, aLogs, 1)
			af := map[string]string{}
			err = json.Unmarshal([]byte(aLogs[0].AdditionalFields), &af)
			require.NoError(t, err)
			assert.Equal(t, coderd.SCIMAuditAdditionalFields, af)
			assert.Equal(t, database.AuditActionCreate, aLogs[0].Action)

			userRes, err := client.Users(ctx, codersdk.UsersRequest{Search: sUser.Emails[0].Value})
			require.NoError(t, err)
			require.Len(t, userRes.Users, 1)

			assert.Equal(t, sUser.Emails[0].Value, userRes.Users[0].Email)
			assert.Equal(t, sUser.UserName, userRes.Users[0].Username)
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

			sUser.Active = false
			res, err = client.Request(ctx, "PATCH", "/scim/v2/Users/"+sUser.ID, sUser, setScimAuth(scimAPIKey))
			require.NoError(t, err)
			_, _ = io.Copy(io.Discard, res.Body)
			_ = res.Body.Close()
			assert.Equal(t, http.StatusOK, res.StatusCode)

			sUser.Active = true
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
			assert.Equal(t, http.StatusNotFound, res.StatusCode)
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
			assert.Equal(t, http.StatusInternalServerError, res.StatusCode)
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

			sUser.Active = false

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
	})
}
