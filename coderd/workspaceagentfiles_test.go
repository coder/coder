package coderd_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent/agenttest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/testutil"
)

func TestWorkspaceAgentFiles(t *testing.T) {
	t.Parallel()

	client, db := coderdtest.NewWithDatabase(t, nil)
	owner := coderdtest.CreateFirstUser(t, client)

	connected := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
		OrganizationID: owner.OrganizationID,
		OwnerID:        owner.UserID,
	}).WithAgent().Do()
	_ = agenttest.New(t, client.URL, connected.AgentToken)
	resources := coderdtest.NewWorkspaceAgentWaiter(t, client, connected.Workspace.ID).Wait()
	agentID := resources[0].Agents[0].ID

	type filesResponse struct {
		status int
		header http.Header
		body   []byte
	}
	filesRequest := func(t *testing.T, c *codersdk.Client, agentID uuid.UUID, method, route string, query url.Values, body any) filesResponse {
		t.Helper()
		ctx := testutil.Context(t, testutil.WaitLong)
		u := fmt.Sprintf("/api/experimental/workspaceagents/%s/files/%s?%s", agentID, route, query.Encode())
		res, err := c.Request(ctx, method, u, body)
		require.NoError(t, err)
		defer res.Body.Close()
		data, err := io.ReadAll(res.Body)
		require.NoError(t, err)
		return filesResponse{status: res.StatusCode, header: res.Header, body: data}
	}

	t.Run("WriteReadList", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		path := filepath.Join(dir, "hello.txt")

		res := filesRequest(t, client, agentID, http.MethodPost, "write-file", url.Values{"path": {path}}, []byte("hello world"))
		require.Equal(t, http.StatusOK, res.status)
		got, err := os.ReadFile(path)
		require.NoError(t, err)
		require.Equal(t, "hello world", string(got))

		res = filesRequest(t, client, agentID, http.MethodGet, "read-file", url.Values{"path": {path}}, nil)
		require.Equal(t, http.StatusOK, res.status)
		require.Equal(t, "text/plain; charset=utf-8", res.header.Get("Content-Type"))
		require.Equal(t, "attachment", res.header.Get("Content-Disposition"))
		require.Equal(t, "sandbox; default-src 'none'", res.header.Get("Content-Security-Policy"))
		require.Equal(t, "hello world", string(res.body))

		res = filesRequest(t, client, agentID, http.MethodPost, "list-directory", url.Values{"path": {dir}}, workspacesdk.LSRequest{
			Path:       []string{},
			Relativity: workspacesdk.LSRelativityRoot,
		})
		require.Equal(t, http.StatusOK, res.status)
		var ls workspacesdk.LSResponse
		require.NoError(t, json.Unmarshal(res.body, &ls))
		require.Equal(t, dir, ls.AbsolutePathString)
		require.Len(t, ls.Contents, 1)
		require.Equal(t, "hello.txt", ls.Contents[0].Name)
	})

	t.Run("AgentStatusPassthrough", func(t *testing.T) {
		t.Parallel()

		res := filesRequest(t, client, agentID, http.MethodGet, "read-file", url.Values{"path": {filepath.Join(t.TempDir(), "missing")}}, nil)
		require.Equal(t, http.StatusNotFound, res.status)
		var sdkRes codersdk.Response
		require.NoError(t, json.Unmarshal(res.body, &sdkRes))
		require.Contains(t, sdkRes.Message, "no such file")
	})

	t.Run("NoSSHPermission", func(t *testing.T) {
		t.Parallel()

		// Template admins can read every workspace but cannot SSH into
		// workspaces they do not own.
		templateAdmin, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.RoleTemplateAdmin())
		ctx := testutil.Context(t, testutil.WaitLong)
		_, err := templateAdmin.WorkspaceAgent(ctx, agentID)
		require.NoError(t, err)

		path := filepath.Join(t.TempDir(), "secret.txt")
		require.NoError(t, os.WriteFile(path, []byte("secret"), 0o600))

		res := filesRequest(t, templateAdmin, agentID, http.MethodGet, "read-file", url.Values{"path": {path}}, nil)
		require.Equal(t, http.StatusNotFound, res.status)
		res = filesRequest(t, templateAdmin, agentID, http.MethodPost, "write-file", url.Values{"path": {path}}, []byte("overwritten"))
		require.Equal(t, http.StatusNotFound, res.status)
		got, err := os.ReadFile(path)
		require.NoError(t, err)
		require.Equal(t, "secret", string(got))
	})

	t.Run("AgentNotConnected", func(t *testing.T) {
		t.Parallel()

		disconnected := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
			OrganizationID: owner.OrganizationID,
			OwnerID:        owner.UserID,
		}).WithAgent().Do()

		res := filesRequest(t, client, disconnected.Agents[0].ID, http.MethodGet, "read-file", url.Values{"path": {"/etc/hostname"}}, nil)
		require.Equal(t, http.StatusConflict, res.status)
	})

	t.Run("OnlyFileRoutes", func(t *testing.T) {
		t.Parallel()

		res := filesRequest(t, client, agentID, http.MethodGet, "read-file-lines", url.Values{"path": {"/etc/hostname"}}, nil)
		require.Equal(t, http.StatusNotFound, res.status)
		res = filesRequest(t, client, agentID, http.MethodPost, "edit-files", nil, []byte("{}"))
		require.Equal(t, http.StatusNotFound, res.status)
	})
}
