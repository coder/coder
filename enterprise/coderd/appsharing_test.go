package coderd_test

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/testutil"
)

func setupAppAuthorizerTest(t *testing.T, allowedSharingLevels []database.AppShareLevel) (workspace codersdk.Workspace, agent codersdk.WorkspaceAgent, user codersdk.User, client *codersdk.Client, clientWithTemplateAccess *codersdk.Client, clientWithNoTemplateAccess *codersdk.Client, clientWithNoAuth *codersdk.Client) {
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
	firstUser := coderdtest.CreateFirstUser(t, client)
	user, err = client.User(ctx, firstUser.UserID.String())
	require.NoError(t, err)
	coderdenttest.AddLicense(t, client, coderdenttest.LicenseOptions{
		ApplicationSharing: true,
	})
	workspace, agent = setupWorkspaceAgent(t, client, firstUser, uint16(tcpAddr.Port))

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

	// Create an unauthenticated codersdk client.
	clientWithNoAuth = codersdk.New(client.URL)

	return workspace, agent, user, client, clientWithTemplateAccess, clientWithNoTemplateAccess, clientWithNoAuth
}

func TestEnterpriseAppAuthorizer(t *testing.T) {
	t.Parallel()

	// For the purposes of these tests we allow all levels.
	workspace, agent, user, client, clientWithTemplateAccess, clientWithNoTemplateAccess, clientWithNoAuth := setupAppAuthorizerTest(t, []database.AppShareLevel{
		database.AppShareLevelOwner,
		database.AppShareLevelTemplate,
		database.AppShareLevelAuthenticated,
		database.AppShareLevelPublic,
	})

	verifyAccess := func(t *testing.T, appName string, client *codersdk.Client, shouldHaveAccess bool) {
		t.Helper()

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		appPath := fmt.Sprintf("/@%s/%s.%s/apps/%s", user.Username, workspace.Name, agent.Name, appName)
		res, err := client.Request(ctx, http.MethodGet, appPath, nil)
		require.NoError(t, err)
		defer res.Body.Close()

		dump, err := httputil.DumpResponse(res, true)
		require.NoError(t, err)
		t.Logf("response dump: %s", dump)

		if !shouldHaveAccess {
			require.Equal(t, http.StatusForbidden, res.StatusCode)
		}

		if shouldHaveAccess {
			require.Equal(t, http.StatusOK, res.StatusCode)
			require.Contains(t, string(dump), "Hello World")
		}
	}

	t.Run("LevelOwner", func(t *testing.T) {
		t.Parallel()

		// Owner should be able to access their own workspace.
		verifyAccess(t, testAppNameOwner, client, true)

		// User with or without template access should not have access to a
		// workspace that they do not own.
		verifyAccess(t, testAppNameOwner, clientWithTemplateAccess, false)
		verifyAccess(t, testAppNameOwner, clientWithNoTemplateAccess, false)

		// Unauthenticated user should not have any access.
		verifyAccess(t, testAppNameOwner, clientWithNoAuth, false)
	})

	t.Run("LevelTemplate", func(t *testing.T) {
		t.Parallel()

		// Owner should be able to access their own workspace.
		verifyAccess(t, testAppNameTemplate, client, true)

		// User with template access should be able to access the workspace.
		verifyAccess(t, testAppNameTemplate, clientWithTemplateAccess, true)

		// User without template access should not have access to a workspace
		// that they do not own.
		verifyAccess(t, testAppNameTemplate, clientWithNoTemplateAccess, false)

		// Unauthenticated user should not have any access.
		verifyAccess(t, testAppNameTemplate, clientWithNoAuth, false)
	})

	t.Run("LevelAuthenticated", func(t *testing.T) {
		t.Parallel()

		// Owner should be able to access their own workspace.
		verifyAccess(t, testAppNameAuthenticated, client, true)

		// User with or without template access should be able to access the
		// workspace.
		verifyAccess(t, testAppNameAuthenticated, clientWithTemplateAccess, true)
		verifyAccess(t, testAppNameAuthenticated, clientWithNoTemplateAccess, true)

		// Unauthenticated user should not have any access.
		verifyAccess(t, testAppNameAuthenticated, clientWithNoAuth, false)
	})

	t.Run("LevelPublic", func(t *testing.T) {
		t.Parallel()

		// Owner should be able to access their own workspace.
		verifyAccess(t, testAppNamePublic, client, true)

		// User with or without template access should be able to access the
		// workspace.
		verifyAccess(t, testAppNamePublic, clientWithTemplateAccess, true)
		verifyAccess(t, testAppNamePublic, clientWithNoTemplateAccess, true)

		// Unauthenticated user should be able to access the workspace.
		verifyAccess(t, testAppNamePublic, clientWithNoAuth, true)
	})
}
