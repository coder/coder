package cli_test

import (
	"context"
	"testing"

	"github.com/coder/coder/cli/clitest"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/codersdk"
	"github.com/stretchr/testify/require"
)

func TestProvisionerRun(t *testing.T) {
	t.Parallel()
	t.Run("Provisioner", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		provisionerResponse, err := client.CreateProvisionerDaemon(context.Background(),
			codersdk.CreateProvisionerDaemonRequest{
				Name: "foobar",
			},
		)
		require.NoError(t, err)
		token := provisionerResponse.AuthToken
		require.NotNil(t, token)

		doneCh := make(chan error)
		defer func() {
			err := <-doneCh
			require.ErrorIs(t, err, context.Canceled, "provisioner command terminated with error")
		}()

		ctx, cancelFunc := context.WithCancel(context.Background())
		defer cancelFunc()

		cmd, root := clitest.New(t, "provisioners", "run", "--token", token.String())
		// command should only have access to provisioner auth token, not user credentials
		err = root.URL().Write(client.URL.String())
		require.NoError(t, err)

		go func() {
			defer close(doneCh)
			doneCh <- cmd.ExecuteContext(ctx)
		}()
	})
}
