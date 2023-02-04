package coderd_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/golang-jwt/jwt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/testutil"
)

// nolint:bodyclose
func TestUserOIDC(t *testing.T) {
	t.Parallel()
	t.Run("Groups", func(t *testing.T) {
		t.Parallel()
		t.Run("Assigns", func(t *testing.T) {
			t.Parallel()

			ctx, _ := testutil.Context(t)
			conf := coderdtest.NewOIDCConfig(t, "")

			config := conf.OIDCConfig(t, jwt.MapClaims{})
			config.AllowSignups = true

			client := coderdenttest.New(t, &coderdenttest.Options{
				Options: &coderdtest.Options{
					OIDCConfig: config,
				},
			})
			_ = coderdtest.CreateFirstUser(t, client)
			coderdenttest.AddLicense(t, client, coderdenttest.LicenseOptions{
				AllFeatures: true,
			})

			admin, err := client.User(ctx, "me")
			require.NoError(t, err)
			require.Len(t, admin.OrganizationIDs, 1)

			groupName := "bingbong"
			group, err := client.CreateGroup(ctx, admin.OrganizationIDs[0], codersdk.CreateGroupRequest{
				Name: groupName,
			})
			require.NoError(t, err)
			require.Len(t, group.Members, 0)

			resp := oidcCallback(t, client, conf.EncodeClaims(t, jwt.MapClaims{
				"email":  "colin@coder.com",
				"groups": []string{groupName},
			}))
			assert.Equal(t, http.StatusTemporaryRedirect, resp.StatusCode)

			group, err = client.Group(ctx, group.ID)
			require.NoError(t, err)
			require.Len(t, group.Members, 1)
		})
		t.Run("NoneMatch", func(t *testing.T) {
			t.Parallel()

			ctx, _ := testutil.Context(t)
			conf := coderdtest.NewOIDCConfig(t, "")

			config := conf.OIDCConfig(t, jwt.MapClaims{})
			config.AllowSignups = true

			client := coderdenttest.New(t, &coderdenttest.Options{
				Options: &coderdtest.Options{
					OIDCConfig: config,
				},
			})
			_ = coderdtest.CreateFirstUser(t, client)
			coderdenttest.AddLicense(t, client, coderdenttest.LicenseOptions{
				AllFeatures: true,
			})

			admin, err := client.User(ctx, "me")
			require.NoError(t, err)
			require.Len(t, admin.OrganizationIDs, 1)

			groupName := "bingbong"
			group, err := client.CreateGroup(ctx, admin.OrganizationIDs[0], codersdk.CreateGroupRequest{
				Name: groupName,
			})
			require.NoError(t, err)
			require.Len(t, group.Members, 0)

			resp := oidcCallback(t, client, conf.EncodeClaims(t, jwt.MapClaims{
				"email":  "colin@coder.com",
				"groups": []string{"coolin"},
			}))
			assert.Equal(t, http.StatusTemporaryRedirect, resp.StatusCode)

			group, err = client.Group(ctx, group.ID)
			require.NoError(t, err)
			require.Len(t, group.Members, 0)
		})
	})
}

func oidcCallback(t *testing.T, client *codersdk.Client, code string) *http.Response {
	t.Helper()
	client.HTTPClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}
	oauthURL, err := client.URL.Parse(fmt.Sprintf("/api/v2/users/oidc/callback?code=%s&state=somestate", code))
	require.NoError(t, err)
	req, err := http.NewRequestWithContext(context.Background(), "GET", oauthURL.String(), nil)
	require.NoError(t, err)
	req.AddCookie(&http.Cookie{
		Name:  codersdk.OAuth2StateCookie,
		Value: "somestate",
	})
	res, err := client.HTTPClient.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()
	data, err := io.ReadAll(res.Body)
	require.NoError(t, err)
	t.Log(string(data))
	return res
}
