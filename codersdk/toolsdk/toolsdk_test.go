package toolsdk_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/coder/aisdk-go"

	agentapi "github.com/coder/agentapi-sdk-go"
	"github.com/coder/coder/v2/agent"
	"github.com/coder/coder/v2/agent/agenttest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/codersdk/toolsdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/provisioner/echo"
	"github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/testutil"
)

// setupWorkspaceForAgent creates a workspace setup exactly like main SSH tests
// nolint:gocritic // This is in a test package and does not end up in the build
func setupWorkspaceForAgent(t *testing.T, opts *coderdtest.Options) (*codersdk.Client, database.WorkspaceTable, string) {
	t.Helper()

	client, store := coderdtest.NewWithDatabase(t, opts)
	client.SetLogger(testutil.Logger(t).Named("client"))
	first := coderdtest.CreateFirstUser(t, client)
	userClient, user := coderdtest.CreateAnotherUserMutators(t, client, first.OrganizationID, nil, func(r *codersdk.CreateUserRequestWithOrgs) {
		r.Username = "myuser"
	})
	// nolint:gocritic // This is in a test package and does not end up in the build
	r := dbfake.WorkspaceBuild(t, store, database.WorkspaceTable{
		Name:           "myworkspace",
		OrganizationID: first.OrganizationID,
		OwnerID:        user.ID,
	}).WithAgent().Do()

	return userClient, r.Workspace, r.AgentToken
}

