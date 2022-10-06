package coderd_test

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/testutil"
)

func setupAppAuthorizerTest(t *testing.T, allowedSharingLevels []database.AppSharingLevel) (workspace codersdk.Workspace, agent codersdk.WorkspaceAgent, user codersdk.User, client *codersdk.Client, clientWithTemplateAccess *codersdk.Client, clientWithNoTemplateAccess *codersdk.Client, clientWithNoAuth *codersdk.Client) {
	//nolint:gosec
	const password = "password"

	// Create a hello world server.
	//nolint:gosec
	ln, err := net.Listen("tcp", ":0")
	require.NoError(t, err)
	server := http.Server{
		ReadHeaderTimeout: time.Minute,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("Hello World"))
		}),
	}
	t.Cleanup(func() {
		_ = server.Close()
		_ = ln.Close()
	})
	go server.Serve(ln)
	tcpAddr, ok := ln.Addr().(*net.TCPAddr)
	require.True(t, ok)

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	t.Cleanup(cancel)

	// Setup a user, template with apps, workspace on a coderdtest using the
	// EnterpriseAppAuthorizer.
	client = coderdenttest.New(t, &coderdenttest.Options{
		AllowedApplicationSharingLevels: allowedSharingLevels,
		Options: &coderdtest.Options{
			IncludeProvisionerDaemon: true,
		},
	})
	client.HTTPClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}

	firstUser := coderdtest.CreateFirstUser(t, client)
	user, err = client.User(ctx, firstUser.UserID.String())
	require.NoError(t, err)
	coderdenttest.AddLicense(t, client, coderdenttest.LicenseOptions{
		ApplicationSharing: true,
	})
	workspace, agent = setupWorkspaceAgent(t, client, firstUser, uint16(tcpAddr.Port))

	// Verify that the apps have the correct sharing levels set.
	workspaceBuild, err := client.WorkspaceBuild(ctx, workspace.LatestBuild.ID)
	require.NoError(t, err)
	found := map[string]codersdk.WorkspaceAppSharingLevel{}
	expected := map[string]codersdk.WorkspaceAppSharingLevel{
		testAppNameOwner:         codersdk.WorkspaceAppSharingLevelOwner,
		testAppNameTemplate:      codersdk.WorkspaceAppSharingLevelTemplate,
		testAppNameAuthenticated: codersdk.WorkspaceAppSharingLevelAuthenticated,
		testAppNamePublic:        codersdk.WorkspaceAppSharingLevelPublic,
	}
	for _, app := range workspaceBuild.Resources[0].Agents[0].Apps {
		found[app.Name] = app.SharingLevel
	}
	require.Equal(t, expected, found, "apps have incorrect sharing levels")

	// Create a user in the same org (should be able to read the template).
	userWithTemplateAccess, err := client.CreateUser(ctx, codersdk.CreateUserRequest{
		Email:          "template-access@coder.com",
		Username:       "template-access",
		Password:       password,
		OrganizationID: firstUser.OrganizationID,
	})
	require.NoError(t, err)

	clientWithTemplateAccess = codersdk.New(client.URL)
	loginRes, err := clientWithTemplateAccess.LoginWithPassword(ctx, codersdk.LoginWithPasswordRequest{
		Email:    userWithTemplateAccess.Email,
		Password: password,
	})
	require.NoError(t, err)
	clientWithTemplateAccess.SessionToken = loginRes.SessionToken
	clientWithTemplateAccess.HTTPClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}

	// Double check that the user can read the template.
	_, err = clientWithTemplateAccess.Template(ctx, workspace.TemplateID)
	require.NoError(t, err)

	// Create a user in a different org (should not be able to read the
	// template).
	differentOrg, err := client.CreateOrganization(ctx, codersdk.CreateOrganizationRequest{
		Name: "a-different-org",
	})
	require.NoError(t, err)
	userWithNoTemplateAccess, err := client.CreateUser(ctx, codersdk.CreateUserRequest{
		Email:          "no-template-access@coder.com",
		Username:       "no-template-access",
		Password:       password,
		OrganizationID: differentOrg.ID,
	})
	require.NoError(t, err)

	clientWithNoTemplateAccess = codersdk.New(client.URL)
	loginRes, err = clientWithNoTemplateAccess.LoginWithPassword(ctx, codersdk.LoginWithPasswordRequest{
		Email:    userWithNoTemplateAccess.Email,
		Password: password,
	})
	require.NoError(t, err)
	clientWithNoTemplateAccess.SessionToken = loginRes.SessionToken
	clientWithNoTemplateAccess.HTTPClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}

	// Double check that the user cannot read the template.
	_, err = clientWithNoTemplateAccess.Template(ctx, workspace.TemplateID)
	require.Error(t, err)

	// Create an unauthenticated codersdk client.
	clientWithNoAuth = codersdk.New(client.URL)
	clientWithNoAuth.HTTPClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}

	return workspace, agent, user, client, clientWithTemplateAccess, clientWithNoTemplateAccess, clientWithNoAuth
}

