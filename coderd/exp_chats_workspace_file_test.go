package coderd_test

import (
	"bytes"
	"context"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent/agenttest"
	"github.com/coder/coder/v2/coderd"
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

func TestValidWorkspaceFileReference(t *testing.T) {
	t.Parallel()

	chatID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	otherChatID := uuid.MustParse("00000000-0000-0000-0000-000000000002")
	tests := []struct {
		name     string
		chatID   uuid.UUID
		path     string
		fileName string
		want     bool
	}{
		{
			name:     "LinuxPath",
			chatID:   chatID,
			path:     "/home/coder/.coder/chats/00000000-0000-0000-0000-000000000001/files/data.csv",
			fileName: "data.csv",
			want:     true,
		},
		{
			name:     "WindowsPath",
			chatID:   chatID,
			path:     `C:\Users\coder\.coder\chats\00000000-0000-0000-0000-000000000001\files\data.csv`,
			fileName: "data.csv",
			want:     true,
		},
		{
			name:     "NilChatID",
			chatID:   uuid.Nil,
			path:     "/home/coder/.coder/chats/00000000-0000-0000-0000-000000000001/files/data.csv",
			fileName: "data.csv",
			want:     false,
		},
		{
			name:     "Traversal",
			chatID:   chatID,
			path:     "/home/coder/.coder/chats/00000000-0000-0000-0000-000000000001/files/../data.csv",
			fileName: "data.csv",
			want:     false,
		},
		{
			name:     "MismatchedChatID",
			chatID:   otherChatID,
			path:     "/home/coder/.coder/chats/00000000-0000-0000-0000-000000000001/files/data.csv",
			fileName: "data.csv",
			want:     false,
		},
		{
			name:     "NameWithSeparator",
			chatID:   chatID,
			path:     "/home/coder/.coder/chats/00000000-0000-0000-0000-000000000001/files/data.csv",
			fileName: "nested/data.csv",
			want:     false,
		},
		{
			name:     "ArbitraryAbsolutePath",
			chatID:   chatID,
			path:     "/etc/passwd",
			fileName: "passwd",
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tt.want, coderd.ValidWorkspaceFileReference(tt.chatID, tt.path, tt.fileName))
		})
	}
}

func TestCreateChatWorkspaceFilePartValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		part   codersdk.ChatInputPart
		detail string
	}{
		{
			name: "InitialChatCreationRejectsWorkspaceReference",
			part: codersdk.ChatInputPart{
				Type:              codersdk.ChatInputPartTypeWorkspaceFileReference,
				WorkspaceFilePath: "/home/coder/.coder/chats/00000000-0000-0000-0000-000000000001/files/data.csv",
				WorkspaceFileName: "data.csv",
				WorkspaceFileSize: 42,
			},
			detail: "content[0].workspace-file-reference requires an existing chat.",
		},
		{
			name: "MissingPath",
			part: codersdk.ChatInputPart{
				Type:              codersdk.ChatInputPartTypeWorkspaceFileReference,
				WorkspaceFileName: "data.csv",
				WorkspaceFileSize: 42,
			},
			detail: "content[0].workspace_file_path is required for workspace-file-reference.",
		},
		{
			name: "MissingName",
			part: codersdk.ChatInputPart{
				Type:              codersdk.ChatInputPartTypeWorkspaceFileReference,
				WorkspaceFilePath: "/home/coder/.coder/chats/chat-id/files/data.csv",
				WorkspaceFileSize: 42,
			},
			detail: "content[0].workspace_file_name is required for workspace-file-reference.",
		},
		{
			name: "NegativeSize",
			part: codersdk.ChatInputPart{
				Type:              codersdk.ChatInputPartTypeWorkspaceFileReference,
				WorkspaceFilePath: "/home/coder/.coder/chats/chat-id/files/data.csv",
				WorkspaceFileName: "data.csv",
				WorkspaceFileSize: -1,
			},
			detail: "content[0].workspace_file_size must be non-negative.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitLong)
			client := newChatClient(t)
			firstUser := coderdtest.CreateFirstUser(t, client.Client)
			_ = createChatModelConfig(t, client)

			_, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
				OrganizationID: firstUser.OrganizationID,
				Content:        []codersdk.ChatInputPart{tt.part},
			})
			sdkErr := requireSDKError(t, err, http.StatusBadRequest)
			require.Equal(t, "Invalid input part.", sdkErr.Message)
			require.Equal(t, tt.detail, sdkErr.Detail)
		})
	}
}