// These tests are dependent on the state of the coder server.
// Running them in parallel is prone to racy behavior.
// nolint:tparallel,paralleltest
func TestTools(t *testing.T) {
	// Given: a running coderd instance using SSH test setup pattern
	setupCtx := testutil.Context(t, testutil.WaitShort)
	client, store := coderdtest.NewWithDatabase(t, nil)
	owner := coderdtest.CreateFirstUser(t, client)
	// Given: a member user with which to test the tools.
	memberClient, member := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
	// Given: a workspace with an agent.
	// nolint:gocritic // This is in a test package and does not end up in the build
	r := dbfake.WorkspaceBuild(t, store, database.WorkspaceTable{
		OrganizationID: owner.OrganizationID,
		OwnerID:        member.ID,
	}).WithAgent(func(agents []*proto.Agent) []*proto.Agent {
		agents[0].Apps = []*proto.App{
			{
				Slug: "some-agent-app",
			},
		}
		return agents
	}).Do()

	// Given: a client configured with the agent token.
	agentClient := agentsdk.New(client.URL, agentsdk.WithFixedToken(r.AgentToken))
	// Get the agent ID from the API. Overriding it in dbfake doesn't work.
	ws, err := client.Workspace(setupCtx, r.Workspace.ID)
	require.NoError(t, err)
	require.NotEmpty(t, ws.LatestBuild.Resources)
	require.NotEmpty(t, ws.LatestBuild.Resources[0].Agents)
	agentID := ws.LatestBuild.Resources[0].Agents[0].ID

	// Given: the workspace agent has written logs.
	agentClient.PatchLogs(setupCtx, agentsdk.PatchLogs{
		Logs: []agentsdk.Log{
			{
				CreatedAt: time.Now(),
				Level:     codersdk.LogLevelInfo,
				Output:    "test log message",
			},
		},
	})

	t.Run("ReportTask", func(t *testing.T) {
		ctx := testutil.Context(t, testutil.WaitShort)
		tb, err := toolsdk.NewDeps(memberClient, toolsdk.WithTaskReporter(func(args toolsdk.ReportTaskArgs) error {
			return agentClient.PatchAppStatus(ctx, agentsdk.PatchAppStatus{
				AppSlug: "some-agent-app",
				Message: args.Summary,
				URI:     args.Link,
				State:   codersdk.WorkspaceAppStatusState(args.State),
			})
		}))
		require.NoError(t, err)
		_, err = testTool(t, toolsdk.ReportTask, tb, toolsdk.ReportTaskArgs{
			Summary: "test summary",
			State:   "complete",
			Link:    "https://example.com",
		})
		require.NoError(t, err)
	})

	t.Run("GetWorkspace", func(t *testing.T) {
		tb, err := toolsdk.NewDeps(memberClient)
		require.NoError(t, err)

		tests := []struct {
			name      string
			workspace string
		}{
			{
				name:      "ByID",
				workspace: r.Workspace.ID.String(),
			},
			{
				name:      "ByName",
				workspace: r.Workspace.Name,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				result, err := testTool(t, toolsdk.GetWorkspace, tb, toolsdk.GetWorkspaceArgs{
					WorkspaceID: tt.workspace,
				})
				require.NoError(t, err)
				require.Equal(t, r.Workspace.ID, result.ID, "expected the workspace ID to match")
			})
		}
	})

	t.Run("ListTemplates", func(t *testing.T) {
		tb, err := toolsdk.NewDeps(memberClient)
		require.NoError(t, err)
		// Get the templates directly for comparison
		expected, err := memberClient.Templates(context.Background(), codersdk.TemplateFilter{})
		require.NoError(t, err)

		result, err := testTool(t, toolsdk.ListTemplates, tb, toolsdk.NoArgs{})

		require.NoError(t, err)
		require.Len(t, result, len(expected))

		// Sort the results by name to ensure the order is consistent
		sort.Slice(expected, func(a, b int) bool {
			return expected[a].Name < expected[b].Name
		})
		sort.Slice(result, func(a, b int) bool {
			return result[a].Name < result[b].Name
		})
		for i, template := range result {
			require.Equal(t, expected[i].ID.String(), template.ID)
		}
	})

	t.Run("Whoami", func(t *testing.T) {
		tb, err := toolsdk.NewDeps(memberClient)
		require.NoError(t, err)
		result, err := testTool(t, toolsdk.GetAuthenticatedUser, tb, toolsdk.NoArgs{})

		require.NoError(t, err)
		require.Equal(t, member.ID, result.ID)
		require.Equal(t, member.Username, result.Username)
	})

	t.Run("ListWorkspaces", func(t *testing.T) {
		tb, err := toolsdk.NewDeps(memberClient)
		require.NoError(t, err)
		result, err := testTool(t, toolsdk.ListWorkspaces, tb, toolsdk.ListWorkspacesArgs{})

		require.NoError(t, err)
		require.Len(t, result, 1, "expected 1 workspace")
		workspace := result[0]
		require.Equal(t, r.Workspace.ID.String(), workspace.ID, "expected the workspace to match the one we created")
	})

	t.Run("CreateWorkspaceBuild", func(t *testing.T) {
		t.Run("Stop", func(t *testing.T) {
			ctx := testutil.Context(t, testutil.WaitShort)
			tb, err := toolsdk.NewDeps(memberClient)
			require.NoError(t, err)
			result, err := testTool(t, toolsdk.CreateWorkspaceBuild, tb, toolsdk.CreateWorkspaceBuildArgs{
				WorkspaceID: r.Workspace.ID.String(),
				Transition:  "stop",
			})

			require.NoError(t, err)
			require.Equal(t, codersdk.WorkspaceTransitionStop, result.Transition)
			require.Equal(t, r.Workspace.ID, result.WorkspaceID)
			require.Equal(t, r.TemplateVersion.ID, result.TemplateVersionID)
			require.Equal(t, codersdk.WorkspaceTransitionStop, result.Transition)

			// Important: cancel the build. We don't run any provisioners, so this
			// will remain in the 'pending' state indefinitely.
			require.NoError(t, client.CancelWorkspaceBuild(ctx, result.ID, codersdk.CancelWorkspaceBuildParams{}))
		})

		t.Run("Start", func(t *testing.T) {
			ctx := testutil.Context(t, testutil.WaitShort)
			tb, err := toolsdk.NewDeps(memberClient)
			require.NoError(t, err)
			result, err := testTool(t, toolsdk.CreateWorkspaceBuild, tb, toolsdk.CreateWorkspaceBuildArgs{
				WorkspaceID: r.Workspace.ID.String(),
				Transition:  "start",
			})

			require.NoError(t, err)
			require.Equal(t, codersdk.WorkspaceTransitionStart, result.Transition)
			require.Equal(t, r.Workspace.ID, result.WorkspaceID)
			require.Equal(t, r.TemplateVersion.ID, result.TemplateVersionID)
			require.Equal(t, codersdk.WorkspaceTransitionStart, result.Transition)

			// Important: cancel the build. We don't run any provisioners, so this
			// will remain in the 'pending' state indefinitely.
			require.NoError(t, client.CancelWorkspaceBuild(ctx, result.ID, codersdk.CancelWorkspaceBuildParams{}))
		})

		t.Run("TemplateVersionChange", func(t *testing.T) {
			ctx := testutil.Context(t, testutil.WaitShort)
			tb, err := toolsdk.NewDeps(memberClient)
			require.NoError(t, err)
			// Get the current template version ID before updating
			workspace, err := memberClient.Workspace(ctx, r.Workspace.ID)
			require.NoError(t, err)
			originalVersionID := workspace.LatestBuild.TemplateVersionID

			// Create a new template version to update to
			newVersion := dbfake.TemplateVersion(t, store).
				// nolint:gocritic // This is in a test package and does not end up in the build
				Seed(database.TemplateVersion{
					OrganizationID: owner.OrganizationID,
					CreatedBy:      owner.UserID,
					TemplateID:     uuid.NullUUID{UUID: r.Template.ID, Valid: true},
				}).Do()

			// Update to new version
			updateBuild, err := testTool(t, toolsdk.CreateWorkspaceBuild, tb, toolsdk.CreateWorkspaceBuildArgs{
				WorkspaceID:       r.Workspace.ID.String(),
				Transition:        "start",
				TemplateVersionID: newVersion.TemplateVersion.ID.String(),
			})
			require.NoError(t, err)
			require.Equal(t, codersdk.WorkspaceTransitionStart, updateBuild.Transition)
			require.Equal(t, r.Workspace.ID.String(), updateBuild.WorkspaceID.String())
			require.Equal(t, newVersion.TemplateVersion.ID.String(), updateBuild.TemplateVersionID.String())
			// Cancel the build so it doesn't remain in the 'pending' state indefinitely.
			require.NoError(t, client.CancelWorkspaceBuild(ctx, updateBuild.ID, codersdk.CancelWorkspaceBuildParams{}))

			// Roll back to the original version
			rollbackBuild, err := testTool(t, toolsdk.CreateWorkspaceBuild, tb, toolsdk.CreateWorkspaceBuildArgs{
				WorkspaceID:       r.Workspace.ID.String(),
				Transition:        "start",
				TemplateVersionID: originalVersionID.String(),
			})
			require.NoError(t, err)
			require.Equal(t, codersdk.WorkspaceTransitionStart, rollbackBuild.Transition)
			require.Equal(t, r.Workspace.ID.String(), rollbackBuild.WorkspaceID.String())
			require.Equal(t, originalVersionID.String(), rollbackBuild.TemplateVersionID.String())
			// Cancel the build so it doesn't remain in the 'pending' state indefinitely.
			require.NoError(t, client.CancelWorkspaceBuild(ctx, rollbackBuild.ID, codersdk.CancelWorkspaceBuildParams{}))
		})
	})

	t.Run("ListTemplateVersionParameters", func(t *testing.T) {
		tb, err := toolsdk.NewDeps(memberClient)
		require.NoError(t, err)
		params, err := testTool(t, toolsdk.ListTemplateVersionParameters, tb, toolsdk.ListTemplateVersionParametersArgs{
			TemplateVersionID: r.TemplateVersion.ID.String(),
		})

		require.NoError(t, err)
		require.Empty(t, params)
	})

	t.Run("GetWorkspaceAgentLogs", func(t *testing.T) {
		tb, err := toolsdk.NewDeps(memberClient)
		require.NoError(t, err)
		logs, err := testTool(t, toolsdk.GetWorkspaceAgentLogs, tb, toolsdk.GetWorkspaceAgentLogsArgs{
			WorkspaceAgentID: agentID.String(),
		})

		require.NoError(t, err)
		require.NotEmpty(t, logs)
	})

	t.Run("GetWorkspaceBuildLogs", func(t *testing.T) {
		tb, err := toolsdk.NewDeps(memberClient)
		require.NoError(t, err)
		logs, err := testTool(t, toolsdk.GetWorkspaceBuildLogs, tb, toolsdk.GetWorkspaceBuildLogsArgs{
			WorkspaceBuildID: r.Build.ID.String(),
		})

		require.NoError(t, err)
		_ = logs // The build may not have any logs yet, so we just check that the function returns successfully
	})

	t.Run("GetTemplateVersionLogs", func(t *testing.T) {
		tb, err := toolsdk.NewDeps(memberClient)
		require.NoError(t, err)
		logs, err := testTool(t, toolsdk.GetTemplateVersionLogs, tb, toolsdk.GetTemplateVersionLogsArgs{
			TemplateVersionID: r.TemplateVersion.ID.String(),
		})

		require.NoError(t, err)
		_ = logs // Just ensuring the call succeeds
	})

	t.Run("UpdateTemplateActiveVersion", func(t *testing.T) {
		tb, err := toolsdk.NewDeps(client)
		require.NoError(t, err)
		result, err := testTool(t, toolsdk.UpdateTemplateActiveVersion, tb, toolsdk.UpdateTemplateActiveVersionArgs{
			TemplateID:        r.Template.ID.String(),
			TemplateVersionID: r.TemplateVersion.ID.String(),
		})

		require.NoError(t, err)
		require.Contains(t, result, "Successfully updated")
	})

	t.Run("DeleteTemplate", func(t *testing.T) {
		tb, err := toolsdk.NewDeps(client)
		require.NoError(t, err)
		_, err = testTool(t, toolsdk.DeleteTemplate, tb, toolsdk.DeleteTemplateArgs{
			TemplateID: r.Template.ID.String(),
		})

		// This will fail with because there already exists a workspace.
		require.ErrorContains(t, err, "All workspaces must be deleted before a template can be removed")
	})

	t.Run("UploadTarFile", func(t *testing.T) {
		files := map[string]string{
			"main.tf": `resource "null_resource" "example" {}`,
		}
		tb, err := toolsdk.NewDeps(memberClient)
		require.NoError(t, err)

		result, err := testTool(t, toolsdk.UploadTarFile, tb, toolsdk.UploadTarFileArgs{
			Files: files,
		})

		require.NoError(t, err)
		require.NotEmpty(t, result.ID)
	})

	t.Run("CreateTemplateVersion", func(t *testing.T) {
		tb, err := toolsdk.NewDeps(client)
		require.NoError(t, err)
		// nolint:gocritic // This is in a test package and does not end up in the build
		file := dbgen.File(t, store, database.File{})
		t.Run("WithoutTemplateID", func(t *testing.T) {
			tv, err := testTool(t, toolsdk.CreateTemplateVersion, tb, toolsdk.CreateTemplateVersionArgs{
				FileID: file.ID.String(),
			})
			require.NoError(t, err)
			require.NotEmpty(t, tv)
		})
		t.Run("WithTemplateID", func(t *testing.T) {
			tv, err := testTool(t, toolsdk.CreateTemplateVersion, tb, toolsdk.CreateTemplateVersionArgs{
				FileID:     file.ID.String(),
				TemplateID: r.Template.ID.String(),
			})
			require.NoError(t, err)
			require.NotEmpty(t, tv)
		})
	})

	t.Run("CreateTemplate", func(t *testing.T) {
		tb, err := toolsdk.NewDeps(client)
		require.NoError(t, err)
		// Create a new template version for use here.
		tv := dbfake.TemplateVersion(t, store).
			// nolint:gocritic // This is in a test package and does not end up in the build
			Seed(database.TemplateVersion{OrganizationID: owner.OrganizationID, CreatedBy: owner.UserID}).
			SkipCreateTemplate().Do()

		// We're going to re-use the pre-existing template version
		_, err = testTool(t, toolsdk.CreateTemplate, tb, toolsdk.CreateTemplateArgs{
			Name:        testutil.GetRandomNameHyphenated(t),
			DisplayName: "Test Template",
			Description: "This is a test template",
			VersionID:   tv.TemplateVersion.ID.String(),
		})

		require.NoError(t, err)
	})

	t.Run("CreateWorkspace", func(t *testing.T) {
		tb, err := toolsdk.NewDeps(client)
		require.NoError(t, err)
		// We need a template version ID to create a workspace
		res, err := testTool(t, toolsdk.CreateWorkspace, tb, toolsdk.CreateWorkspaceArgs{
			User:              "me",
			TemplateVersionID: r.TemplateVersion.ID.String(),
			Name:              testutil.GetRandomNameHyphenated(t),
			RichParameters:    map[string]string{},
		})

		// The creation might fail for various reasons, but the important thing is
		// to mark it as tested
		require.NoError(t, err)
		require.NotEmpty(t, res.ID, "expected a workspace ID")
	})

	t.Run("WorkspaceSSHExec", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("WorkspaceSSHExec is not supported on Windows")
		}
		// Setup workspace exactly like main SSH tests
		client, workspace, agentToken := setupWorkspaceForAgent(t, nil)

		// Start agent and wait for it to be ready (following main SSH test pattern)
		_ = agenttest.New(t, client.URL, agentToken)

		// Wait for workspace agents to be ready like main SSH tests do
		coderdtest.NewWorkspaceAgentWaiter(t, client, workspace.ID).Wait()

		// Create tool dependencies using client
		tb, err := toolsdk.NewDeps(client)
		require.NoError(t, err)

		// Test basic command execution
		result, err := testTool(t, toolsdk.WorkspaceBash, tb, toolsdk.WorkspaceBashArgs{
			Workspace: workspace.Name,
			Command:   "echo 'hello world'",
		})
		require.NoError(t, err)
		require.Equal(t, 0, result.ExitCode)
		require.Equal(t, "hello world", result.Output)

		// Test output trimming
		result, err = testTool(t, toolsdk.WorkspaceBash, tb, toolsdk.WorkspaceBashArgs{
			Workspace: workspace.Name,
			Command:   "echo -e '\\n  test with whitespace  \\n'",
		})
		require.NoError(t, err)
		require.Equal(t, 0, result.ExitCode)
		require.Equal(t, "test with whitespace", result.Output) // Should be trimmed

		// Test non-zero exit code
		result, err = testTool(t, toolsdk.WorkspaceBash, tb, toolsdk.WorkspaceBashArgs{
			Workspace: workspace.Name,
			Command:   "exit 42",
		})
		require.NoError(t, err)
		require.Equal(t, 42, result.ExitCode)
		require.Empty(t, result.Output)

		// Test with workspace owner format - using the myuser from setup
		result, err = testTool(t, toolsdk.WorkspaceBash, tb, toolsdk.WorkspaceBashArgs{
			Workspace: "myuser/" + workspace.Name,
			Command:   "echo 'owner format works'",
		})
		require.NoError(t, err)
		require.Equal(t, 0, result.ExitCode)
		require.Equal(t, "owner format works", result.Output)
	})

	t.Run("WorkspaceLS", func(t *testing.T) {
		t.Parallel()

		client, workspace, agentToken := setupWorkspaceForAgent(t, nil)
		fs := afero.NewMemMapFs()
		_ = agenttest.New(t, client.URL, agentToken, func(opts *agent.Options) {
			opts.Filesystem = fs
		})
		coderdtest.NewWorkspaceAgentWaiter(t, client, workspace.ID).Wait()
		tb, err := toolsdk.NewDeps(client)
		require.NoError(t, err)

		tmpdir := os.TempDir()

		dirPath := filepath.Join(tmpdir, "dir1/dir2")
		err = fs.MkdirAll(dirPath, 0o755)
		require.NoError(t, err)

		filePath := filepath.Join(tmpdir, "dir1", "foo")
		err = afero.WriteFile(fs, filePath, []byte("foo bar"), 0o644)
		require.NoError(t, err)

		_, err = testTool(t, toolsdk.WorkspaceLS, tb, toolsdk.WorkspaceLSArgs{
			Workspace: workspace.Name,
			Path:      "relative",
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "path must be absolute")

		res, err := testTool(t, toolsdk.WorkspaceLS, tb, toolsdk.WorkspaceLSArgs{
			Workspace: workspace.Name,
			Path:      filepath.Dir(dirPath),
		})
		require.NoError(t, err)
		require.Equal(t, []toolsdk.WorkspaceLSFile{
			{
				Path:  dirPath,
				IsDir: true,
			},
			{
				Path:  filePath,
				IsDir: false,
			},
		}, res.Contents)
	})

	t.Run("WorkspaceReadFile", func(t *testing.T) {
		t.Parallel()

		client, workspace, agentToken := setupWorkspaceForAgent(t, nil)
		fs := afero.NewMemMapFs()
		_ = agenttest.New(t, client.URL, agentToken, func(opts *agent.Options) {
			opts.Filesystem = fs
		})
		coderdtest.NewWorkspaceAgentWaiter(t, client, workspace.ID).Wait()
		tb, err := toolsdk.NewDeps(client)
		require.NoError(t, err)

		tmpdir := os.TempDir()
		filePath := filepath.Join(tmpdir, "file")
		err = afero.WriteFile(fs, filePath, []byte("content"), 0o644)
		require.NoError(t, err)

		largeFilePath := filepath.Join(tmpdir, "large")
		largeFile, err := fs.Create(largeFilePath)
		require.NoError(t, err)
		err = largeFile.Truncate(1 << 21)
		require.NoError(t, err)

		imagePath := filepath.Join(tmpdir, "file.png")
		err = afero.WriteFile(fs, imagePath, []byte("not really an image"), 0o644)
		require.NoError(t, err)

		tests := []struct {
			name     string
			path     string
			limit    int64
			offset   int64
			mimeType string
			bytes    []byte
			length   int
			error    string
		}{
			{
				name:  "NonExistent",
				path:  filepath.Join(tmpdir, "does-not-exist"),
				error: "file does not exist",
			},
			{
				name:     "Exists",
				path:     filePath,
				bytes:    []byte("content"),
				mimeType: "application/octet-stream",
			},
			{
				name:     "Limit1Offset2",
				path:     filePath,
				limit:    1,
				offset:   2,
				bytes:    []byte("n"),
				mimeType: "application/octet-stream",
			},
			{
				name:     "DefaultMaxLimit",
				path:     largeFilePath,
				length:   1 << 20,
				mimeType: "application/octet-stream",
			},
			{
				name:  "ExceedMaxLimit",
				path:  filePath,
				limit: 1 << 21,
				error: "limit must be 1048576 or less, got 2097152",
			},
			{
				name:     "ImageMimeType",
				path:     imagePath,
				bytes:    []byte("not really an image"),
				mimeType: "image/png",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				resp, err := testTool(t, toolsdk.WorkspaceReadFile, tb, toolsdk.WorkspaceReadFileArgs{
					Workspace: workspace.Name,
					Path:      tt.path,
					Limit:     tt.limit,
					Offset:    tt.offset,
				})
				if tt.error != "" {
					require.Error(t, err)
					require.Contains(t, err.Error(), tt.error)
				} else {
					require.NoError(t, err)
					if tt.length != 0 {
						require.Len(t, resp.Content, tt.length)
					}
					if tt.bytes != nil {
						require.Equal(t, tt.bytes, resp.Content)
					}
					require.Equal(t, tt.mimeType, resp.MimeType)
				}
			})
		}
	})

	t.Run("WorkspaceWriteFile", func(t *testing.T) {
		t.Parallel()

		client, workspace, agentToken := setupWorkspaceForAgent(t, nil)
		fs := afero.NewMemMapFs()
		_ = agenttest.New(t, client.URL, agentToken, func(opts *agent.Options) {
			opts.Filesystem = fs
		})
		coderdtest.NewWorkspaceAgentWaiter(t, client, workspace.ID).Wait()
		tb, err := toolsdk.NewDeps(client)
		require.NoError(t, err)

		tmpdir := os.TempDir()
		filePath := filepath.Join(tmpdir, "write")

		_, err = testTool(t, toolsdk.WorkspaceWriteFile, tb, toolsdk.WorkspaceWriteFileArgs{
			Workspace: workspace.Name,
			Path:      filePath,
			Content:   []byte("content"),
		})
		require.NoError(t, err)

		b, err := afero.ReadFile(fs, filePath)
		require.NoError(t, err)
		require.Equal(t, []byte("content"), b)
	})

	t.Run("WorkspaceEditFile", func(t *testing.T) {
		t.Parallel()

		client, workspace, agentToken := setupWorkspaceForAgent(t, nil)
		fs := afero.NewMemMapFs()
		_ = agenttest.New(t, client.URL, agentToken, func(opts *agent.Options) {
			opts.Filesystem = fs
		})
		coderdtest.NewWorkspaceAgentWaiter(t, client, workspace.ID).Wait()
		tb, err := toolsdk.NewDeps(client)
		require.NoError(t, err)

		tmpdir := os.TempDir()
		filePath := filepath.Join(tmpdir, "edit")
		err = afero.WriteFile(fs, filePath, []byte("foo bar"), 0o644)
		require.NoError(t, err)

		_, err = testTool(t, toolsdk.WorkspaceEditFile, tb, toolsdk.WorkspaceEditFileArgs{
			Workspace: workspace.Name,
			Path:      filePath,
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "must specify at least one edit")

		_, err = testTool(t, toolsdk.WorkspaceEditFile, tb, toolsdk.WorkspaceEditFileArgs{
			Workspace: workspace.Name,
			Path:      filePath,
			Edits: []workspacesdk.FileEdit{
				{
					Search:  "foo",
					Replace: "bar",
				},
			},
		})
		require.NoError(t, err)
		b, err := afero.ReadFile(fs, filePath)
		require.NoError(t, err)
		require.Equal(t, "bar bar", string(b))
	})

	t.Run("WorkspaceEditFiles", func(t *testing.T) {
		t.Parallel()

		client, workspace, agentToken := setupWorkspaceForAgent(t, nil)
		fs := afero.NewMemMapFs()
		_ = agenttest.New(t, client.URL, agentToken, func(opts *agent.Options) {
			opts.Filesystem = fs
		})
		coderdtest.NewWorkspaceAgentWaiter(t, client, workspace.ID).Wait()
		tb, err := toolsdk.NewDeps(client)
		require.NoError(t, err)

		tmpdir := os.TempDir()
		filePath1 := filepath.Join(tmpdir, "edit1")
		err = afero.WriteFile(fs, filePath1, []byte("foo1 bar1"), 0o644)
		require.NoError(t, err)

		filePath2 := filepath.Join(tmpdir, "edit2")
		err = afero.WriteFile(fs, filePath2, []byte("foo2 bar2"), 0o644)
		require.NoError(t, err)

		_, err = testTool(t, toolsdk.WorkspaceEditFiles, tb, toolsdk.WorkspaceEditFilesArgs{
			Workspace: workspace.Name,
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "must specify at least one file")

		_, err = testTool(t, toolsdk.WorkspaceEditFiles, tb, toolsdk.WorkspaceEditFilesArgs{
			Workspace: workspace.Name,
			Files: []workspacesdk.FileEdits{
				{
					Path: filePath1,
					Edits: []workspacesdk.FileEdit{
						{
							Search:  "foo1",
							Replace: "bar1",
						},
					},
				},
				{
					Path: filePath2,
					Edits: []workspacesdk.FileEdit{
						{
							Search:  "foo2",
							Replace: "bar2",
						},
					},
				},
			},
		})
		require.NoError(t, err)

		b, err := afero.ReadFile(fs, filePath1)
		require.NoError(t, err)
		require.Equal(t, "bar1 bar1", string(b))

		b, err = afero.ReadFile(fs, filePath2)
		require.NoError(t, err)
		require.Equal(t, "bar2 bar2", string(b))
	})

	t.Run("WorkspacePortForward", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name      string
			workspace string
			host      string
			port      int
			expect    string
			error     string
		}{
			{
				name:      "OK",
				workspace: "myuser/myworkspace",
				port:      1234,
				host:      "*.test.coder.com",
				expect:    "%s://1234--dev--myworkspace--myuser.test.coder.com:%s",
			},
			{
				name:      "NonExistentWorkspace",
				workspace: "doesnotexist",
				port:      1234,
				host:      "*.test.coder.com",
				error:     "failed to find workspace",
			},
			{
				name:      "NoAppHost",
				host:      "",
				workspace: "myuser/myworkspace",
				port:      1234,
				error:     "no app host",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				client, workspace, agentToken := setupWorkspaceForAgent(t, &coderdtest.Options{
					AppHostname: tt.host,
				})
				_ = agenttest.New(t, client.URL, agentToken)
				coderdtest.NewWorkspaceAgentWaiter(t, client, workspace.ID).Wait()
				tb, err := toolsdk.NewDeps(client)
				require.NoError(t, err)

				res, err := testTool(t, toolsdk.WorkspacePortForward, tb, toolsdk.WorkspacePortForwardArgs{
					Workspace: tt.workspace,
					Port:      tt.port,
				})
				if tt.error != "" {
					require.Error(t, err)
					require.ErrorContains(t, err, tt.error)
				} else {
					require.NoError(t, err)
					require.Equal(t, fmt.Sprintf(tt.expect, client.URL.Scheme, client.URL.Port()), res.URL)
				}
			})
		}
	})

	t.Run("CreateTask", func(t *testing.T) {
		t.Parallel()

		presetID := uuid.New()
		// nolint:gocritic // This is in a test package and does not end up in the build
		aiTV := dbfake.TemplateVersion(t, store).Seed(database.TemplateVersion{
			OrganizationID: owner.OrganizationID,
			CreatedBy:      member.ID,
			HasAITask: sql.NullBool{
				Bool:  true,
				Valid: true,
			},
		}).Preset(database.TemplateVersionPreset{
			ID: presetID,
			DesiredInstances: sql.NullInt32{
				Int32: 1,
				Valid: true,
			},
		}).Do()

		tests := []struct {
			name  string
			args  toolsdk.CreateTaskArgs
			error string
		}{
			{
				name: "OK",
				args: toolsdk.CreateTaskArgs{
					TemplateVersionID: aiTV.TemplateVersion.ID.String(),
					Input:             "do a barrel roll",
					User:              "me",
				},
			},
			{
				name: "NoUser",
				args: toolsdk.CreateTaskArgs{
					TemplateVersionID: aiTV.TemplateVersion.ID.String(),
					Input:             "do another barrel roll",
				},
			},
			{
				name: "NoInput",
				args: toolsdk.CreateTaskArgs{
					TemplateVersionID: aiTV.TemplateVersion.ID.String(),
				},
				error: "input is required",
			},
			{
				name: "NotTaskTemplate",
				args: toolsdk.CreateTaskArgs{
					TemplateVersionID: r.TemplateVersion.ID.String(),
					Input:             "do yet another barrel roll",
				},
				error: "Template does not have a valid \"coder_ai_task\" resource.",
			},
			{
				name: "WithPreset",
				args: toolsdk.CreateTaskArgs{
					TemplateVersionID:       r.TemplateVersion.ID.String(),
					TemplateVersionPresetID: presetID.String(),
					Input:                   "not enough barrel rolls",
				},
				error: "Template does not have a valid \"coder_ai_task\" resource.",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				tb, err := toolsdk.NewDeps(memberClient)
				require.NoError(t, err)

				_, err = testTool(t, toolsdk.CreateTask, tb, tt.args)
				if tt.error != "" {
					require.Error(t, err)
					require.ErrorContains(t, err, tt.error)
				} else {
					require.NoError(t, err)
				}
			})
		}
	})

	t.Run("DeleteTask", func(t *testing.T) {
		t.Parallel()

		// nolint:gocritic // This is in a test package and does not end up in the build
		aiTV := dbfake.TemplateVersion(t, store).Seed(database.TemplateVersion{
			OrganizationID: owner.OrganizationID,
			CreatedBy:      member.ID,
			HasAITask: sql.NullBool{
				Bool:  true,
				Valid: true,
			},
		}).Do()

		build1 := dbfake.WorkspaceBuild(t, store, database.WorkspaceTable{
			Name:           "delete-task-workspace-1",
			OrganizationID: owner.OrganizationID,
			OwnerID:        member.ID,
			TemplateID:     aiTV.Template.ID,
		}).WithTask(database.TaskTable{
			Name:   "delete-task-1",
			Prompt: "delete task 1",
		}, nil).Do()
		task1 := build1.Task

		build2 := dbfake.WorkspaceBuild(t, store, database.WorkspaceTable{
			Name:           "delete-task-workspace-2",
			OrganizationID: owner.OrganizationID,
			OwnerID:        member.ID,
			TemplateID:     aiTV.Template.ID,
		}).WithTask(database.TaskTable{
			Name:   "delete-task-2",
			Prompt: "delete task 2",
		}, nil).Do()
		task2 := build2.Task

		tests := []struct {
			name  string
			args  toolsdk.DeleteTaskArgs
			error string
		}{
			{
				name: "ByUUID",
				args: toolsdk.DeleteTaskArgs{
					TaskID: task1.ID.String(),
				},
			},
			{
				name: "ByIdentifier",
				args: toolsdk.DeleteTaskArgs{
					TaskID: task2.Name,
				},
			},
			{
				name:  "NoID",
				args:  toolsdk.DeleteTaskArgs{},
				error: "task_id is required",
			},
			{
				name: "NoTaskByID",
				args: toolsdk.DeleteTaskArgs{
					TaskID: uuid.New().String(),
				},
				error: "Resource not found",
			},
			{
				name: "NoTaskByWorkspaceIdentifier",
				args: toolsdk.DeleteTaskArgs{
					TaskID: "non-existent",
				},
				error: "Resource not found",
			},
			{
				name: "ExistsButNotATask",
				args: toolsdk.DeleteTaskArgs{
					TaskID: r.Workspace.ID.String(),
				},
				error: "Resource not found",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				tb, err := toolsdk.NewDeps(memberClient)
				require.NoError(t, err)

				_, err = testTool(t, toolsdk.DeleteTask, tb, tt.args)
				if tt.error != "" {
					require.Error(t, err)
					require.ErrorContains(t, err, tt.error)
				} else {
					require.NoError(t, err)
				}
			})
		}
	})

	t.Run("ListTasks", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		owner := coderdtest.CreateFirstUser(t, client)
		_, member := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
		taskClient, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)

		// Create a template with AI task support using the proper flow.
		version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, &echo.Responses{
			Parse:          echo.ParseComplete,
			ProvisionApply: echo.ApplyComplete,
			ProvisionGraph: []*proto.Response{
				{Type: &proto.Response_Graph{Graph: &proto.GraphComplete{
					Parameters: []*proto.RichParameter{{Name: "AI Prompt", Type: "string"}},
					HasAiTasks: true,
				}}},
			},
		})
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID)

		// This task should not show up since listing is user-scoped.
		_, err := client.CreateTask(ctx, member.Username, codersdk.CreateTaskRequest{
			TemplateVersionID: template.ActiveVersionID,
			Input:             "task for member",
			Name:              "list-task-workspace-member",
		})
		require.NoError(t, err)

		// Create tasks for taskUser. These should show up in the list.
		for i := range 5 {
			taskName := fmt.Sprintf("list-task-workspace-%d", i)
			task, err := taskClient.CreateTask(ctx, codersdk.Me, codersdk.CreateTaskRequest{
				TemplateVersionID: template.ActiveVersionID,
				Input:             fmt.Sprintf("task %d", i),
				Name:              taskName,
			})
			require.NoError(t, err)
			require.True(t, task.WorkspaceID.Valid, "task should have workspace ID")

			// For the first task, stop the workspace to make it paused.
			if i == 0 {
				ws, err := taskClient.Workspace(ctx, task.WorkspaceID.UUID)
				require.NoError(t, err)
				coderdtest.AwaitWorkspaceBuildJobCompleted(t, taskClient, ws.LatestBuild.ID)

				// Stop the workspace to set task status to paused.
				build, err := taskClient.CreateWorkspaceBuild(ctx, task.WorkspaceID.UUID, codersdk.CreateWorkspaceBuildRequest{
					Transition: codersdk.WorkspaceTransitionStop,
				})
				require.NoError(t, err)
				coderdtest.AwaitWorkspaceBuildJobCompleted(t, taskClient, build.ID)
			}
		}

		tests := []struct {
			name     string
			args     toolsdk.ListTasksArgs
			expected []string
			error    string
		}{
			{
				name: "ListAllOwned",
				args: toolsdk.ListTasksArgs{},
				expected: []string{
					"list-task-workspace-0",
					"list-task-workspace-1",
					"list-task-workspace-2",
					"list-task-workspace-3",
					"list-task-workspace-4",
				},
			},
			{
				name: "ListFiltered",
				args: toolsdk.ListTasksArgs{
					Status: codersdk.TaskStatusPaused,
				},
				expected: []string{
					"list-task-workspace-0",
				},
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				tb, err := toolsdk.NewDeps(taskClient)
				require.NoError(t, err)

				res, err := testTool(t, toolsdk.ListTasks, tb, tt.args)
				if tt.error != "" {
					require.Error(t, err)
					require.ErrorContains(t, err, tt.error)
				} else {
					require.NoError(t, err)
					require.Len(t, res.Tasks, len(tt.expected))
					for _, task := range res.Tasks {
						require.Contains(t, tt.expected, task.Name)
					}
				}
			})
		}
	})

	t.Run("GetTask", func(t *testing.T) {
		t.Parallel()

		// nolint:gocritic // This is in a test package and does not end up in the build
		aiTV := dbfake.TemplateVersion(t, store).Seed(database.TemplateVersion{
			OrganizationID: owner.OrganizationID,
			CreatedBy:      member.ID,
			HasAITask: sql.NullBool{
				Bool:  true,
				Valid: true,
			},
		}).Do()

		build := dbfake.WorkspaceBuild(t, store, database.WorkspaceTable{
			Name:           "get-task-workspace-1",
			OrganizationID: owner.OrganizationID,
			OwnerID:        member.ID,
			TemplateID:     aiTV.Template.ID,
		}).WithTask(database.TaskTable{
			Name:   "get-task-1",
			Prompt: "get task",
		}, nil).Do()
		task := build.Task

		tests := []struct {
			name     string
			args     toolsdk.GetTaskStatusArgs
			expected codersdk.TaskStatus
			error    string
		}{
			{
				name: "ByUUID",
				args: toolsdk.GetTaskStatusArgs{
					TaskID: task.ID.String(),
				},
				expected: codersdk.TaskStatusInitializing,
			},
			{
				name: "ByIdentifier",
				args: toolsdk.GetTaskStatusArgs{
					TaskID: task.Name,
				},
				expected: codersdk.TaskStatusInitializing,
			},
			{
				name:  "NoID",
				args:  toolsdk.GetTaskStatusArgs{},
				error: "task_id is required",
			},
			{
				name: "NoTaskByID",
				args: toolsdk.GetTaskStatusArgs{
					TaskID: uuid.New().String(),
				},
				error: "Resource not found",
			},
			{
				name: "NoTaskByWorkspaceIdentifier",
				args: toolsdk.GetTaskStatusArgs{
					TaskID: "non-existent",
				},
				error: "Resource not found",
			},
			{
				name: "ExistsButNotATask",
				args: toolsdk.GetTaskStatusArgs{
					TaskID: r.Workspace.ID.String(),
				},
				error: "Resource not found",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				tb, err := toolsdk.NewDeps(memberClient)
				require.NoError(t, err)

				res, err := testTool(t, toolsdk.GetTaskStatus, tb, tt.args)
				if tt.error != "" {
					require.Error(t, err)
					require.ErrorContains(t, err, tt.error)
				} else {
					require.NoError(t, err)
					require.Equal(t, tt.expected, res.Status)
				}
			})
		}
	})

	t.Run("WorkspaceListApps", func(t *testing.T) {
		t.Parallel()

		// nolint:gocritic // This is in a test package and does not end up in the build
		_ = dbfake.WorkspaceBuild(t, store, database.WorkspaceTable{
			Name:           "list-app-workspace-one-agent",
			OrganizationID: owner.OrganizationID,
			OwnerID:        member.ID,
		}).WithAgent(func(agents []*proto.Agent) []*proto.Agent {
			agents[0].Apps = []*proto.App{
				{
					Slug: "zero",
					Url:  "http://zero.dev.coder.com",
				},
			}
			return agents
		}).Do()

		// nolint:gocritic // This is in a test package and does not end up in the build
		_ = dbfake.WorkspaceBuild(t, store, database.WorkspaceTable{
			Name:           "list-app-workspace-multi-agent",
			OrganizationID: owner.OrganizationID,
			OwnerID:        member.ID,
		}).WithAgent(func(agents []*proto.Agent) []*proto.Agent {
			agents[0].Apps = []*proto.App{
				{
					Slug: "one",
					Url:  "http://one.dev.coder.com",
				},
				{
					Slug: "two",
					Url:  "http://two.dev.coder.com",
				},
				{
					Slug: "three",
					Url:  "http://three.dev.coder.com",
				},
			}
			agents = append(agents, &proto.Agent{
				Id:   uuid.NewString(),
				Name: "dev2",
				Auth: &proto.Agent_Token{
					Token: uuid.NewString(),
				},
				Env: map[string]string{},
				Apps: []*proto.App{
					{
						Slug: "four",
						Url:  "http://four.dev.coder.com",
					},
				},
			})
			return agents
		}).Do()

		tests := []struct {
			name     string
			args     toolsdk.WorkspaceListAppsArgs
			expected []toolsdk.WorkspaceListApp
			error    string
		}{
			{
				name: "NonExistentWorkspace",
				args: toolsdk.WorkspaceListAppsArgs{
					Workspace: "list-appp-workspace-does-not-exist",
				},
				error: "failed to find workspace",
			},
			{
				name: "OneAgentOneApp",
				args: toolsdk.WorkspaceListAppsArgs{
					Workspace: "list-app-workspace-one-agent",
				},
				expected: []toolsdk.WorkspaceListApp{
					{
						Name: "zero",
						URL:  "http://zero.dev.coder.com",
					},
				},
			},
			{
				name: "MultiAgent",
				args: toolsdk.WorkspaceListAppsArgs{
					Workspace: "list-app-workspace-multi-agent",
				},
				error: "multiple agents found, please specify the agent name",
			},
			{
				name: "MultiAgentOneApp",
				args: toolsdk.WorkspaceListAppsArgs{
					Workspace: "list-app-workspace-multi-agent.dev2",
				},
				expected: []toolsdk.WorkspaceListApp{
					{
						Name: "four",
						URL:  "http://four.dev.coder.com",
					},
				},
			},
			{
				name: "MultiAgentMultiApp",
				args: toolsdk.WorkspaceListAppsArgs{
					Workspace: "list-app-workspace-multi-agent.dev",
				},
				expected: []toolsdk.WorkspaceListApp{
					{
						Name: "one",
						URL:  "http://one.dev.coder.com",
					},
					{
						Name: "three",
						URL:  "http://three.dev.coder.com",
					},
					{
						Name: "two",
						URL:  "http://two.dev.coder.com",
					},
				},
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				tb, err := toolsdk.NewDeps(memberClient)
				require.NoError(t, err)

				res, err := testTool(t, toolsdk.WorkspaceListApps, tb, tt.args)
				if tt.error != "" {
					require.Error(t, err)
					require.ErrorContains(t, err, tt.error)
				} else {
					require.NoError(t, err)
					require.Equal(t, tt.expected, res.Apps)
				}
			})
		}
	})

	t.Run("SendTaskInput", func(t *testing.T) {
		t.Parallel()

		// Start a fake AgentAPI that accepts GET /status and POST /message.
		srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet && r.URL.Path == "/status" {
				httpapi.Write(r.Context(), rw, http.StatusOK, agentapi.GetStatusResponse{
					Status: agentapi.StatusStable,
				})
				return
			}
			if r.Method == http.MethodPost && r.URL.Path == "/message" {
				rw.Header().Set("Content-Type", "application/json")

				var req agentapi.PostMessageParams
				ok := httpapi.Read(r.Context(), rw, r, &req)
				assert.True(t, ok, "failed to read request")

				assert.Equal(t, req.Content, "frob the baz")
				assert.Equal(t, req.Type, agentapi.MessageTypeUser)

				httpapi.Write(r.Context(), rw, http.StatusOK, agentapi.PostMessageResponse{
					Ok: true,
				})
				return
			}
			rw.WriteHeader(http.StatusInternalServerError)
		}))
		t.Cleanup(srv.Close)

		// nolint:gocritic // This is in a test package and does not end up in the build
		aiTV := dbfake.TemplateVersion(t, store).Seed(database.TemplateVersion{
			OrganizationID: owner.OrganizationID,
			CreatedBy:      member.ID,
			HasAITask: sql.NullBool{
				Bool:  true,
				Valid: true,
			},
		}).Do()

		ws := dbfake.WorkspaceBuild(t, store, database.WorkspaceTable{
			Name:           "send-task-input-ws",
			OrganizationID: owner.OrganizationID,
			OwnerID:        member.ID,
			TemplateID:     aiTV.Template.ID,
		}).WithTask(database.TaskTable{
			Name:   "send-task-input",
			Prompt: "send task input",
		}, &proto.App{Url: srv.URL}).Do()
		task := ws.Task

		_ = agenttest.New(t, client.URL, ws.AgentToken)
		coderdtest.NewWorkspaceAgentWaiter(t, client, ws.Workspace.ID).
			WaitFor(coderdtest.AgentsReady)

		ctx := testutil.Context(t, testutil.WaitShort)

		// Ensure the app is healthy (required to send task input).
		err = store.UpdateWorkspaceAppHealthByID(dbauthz.AsSystemRestricted(ctx), database.UpdateWorkspaceAppHealthByIDParams{
			ID:     task.WorkspaceAppID.UUID,
			Health: database.WorkspaceAppHealthHealthy,
		})
		require.NoError(t, err)

		tests := []struct {
			name  string
			args  toolsdk.SendTaskInputArgs
			error string
		}{
			{
				name: "ByUUID",
				args: toolsdk.SendTaskInputArgs{
					TaskID: task.ID.String(),
					Input:  "frob the baz",
				},
			},
			{
				name: "ByIdentifier",
				args: toolsdk.SendTaskInputArgs{
					TaskID: task.Name,
					Input:  "frob the baz",
				},
			},
			{
				name:  "NoID",
				args:  toolsdk.SendTaskInputArgs{},
				error: "task_id is required",
			},
			{
				name: "NoInput",
				args: toolsdk.SendTaskInputArgs{
					TaskID: "send-task-input",
				},
				error: "input is required",
			},
			{
				name: "NoTaskByID",
				args: toolsdk.SendTaskInputArgs{
					TaskID: uuid.New().String(),
					Input:  "this is ignored",
				},
				error: "Resource not found",
			},
			{
				name: "NoTaskByWorkspaceIdentifier",
				args: toolsdk.SendTaskInputArgs{
					TaskID: "non-existent",
					Input:  "this is ignored",
				},
				error: "Resource not found",
			},
			{
				name: "ExistsButNotATask",
				args: toolsdk.SendTaskInputArgs{
					TaskID: r.Workspace.ID.String(),
					Input:  "this is ignored",
				},
				error: "Resource not found",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				tb, err := toolsdk.NewDeps(memberClient)
				require.NoError(t, err)

				_, err = testTool(t, toolsdk.SendTaskInput, tb, tt.args)
				if tt.error != "" {
					require.Error(t, err)
					require.ErrorContains(t, err, tt.error)
				} else {
					require.NoError(t, err)
				}
			})
		}
	})

	t.Run("GetTaskLogs", func(t *testing.T) {
		t.Parallel()

		messages := []agentapi.Message{
			{
				Id:      0,
				Content: "welcome",
				Role:    agentapi.RoleAgent,
			},
			{
				Id:      1,
				Content: "frob the dazzle",
				Role:    agentapi.RoleUser,
			},
			{
				Id:      2,
				Content: "frob dazzled",
				Role:    agentapi.RoleAgent,
			},
		}

		// Start a fake AgentAPI that returns some messages.
		srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet && r.URL.Path == "/messages" {
				httpapi.Write(r.Context(), rw, http.StatusOK, agentapi.GetMessagesResponse{
					Messages: messages,
				})
				return
			}
			rw.WriteHeader(http.StatusInternalServerError)
		}))
		t.Cleanup(srv.Close)

		// nolint:gocritic // This is in a test package and does not end up in the build
		aiTV := dbfake.TemplateVersion(t, store).Seed(database.TemplateVersion{
			OrganizationID: owner.OrganizationID,
			CreatedBy:      member.ID,
			HasAITask: sql.NullBool{
				Bool:  true,
				Valid: true,
			},
		}).Do()

		ws := dbfake.WorkspaceBuild(t, store, database.WorkspaceTable{
			Name:           "get-task-logs-ws",
			OrganizationID: owner.OrganizationID,
			OwnerID:        member.ID,
			TemplateID:     aiTV.Template.ID,
		}).WithTask(database.TaskTable{
			Name:   "get-task-logs",
			Prompt: "get task logs",
		}, &proto.App{Url: srv.URL}).Do()
		task := ws.Task

		_ = agenttest.New(t, client.URL, ws.AgentToken)
		coderdtest.NewWorkspaceAgentWaiter(t, client, ws.Workspace.ID).
			WaitFor(coderdtest.AgentsReady)

		ctx := testutil.Context(t, testutil.WaitShort)

		// Ensure the app is healthy (required to read task logs).
		err = store.UpdateWorkspaceAppHealthByID(dbauthz.AsSystemRestricted(ctx), database.UpdateWorkspaceAppHealthByIDParams{
			ID:     task.WorkspaceAppID.UUID,
			Health: database.WorkspaceAppHealthHealthy,
		})
		require.NoError(t, err)

		tests := []struct {
			name     string
			args     toolsdk.GetTaskLogsArgs
			expected []agentapi.Message
			error    string
		}{
			{
				name: "ByUUID",
				args: toolsdk.GetTaskLogsArgs{
					TaskID: task.ID.String(),
				},
				expected: messages,
			},
			{
				name: "ByIdentifier",
				args: toolsdk.GetTaskLogsArgs{
					TaskID: task.Name,
				},
				expected: messages,
			},
			{
				name:  "NoID",
				args:  toolsdk.GetTaskLogsArgs{},
				error: "task_id is required",
			},
			{
				name: "NoTaskByID",
				args: toolsdk.GetTaskLogsArgs{
					TaskID: uuid.New().String(),
				},
				error: "Resource not found",
			},
			{
				name: "NoTaskByWorkspaceIdentifier",
				args: toolsdk.GetTaskLogsArgs{
					TaskID: "non-existent",
				},
				error: "Resource not found",
			},
			{
				name: "ExistsButNotATask",
				args: toolsdk.GetTaskLogsArgs{
					TaskID: r.Workspace.ID.String(),
				},
				error: "Resource not found",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				tb, err := toolsdk.NewDeps(memberClient)
				require.NoError(t, err)

				res, err := testTool(t, toolsdk.GetTaskLogs, tb, tt.args)
				if tt.error != "" {
					require.Error(t, err)
					require.ErrorContains(t, err, tt.error)
				} else {
					require.NoError(t, err)
					require.Len(t, res.Logs, len(tt.expected))
					for i, msg := range tt.expected {
						require.Equal(t, msg.Id, int64(res.Logs[i].ID))
						require.Equal(t, msg.Content, res.Logs[i].Content)
						if msg.Role == agentapi.RoleUser {
							require.Equal(t, codersdk.TaskLogTypeInput, res.Logs[i].Type)
						} else {
							require.Equal(t, codersdk.TaskLogTypeOutput, res.Logs[i].Type)
						}
						require.Equal(t, msg.Time, res.Logs[i].Time)
					}
				}
			})
		}
	})
}

