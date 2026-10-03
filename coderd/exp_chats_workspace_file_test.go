package coderd_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"

	"github.com/coder/coder/v2/agent"
	"github.com/coder/coder/v2/agent/agenttest"
	"github.com/coder/coder/v2/agent/usershell"
	"github.com/coder/coder/v2/coderd"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

// uploadChatWorkspaceFile bypasses the SDK helper so tests can drive
// the endpoint with a raw (or missing) Content-Disposition header that
// the helper would otherwise set automatically.
func uploadChatWorkspaceFile(
	ctx context.Context,
	t *testing.T,
	client *codersdk.ExperimentalClient,
	chatID, contentDisposition, contentType string,
	body []byte,
) (*http.Response, error) {
	t.Helper()
	return client.Request(ctx, http.MethodPost,
		"/api/v2/chats/"+chatID+"/workspace-files",
		bytes.NewReader(body),
		func(r *http.Request) {
			if contentType != "" {
				r.Header.Set("Content-Type", contentType)
			}
			if contentDisposition != "" {
				r.Header.Set("Content-Disposition", contentDisposition)
			}
		},
	)
}

func TestNormalizeWorkspaceFileReference(t *testing.T) {
	t.Parallel()

	chatID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	otherChatID := uuid.MustParse("00000000-0000-0000-0000-000000000002")
	tests := []struct {
		name     string
		chatID   uuid.UUID
		path     string
		fileName string
		want     string
	}{
		{
			name:     "LinuxPath",
			chatID:   chatID,
			path:     "/home/coder/.coder/chats/00000000-0000-0000-0000-000000000001/files/data.csv",
			fileName: "data.csv",
			want:     "/home/coder/.coder/chats/00000000-0000-0000-0000-000000000001/files/data.csv",
		},
		{
			name:     "WindowsPath",
			chatID:   chatID,
			path:     `C:\Users\coder\.coder\chats\00000000-0000-0000-0000-000000000001\files\data.csv`,
			fileName: "data.csv",
			want:     `C:\Users\coder\.coder\chats\00000000-0000-0000-0000-000000000001\files\data.csv`,
		},
		{
			name:     "NilChatID",
			chatID:   uuid.Nil,
			path:     "/home/coder/.coder/chats/00000000-0000-0000-0000-000000000001/files/data.csv",
			fileName: "data.csv",
		},
		{
			name:     "Traversal",
			chatID:   chatID,
			path:     "/home/coder/.coder/chats/00000000-0000-0000-0000-000000000001/files/../data.csv",
			fileName: "data.csv",
		},
		{
			name:     "MismatchedChatID",
			chatID:   otherChatID,
			path:     "/home/coder/.coder/chats/00000000-0000-0000-0000-000000000001/files/data.csv",
			fileName: "data.csv",
		},
		{
			name:     "NULInPath",
			chatID:   chatID,
			path:     "/home/coder/.coder/chats/00000000-0000-0000-0000-000000000001/files/data\x00.csv",
			fileName: "data\x00.csv",
		},
		{
			name:     "NameWithSeparator",
			chatID:   chatID,
			path:     "/home/coder/.coder/chats/00000000-0000-0000-0000-000000000001/files/data.csv",
			fileName: "nested/data.csv",
		},
		{
			name:     "UncleanPrefixIsCleaned",
			chatID:   chatID,
			path:     " /home/coder/./tmp/../.coder/chats/00000000-0000-0000-0000-000000000001/files/data.csv ",
			fileName: "data.csv",
			want:     "/home/coder/.coder/chats/00000000-0000-0000-0000-000000000001/files/data.csv",
		},
		{
			name:     "WindowsUncleanPrefixIsCleaned",
			chatID:   chatID,
			path:     `C:\Users\coder\tmp\..\.coder\chats\00000000-0000-0000-0000-000000000001\files\data.csv`,
			fileName: "data.csv",
			want:     `C:\Users\coder\.coder\chats\00000000-0000-0000-0000-000000000001\files\data.csv`,
		},
		{
			name:     "NewlineInPrefix",
			chatID:   chatID,
			path:     "/home/co\nder/.coder/chats/00000000-0000-0000-0000-000000000001/files/data.csv",
			fileName: "data.csv",
		},
		{
			name:     "PathTooLong",
			chatID:   chatID,
			path:     "/" + strings.Repeat("a", 4096) + "/.coder/chats/00000000-0000-0000-0000-000000000001/files/data.csv",
			fileName: "data.csv",
		},
		{
			name:     "ArbitraryAbsolutePath",
			chatID:   chatID,
			path:     "/etc/passwd",
			fileName: "passwd",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, ok := coderd.NormalizeWorkspaceFileReference(tt.chatID, tt.path, tt.fileName)
			require.Equal(t, tt.want != "", ok)
			require.Equal(t, tt.want, got)
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
			_ = createChatModel(t, client)

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
	_ = createChatModel(t, client)

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

func TestCreateChatMessageWorkspaceFileWorkspaceValidation(t *testing.T) {
	t.Parallel()

	validPart := func(chatID uuid.UUID) codersdk.ChatInputPart {
		return codersdk.ChatInputPart{
			Type:                   codersdk.ChatInputPartTypeWorkspaceFileReference,
			WorkspaceFilePath:      "/home/coder/.coder/chats/" + chatID.String() + "/files/data.csv",
			WorkspaceFileName:      "data.csv",
			WorkspaceFileSize:      42,
			WorkspaceFileMediaType: "text/csv",
		}
	}

	t.Run("MissingWorkspaceID", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		_ = createChatModel(t, client)

		chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: firstUser.OrganizationID,
			Content: []codersdk.ChatInputPart{
				{Type: codersdk.ChatInputPartTypeText, Text: "hello"},
			},
		})
		require.NoError(t, err)

		_, err = client.CreateChatMessage(ctx, chat.ID, codersdk.CreateChatMessageRequest{
			Content: []codersdk.ChatInputPart{validPart(chat.ID)},
		})
		sdkErr := requireSDKError(t, err, http.StatusBadRequest)
		require.Equal(t, "Invalid input part.", sdkErr.Message)
		require.Equal(t, "content[0].workspace_file_workspace_id is required for workspace-file-reference.", sdkErr.Detail)
	})

	t.Run("WorkspaceMismatch", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, db := newChatClientWithDatabase(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		_ = createChatModel(t, client)

		workspaceBuild := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
			OrganizationID: firstUser.OrganizationID,
			OwnerID:        firstUser.UserID,
		}).WithAgent().Do()

		chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: firstUser.OrganizationID,
			WorkspaceID:    &workspaceBuild.Workspace.ID,
			Content: []codersdk.ChatInputPart{
				{Type: codersdk.ChatInputPartTypeText, Text: "hello"},
			},
		})
		require.NoError(t, err)

		// References uploaded to a different workspace than the chat's
		// current binding are unreadable for the bound agent.
		part := validPart(chat.ID)
		part.WorkspaceFileWorkspaceID = uuid.New()
		_, err = client.CreateChatMessage(ctx, chat.ID, codersdk.CreateChatMessageRequest{
			Content: []codersdk.ChatInputPart{part},
		})
		sdkErr := requireSDKError(t, err, http.StatusBadRequest)
		require.Equal(t, "Invalid input part.", sdkErr.Message)
		require.Equal(t, "content[0].workspace_file_workspace_id must match the chat's bound workspace. Re-upload the file to the current workspace.", sdkErr.Detail)
	})

	t.Run("UnboundChat", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		_ = createChatModel(t, client)

		chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: firstUser.OrganizationID,
			Content: []codersdk.ChatInputPart{
				{Type: codersdk.ChatInputPartTypeText, Text: "hello"},
			},
		})
		require.NoError(t, err)

		part := validPart(chat.ID)
		part.WorkspaceFileWorkspaceID = uuid.New()
		_, err = client.CreateChatMessage(ctx, chat.ID, codersdk.CreateChatMessageRequest{
			Content: []codersdk.ChatInputPart{part},
		})
		sdkErr := requireSDKError(t, err, http.StatusBadRequest)
		require.Equal(t, "Invalid input part.", sdkErr.Message)
		require.Equal(t, "content[0].workspace_file_workspace_id must match the chat's bound workspace. Re-upload the file to the current workspace.", sdkErr.Detail)
	})

	newBoundChat := func(ctx context.Context, t *testing.T) (*codersdk.ExperimentalClient, codersdk.Chat) {
		t.Helper()
		client, db := newChatClientWithDatabase(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		_ = createChatModel(t, client)
		workspaceBuild := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
			OrganizationID: firstUser.OrganizationID,
			OwnerID:        firstUser.UserID,
		}).WithAgent().Do()
		chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: firstUser.OrganizationID,
			WorkspaceID:    &workspaceBuild.Workspace.ID,
			Content: []codersdk.ChatInputPart{
				{Type: codersdk.ChatInputPartTypeText, Text: "hello"},
			},
		})
		require.NoError(t, err)
		return client, chat
	}

	t.Run("UnsanitizedName", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, chat := newBoundChat(ctx, t)

		for _, name := range []string{" data.csv", "da\u202eta.csv", "da|ta.csv"} {
			part := validPart(chat.ID)
			part.WorkspaceFileWorkspaceID = *chat.WorkspaceID
			part.WorkspaceFileName = name
			part.WorkspaceFilePath = "/home/coder/.coder/chats/" + chat.ID.String() + "/files/" + name
			_, err := client.CreateChatMessage(ctx, chat.ID, codersdk.CreateChatMessageRequest{
				Content: []codersdk.ChatInputPart{part},
			})
			sdkErr := requireSDKError(t, err, http.StatusBadRequest)
			require.Equal(t, "content[0].workspace_file_name must be a name returned by the workspace file upload endpoint.", sdkErr.Detail, "name %q", name)
		}
	})

	t.Run("PersistsNormalizedReference", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, chat := newBoundChat(ctx, t)

		part := validPart(chat.ID)
		part.WorkspaceFileWorkspaceID = *chat.WorkspaceID
		part.WorkspaceFilePath = " /home/coder/./tmp/../.coder/chats/" + chat.ID.String() + "/files/data.csv "
		part.WorkspaceFileMediaType = "Text/CSV; charset=utf-8"
		resp, err := client.CreateChatMessage(ctx, chat.ID, codersdk.CreateChatMessageRequest{
			Content: []codersdk.ChatInputPart{part},
		})
		require.NoError(t, err)
		var parts []codersdk.ChatMessagePart
		if resp.Queued {
			require.NotNil(t, resp.QueuedMessage)
			parts = resp.QueuedMessage.Content
		} else {
			require.NotNil(t, resp.Message)
			parts = resp.Message.Content
		}
		require.Contains(t, parts, codersdk.ChatMessageWorkspaceFileReference(
			*chat.WorkspaceID,
			"/home/coder/.coder/chats/"+chat.ID.String()+"/files/data.csv",
			"data.csv",
			42,
			"text/csv",
		))
	})

	t.Run("EditSharesValidation", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, chat := newBoundChat(ctx, t)

		messages, err := client.GetChatMessages(ctx, chat.ID, nil)
		require.NoError(t, err)
		var userMessageID int64
		for _, message := range messages.Messages {
			if message.Role == codersdk.ChatMessageRoleUser {
				userMessageID = message.ID
				break
			}
		}
		require.NotZero(t, userMessageID)

		// Edits go through the same validation as sends, so a reference
		// from another workspace is rejected.
		stale := validPart(chat.ID)
		stale.WorkspaceFileWorkspaceID = uuid.New()
		_, err = client.EditChatMessage(ctx, chat.ID, userMessageID, codersdk.EditChatMessageRequest{
			Content: []codersdk.ChatInputPart{stale},
		})
		sdkErr := requireSDKError(t, err, http.StatusBadRequest)
		require.Equal(t, "content[0].workspace_file_workspace_id must match the chat's bound workspace. Re-upload the file to the current workspace.", sdkErr.Detail)

		current := validPart(chat.ID)
		current.WorkspaceFileWorkspaceID = *chat.WorkspaceID
		current.WorkspaceFilePath = "/home/coder/./.coder/chats/" + chat.ID.String() + "/files/data.csv"
		edited, err := client.EditChatMessage(ctx, chat.ID, userMessageID, codersdk.EditChatMessageRequest{
			Content: []codersdk.ChatInputPart{current},
		})
		require.NoError(t, err)
		require.Contains(t, edited.Message.Content, codersdk.ChatMessageWorkspaceFileReference(
			*chat.WorkspaceID,
			"/home/coder/.coder/chats/"+chat.ID.String()+"/files/data.csv",
			"data.csv",
			42,
			"text/csv",
		))
	})
}

