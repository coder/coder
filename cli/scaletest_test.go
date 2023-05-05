package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/agent"
	"github.com/coder/coder/cli/clitest"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/codersdk/agentsdk"
	"github.com/coder/coder/provisioner/echo"
	"github.com/coder/coder/provisionersdk/proto"
	"github.com/coder/coder/pty/ptytest"
	"github.com/coder/coder/scaletest/harness"
	"github.com/coder/coder/testutil"
)

func TestScaleTestCreateWorkspaces(t *testing.T) {
	t.Skipf("This test is flakey. See https://github.com/coder/coder/issues/4942")
	t.Parallel()

	// This test does a create-workspaces scale test with --no-cleanup, checks
	// that the created resources are OK, and then runs a cleanup.
	t.Run("WorkspaceBuildNoCleanup", func(t *testing.T) {
		t.Parallel()

		ctx, cancelFunc := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancelFunc()

		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		user := coderdtest.CreateFirstUser(t, client)

		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)

		// Write a parameters file.
		tDir := t.TempDir()
		paramsFile := filepath.Join(tDir, "params.yaml")
		outputFile := filepath.Join(tDir, "output.json")

		f, err := os.Create(paramsFile)
		require.NoError(t, err)
		defer f.Close()
		_, err = f.WriteString(`---
param1: foo
param2: true
param3: 1
`)
		require.NoError(t, err)
		err = f.Close()
		require.NoError(t, err)

		inv, root := clitest.New(t, "scaletest", "create-workspaces",
			"--count", "2",
			"--template", template.Name,
			"--parameters-file", paramsFile,
			"--parameter", "param1=bar",
			"--parameter", "param4=baz",
			"--no-cleanup",
			// This flag is important for tests because agents will never be
			// started.
			"--no-wait-for-agents",
			// Run and connect flags cannot be tested because they require an
			// agent.
			"--concurrency", "2",
			"--timeout", "30s",
			"--job-timeout", "15s",
			"--cleanup-concurrency", "1",
			"--cleanup-timeout", "30s",
			"--cleanup-job-timeout", "15s",
			"--output", "text",
			"--output", "json:"+outputFile,
		)
		clitest.SetupConfig(t, client, root)
		pty := ptytest.New(t)
		inv.Stdout = pty.Output()
		inv.Stderr = pty.Output()

		done := make(chan any)
		go func() {
			err := inv.WithContext(ctx).Run()
			assert.NoError(t, err)
			close(done)
		}()
		pty.ExpectMatch("Test results:")
		pty.ExpectMatch("Pass:  2")
		select {
		case <-done:
		case <-ctx.Done():
		}
		cancelFunc()
		<-done

		// Recreate the context.
		ctx, cancelFunc = context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancelFunc()

		// Verify the output file.
		f, err = os.Open(outputFile)
		require.NoError(t, err)
		defer f.Close()
		var res harness.Results
		err = json.NewDecoder(f).Decode(&res)
		require.NoError(t, err)

		require.EqualValues(t, 2, res.TotalRuns)
		require.EqualValues(t, 2, res.TotalPass)

		// Find the workspaces and users and check that they are what we expect.
		workspaces, err := client.Workspaces(ctx, codersdk.WorkspaceFilter{
			Offset: 0,
			Limit:  100,
		})
		require.NoError(t, err)
		require.Len(t, workspaces.Workspaces, 2)

		seenUsers := map[string]struct{}{}
		for _, w := range workspaces.Workspaces {
			// Sadly we can't verify params as the API doesn't seem to return
			// them.

			// Verify that the user is a unique scaletest user.
			u, err := client.User(ctx, w.OwnerID.String())
			require.NoError(t, err)

			_, ok := seenUsers[u.ID.String()]
			require.False(t, ok, "user has more than one workspace")
			seenUsers[u.ID.String()] = struct{}{}

			require.Contains(t, u.Username, "scaletest-")
			require.Contains(t, u.Email, "scaletest")
		}

		require.Len(t, seenUsers, len(workspaces.Workspaces))

		// Check that there are exactly 3 users.
		users, err := client.Users(ctx, codersdk.UsersRequest{
			Pagination: codersdk.Pagination{
				Offset: 0,
				Limit:  100,
			},
		})
		require.NoError(t, err)
		require.Len(t, users.Users, len(seenUsers)+1)

		// Cleanup.
		inv, root = clitest.New(t, "scaletest", "cleanup",
			"--cleanup-concurrency", "1",
			"--cleanup-timeout", "30s",
			"--cleanup-job-timeout", "15s",
		)
		clitest.SetupConfig(t, client, root)
		pty = ptytest.New(t)
		inv.Stdout = pty.Output()
		inv.Stderr = pty.Output()

		done = make(chan any)
		go func() {
			err := inv.WithContext(ctx).Run()
			assert.NoError(t, err)
			close(done)
		}()
		pty.ExpectMatch("Test results:")
		pty.ExpectMatch("Pass:  2")
		pty.ExpectMatch("Test results:")
		pty.ExpectMatch("Pass:  2")
		select {
		case <-done:
		case <-ctx.Done():
		}
		cancelFunc()
		<-done

		// Recreate the context (again).
		ctx, cancelFunc = context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancelFunc()

		// Verify that the workspaces are gone.
		workspaces, err = client.Workspaces(ctx, codersdk.WorkspaceFilter{
			Offset: 0,
			Limit:  100,
		})
		require.NoError(t, err)
		require.Len(t, workspaces.Workspaces, 0)

		// Verify that the users are gone.
		users, err = client.Users(ctx, codersdk.UsersRequest{
			Pagination: codersdk.Pagination{
				Offset: 0,
				Limit:  100,
			},
		})
		require.NoError(t, err)
		require.Len(t, users.Users, 1)
	})
}

