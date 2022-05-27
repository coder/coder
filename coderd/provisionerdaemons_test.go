package coderd_test

import (
	"context"
	"crypto/rand"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/provisionersdk"
)

func TestProvisionerDaemons(t *testing.T) {
	t.Parallel()
	t.Run("PayloadTooBig", func(t *testing.T) {
		t.Parallel()
		if runtime.GOOS == "windows" {
			// Takes too long to allocate memory on Windows!
			t.Skip()
		}
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerD: true})
		user := coderdtest.CreateFirstUser(t, client)
		data := make([]byte, provisionersdk.MaxMessageSize)
		rand.Read(data)
		resp, err := client.Upload(context.Background(), codersdk.ContentTypeTar, data)
		require.NoError(t, err)
		t.Log(resp.Hash)

		version, err := client.CreateTemplateVersion(context.Background(), user.OrganizationID, codersdk.CreateTemplateVersionRequest{
			StorageMethod: codersdk.ProvisionerStorageMethodFile,
			StorageSource: resp.Hash,
			Provisioner:   codersdk.ProvisionerTypeEcho,
		})
		require.NoError(t, err)
		require.Eventually(t, func() bool {
			var err error
			version, err = client.TemplateVersion(context.Background(), version.ID)
			require.NoError(t, err)
			return version.Job.Error != ""
		}, 5*time.Second, 25*time.Millisecond)
	})
}

func TestProvisionerDaemonsByOrganization(t *testing.T) {
	t.Parallel()
	t.Run("NoAuth", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_, err := client.ProvisionerDaemons(context.Background())
		require.Error(t, err)
	})

	t.Run("Get", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		_, err := client.ProvisionerDaemons(context.Background())
		require.NoError(t, err)
	})
}
