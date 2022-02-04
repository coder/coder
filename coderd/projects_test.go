package coderd_test

import (
	"archive/tar"
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/database"
)

func TestProjects(t *testing.T) {
	t.Parallel()

	t.Run("Create", func(t *testing.T) {
		t.Parallel()
		server := coderdtest.New(t)
		user := server.RandomInitialUser(t)
		_, err := server.Client.CreateProject(context.Background(), user.Organization, coderd.CreateProjectRequest{
			Name:        "someproject",
			Provisioner: database.ProvisionerTypeTerraform,
		})
		require.NoError(t, err)
	})

	t.Run("AlreadyExists", func(t *testing.T) {
		t.Parallel()
		server := coderdtest.New(t)
		user := server.RandomInitialUser(t)
		_, err := server.Client.CreateProject(context.Background(), user.Organization, coderd.CreateProjectRequest{
			Name:        "someproject",
			Provisioner: database.ProvisionerTypeTerraform,
		})
		require.NoError(t, err)
		_, err = server.Client.CreateProject(context.Background(), user.Organization, coderd.CreateProjectRequest{
			Name:        "someproject",
			Provisioner: database.ProvisionerTypeTerraform,
		})
		require.Error(t, err)
	})

	t.Run("ListEmpty", func(t *testing.T) {
		t.Parallel()
		server := coderdtest.New(t)
		_ = server.RandomInitialUser(t)
		projects, err := server.Client.Projects(context.Background(), "")
		require.NoError(t, err)
		require.Len(t, projects, 0)
	})

	t.Run("List", func(t *testing.T) {
		t.Parallel()
		server := coderdtest.New(t)
		user := server.RandomInitialUser(t)
		_, err := server.Client.CreateProject(context.Background(), user.Organization, coderd.CreateProjectRequest{
			Name:        "someproject",
			Provisioner: database.ProvisionerTypeTerraform,
		})
		require.NoError(t, err)
		// Ensure global query works.
		projects, err := server.Client.Projects(context.Background(), "")
		require.NoError(t, err)
		require.Len(t, projects, 1)

		// Ensure specified query works.
		projects, err = server.Client.Projects(context.Background(), user.Organization)
		require.NoError(t, err)
		require.Len(t, projects, 1)
	})

	t.Run("ListEmpty", func(t *testing.T) {
		t.Parallel()
		server := coderdtest.New(t)
		user := server.RandomInitialUser(t)

		projects, err := server.Client.Projects(context.Background(), user.Organization)
		require.NoError(t, err)
		require.Len(t, projects, 0)
	})

	t.Run("Single", func(t *testing.T) {
		t.Parallel()
		server := coderdtest.New(t)
		user := server.RandomInitialUser(t)
		project, err := server.Client.CreateProject(context.Background(), user.Organization, coderd.CreateProjectRequest{
			Name:        "someproject",
			Provisioner: database.ProvisionerTypeTerraform,
		})
		require.NoError(t, err)
		_, err = server.Client.Project(context.Background(), user.Organization, project.Name)
		require.NoError(t, err)
	})

	t.Run("Parameters", func(t *testing.T) {
		t.Parallel()
		server := coderdtest.New(t)
		user := server.RandomInitialUser(t)
		project, err := server.Client.CreateProject(context.Background(), user.Organization, coderd.CreateProjectRequest{
			Name:        "someproject",
			Provisioner: database.ProvisionerTypeTerraform,
		})
		require.NoError(t, err)
		_, err = server.Client.ProjectParameters(context.Background(), user.Organization, project.Name)
		require.NoError(t, err)
	})

	t.Run("CreateParameter", func(t *testing.T) {
		t.Parallel()
		server := coderdtest.New(t)
		user := server.RandomInitialUser(t)
		project, err := server.Client.CreateProject(context.Background(), user.Organization, coderd.CreateProjectRequest{
			Name:        "someproject",
			Provisioner: database.ProvisionerTypeTerraform,
		})
		require.NoError(t, err)
		_, err = server.Client.CreateProjectParameter(context.Background(), user.Organization, project.Name, coderd.CreateParameterValueRequest{
			Name:              "hi",
			SourceValue:       "tomato",
			SourceScheme:      database.ParameterSourceSchemeData,
			DestinationScheme: database.ParameterDestinationSchemeEnvironmentVariable,
			DestinationValue:  "moo",
		})
		require.NoError(t, err)
	})

	t.Run("Import", func(t *testing.T) {
		t.Parallel()
		server := coderdtest.New(t)
		user := server.RandomInitialUser(t)
		_ = server.AddProvisionerd(t)
		project, err := server.Client.CreateProject(context.Background(), user.Organization, coderd.CreateProjectRequest{
			Name:        "someproject",
			Provisioner: database.ProvisionerTypeTerraform,
		})
		require.NoError(t, err)
		var buffer bytes.Buffer
		writer := tar.NewWriter(&buffer)
		content := `variable "example" {
	default = "hi"
}`
		err = writer.WriteHeader(&tar.Header{
			Name: "main.tf",
			Size: int64(len(content)),
		})
		require.NoError(t, err)
		_, err = writer.Write([]byte(content))
		require.NoError(t, err)
		history, err := server.Client.CreateProjectHistory(context.Background(), user.Organization, project.Name, coderd.CreateProjectHistoryRequest{
			StorageMethod: database.ProjectStorageMethodInlineArchive,
			StorageSource: buffer.Bytes(),
		})
		require.NoError(t, err)
		require.Eventually(t, func() bool {
			projectHistory, err := server.Client.ProjectHistory(context.Background(), user.Organization, project.Name, history.Name)
			require.NoError(t, err)
			return projectHistory.Import.Status.Completed()
		}, 15*time.Second, 10*time.Millisecond)
		params, err := server.Client.ProjectHistoryParameters(context.Background(), user.Organization, project.Name, history.Name)
		require.NoError(t, err)
		require.Len(t, params, 1)
		require.Equal(t, "example", params[0].Name)
	})
}
