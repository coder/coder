package codersdk_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/provisionerd/proto"
)

func TestProvisionerDaemons(t *testing.T) {
	t.Parallel()
	t.Run("Get", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		_, err := client.ProvisionerDaemons(context.Background())
		require.NoError(t, err)
	})
}

func TestProvisionerDaemonClient(t *testing.T) {
	t.Parallel()
	t.Run("Error", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		ctx, cancelFunc := context.WithCancel(context.Background())
		daemon, err := client.ProvisionerDaemonClient(ctx)
		require.NoError(t, err)
		cancelFunc()
		_, err = daemon.AcquireJob(context.Background(), &proto.Empty{})
		require.Error(t, err)
	})

	t.Run("Connect", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		ctx, cancelFunc := context.WithCancel(context.Background())
		defer cancelFunc()
		daemon, err := client.ProvisionerDaemonClient(ctx)
		require.NoError(t, err)
		_, err = daemon.AcquireJob(ctx, &proto.Empty{})
		require.NoError(t, err)
	})
}