// homeDirEnvInfo pins the agent's home directory so upload tests can run
// in parallel instead of overriding HOME for the whole process.
type homeDirEnvInfo struct {
	usershell.SystemEnvInfo
	home string
}

func (e homeDirEnvInfo) HomeDir() (string, error) { return e.home, nil }

// startChatUploadAgent connects a real agent with its own home directory
// and returns that directory.
func startChatUploadAgent(t *testing.T, client *codersdk.ExperimentalClient, workspaceBuild dbfake.WorkspaceResponse) string {
	t.Helper()
	home := t.TempDir()
	_ = agenttest.New(t, client.URL, workspaceBuild.AgentToken, func(o *agent.Options) {
		o.EnvInfo = homeDirEnvInfo{home: home}
	})
	coderdtest.NewWorkspaceAgentWaiter(t, client.Client, workspaceBuild.Workspace.ID).WaitFor(coderdtest.AgentsReady)
	return home
}

func createBoundChat(ctx context.Context, t *testing.T, client *codersdk.ExperimentalClient, organizationID, workspaceID uuid.UUID) codersdk.Chat {
	t.Helper()
	chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
		OrganizationID: organizationID,
		WorkspaceID:    &workspaceID,
		Content: []codersdk.ChatInputPart{
			{Type: codersdk.ChatInputPartTypeText, Text: "upload a file"},
		},
	})
	require.NoError(t, err)
	return chat
}