// TestedTools keeps track of which tools have been tested.
var testedTools sync.Map

// testTool is a helper function to test a tool and mark it as tested.
// Note that we test the _generic_ version of the tool and not the typed one.
// This is to mimic how we expect external callers to use the tool.
func testTool[Arg, Ret any](t *testing.T, tool toolsdk.Tool[Arg, Ret], tb toolsdk.Deps, args Arg) (Ret, error) {
	t.Helper()
	defer func() { testedTools.Store(tool.Name, true) }()
	toolArgs, err := json.Marshal(args)
	require.NoError(t, err, "failed to marshal args")
	result, err := tool.Generic().Handler(t.Context(), tb, toolArgs)
	var ret Ret
	require.NoError(t, json.Unmarshal(result, &ret), "failed to unmarshal result %q", string(result))
	return ret, err
}

func TestWithRecovery(t *testing.T) {
	t.Parallel()
	t.Run("OK", func(t *testing.T) {
		t.Parallel()
		fakeTool := toolsdk.GenericTool{
			Tool: aisdk.Tool{
				Name:        "echo",
				Description: "Echoes the input.",
			},
			Handler: func(ctx context.Context, tb toolsdk.Deps, args json.RawMessage) (json.RawMessage, error) {
				return args, nil
			},
		}

		wrapped := toolsdk.WithRecover(fakeTool.Handler)
		v, err := wrapped(context.Background(), toolsdk.Deps{}, []byte(`{}`))
		require.NoError(t, err)
		require.JSONEq(t, `{}`, string(v))
	})

	t.Run("Error", func(t *testing.T) {
		t.Parallel()
		fakeTool := toolsdk.GenericTool{
			Tool: aisdk.Tool{
				Name:        "fake_tool",
				Description: "Returns an error for testing.",
			},
			Handler: func(ctx context.Context, tb toolsdk.Deps, args json.RawMessage) (json.RawMessage, error) {
				return nil, assert.AnError
			},
		}
		wrapped := toolsdk.WithRecover(fakeTool.Handler)
		v, err := wrapped(context.Background(), toolsdk.Deps{}, []byte(`{}`))
		require.Nil(t, v)
		require.ErrorIs(t, err, assert.AnError)
	})

	t.Run("Panic", func(t *testing.T) {
		t.Parallel()
		panicTool := toolsdk.GenericTool{
			Tool: aisdk.Tool{
				Name:        "panic_tool",
				Description: "Panics for testing.",
			},
			Handler: func(ctx context.Context, tb toolsdk.Deps, args json.RawMessage) (json.RawMessage, error) {
				panic("you can't sweat this fever out")
			},
		}

		wrapped := toolsdk.WithRecover(panicTool.Handler)
		v, err := wrapped(context.Background(), toolsdk.Deps{}, []byte("disco"))
		require.Empty(t, v)
		require.ErrorContains(t, err, "you can't sweat this fever out")
	})
}

