package coderd_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/database"
	"github.com/coder/coder/provisioner/echo"
	"github.com/coder/coder/provisionersdk/proto"
)

func TestProjects(t *testing.T) {
	t.Parallel()

	t.Run("Create", func(t *testing.T) {
		t.Parallel()
		server := coderdtest.New(t)
		user := server.RandomInitialUser(t)
		_, err := server.Client.CreateProject(context.Background(), user.Organization, coderd.CreateProjectRequest{
			Name:        "someproject",
			Provisioner: database.ProvisionerTypeEcho,
		})
		require.NoError(t, err)
	})

	t.Run("AlreadyExists", func(t *testing.T) {
		t.Parallel()
		server := coderdtest.New(t)
		user := server.RandomInitialUser(t)
		_, err := server.Client.CreateProject(context.Background(), user.Organization, coderd.CreateProjectRequest{
			Name:        "someproject",
			Provisioner: database.ProvisionerTypeEcho,
		})
		require.NoError(t, err)
		_, err = server.Client.CreateProject(context.Background(), user.Organization, coderd.CreateProjectRequest{
			Name:        "someproject",
			Provisioner: database.ProvisionerTypeEcho,
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
			Provisioner: database.ProvisionerTypeEcho,
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
			Provisioner: database.ProvisionerTypeEcho,
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
			Provisioner: database.ProvisionerTypeEcho,
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
			Provisioner: database.ProvisionerTypeEcho,
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
			Provisioner: database.ProvisionerTypeEcho,
		})
		require.NoError(t, err)
		data, err := echo.Tar([]*proto.Parse_Response{{
			Type: &proto.Parse_Response_Complete{
				Complete: &proto.Parse_Complete{
					ParameterSchemas: []*proto.ParameterSchema{{
						Name: "example",
					}},
				},
			},
		}}, nil)
		require.NoError(t, err)
		version, err := server.Client.CreateProjectVersion(context.Background(), user.Organization, project.Name, coderd.CreateProjectVersionRequest{
			StorageMethod: database.ProjectStorageMethodInlineArchive,
			StorageSource: data,
		})
		require.NoError(t, err)
		require.Eventually(t, func() bool {
			projectVersion, err := server.Client.ProjectVersion(context.Background(), user.Organization, project.Name, version.Name)
			require.NoError(t, err)
			return projectVersion.Import.Status.Completed()
		}, 15*time.Second, 10*time.Millisecond)
		params, err := server.Client.ProjectVersionParameters(context.Background(), user.Organization, project.Name, version.Name)
		require.NoError(t, err)
		require.Len(t, params, 1)
		require.Equal(t, "example", params[0].Name)
	})
}