func TestEnterpriseAppAuthorizer(t *testing.T) {
	t.Parallel()

	verifyAccess := func(t *testing.T, username, workspaceName, agentName, appName string, client *codersdk.Client, shouldHaveAccess, shouldRedirectToLogin bool) {
		t.Helper()

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		appPath := fmt.Sprintf("/@%s/%s.%s/apps/%s/", username, workspaceName, agentName, appName)
		res, err := client.Request(ctx, http.MethodGet, appPath, nil)
		require.NoError(t, err)
		defer res.Body.Close()

		dump, err := httputil.DumpResponse(res, true)
		require.NoError(t, err)
		t.Logf("response dump: %s", dump)

		if !shouldHaveAccess {
			if shouldRedirectToLogin {
				assert.Equal(t, http.StatusTemporaryRedirect, res.StatusCode, "should not have access, expected temporary redirect")
				location, err := res.Location()
				require.NoError(t, err)
				assert.Equal(t, "/login", location.Path, "should not have access, expected redirect to /login")
			} else {
				// If the user doesn't have access we return 404 to avoid
				// leaking information about the existence of the app.
				assert.Equal(t, http.StatusNotFound, res.StatusCode, "should not have access, expected not found")
			}
		}

		if shouldHaveAccess {
			assert.Equal(t, http.StatusOK, res.StatusCode, "should have access, expected ok")
			assert.Contains(t, string(dump), "Hello World", "should have access, expected hello world")
		}
	}

	t.Run("Disabled", func(t *testing.T) {
		t.Parallel()
		workspace, agent, user, client, clientWithTemplateAccess, clientWithNoTemplateAccess, clientWithNoAuth := setupAppAuthorizerTest(t, []database.AppSharingLevel{
			// Disabled basically means only the owner level is allowed. This
			// should have feature parity with the AGPL version.
			database.AppSharingLevelOwner,
		})

		// Owner should be able to access their own workspace.
		verifyAccess(t, user.Username, workspace.Name, agent.Name, testAppNameOwner, client, true, false)

		// User with or without template access should not have access to a
		// workspace that they do not own.
		verifyAccess(t, user.Username, workspace.Name, agent.Name, testAppNameOwner, clientWithTemplateAccess, false, false)
		verifyAccess(t, user.Username, workspace.Name, agent.Name, testAppNameOwner, clientWithNoTemplateAccess, false, false)

		// Unauthenticated user should not have any access.
		verifyAccess(t, user.Username, workspace.Name, agent.Name, testAppNameOwner, clientWithNoAuth, false, true)
	})

	t.Run("Level", func(t *testing.T) {
		t.Parallel()

		// For the purposes of the level tests we allow all levels.
		workspace, agent, user, client, clientWithTemplateAccess, clientWithNoTemplateAccess, clientWithNoAuth := setupAppAuthorizerTest(t, []database.AppSharingLevel{
			database.AppSharingLevelOwner,
			database.AppSharingLevelTemplate,
			database.AppSharingLevelAuthenticated,
			database.AppSharingLevelPublic,
		})

		t.Run("Owner", func(t *testing.T) {
			t.Parallel()

			// Owner should be able to access their own workspace.
			verifyAccess(t, user.Username, workspace.Name, agent.Name, testAppNameOwner, client, true, false)

			// User with or without template access should not have access to a
			// workspace that they do not own.
			verifyAccess(t, user.Username, workspace.Name, agent.Name, testAppNameOwner, clientWithTemplateAccess, false, false)
			verifyAccess(t, user.Username, workspace.Name, agent.Name, testAppNameOwner, clientWithNoTemplateAccess, false, false)

			// Unauthenticated user should not have any access.
			verifyAccess(t, user.Username, workspace.Name, agent.Name, testAppNameOwner, clientWithNoAuth, false, true)
		})

		t.Run("Template", func(t *testing.T) {
			t.Parallel()

			// Owner should be able to access their own workspace.
			verifyAccess(t, user.Username, workspace.Name, agent.Name, testAppNameTemplate, client, true, false)

			// User with template access should be able to access the workspace.
			verifyAccess(t, user.Username, workspace.Name, agent.Name, testAppNameTemplate, clientWithTemplateAccess, true, false)

			// User without template access should not have access to a workspace
			// that they do not own.
			verifyAccess(t, user.Username, workspace.Name, agent.Name, testAppNameTemplate, clientWithNoTemplateAccess, false, false)

			// Unauthenticated user should not have any access.
			verifyAccess(t, user.Username, workspace.Name, agent.Name, testAppNameTemplate, clientWithNoAuth, false, true)
		})

		t.Run("Authenticated", func(t *testing.T) {
			t.Parallel()

			// Owner should be able to access their own workspace.
			verifyAccess(t, user.Username, workspace.Name, agent.Name, testAppNameAuthenticated, client, true, false)

			// User with or without template access should be able to access the
			// workspace.
			verifyAccess(t, user.Username, workspace.Name, agent.Name, testAppNameAuthenticated, clientWithTemplateAccess, true, false)
			verifyAccess(t, user.Username, workspace.Name, agent.Name, testAppNameAuthenticated, clientWithNoTemplateAccess, true, false)

			// Unauthenticated user should not have any access.
			verifyAccess(t, user.Username, workspace.Name, agent.Name, testAppNameAuthenticated, clientWithNoAuth, false, true)
		})

		t.Run("Public", func(t *testing.T) {
			t.Parallel()

			// Owner should be able to access their own workspace.
			verifyAccess(t, user.Username, workspace.Name, agent.Name, testAppNamePublic, client, true, false)

			// User with or without template access should be able to access the
			// workspace.
			verifyAccess(t, user.Username, workspace.Name, agent.Name, testAppNamePublic, clientWithTemplateAccess, true, false)
			verifyAccess(t, user.Username, workspace.Name, agent.Name, testAppNamePublic, clientWithNoTemplateAccess, true, false)

			// Unauthenticated user should be able to access the workspace.
			verifyAccess(t, user.Username, workspace.Name, agent.Name, testAppNamePublic, clientWithNoAuth, true, false)
		})
	})

	t.Run("LevelBlockedByAdmin", func(t *testing.T) {
		t.Parallel()

		t.Run("Owner", func(t *testing.T) {
			t.Parallel()

			// All levels allowed except owner.
			workspace, agent, user, client, clientWithTemplateAccess, clientWithNoTemplateAccess, clientWithNoAuth := setupAppAuthorizerTest(t, []database.AppSharingLevel{
				database.AppSharingLevelTemplate,
				database.AppSharingLevelAuthenticated,
				database.AppSharingLevelPublic,
			})

			// Owner can always access their own workspace.
			verifyAccess(t, user.Username, workspace.Name, agent.Name, testAppNameOwner, client, true, false)

			// All other users should always be blocked anyways.
			verifyAccess(t, user.Username, workspace.Name, agent.Name, testAppNameOwner, clientWithTemplateAccess, false, false)
			verifyAccess(t, user.Username, workspace.Name, agent.Name, testAppNameOwner, clientWithNoTemplateAccess, false, false)
			verifyAccess(t, user.Username, workspace.Name, agent.Name, testAppNameOwner, clientWithNoAuth, false, true)
		})

		t.Run("Template", func(t *testing.T) {
			t.Parallel()

			// All levels allowed except template.
			workspace, agent, user, client, clientWithTemplateAccess, clientWithNoTemplateAccess, clientWithNoAuth := setupAppAuthorizerTest(t, []database.AppSharingLevel{
				database.AppSharingLevelOwner,
				database.AppSharingLevelAuthenticated,
				database.AppSharingLevelPublic,
			})

			// Owner can always access their own workspace.
			verifyAccess(t, user.Username, workspace.Name, agent.Name, testAppNameTemplate, client, true, false)

			// User with template access should not be able to access the
			// workspace as the template level is disallowed.
			verifyAccess(t, user.Username, workspace.Name, agent.Name, testAppNameTemplate, clientWithTemplateAccess, false, false)

			// All other users should always be blocked anyways.
			verifyAccess(t, user.Username, workspace.Name, agent.Name, testAppNameTemplate, clientWithNoTemplateAccess, false, false)
			verifyAccess(t, user.Username, workspace.Name, agent.Name, testAppNameTemplate, clientWithNoAuth, false, true)
		})

		t.Run("Authenticated", func(t *testing.T) {
			t.Parallel()

			// All levels allowed except authenticated.
			workspace, agent, user, client, clientWithTemplateAccess, clientWithNoTemplateAccess, clientWithNoAuth := setupAppAuthorizerTest(t, []database.AppSharingLevel{
				database.AppSharingLevelOwner,
				database.AppSharingLevelTemplate,
				database.AppSharingLevelPublic,
			})

			// Owner can always access their own workspace.
			verifyAccess(t, user.Username, workspace.Name, agent.Name, testAppNameAuthenticated, client, true, false)

			// User with or without template access should not be able to access
			// the workspace as the authenticated level is disallowed.
			verifyAccess(t, user.Username, workspace.Name, agent.Name, testAppNameAuthenticated, clientWithTemplateAccess, false, false)
			verifyAccess(t, user.Username, workspace.Name, agent.Name, testAppNameAuthenticated, clientWithNoTemplateAccess, false, false)

			// Unauthenticated users should be blocked anyways.
			verifyAccess(t, user.Username, workspace.Name, agent.Name, testAppNameAuthenticated, clientWithNoAuth, false, true)
		})

		t.Run("Public", func(t *testing.T) {
			t.Parallel()

			// All levels allowed except public.
			workspace, agent, user, client, clientWithTemplateAccess, clientWithNoTemplateAccess, clientWithNoAuth := setupAppAuthorizerTest(t, []database.AppSharingLevel{
				database.AppSharingLevelOwner,
				database.AppSharingLevelTemplate,
				database.AppSharingLevelAuthenticated,
			})

			// Owner can always access their own workspace.
			verifyAccess(t, user.Username, workspace.Name, agent.Name, testAppNamePublic, client, true, false)

			// All other users should be blocked because the public level is
			// disallowed.
			verifyAccess(t, user.Username, workspace.Name, agent.Name, testAppNamePublic, clientWithTemplateAccess, false, false)
			verifyAccess(t, user.Username, workspace.Name, agent.Name, testAppNamePublic, clientWithNoTemplateAccess, false, false)
			verifyAccess(t, user.Username, workspace.Name, agent.Name, testAppNamePublic, clientWithNoAuth, false, true)
		})
	})
}