type testContextKey struct{}

func TestWithCleanContext(t *testing.T) {
	t.Parallel()

	t.Run("NoContextKeys", func(t *testing.T) {
		t.Parallel()

		// This test is to ensure that the context values are not set in the
		// toolsdk package.
		ctxTool := toolsdk.GenericTool{
			Tool: aisdk.Tool{
				Name:        "context_tool",
				Description: "Returns the context value for testing.",
			},
			Handler: func(toolCtx context.Context, tb toolsdk.Deps, args json.RawMessage) (json.RawMessage, error) {
				v := toolCtx.Value(testContextKey{})
				assert.Nil(t, v, "expected the context value to be nil")
				return nil, nil
			},
		}

		wrapped := toolsdk.WithCleanContext(ctxTool.Handler)
		ctx := context.WithValue(context.Background(), testContextKey{}, "test")
		_, _ = wrapped(ctx, toolsdk.Deps{}, []byte(`{}`))
	})

	t.Run("PropagateCancel", func(t *testing.T) {
		t.Parallel()

		// This test is to ensure that the context is canceled properly.
		callCh := make(chan struct{})
		ctxTool := toolsdk.GenericTool{
			Tool: aisdk.Tool{
				Name:        "context_tool",
				Description: "Returns the context value for testing.",
			},
			Handler: func(toolCtx context.Context, tb toolsdk.Deps, args json.RawMessage) (json.RawMessage, error) {
				defer close(callCh)
				// Wait for the context to be canceled
				<-toolCtx.Done()
				return nil, toolCtx.Err()
			},
		}
		wrapped := toolsdk.WithCleanContext(ctxTool.Handler)
		errCh := make(chan error, 1)

		tCtx := testutil.Context(t, testutil.WaitShort)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)
		go func() {
			_, err := wrapped(ctx, toolsdk.Deps{}, []byte(`{}`))
			errCh <- err
		}()

		cancel()

		// Ensure the tool is called
		select {
		case <-callCh:
		case <-tCtx.Done():
			require.Fail(t, "test timed out before handler was called")
		}

		// Ensure the correct error is returned
		select {
		case <-tCtx.Done():
			require.Fail(t, "test timed out")
		case err := <-errCh:
			// Context was canceled and the done channel was closed
			require.ErrorIs(t, err, context.Canceled)
		}
	})

	t.Run("PropagateDeadline", func(t *testing.T) {
		t.Parallel()

		// This test ensures that the context deadline is propagated to the child
		// from the parent.
		ctxTool := toolsdk.GenericTool{
			Tool: aisdk.Tool{
				Name:        "context_tool_deadline",
				Description: "Checks if context has deadline.",
			},
			Handler: func(toolCtx context.Context, tb toolsdk.Deps, args json.RawMessage) (json.RawMessage, error) {
				_, ok := toolCtx.Deadline()
				assert.True(t, ok, "expected deadline to be set on the child context")
				return nil, nil
			},
		}

		wrapped := toolsdk.WithCleanContext(ctxTool.Handler)
		parent, cancel := context.WithTimeout(context.Background(), testutil.IntervalFast)
		t.Cleanup(cancel)
		_, err := wrapped(parent, toolsdk.Deps{}, []byte(`{}`))
		require.NoError(t, err)
	})
}

