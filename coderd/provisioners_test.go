package coderd_test

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/database"
)

func TestProvisionerd(t *testing.T) {
	t.Parallel()
	t.Run("ListDaemons", func(t *testing.T) {
		t.Parallel()
		server := coderdtest.New(t)
		_ = server.AddProvisionerd(t)
		require.Eventually(t, func() bool {
			daemons, err := server.Client.ProvisionerDaemons(context.Background())
			require.NoError(t, err)
			return len(daemons) > 0
		}, time.Second, 10*time.Millisecond)
	})

	t.Run("RunJob", func(t *testing.T) {
		t.Parallel()
		server := coderdtest.New(t)
		user := server.RandomInitialUser(t)
		_ = server.AddProvisionerd(t)

		project, err := server.Client.CreateProject(context.Background(), user.Organization, coderd.CreateProjectRequest{
			Name:        "my-project",
			Provisioner: database.ProvisionerTypeTerraform,
		})
		require.NoError(t, err)

		var buffer bytes.Buffer
		writer := tar.NewWriter(&buffer)
		content := `variable "frog" {}
	resource "null_resource" "dev" {}`
		err = writer.WriteHeader(&tar.Header{
			Name: "main.tf",
			Size: int64(len(content)),
		})
		require.NoError(t, err)
		_, err = writer.Write([]byte(content))
		require.NoError(t, err)

		projectHistory, err := server.Client.CreateProjectHistory(context.Background(), user.Organization, project.Name, coderd.CreateProjectVersionRequest{
			StorageMethod: database.ProjectStorageMethodInlineArchive,
			StorageSource: buffer.Bytes(),
		})
		require.NoError(t, err)

		workspace, err := server.Client.CreateWorkspace(context.Background(), "", coderd.CreateWorkspaceRequest{
			ProjectID: project.ID,
			Name:      "wowie",
		})
		require.NoError(t, err)

		workspaceHistory, err := server.Client.CreateWorkspaceHistory(context.Background(), "", workspace.Name, coderd.CreateWorkspaceHistoryRequest{
			ProjectHistoryID: projectHistory.ID,
			Transition:       database.WorkspaceTransitionCreate,
		})
		require.NoError(t, err)

		logs, err := server.Client.FollowWorkspaceHistoryLogs(context.Background(), "me", workspace.Name, workspaceHistory.Name)
		require.NoError(t, err)

		for {
			log := <-logs
			fmt.Printf("Got %s %s\n", log.CreatedAt, log.Output)
		}
	})
}
