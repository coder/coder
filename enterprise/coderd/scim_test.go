package coderd_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/cryptorand"
	"github.com/coder/coder/enterprise/coderd"
	"github.com/coder/coder/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/enterprise/coderd/license"
	"github.com/coder/coder/testutil"
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
			defer res.Body.Close()
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

			res, err := client.Request(ctx, "PATCH", "/scim/v2/Users/bob", struct{}{})
			require.NoError(t, err)
			defer res.Body.Close()
			assert.Equal(t, http.StatusInternalServerError, res.StatusCode)
		})

		t.Run("OK", func(t *testing.T) {
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
			defer res.Body.Close()
			assert.Equal(t, http.StatusOK, res.StatusCode)

			userRes, err := client.Users(ctx, codersdk.UsersRequest{Search: sUser.Emails[0].Value})
			require.NoError(t, err)
			require.Len(t, userRes.Users, 1)
			assert.Equal(t, codersdk.UserStatusSuspended, userRes.Users[0].Status)
		})
	})
}
