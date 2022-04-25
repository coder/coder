package coderd_test

import (
	"context"
	"crypto/rand"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/coderd/database"
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
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		coderdtest.NewProvisionerDaemon(t, client)
		data := make([]byte, provisionersdk.MaxMessageSize)
		rand.Read(data)
		resp, err := client.Upload(context.Background(), codersdk.ContentTypeTar, data)
		require.NoError(t, err)
		t.Log(resp.Hash)

		version, err := client.CreateTemplateVersion(context.Background(), user.OrganizationID, codersdk.CreateTemplateVersionRequest{
			StorageMethod: database.ProvisionerStorageMethodFile,
			StorageSource: resp.Hash,
			Provisioner:   database.ProvisionerTypeEcho,
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