func TestToolSchemaFields(t *testing.T) {
	t.Parallel()

	// Test that all tools have the required Schema fields (Properties and Required)
	for _, tool := range toolsdk.All {
		t.Run(tool.Name, func(t *testing.T) {
			t.Parallel()

			// Check that Properties is not nil
			require.NotNil(t, tool.Schema.Properties,
				"Tool %q missing Schema.Properties", tool.Name)

			// Check that Required is not nil
			require.NotNil(t, tool.Schema.Required,
				"Tool %q missing Schema.Required", tool.Name)

			// Ensure Properties has entries for all required fields
			for _, requiredField := range tool.Schema.Required {
				_, exists := tool.Schema.Properties[requiredField]
				require.True(t, exists,
					"Tool %q requires field %q but it is not defined in Properties",
					tool.Name, requiredField)
			}
		})
	}
}

// TestMain runs after all tests to ensure that all tools in this package have
// been tested once.
func TestMain(m *testing.M) {
	// Initialize testedTools
	for _, tool := range toolsdk.All {
		testedTools.Store(tool.Name, false)
	}

	code := m.Run()

	// Ensure all tools have been tested
	var untested []string
	for _, tool := range toolsdk.All {
		if tested, ok := testedTools.Load(tool.Name); !ok || !tested.(bool) {
			// Test is skipped on Windows
			if runtime.GOOS == "windows" && tool.Name == "coder_workspace_bash" {
				continue
			}
			untested = append(untested, tool.Name)
		}
	}

	if len(untested) > 0 && code == 0 {
		code = 1
		println("The following tools were not tested:")
		for _, tool := range untested {
			println(" - " + tool)
		}
		println("Please ensure that all tools are tested using testTool().")
		println("If you just added a new tool, please add a test for it.")
		println("NOTE: if you just ran an individual test, this is expected.")
	}

	// Check for goroutine leaks. Below is adapted from goleak.VerifyTestMain:
	if code == 0 {
		if err := goleak.Find(testutil.GoleakOptions...); err != nil {
			println("goleak: Errors on successful test run: ", err.Error())
			code = 1
		}
	}

	os.Exit(code)
}

