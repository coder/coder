package coderd_test

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestPreviewSignIn(t *testing.T) {
	t.Parallel()

	dv := coderdtest.DeploymentValues(t)
	dv.Experiments = []string{string(codersdk.ExperimentOpenPreview)}
	client, _, api := coderdtest.NewWithAPI(t, &coderdtest.Options{
		AppHostname:      "*.apps.test.coder.com",
		DeploymentValues: dv,
	})
	first := coderdtest.CreateFirstUser(t, client)
	owner, err := client.User(testutil.Context(t, testutil.WaitShort), codersdk.Me)
	require.NoError(t, err)
	siteAdmin, _ := coderdtest.CreateAnotherUser(t, client, first.OrganizationID, rbac.RoleOwner())
	ws := dbfake.WorkspaceBuild(t, api.Database, database.WorkspaceTable{
		Name:           "site",
		OwnerID:        owner.ID,
		OrganizationID: first.OrganizationID,
	}).WithAgent().Do()

	signIn := func(c *codersdk.Client, req codersdk.PreviewSignInRequest) (codersdk.PreviewSignInResponse, error) {
		return codersdk.NewExperimentalClient(c).PreviewSignIn(testutil.Context(t, testutil.WaitShort), req)
	}
	valid := codersdk.PreviewSignInRequest{
		WorkspaceID: ws.Workspace.ID,
		AgentName:   "dev",
		PreviewSlug: "preview--8080",
		URLPath:     "/docs/?page=2",
	}

	t.Run("SignsInOnlyThePreviewHost", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)

		res, err := signIn(client, valid)
		require.NoError(t, err)
		host := "preview--8080--dev--" + ws.Workspace.Name + "--" + owner.Username + ".apps.test.coder.com:" + client.URL.Port()
		require.Equal(t, "http://"+host, res.Origin)
		require.Equal(t, "allow-scripts allow-same-origin allow-forms allow-modals allow-downloads", res.Sandbox)
		previewURL, err := url.Parse(res.URL)
		require.NoError(t, err)
		require.Equal(t, host, previewURL.Host)
		require.Equal(t, "/docs/", previewURL.Path)
		require.True(t, strings.HasPrefix(previewURL.RawQuery, "page=2&coder_preview_sign_in="), previewURL.RawQuery)

		// The preview host trades the token for a cookie on itself only.
		follow := func(host string) (int, http.Header) {
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, client.URL.String()+previewURL.RequestURI(), nil)
			require.NoError(t, err)
			req.Host = host
			req.Header.Set("Sec-Fetch-Dest", "iframe")
			noRedirect := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			}}
			res, err := noRedirect.Do(req)
			require.NoError(t, err)
			defer res.Body.Close()
			return res.StatusCode, res.Header
		}
		status, header := follow(strings.Replace(host, "preview--8080", "preview--3000", 1))
		require.Equal(t, http.StatusBadRequest, status)
		require.Empty(t, header.Values("Set-Cookie"))

		status, header = follow(host)
		require.Equal(t, http.StatusSeeOther, status)
		require.Equal(t, "/docs/?page=2", header.Get("Location"))
		cookies := (&http.Response{Header: header}).Cookies()
		require.Len(t, cookies, 1)
		require.Equal(t, codersdk.PreviewSessionTokenCookie, cookies[0].Name)
		require.Empty(t, cookies[0].Domain)
		require.True(t, cookies[0].HttpOnly)

		// The cookie holds an application_connect key of the owner who
		// signed in.
		keyID, _, _ := strings.Cut(cookies[0].Value, "-")
		//nolint:gocritic // The test reads the key row the endpoint created.
		key, err := api.Database.GetAPIKeyByID(dbauthz.AsSystemRestricted(ctx), keyID)
		require.NoError(t, err)
		require.Equal(t, owner.ID, key.UserID)
		require.Equal(t, database.APIKeyScopes{database.ApiKeyScopeCoderApplicationConnect}, key.Scopes)
	})

	t.Run("Refused", func(t *testing.T) {
		t.Parallel()

		for _, tc := range []struct {
			name       string
			client     *codersdk.Client
			mutate     func(*codersdk.PreviewSignInRequest)
			wantStatus int
		}{
			// A site owner can connect to the workspace's apps, but a preview
			// opens only for the workspace owner.
			{name: "NotWorkspaceOwner", client: siteAdmin, wantStatus: http.StatusNotFound},
			{name: "UnknownWorkspace", mutate: func(r *codersdk.PreviewSignInRequest) { r.WorkspaceID = uuid.New() }, wantStatus: http.StatusNotFound},
			{name: "AppSlug", mutate: func(r *codersdk.PreviewSignInRequest) { r.PreviewSlug = "code-server" }, wantStatus: http.StatusBadRequest},
			{name: "ReservedPort", mutate: func(r *codersdk.PreviewSignInRequest) { r.PreviewSlug = "preview--4" }, wantStatus: http.StatusBadRequest},
			{name: "OffHostPath", mutate: func(r *codersdk.PreviewSignInRequest) { r.URLPath = "//evil.example.com/" }, wantStatus: http.StatusBadRequest},
			{name: "UnknownAgent", mutate: func(r *codersdk.PreviewSignInRequest) { r.AgentName = "other" }, wantStatus: http.StatusBadRequest},
			{name: "UnknownRegion", mutate: func(r *codersdk.PreviewSignInRequest) { r.RegionID = uuid.New() }, wantStatus: http.StatusBadRequest},
		} {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				req := valid
				if tc.mutate != nil {
					tc.mutate(&req)
				}
				c := client
				if tc.client != nil {
					c = tc.client
				}
				_, err := signIn(c, req)
				var sdkErr *codersdk.Error
				require.ErrorAs(t, err, &sdkErr)
				require.Equal(t, tc.wantStatus, sdkErr.StatusCode())
			})
		}
	})
}