func TestPostChatWorkspaceFile(t *testing.T) {
	t.Parallel()

	t.Run("Success", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, db, api := newChatClientWithAPIAndDatabase(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		_ = createChatModel(t, client)

		workspaceBuild := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
			OrganizationID: firstUser.OrganizationID,
			OwnerID:        firstUser.UserID,
		}).WithAgent().Do()
		home := startChatUploadAgent(t, client, workspaceBuild)
		chat := createBoundChat(ctx, t, client, firstUser.OrganizationID, workspaceBuild.Workspace.ID)

		payload := bytes.Repeat([]byte{0x50, 0x4b}, 16)
		resp, err := client.UploadChatWorkspaceFile(ctx, chat.ID, "application/zip", "archive.zip", bytes.NewReader(payload))
		require.NoError(t, err)
		require.Equal(t, "archive.zip", resp.Name)
		require.Equal(t, int64(len(payload)), resp.Size)
		require.Equal(t, "application/zip", resp.MediaType)
		require.Equal(t, workspaceBuild.Workspace.ID, resp.WorkspaceID)
		require.True(t, strings.HasPrefix(resp.Path, home), "expected path under home, got %q (home=%q)", resp.Path, home)

		bytesOnDisk, err := os.ReadFile(resp.Path)
		require.NoError(t, err)
		require.Equal(t, payload, bytesOnDisk)

		messageResp, err := client.CreateChatMessage(ctx, chat.ID, codersdk.CreateChatMessageRequest{
			Content: []codersdk.ChatInputPart{
				{
					Type:                     codersdk.ChatInputPartTypeWorkspaceFileReference,
					WorkspaceFilePath:        resp.Path,
					WorkspaceFileName:        resp.Name,
					WorkspaceFileSize:        resp.Size,
					WorkspaceFileMediaType:   resp.MediaType,
					WorkspaceFileWorkspaceID: resp.WorkspaceID,
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
			resp.WorkspaceID,
			resp.Path,
			resp.Name,
			resp.Size,
			resp.MediaType,
		))

		// Archiving requires the chat generation to settle first.
		coderdtest.WaitForChatSettled(ctx, t, api, chat.ID)
		archived := true
		err = client.UpdateChat(ctx, chat.ID, codersdk.UpdateChatRequest{
			Archived: &archived,
		})
		require.NoError(t, err)
		_, err = os.Stat(resp.Path)
		require.NoError(t, err, "archive should not remove workspace files")
	})

	t.Run("EmptyCreatedChat", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, db, api := newChatClientWithAPIAndDatabase(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		_ = createChatModel(t, client)

		workspaceBuild := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
			OrganizationID: firstUser.OrganizationID,
			OwnerID:        firstUser.UserID,
		}).WithAgent().Do()
		_ = startChatUploadAgent(t, client, workspaceBuild)

		// Uploads only need the chat ID and workspace binding, so a
		// chat created idle with no messages accepts them. This is
		// the sequencing used by the new-chat page: create empty,
		// upload, then send the first message with the references.
		chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: firstUser.OrganizationID,
			WorkspaceID:    &workspaceBuild.Workspace.ID,
		})
		require.NoError(t, err)
		require.Equal(t, codersdk.ChatStatusWaiting, chat.Status)

		payload := bytes.Repeat([]byte{0x50, 0x4b}, 16)
		resp, err := client.UploadChatWorkspaceFile(ctx, chat.ID, "application/zip", "archive.zip", bytes.NewReader(payload))
		require.NoError(t, err)
		require.Equal(t, workspaceBuild.Workspace.ID, resp.WorkspaceID)

		bytesOnDisk, err := os.ReadFile(resp.Path)
		require.NoError(t, err)
		require.Equal(t, payload, bytesOnDisk)

		// The first message carries text plus the uploaded reference
		// and starts generation: the idle chat inserts it directly.
		messageResp, err := client.CreateChatMessage(ctx, chat.ID, codersdk.CreateChatMessageRequest{
			Content: []codersdk.ChatInputPart{
				{Type: codersdk.ChatInputPartTypeText, Text: "inspect the archive"},
				{
					Type:                     codersdk.ChatInputPartTypeWorkspaceFileReference,
					WorkspaceFilePath:        resp.Path,
					WorkspaceFileName:        resp.Name,
					WorkspaceFileSize:        resp.Size,
					WorkspaceFileMediaType:   resp.MediaType,
					WorkspaceFileWorkspaceID: resp.WorkspaceID,
				},
			},
		})
		require.NoError(t, err)
		require.False(t, messageResp.Queued)
		require.NotNil(t, messageResp.Message)
		require.Contains(t, messageResp.Message.Content, codersdk.ChatMessageWorkspaceFileReference(
			resp.WorkspaceID,
			resp.Path,
			resp.Name,
			resp.Size,
			resp.MediaType,
		))

		coderdtest.WaitForChatSettled(ctx, t, api, chat.ID)
	})

	t.Run("ConcurrentSameName", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, db := newChatClientWithDatabase(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		_ = createChatModel(t, client)

		workspaceBuild := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
			OrganizationID: firstUser.OrganizationID,
			OwnerID:        firstUser.UserID,
		}).WithAgent().Do()
		_ = startChatUploadAgent(t, client, workspaceBuild)
		chat := createBoundChat(ctx, t, client, firstUser.OrganizationID, workspaceBuild.Workspace.ID)

		// Same-name uploads racing each other must land on distinct
		// paths instead of overwriting one another.
		payloads := [][]byte{[]byte("first"), []byte("second")}
		results := make([]codersdk.UploadChatWorkspaceFileResponse, len(payloads))
		var eg errgroup.Group
		for i, payload := range payloads {
			eg.Go(func() error {
				resp, err := client.UploadChatWorkspaceFile(ctx, chat.ID, "application/zip", "archive.zip", bytes.NewReader(payload))
				results[i] = resp
				return err
			})
		}
		require.NoError(t, eg.Wait())

		require.ElementsMatch(t, []string{"archive.zip", "archive_2.zip"}, []string{results[0].Name, results[1].Name})
		for i, result := range results {
			bytesOnDisk, err := os.ReadFile(result.Path)
			require.NoError(t, err)
			require.Equal(t, payloads[i], bytesOnDisk)
		}
	})

	t.Run("RFC5987Filename", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, db := newChatClientWithDatabase(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		_ = createChatModel(t, client)

		workspaceBuild := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
			OrganizationID: firstUser.OrganizationID,
			OwnerID:        firstUser.UserID,
		}).WithAgent().Do()
		_ = startChatUploadAgent(t, client, workspaceBuild)
		chat := createBoundChat(ctx, t, client, firstUser.OrganizationID, workspaceBuild.Workspace.ID)

		res, err := uploadChatWorkspaceFile(ctx, t, client, chat.ID.String(),
			"attachment; filename*=UTF-8''r%C3%A9sum%C3%A9.pdf", "application/pdf", []byte("%PDF"))
		require.NoError(t, err)
		defer res.Body.Close()
		require.Equal(t, http.StatusCreated, res.StatusCode)
		var uploaded codersdk.UploadChatWorkspaceFileResponse
		require.NoError(t, json.NewDecoder(res.Body).Decode(&uploaded))
		require.Equal(t, "r\u00e9sum\u00e9.pdf", uploaded.Name)
		require.Equal(t, uploaded.Name, filepath.Base(uploaded.Path))
	})

	t.Run("MissingFilename", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, db := newChatClientWithDatabase(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		_ = createChatModel(t, client)

		workspaceBuild := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
			OrganizationID: firstUser.OrganizationID,
			OwnerID:        firstUser.UserID,
		}).WithAgent().Do()
		chat := createBoundChat(ctx, t, client, firstUser.OrganizationID, workspaceBuild.Workspace.ID)

		res, err := uploadChatWorkspaceFile(ctx, t, client, chat.ID.String(), "", "application/zip", []byte("PK"))
		require.NoError(t, err)
		defer res.Body.Close()
		require.Equal(t, http.StatusBadRequest, res.StatusCode)
	})

	t.Run("NoWorkspace", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		_ = createChatModel(t, client)

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

	t.Run("RateLimited", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t, func(o *coderdtest.Options) {
			o.FilesRateLimit = 1
		})
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		_ = createChatModel(t, client)

		chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: firstUser.OrganizationID,
			Content: []codersdk.ChatInputPart{
				{Type: codersdk.ChatInputPartTypeText, Text: "rate limited"},
			},
		})
		require.NoError(t, err)

		_, err = client.UploadChatWorkspaceFile(ctx, chat.ID, "application/zip", "data.zip", bytes.NewReader([]byte("PK")))
		requireSDKError(t, err, http.StatusConflict)
		_, err = client.UploadChatWorkspaceFile(ctx, chat.ID, "application/zip", "data.zip", bytes.NewReader([]byte("PK")))
		requireSDKError(t, err, http.StatusTooManyRequests)
	})

	t.Run("NoAgents", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, db := newChatClientWithDatabase(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		_ = createChatModel(t, client)

		workspaceBuild := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
			OrganizationID: firstUser.OrganizationID,
			OwnerID:        firstUser.UserID,
		}).Do()
		chat := createBoundChat(ctx, t, client, firstUser.OrganizationID, workspaceBuild.Workspace.ID)

		_, err := client.UploadChatWorkspaceFile(ctx, chat.ID, "application/zip", "data.zip", bytes.NewReader([]byte("PK")))
		sdkErr := requireSDKError(t, err, http.StatusConflict)
		require.Equal(t, "Chat workspace has no agents.", sdkErr.Message)
	})

	for _, tc := range []struct {
		name    string
		build   func(dbfake.WorkspaceBuildBuilder) dbfake.WorkspaceBuildBuilder
		message string
	}{
		{
			name: "WorkspaceStopped",
			build: func(b dbfake.WorkspaceBuildBuilder) dbfake.WorkspaceBuildBuilder {
				return b.Seed(database.WorkspaceBuild{Transition: database.WorkspaceTransitionStop})
			},
			message: "Workspace is stopped. Start the workspace before uploading files.",
		},
		{
			name: "WorkspaceStarting",
			build: func(b dbfake.WorkspaceBuildBuilder) dbfake.WorkspaceBuildBuilder {
				return b.Starting()
			},
			message: "Workspace is starting. Wait for the workspace to start before uploading files.",
		},
		{
			name: "WorkspacePending",
			build: func(b dbfake.WorkspaceBuildBuilder) dbfake.WorkspaceBuildBuilder {
				return b.Pending()
			},
			message: "Workspace is pending. Wait for the workspace to start before uploading files.",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitLong)
			client, db := newChatClientWithDatabase(t)
			firstUser := coderdtest.CreateFirstUser(t, client.Client)
			_ = createChatModel(t, client)

			workspaceBuild := tc.build(dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
				OrganizationID: firstUser.OrganizationID,
				OwnerID:        firstUser.UserID,
			}).WithAgent()).Do()
			chat := createBoundChat(ctx, t, client, firstUser.OrganizationID, workspaceBuild.Workspace.ID)

			_, err := client.UploadChatWorkspaceFile(ctx, chat.ID, "application/zip", "data.zip", bytes.NewReader([]byte("PK")))
			sdkErr := requireSDKError(t, err, http.StatusConflict)
			require.Equal(t, tc.message, sdkErr.Message)
		})
	}

	t.Run("WorkspaceDeleted", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, db := newChatClientWithDatabase(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		_ = createChatModel(t, client)

		workspaceBuild := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
			OrganizationID: firstUser.OrganizationID,
			OwnerID:        firstUser.UserID,
		}).WithAgent().Do()
		chat := createBoundChat(ctx, t, client, firstUser.OrganizationID, workspaceBuild.Workspace.ID)
		err := db.UpdateWorkspaceDeletedByID(dbauthz.AsSystemRestricted(ctx), database.UpdateWorkspaceDeletedByIDParams{
			ID:      workspaceBuild.Workspace.ID,
			Deleted: true,
		})
		require.NoError(t, err)

		_, err = client.UploadChatWorkspaceFile(ctx, chat.ID, "application/zip", "data.zip", bytes.NewReader([]byte("PK")))
		sdkErr := requireSDKError(t, err, http.StatusConflict)
		require.Equal(t, "Chat workspace was deleted.", sdkErr.Message)
	})

	t.Run("NoConnectedAgent", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, db := newChatClientWithDatabase(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		_ = createChatModel(t, client)

		// dbfake.WorkspaceBuild + WithAgent records an agent in the
		// database but never connects it, so the handler should see a
		// disconnected agent.
		workspaceBuild := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
			OrganizationID: firstUser.OrganizationID,
			OwnerID:        firstUser.UserID,
		}).WithAgent().Do()
		chat := createBoundChat(ctx, t, client, firstUser.OrganizationID, workspaceBuild.Workspace.ID)

		_, err := client.UploadChatWorkspaceFile(ctx, chat.ID, "application/zip", "data.zip", bytes.NewReader([]byte("PK")))
		sdkErr := requireSDKError(t, err, http.StatusConflict)
		require.Contains(t, sdkErr.Message, "Agent status")
	})

	t.Run("SharedWorkspaceRequiresSSH", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, db := newChatClientWithDatabase(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		_ = createChatModel(t, client)

		memberClientRaw, member := coderdtest.CreateAnotherUser(t, client.Client, firstUser.OrganizationID)
		memberClient := codersdk.NewExperimentalClient(memberClientRaw)
		aclWith := func(actions ...policy.Action) database.WorkspaceACL {
			return database.WorkspaceACL{member.ID.String(): {Permissions: actions}}
		}

		workspaceBuild := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
			OrganizationID: firstUser.OrganizationID,
			OwnerID:        firstUser.UserID,
			UserACL:        aclWith(policy.ActionRead, policy.ActionSSH, policy.ActionApplicationConnect),
		}).WithAgent().Do()
		chat := createBoundChat(ctx, t, memberClient, firstUser.OrganizationID, workspaceBuild.Workspace.ID)

		// An SSH grant passes authorization and reaches the agent check.
		_, err := memberClient.UploadChatWorkspaceFile(ctx, chat.ID, "application/zip", "data.zip", bytes.NewReader([]byte("PK")))
		sdkErr := requireSDKError(t, err, http.StatusConflict)
		require.Contains(t, sdkErr.Message, "Agent status")

		owner, err := client.User(ctx, codersdk.Me)
		require.NoError(t, err)
		err = db.UpdateWorkspaceACLByID(dbauthz.As(ctx, coderdtest.AuthzUserSubject(owner)), database.UpdateWorkspaceACLByIDParams{
			ID:       workspaceBuild.Workspace.ID,
			UserACL:  aclWith(policy.ActionRead, policy.ActionApplicationConnect),
			GroupACL: database.WorkspaceACL{},
		})
		require.NoError(t, err)

		_, err = memberClient.UploadChatWorkspaceFile(ctx, chat.ID, "application/zip", "data.zip", bytes.NewReader([]byte("PK")))
		sdkErr = requireSDKError(t, err, http.StatusForbidden)
		require.Equal(t, "Chat workspace not found.", sdkErr.Message)
	})

	t.Run("AppConnectOnlyToken", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, db := newChatClientWithDatabase(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		_ = createChatModel(t, client)

		workspaceBuild := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
			OrganizationID: firstUser.OrganizationID,
			OwnerID:        firstUser.UserID,
		}).WithAgent().Do()
		chat := createBoundChat(ctx, t, client, firstUser.OrganizationID, workspaceBuild.Workspace.ID)

		// chat:* cannot be requested through the token API, so insert
		// the scoped key directly.
		_, token := dbgen.APIKey(t, db, database.APIKey{
			UserID: firstUser.UserID,
			Scopes: database.APIKeyScopes{
				database.ApiKeyScopeChat,
				database.ApiKeyScopeWorkspaceRead,
				database.ApiKeyScopeWorkspaceApplicationConnect,
			},
		})
		scopedClient := codersdk.NewExperimentalClient(codersdk.New(client.URL, codersdk.WithSessionToken(token)))

		_, err := scopedClient.UploadChatWorkspaceFile(ctx, chat.ID, "application/zip", "data.zip", bytes.NewReader([]byte("PK")))
		sdkErr := requireSDKError(t, err, http.StatusForbidden)
		require.Equal(t, "Chat workspace not found.", sdkErr.Message)
	})

	t.Run("Archived", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, api := newChatClientWithAPI(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		_ = createChatModel(t, client)

		chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: firstUser.OrganizationID,
			Content: []codersdk.ChatInputPart{
				{Type: codersdk.ChatInputPartTypeText, Text: "archived"},
			},
		})
		require.NoError(t, err)

		// Archiving requires the chat generation to settle first.
		coderdtest.WaitForChatSettled(ctx, t, api, chat.ID)
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
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		adminClient, _ := newChatClientWithDatabase(t)
		firstUser := coderdtest.CreateFirstUser(t, adminClient.Client)
		_ = createChatModel(t, adminClient)

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
