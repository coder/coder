package codersdk_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/provisioner/echo"
	"github.com/coder/coder/provisionersdk/proto"
)

func TestCreateProjectImportJob(t *testing.T) {
	t.Parallel()
	t.Run("Error", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		_, err := client.CreateProjectImportJob(context.Background(), "", coderd.CreateProjectImportJobRequest{})
		require.Error(t, err)
	})

	t.Run("Create", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		user := coderdtest.CreateInitialUser(t, client)
		_ = coderdtest.CreateProjectImportJob(t, client, user.Organization, nil)
	})
}

func TestProjectImportJob(t *testing.T) {
	t.Parallel()
	t.Run("Error", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		_, err := client.ProjectImportJob(context.Background(), "", uuid.New())
		require.Error(t, err)
	})

	t.Run("Get", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		user := coderdtest.CreateInitialUser(t, client)
		job := coderdtest.CreateProjectImportJob(t, client, user.Organization, nil)
		_, err := client.ProjectImportJob(context.Background(), user.Organization, job.ID)
		require.NoError(t, err)
	})
}

func TestProjectImportJobLogsBefore(t *testing.T) {
	t.Parallel()
	t.Run("Error", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		_, err := client.ProjectImportJobLogsBefore(context.Background(), "", uuid.New(), time.Time{})
		require.Error(t, err)
	})

	t.Run("Get", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		user := coderdtest.CreateInitialUser(t, client)
		coderdtest.NewProvisionerDaemon(t, client)
		before := time.Now()
		job := coderdtest.CreateProjectImportJob(t, client, user.Organization, &echo.Responses{
			Parse: []*proto.Parse_Response{{
				Type: &proto.Parse_Response_Log{
					Log: &proto.Log{
						Output: "hello",
					},
				},
			}},
			Provision: echo.ProvisionComplete,
		})
		logs, err := client.ProjectImportJobLogsAfter(context.Background(), user.Organization, job.ID, before)
		require.NoError(t, err)
		<-logs
	})
}

func TestProjectImportJobLogsAfter(t *testing.T) {
	t.Parallel()
	t.Run("Error", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		_, err := client.ProjectImportJobLogsAfter(context.Background(), "", uuid.New(), time.Time{})
		require.Error(t, err)
	})

	t.Run("Get", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		user := coderdtest.CreateInitialUser(t, client)
		coderdtest.NewProvisionerDaemon(t, client)
		job := coderdtest.CreateProjectImportJob(t, client, user.Organization, &echo.Responses{
			Parse: []*proto.Parse_Response{{
				Type: &proto.Parse_Response_Log{
					Log: &proto.Log{
						Output: "hello",
					},
				},
			}, {
				Type: &proto.Parse_Response_Complete{
					Complete: &proto.Parse_Complete{},
				},
			}},
			Provision: echo.ProvisionComplete,
		})
		coderdtest.AwaitProjectImportJob(t, client, user.Organization, job.ID)
		logs, err := client.ProjectImportJobLogsBefore(context.Background(), user.Organization, job.ID, time.Time{})
		require.NoError(t, err)
		require.Len(t, logs, 1)
	})
}

func TestProjectImportJobSchemas(t *testing.T) {
	t.Parallel()
	t.Run("Error", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		_, err := client.ProjectImportJobSchemas(context.Background(), "", uuid.New())
		require.Error(t, err)
	})
}

func TestProjectImportJobParameters(t *testing.T) {
	t.Parallel()
	t.Run("Error", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		_, err := client.ProjectImportJobParameters(context.Background(), "", uuid.New())
		require.Error(t, err)
	})
}

func TestProjectImportJobResources(t *testing.T) {
	t.Parallel()
	t.Run("Error", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		_, err := client.ProjectImportJobResources(context.Background(), "", uuid.New())
		require.Error(t, err)
	})
}