// This test pretends to stand up a workspace and run a no-op traffic generation test.
// It's not a real test, but it's useful for debugging.
// We do not perform any cleanup.
func TestScaleTestWorkspaceTraffic(t *testing.T) {
	t.Parallel()

	ctx, cancelFunc := context.WithTimeout(context.Background(), testutil.WaitMedium)
	defer cancelFunc()

	client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
	user := coderdtest.CreateFirstUser(t, client)

	authToken := uuid.NewString()
	version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
		Parse:         echo.ParseComplete,
		ProvisionPlan: echo.ProvisionComplete,
		ProvisionApply: []*proto.Provision_Response{{
			Type: &proto.Provision_Response_Complete{
				Complete: &proto.Provision_Complete{
					Resources: []*proto.Resource{{
						Name: "example",
						Type: "aws_instance",
						Agents: []*proto.Agent{{
							Id:   uuid.NewString(),
							Name: "agent",
							Auth: &proto.Agent_Token{
								Token: authToken,
							},
							Apps: []*proto.App{},
						}},
					}},
				},
			},
		}},
	})
	template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
	coderdtest.AwaitTemplateVersionJob(t, client, version.ID)

	ws := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID, func(cwr *codersdk.CreateWorkspaceRequest) {
		cwr.Name = "scaletest-test"
	})
	coderdtest.AwaitWorkspaceBuildJob(t, client, ws.LatestBuild.ID)

	agentClient := agentsdk.New(client.URL)
	agentClient.SetSessionToken(authToken)
	agentCloser := agent.New(agent.Options{
		Client: agentClient,
	})
	t.Cleanup(func() {
		_ = agentCloser.Close()
	})

	coderdtest.AwaitWorkspaceAgents(t, client, ws.ID)

	inv, root := clitest.New(t, "scaletest", "workspace-traffic",
		"--timeout", "1s",
		"--bytes-per-tick", "1024",
		"--tick-interval", "100ms",
	)
	clitest.SetupConfig(t, client, root)
	var stdout, stderr bytes.Buffer
	inv.Stdout = &stdout
	inv.Stderr = &stderr
	err := inv.WithContext(ctx).Run()
	require.NoError(t, err)
	require.Contains(t, stdout.String(), "Pass:  1")
}