func TestCreateChatMessageWorkspaceFilePartRejectsInvalidPath(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client := newChatClient(t)
	firstUser := coderdtest.CreateFirstUser(t, client.Client)
	_ = createChatModelConfig(t, client)

	chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
		OrganizationID: firstUser.OrganizationID,
		Content: []codersdk.ChatInputPart{
			{Type: codersdk.ChatInputPartTypeText, Text: "hello"},
		},
	})
	require.NoError(t, err)

	_, err = client.CreateChatMessage(ctx, chat.ID, codersdk.CreateChatMessageRequest{
		Content: []codersdk.ChatInputPart{
			{
				Type:                   codersdk.ChatInputPartTypeWorkspaceFileReference,
				WorkspaceFilePath:      "/etc/passwd",
				WorkspaceFileName:      "passwd",
				WorkspaceFileSize:      42,
				WorkspaceFileMediaType: "text/plain",
			},
		},
	})
	sdkErr := requireSDKError(t, err, http.StatusBadRequest)
	require.Equal(t, "Invalid input part.", sdkErr.Message)
	require.Equal(t, "content[0].workspace_file_path must reference a file uploaded to this chat.", sdkErr.Detail)
}

//nolint:paralleltest // t.Setenv on agent home dir requires sequential subtests.
func TestPostChatWorkspaceFile(t *testing.T) {
	// Subtests use t.Setenv to control the agent's home directory.

	t.Run("Success", func(t *testing.T) {
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

		messageResp, err := client.CreateChatMessage(ctx, chat.ID, codersdk.CreateChatMessageRequest{
			Content: []codersdk.ChatInputPart{
				{
					Type:                   codersdk.ChatInputPartTypeWorkspaceFileReference,
					WorkspaceFilePath:      resp.Path,
					WorkspaceFileName:      resp.Name,
					WorkspaceFileSize:      resp.Size,
					WorkspaceFileMediaType: resp.MediaType,
				},
			},
		})
		require.NoError(t, err)
		var messageParts []codersdk.ChatMessagePart
		if messageResp.Queued {
			require.NotNil(t, messageResp.QueuedMessage)
			messageParts = messageResp.QueuedMessage.Content
		} else {
			require.NotNil(t, messageResp.Message)
			messageParts = messageResp.Message.Content
		}
		require.Contains(t, messageParts, codersdk.ChatMessageWorkspaceFileReference(
			resp.Path,
			resp.Name,
			resp.Size,
			resp.MediaType,
		))

		archived := true
		err = client.UpdateChat(ctx, chat.ID, codersdk.UpdateChatRequest{
			Archived: &archived,
		})
		require.NoError(t, err)
		_, err = os.Stat(resp.Path)
		require.NoError(t, err, "archive should not remove workspace files")
		_, err = os.Stat(second.Path)
		require.NoError(t, err, "archive should not remove workspace files")
	})

	t.Run("MissingFilename", func(t *testing.T) {
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

		// Missing Content-Disposition fails filename validation.
		resp, err := uploadChatWorkspaceFile(ctx, t, client, chat.ID.String(), "", "application/zip", []byte("PK"))
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("NoWorkspace", func(t *testing.T) {
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

	t.Run("NoAgents", func(t *testing.T) {
		ctx := testutil.Context(t, testutil.WaitLong)
		client, db := newChatClientWithDatabase(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		_ = createChatModelConfig(t, client)

		workspaceBuild := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
			OrganizationID: firstUser.OrganizationID,
			OwnerID:        firstUser.UserID,
		}).Do()

		chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: firstUser.OrganizationID,
			WorkspaceID:    &workspaceBuild.Workspace.ID,
			Content: []codersdk.ChatInputPart{
				{Type: codersdk.ChatInputPartTypeText, Text: "no agents"},
			},
		})
		require.NoError(t, err)

		_, err = client.UploadChatWorkspaceFile(ctx, chat.ID, "application/zip", "data.zip", bytes.NewReader([]byte("PK")))
		sdkErr := requireSDKError(t, err, http.StatusConflict)
		require.Equal(t, "Chat workspace has no agents.", sdkErr.Message)
	})

	t.Run("NoConnectedAgent", func(t *testing.T) {
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
