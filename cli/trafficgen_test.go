package cli_test

import (
	"bytes"
	"context"
	"github.com/coder/coder/agent"
	"github.com/coder/coder/cli/clitest"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/codersdk/agentsdk"
	"github.com/coder/coder/provisioner/echo"
	"github.com/coder/coder/provisionersdk/proto"
	"github.com/coder/coder/testutil"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
)

// This test pretends to stand up a workspace and run a no-op traffic generation test.
// It's not a real test, but it's useful for debugging.
// We do not perform any cleanup.
func TestTrafficGen(t *testing.T) {
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

	ws := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
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

	inv, root := clitest.New(t, "trafficgen", ws.Name,
		"--duration", "1s",
		"--bps", "100",
	)
	clitest.SetupConfig(t, client, root)
	var stdout, stderr bytes.Buffer
	inv.Stdout = &stdout
	inv.Stderr = &stderr
	err := inv.WithContext(ctx).Run()
	require.NoError(t, err)
	stdoutStr := stdout.String()
	stderrStr := stderr.String()
	require.Empty(t, stderrStr)
	lines := strings.Split(strings.TrimSpace(stdoutStr), "\n")
	require.Len(t, lines, 4)
	require.Equal(t, "Test results:", lines[0])
	require.Regexp(t, `Took:\s+\d+\.\d+s`, lines[1])
	require.Regexp(t, `Sent:\s+\d+ bytes`, lines[2])
	require.Regexp(t, `Rcvd:\s+\d+ bytes`, lines[3])
}
