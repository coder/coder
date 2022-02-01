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
)

func TestProjectHistory(t *testing.T) {
	t.Parallel()

	t.Run("NoHistory", func(t *testing.T) {
		t.Parallel()
		server := coderdtest.New(t)
		user := server.RandomInitialUser(t)
		project, err := server.Client.CreateProject(context.Background(), user.Organization, coderd.CreateProjectRequest{
			Name:        "someproject",
			Provisioner: database.ProvisionerTypeTerraform,
		})
		require.NoError(t, err)
		versions, err := server.Client.ProjectHistory(context.Background(), user.Organization, project.Name)
		require.NoError(t, err)
		require.Len(t, versions, 0)
	})

	t.Run("CreateHistory", func(t *testing.T) {
		t.Parallel()
		server := coderdtest.New(t)
		user := server.RandomInitialUser(t)
		project, err := server.Client.CreateProject(context.Background(), user.Organization, coderd.CreateProjectRequest{
			Name:        "someproject",
			Provisioner: database.ProvisionerTypeTerraform,
		})
		require.NoError(t, err)
		var buffer bytes.Buffer
		writer := tar.NewWriter(&buffer)
		err = writer.WriteHeader(&tar.Header{
			Name: "file",
			Size: 1 << 10,
		})
		require.NoError(t, err)
		_, err = writer.Write(make([]byte, 1<<10))
		require.NoError(t, err)
		_, err = server.Client.CreateProjectHistory(context.Background(), user.Organization, project.Name, coderd.CreateProjectHistoryRequest{
			StorageMethod: database.ProjectStorageMethodInlineArchive,
			StorageSource: buffer.Bytes(),
		})
		require.NoError(t, err)
		versions, err := server.Client.ProjectHistory(context.Background(), user.Organization, project.Name)
		require.NoError(t, err)
		require.Len(t, versions, 1)
	})

	t.Run("CreateHistoryArchiveTooBig", func(t *testing.T) {
		t.Parallel()
		server := coderdtest.New(t)
		user := server.RandomInitialUser(t)
		project, err := server.Client.CreateProject(context.Background(), user.Organization, coderd.CreateProjectRequest{
			Name:        "someproject",
			Provisioner: database.ProvisionerTypeTerraform,
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
		_, err = server.Client.CreateProjectHistory(context.Background(), user.Organization, project.Name, coderd.CreateProjectHistoryRequest{
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
			Provisioner: database.ProvisionerTypeTerraform,
		})
		require.NoError(t, err)
		_, err = server.Client.CreateProjectHistory(context.Background(), user.Organization, project.Name, coderd.CreateProjectHistoryRequest{
			StorageMethod: database.ProjectStorageMethodInlineArchive,
			StorageSource: []byte{},
		})
		require.Error(t, err)
	})
}
