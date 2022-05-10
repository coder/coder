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
		api := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, api.Client)
		coderdtest.NewProvisionerDaemon(t, api.Client)
		data := make([]byte, provisionersdk.MaxMessageSize)
		rand.Read(data)
		resp, err := api.Client.Upload(context.Background(), codersdk.ContentTypeTar, data)
		require.NoError(t, err)
		t.Log(resp.Hash)

		version, err := api.Client.CreateTemplateVersion(context.Background(), user.OrganizationID, codersdk.CreateTemplateVersionRequest{
			StorageMethod: database.ProvisionerStorageMethodFile,
			StorageSource: resp.Hash,
			Provisioner:   database.ProvisionerTypeEcho,
		})
		require.NoError(t, err)
		require.Eventually(t, func() bool {
			var err error
			version, err = api.Client.TemplateVersion(context.Background(), version.ID)
			require.NoError(t, err)
			return version.Job.Error != ""
		}, 5*time.Second, 25*time.Millisecond)
	})
}
