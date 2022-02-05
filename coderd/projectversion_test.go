package coderd_test

import (
	"archive/tar"
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/database"
	"github.com/coder/coder/provisioner/echo"
	"github.com/coder/coder/provisionersdk/proto"
)

func TestProjectVersion(t *testing.T) {
	t.Parallel()

	t.Run("NoHistory", func(t *testing.T) {
		t.Parallel()
		server := coderdtest.New(t)
		user := server.RandomInitialUser(t)
		project, err := server.Client.CreateProject(context.Background(), user.Organization, coderd.CreateProjectRequest{
			Name:        "someproject",
			Provisioner: database.ProvisionerTypeEcho,
		})
		require.NoError(t, err)
		versions, err := server.Client.ProjectVersions(context.Background(), user.Organization, project.Name)
		require.NoError(t, err)
		require.Len(t, versions, 0)
	})

	t.Run("CreateVersion", func(t *testing.T) {
		t.Parallel()
		server := coderdtest.New(t)
		user := server.RandomInitialUser(t)
		project, err := server.Client.CreateProject(context.Background(), user.Organization, coderd.CreateProjectRequest{
			Name:        "someproject",
			Provisioner: database.ProvisionerTypeEcho,
		})
		require.NoError(t, err)
		data, err := echo.Tar([]*proto.Parse_Response{{
			Type: &proto.Parse_Response_Complete{
				Complete: &proto.Parse_Complete{},
			},
		}}, nil)
		require.NoError(t, err)
		version, err := server.Client.CreateProjectVersion(context.Background(), user.Organization, project.Name, coderd.CreateProjectVersionRequest{
			StorageMethod: database.ProjectStorageMethodInlineArchive,
			StorageSource: data,
		})
		require.NoError(t, err)
		versions, err := server.Client.ProjectVersions(context.Background(), user.Organization, project.Name)
		require.NoError(t, err)
		require.Len(t, versions, 1)

		_, err = server.Client.ProjectVersion(context.Background(), user.Organization, project.Name, version.Name)
		require.NoError(t, err)
	})

	t.Run("CreateHistoryArchiveTooBig", func(t *testing.T) {
		t.Parallel()
		server := coderdtest.New(t)
		user := server.RandomInitialUser(t)
		project, err := server.Client.CreateProject(context.Background(), user.Organization, coderd.CreateProjectRequest{
			Name:        "someproject",
			Provisioner: database.ProvisionerTypeEcho,
		})
		require.NoError(t, err)
		var buffer bytes.Buffer
		writer := tar.NewWriter(&buffer)
		err = writer.WriteHeader(&tar.Header{
			Name: "file",
			Size: 1 << 21,
		})
		require.NoError(t, err)
		_, err = writer.Write(make([]byte, 1<<21))
		require.NoError(t, err)
		_, err = server.Client.CreateProjectVersion(context.Background(), user.Organization, project.Name, coderd.CreateProjectVersionRequest{
			StorageMethod: database.ProjectStorageMethodInlineArchive,
			StorageSource: buffer.Bytes(),
		})
		require.Error(t, err)
	})

	t.Run("CreateHistoryInvalidArchive", func(t *testing.T) {
		t.Parallel()
		server := coderdtest.New(t)
		user := server.RandomInitialUser(t)
		project, err := server.Client.CreateProject(context.Background(), user.Organization, coderd.CreateProjectRequest{
			Name:        "someproject",
			Provisioner: database.ProvisionerTypeEcho,
		})
		require.NoError(t, err)
		_, err = server.Client.CreateProjectVersion(context.Background(), user.Organization, project.Name, coderd.CreateProjectVersionRequest{
			StorageMethod: database.ProjectStorageMethodInlineArchive,
			StorageSource: []byte{},
		})
		require.Error(t, err)
	})
}
