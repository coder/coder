package coderd_test

import (
	"bytes"
	"context"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent/agenttest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

// uploadChatWorkspaceFile bypasses the SDK helper so tests can drive
// the endpoint with custom headers (e.g. missing Content-Disposition)
// that the helper would otherwise set automatically.
func uploadChatWorkspaceFile(
	ctx context.Context,
	t *testing.T,
	client *codersdk.ExperimentalClient,
	chatID, filename, contentType string,
	body []byte,
) (*http.Response, error) {
	t.Helper()
	return client.Request(ctx, http.MethodPost,
		"/api/experimental/chats/"+chatID+"/workspace-files",
		bytes.NewReader(body),
		func(r *http.Request) {
			if contentType != "" {
				r.Header.Set("Content-Type", contentType)
			}
			if filename != "" {
				r.Header.Set("Content-Disposition", `attachment; filename="`+filename+`"`)
			}
		},
	)
}

//nolint:paralleltest // t.Setenv on agent home dir requires sequential subtests.
func TestPostChatWorkspaceFile(t *testing.T) {
	// Subtests use t.Setenv to control the agent's home directory.

	t.Run("Success", func(t *testing.T) {
		// Sequential: tests use t.Setenv to control agent home dir.
		ctx := testutil.Context(t, testutil.WaitLong)
		client, db := newChatClientWithDatabase(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		_ = createChatModelConfig(t, client)

		workspaceBuild := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
			OrganizationID: firstUser.OrganizationID,
			OwnerID:        firstUser.UserID,
		}).WithAgent().Do()

		// Connect a real agent so the handler can dial the workspace
		// and stream bytes into the agent's home directory.
		home := t.TempDir()
		t.Setenv("HOME", home)
		t.Setenv("USERPROFILE", home)
		_ = agenttest.New(t, client.URL, workspaceBuild.AgentToken)
		coderdtest.NewWorkspaceAgentWaiter(t, client.Client, workspaceBuild.Workspace.ID).WaitFor(coderdtest.AgentsReady)

		chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: firstUser.OrganizationID,
			WorkspaceID:    &workspaceBuild.Workspace.ID,
			Content: []codersdk.ChatInputPart{
				{Type: codersdk.ChatInputPartTypeText, Text: "upload a file"},
			},
		})
		require.NoError(t, err)

		payload := bytes.Repeat([]byte{0x50, 0x4b}, 16)
		resp, err := client.UploadChatWorkspaceFile(ctx, chat.ID, "application/zip", "archive.zip", bytes.NewReader(payload))
		require.NoError(t, err)
		require.Equal(t, "archive.zip", resp.Name)
		require.Equal(t, int64(len(payload)), resp.Size)
		require.Equal(t, "application/zip", resp.MediaType)
		require.True(t, strings.HasPrefix(resp.Path, home), "expected path under home, got %q (home=%q)", resp.Path, home)

		bytesOnDisk, err := os.ReadFile(resp.Path)
		require.NoError(t, err)
		require.Equal(t, payload, bytesOnDisk)

		// Concurrent same-name upload must land on a distinct path,
		// not silently overwrite the first file. This exercises the
		// O_EXCL collision-suffix retry in writeUploadExclusive.
		second, err := client.UploadChatWorkspaceFile(ctx, chat.ID, "application/zip", "archive.zip", bytes.NewReader([]byte("second")))
		require.NoError(t, err)
		require.Equal(t, "archive_2.zip", second.Name)
		require.NotEqual(t, resp.Path, second.Path)
		_, err = os.Stat(resp.Path)
		require.NoError(t, err, "original upload was overwritten")

		archived := true
		err = client.UpdateChat(ctx, chat.ID, codersdk.UpdateChatRequest{
			Archived: &archived,
		})
		require.NoError(t, err)
		_, err = os.Stat(resp.Path)
		require.True(t, os.IsNotExist(err), "archive should clean up the first uploaded file")
		_, err = os.Stat(second.Path)
		require.True(t, os.IsNotExist(err), "archive should clean up the second uploaded file")
	})

	t.Run("MissingFilename", func(t *testing.T) {
		// Sequential: tests use t.Setenv to control agent home dir.
		ctx := testutil.Context(t, testutil.WaitLong)
		client, db := newChatClientWithDatabase(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		_ = createChatModelConfig(t, client)

		workspaceBuild := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
			OrganizationID: firstUser.OrganizationID,
			OwnerID:        firstUser.UserID,
		}).WithAgent().Do()

		home := t.TempDir()
		t.Setenv("HOME", home)
		t.Setenv("USERPROFILE", home)
		_ = agenttest.New(t, client.URL, workspaceBuild.AgentToken)
		coderdtest.NewWorkspaceAgentWaiter(t, client.Client, workspaceBuild.Workspace.ID).WaitFor(coderdtest.AgentsReady)

		chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: firstUser.OrganizationID,
			WorkspaceID:    &workspaceBuild.Workspace.ID,
			Content: []codersdk.ChatInputPart{
				{Type: codersdk.ChatInputPartTypeText, Text: "no filename"},
			},
		})
		require.NoError(t, err)

		// Connected agent + no Content-Disposition reaches the
		// filename validation branch and returns 400.
		resp, err := uploadChatWorkspaceFile(ctx, t, client, chat.ID.String(), "", "application/zip", []byte("PK"))
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("NoWorkspace", func(t *testing.T) {
		// Sequential: tests use t.Setenv to control agent home dir.
		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		_ = createChatModelConfig(t, client)

		chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: firstUser.OrganizationID,
			Content: []codersdk.ChatInputPart{
				{Type: codersdk.ChatInputPartTypeText, Text: "no workspace"},
			},
		})
		require.NoError(t, err)

		_, err = client.UploadChatWorkspaceFile(ctx, chat.ID, "application/zip", "data.zip", bytes.NewReader([]byte("PK")))
		sdkErr := requireSDKError(t, err, http.StatusConflict)
		require.Contains(t, sdkErr.Message, "no workspace")
	})

	t.Run("NoConnectedAgent", func(t *testing.T) {
		// Sequential: tests use t.Setenv to control agent home dir.
		ctx := testutil.Context(t, testutil.WaitLong)
		client, db := newChatClientWithDatabase(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		_ = createChatModelConfig(t, client)

		// dbfake.WorkspaceBuild + WithAgent records an agent in the
		// database but never connects it, so the handler should see a
		// disconnected agent.
		workspaceBuild := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
			OrganizationID: firstUser.OrganizationID,
			OwnerID:        firstUser.UserID,
		}).WithAgent().Do()

		chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: firstUser.OrganizationID,
			WorkspaceID:    &workspaceBuild.Workspace.ID,
			Content: []codersdk.ChatInputPart{
				{Type: codersdk.ChatInputPartTypeText, Text: "no agent"},
			},
		})
		require.NoError(t, err)

		_, err = client.UploadChatWorkspaceFile(ctx, chat.ID, "application/zip", "data.zip", bytes.NewReader([]byte("PK")))
		sdkErr := requireSDKError(t, err, http.StatusConflict)
		require.Contains(t, sdkErr.Message, "Agent status")
	})

	t.Run("Archived", func(t *testing.T) {
		// Sequential: tests use t.Setenv to control agent home dir.
		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		_ = createChatModelConfig(t, client)

		chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: firstUser.OrganizationID,
			Content: []codersdk.ChatInputPart{
				{Type: codersdk.ChatInputPartTypeText, Text: "archived"},
			},
		})
		require.NoError(t, err)

		archived := true
		err = client.UpdateChat(ctx, chat.ID, codersdk.UpdateChatRequest{
			Archived: &archived,
		})
		require.NoError(t, err)

		_, err = client.UploadChatWorkspaceFile(ctx, chat.ID, "application/zip", "data.zip", bytes.NewReader([]byte("PK")))
		sdkErr := requireSDKError(t, err, http.StatusBadRequest)
		require.Contains(t, sdkErr.Message, "archived")
	})

	t.Run("NotOwner", func(t *testing.T) {
		// Sequential: tests use t.Setenv to control agent home dir.
		ctx := testutil.Context(t, testutil.WaitLong)
		adminClient, _ := newChatClientWithDatabase(t)
		firstUser := coderdtest.CreateFirstUser(t, adminClient.Client)
		_ = createChatModelConfig(t, adminClient)

		// The chat is created by the first (owner) user. The second
		// user has org-admin so they pass RBAC for ActionUpdate, but
		// the owner-only check should still reject them.
		secondClientRaw, _ := coderdtest.CreateAnotherUser(t, adminClient.Client, firstUser.OrganizationID,
			rbac.ScopedRoleOrgAdmin(firstUser.OrganizationID))
		secondClient := codersdk.NewExperimentalClient(secondClientRaw)

		chat, err := adminClient.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: firstUser.OrganizationID,
			Content: []codersdk.ChatInputPart{
				{Type: codersdk.ChatInputPartTypeText, Text: "not owner"},
			},
		})
		require.NoError(t, err)

		_, err = secondClient.UploadChatWorkspaceFile(ctx, chat.ID, "application/zip", "data.zip", bytes.NewReader([]byte("PK")))
		requireSDKError(t, err, http.StatusForbidden)
	})
}