func TestReportTaskNilPointerDeref(t *testing.T) {
	t.Parallel()

	// Create deps without a task reporter (simulating remote MCP server scenario)
	client, _ := coderdtest.NewWithDatabase(t, nil)
	deps, err := toolsdk.NewDeps(client)
	require.NoError(t, err)

	// Prepare test arguments
	args := toolsdk.ReportTaskArgs{
		Summary: "Test task",
		Link:    "https://example.com",
		State:   string(codersdk.WorkspaceAppStatusStateWorking),
	}

	_, err = toolsdk.ReportTask.Handler(t.Context(), deps, args)

	// We expect an error, not a panic
	require.Error(t, err)
	require.Contains(t, err.Error(), "task reporting not available")
}

func TestReportTaskWithReporter(t *testing.T) {
	t.Parallel()

	// Create deps with a task reporter
	client, _ := coderdtest.NewWithDatabase(t, nil)

	called := false
	reporter := func(args toolsdk.ReportTaskArgs) error {
		called = true
		require.Equal(t, "Test task", args.Summary)
		require.Equal(t, "https://example.com", args.Link)
		require.Equal(t, string(codersdk.WorkspaceAppStatusStateWorking), args.State)
		return nil
	}

	deps, err := toolsdk.NewDeps(client, toolsdk.WithTaskReporter(reporter))
	require.NoError(t, err)

	args := toolsdk.ReportTaskArgs{
		Summary: "Test task",
		Link:    "https://example.com",
		State:   string(codersdk.WorkspaceAppStatusStateWorking),
	}

	result, err := toolsdk.ReportTask.Handler(t.Context(), deps, args)
	require.NoError(t, err)
	require.True(t, called)

	// Verify response
	require.Equal(t, "Thanks for reporting!", result.Message)
}

func TestNormalizeWorkspaceInput(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "SimpleWorkspace",
			input:    "workspace",
			expected: "workspace",
		},
		{
			name:     "WorkspaceWithAgent",
			input:    "workspace.agent",
			expected: "workspace.agent",
		},
		{
			name:     "OwnerAndWorkspace",
			input:    "owner/workspace",
			expected: "owner/workspace",
		},
		{
			name:     "OwnerDashWorkspace",
			input:    "owner--workspace",
			expected: "owner/workspace",
		},
		{
			name:     "OwnerWorkspaceAgent",
			input:    "owner/workspace.agent",
			expected: "owner/workspace.agent",
		},
		{
			name:     "OwnerDashWorkspaceAgent",
			input:    "owner--workspace.agent",
			expected: "owner/workspace.agent",
		},
		{
			name:     "CoderConnectFormat",
			input:    "agent.workspace.owner", // Special Coder Connect reverse format
			expected: "owner/workspace.agent",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result := toolsdk.NormalizeWorkspaceInput(tc.input)
			require.Equal(t, tc.expected, result, "Input %q should normalize to %q but got %q", tc.input, tc.expected, result)
		})
	}
}
