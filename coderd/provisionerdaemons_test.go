package coderd_test

import (
	"context"
	"crypto/rand"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/provisionersdk"
	"github.com/coder/coder/testutil"
)

func TestProvisionerDaemons(t *testing.T) {
	t.Parallel()
	t.Run("PayloadTooBig", func(t *testing.T) {
		t.Parallel()
		if runtime.GOOS == "windows" {
			// Takes too long to allocate memory on Windows!
			t.Skip()
		}
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		user := coderdtest.CreateFirstUser(t, client)
		data := make([]byte, provisionersdk.MaxMessageSize)
		rand.Read(data)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		resp, err := client.Upload(ctx, codersdk.ContentTypeTar, data)
		require.NoError(t, err)
		t.Log(resp.ID)

		version, err := client.CreateTemplateVersion(ctx, user.OrganizationID, codersdk.CreateTemplateVersionRequest{
			StorageMethod: codersdk.ProvisionerStorageMethodFile,
			FileID:        resp.ID,
			Provisioner:   codersdk.ProvisionerTypeEcho,
		})
		require.NoError(t, err)
		require.Eventually(t, func() bool {
			var err error
			version, err = client.TemplateVersion(ctx, version.ID)
			return assert.NoError(t, err) && version.Job.Error != ""
		}, testutil.WaitShort, testutil.IntervalFast)
	})
}

func TestProvisionerDaemonsByOrganization(t *testing.T) {
	t.Parallel()
	t.Run("NoAuth", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		_, err := client.ProvisionerDaemons(ctx)
		require.Error(t, err)
	})

	t.Run("Get", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		_, err := client.ProvisionerDaemons(ctx)
		require.NoError(t, err)
	})
}
