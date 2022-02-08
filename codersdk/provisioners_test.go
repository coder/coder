package codersdk_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/database"
	"github.com/coder/coder/provisioner/echo"
	"github.com/coder/coder/provisionerd/proto"
	sdkproto "github.com/coder/coder/provisionersdk/proto"
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
func TestProvisionerJobLogs(t *testing.T) {
	t.Parallel()
	t.Run("Error", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		_, err := client.ProvisionerJobLogs(context.Background(), "nothing", uuid.New())
		require.Error(t, err)
	})

	t.Run("List", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		user := coderdtest.CreateInitialUser(t, client)
		_ = coderdtest.NewProvisionerDaemon(t, client)
		job := coderdtest.CreateProjectImportProvisionerJob(t, client, user.Organization, nil)
		_, err := client.ProvisionerJobLogs(context.Background(), user.Organization, job.ID)
		require.NoError(t, err)
	})
}

func TestFollowProvisionerJobLogsAfter(t *testing.T) {
	t.Parallel()
	t.Run("Error", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		_, err := client.FollowProvisionerJobLogsAfter(context.Background(), "nothing", uuid.New(), time.Time{})
		require.Error(t, err)
	})

	t.Run("Stream", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		user := coderdtest.CreateInitialUser(t, client)
		_ = coderdtest.NewProvisionerDaemon(t, client)
		before := database.Now()
		job := coderdtest.CreateProjectImportProvisionerJob(t, client, user.Organization, &echo.Responses{
			Parse: []*sdkproto.Parse_Response{{
				Type: &sdkproto.Parse_Response_Log{
					Log: &sdkproto.Log{
						Output: "hello",
					},
				},
			}, {
				Type: &sdkproto.Parse_Response_Complete{
					Complete: &sdkproto.Parse_Complete{},
				},
			}},
			Provision: echo.ProvisionComplete,
		})
		logs, err := client.FollowProvisionerJobLogsAfter(context.Background(), user.Organization, job.ID, before)
		require.NoError(t, err)
		_, ok := <-logs
		require.True(t, ok)
		_, ok = <-logs
		require.False(t, ok)
	})
}
