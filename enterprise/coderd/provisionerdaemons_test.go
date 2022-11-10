package coderd_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/enterprise/coderd/coderdenttest"
)

func TestProvisionerDaemonServe(t *testing.T) {
	t.Parallel()
	t.Run("Serve", func(t *testing.T) {
		t.Parallel()
		client := coderdenttest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		srv, err := client.ServeProvisionerDaemon(context.Background(), user.OrganizationID, []codersdk.ProvisionerType{
			codersdk.ProvisionerTypeEcho,
		}, map[string]string{})
		require.NoError(t, err)
		srv.DRPCConn().Close()
	})
}

func TestPostProvisionerDaemon(t *testing.T) {
	t.Parallel()
}
