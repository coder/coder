package coderd_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/websocket"
)

func TestWorkspaceAgentACPExperiment(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name        string
		experiments []string
	}{
		{name: "Disabled"},
		{name: "NotIncludedInWildcard", experiments: []string{"*"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			dv := coderdtest.DeploymentValues(t)
			dv.Experiments = tc.experiments
			client, db := coderdtest.NewWithDatabase(t, &coderdtest.Options{DeploymentValues: dv})
			user := coderdtest.CreateFirstUser(t, client)
			workspace := newACPWorkspace(t, db, user.OrganizationID, user.UserID, "linux")

			ctx := testutil.Context(t, testutil.WaitLong)
			conn, statusCode, err := dialWorkspaceAgentACP(ctx, t, client, workspace.Agents[0].ID, nil)
			require.Error(t, err)
			require.Nil(t, conn)
			require.Equal(t, http.StatusNotFound, statusCode)
		})
	}
}

func TestWorkspaceAgentACPAuthorization(t *testing.T) {
	t.Parallel()

	client, db := coderdtest.NewWithDatabase(t, acpTestOptions(t))
	admin := coderdtest.CreateFirstUser(t, client)
	owner, ownerUser := coderdtest.CreateAnotherUser(t, client, admin.OrganizationID)
	other, _ := coderdtest.CreateAnotherUser(t, client, admin.OrganizationID)
	templateAdmin, _ := coderdtest.CreateAnotherUser(t, client, admin.OrganizationID, rbac.RoleTemplateAdmin())
	workspace := newACPWorkspace(t, db, admin.OrganizationID, ownerUser.ID, "linux")
	anonymous := codersdk.New(client.URL, codersdk.WithHTTPClient(client.HTTPClient))
	proxy, secret := dbgen.WorkspaceProxy(t, db, database.WorkspaceProxy{})

	// Reading the agent is deliberately insufficient to execute commands.
	_, err := templateAdmin.WorkspaceAgent(testutil.Context(t, testutil.WaitLong), workspace.Agents[0].ID)
	require.NoError(t, err)

	for _, tc := range []struct {
		name       string
		client     *codersdk.Client
		headers    http.Header
		statusCode int
	}{
		{name: "NoAuthentication", client: anonymous, statusCode: http.StatusUnauthorized},
		{name: "OtherUser", client: other, statusCode: http.StatusNotFound},
		{name: "ReadableWithoutSSH", client: templateAdmin, statusCode: http.StatusNotFound},
		{
			name:   "WorkspaceProxy",
			client: anonymous,
			headers: http.Header{
				httpmw.WorkspaceProxyAuthTokenHeader: []string{fmt.Sprintf("%s:%s", proxy.ID, secret)},
			},
			statusCode: http.StatusUnauthorized,
		},
		{
			name:   "WorkspaceAgentToken",
			client: anonymous,
			headers: http.Header{
				codersdk.SessionTokenHeader: []string{workspace.AgentToken},
			},
			statusCode: http.StatusUnauthorized,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitLong)
			conn, statusCode, err := dialWorkspaceAgentACP(ctx, t, tc.client, workspace.Agents[0].ID, tc.headers)
			require.Error(t, err)
			require.Nil(t, conn)
			require.Equal(t, tc.statusCode, statusCode)
		})
	}

	for _, agentOS := range []string{"darwin", "windows", ""} {
		t.Run("UnsupportedOS/"+agentOS, func(t *testing.T) {
			t.Parallel()

			workspace := newACPWorkspace(t, db, admin.OrganizationID, ownerUser.ID, agentOS)
			ctx := testutil.Context(t, testutil.WaitLong)
			conn, statusCode, err := dialWorkspaceAgentACP(ctx, t, owner, workspace.Agents[0].ID, nil)
			require.Error(t, err)
			require.Nil(t, conn)
			require.Equal(t, http.StatusBadRequest, statusCode)
		})
	}
}

func TestWorkspaceAgentACPBrowserOnly(t *testing.T) {
	t.Parallel()

	client, _, api := coderdtest.NewWithAPI(t, acpTestOptions(t))
	user := coderdtest.CreateFirstUser(t, client)
	workspace := newACPWorkspace(t, api.Database, user.OrganizationID, user.UserID, "linux")
	blocked := make(chan struct{}, 1)
	override := func(w http.ResponseWriter) bool {
		blocked <- struct{}{}
		http.Error(w, "Non-browser connections are disabled for your deployment.", http.StatusConflict)
		return true
	}
	api.WorkspaceClientCoordinateOverride.Store(&override)

	ctx := testutil.Context(t, testutil.WaitLong)
	conn, statusCode, err := dialWorkspaceAgentACP(ctx, t, client, workspace.Agents[0].ID, nil)
	require.Error(t, err)
	require.Nil(t, conn)
	require.Equal(t, http.StatusConflict, statusCode)
	testutil.TryReceive(ctx, t, blocked)
}

func acpTestOptions(t *testing.T) *coderdtest.Options {
	t.Helper()
	dv := coderdtest.DeploymentValues(t)
	dv.Experiments = []string{string(codersdk.ExperimentNativeAgents)}
	return &coderdtest.Options{DeploymentValues: dv}
}

func newACPWorkspace(t *testing.T, db database.Store, orgID, ownerID uuid.UUID, agentOS string) dbfake.WorkspaceResponse {
	t.Helper()
	return dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
		OrganizationID: orgID,
		OwnerID:        ownerID,
	}).WithAgent(func(agents []*proto.Agent) []*proto.Agent {
		agents[0].OperatingSystem = agentOS
		return agents
	}).Do()
}

func dialWorkspaceAgentACP(ctx context.Context, t *testing.T, client *codersdk.Client, agentID uuid.UUID, headers http.Header) (*websocket.Conn, int, error) {
	t.Helper()
	headers = headers.Clone()
	if headers == nil {
		headers = make(http.Header)
	}
	if client.SessionToken() != "" {
		headers.Set(codersdk.SessionTokenHeader, client.SessionToken())
	}
	u := client.URL.JoinPath("api", "v2", "workspaceagents", agentID.String(), "experimental", "acp")
	//nolint:bodyclose // websocket.Dial closes the HTTP response body.
	conn, response, err := websocket.Dial(ctx, u.String(), &websocket.DialOptions{
		HTTPClient: client.HTTPClient,
		HTTPHeader: headers,
	})
	if conn != nil {
		conn.SetReadLimit(-1)
		t.Cleanup(func() { _ = conn.CloseNow() })
	}
	var statusCode int
	if response != nil {
		statusCode = response.StatusCode
	}
	return conn, statusCode, err
}
