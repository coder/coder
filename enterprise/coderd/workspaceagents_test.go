package coderd_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/agent"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/provisioner/echo"
	"github.com/coder/coder/provisionersdk/proto"
)

func TestBlockNonBrowser(t *testing.T) {
	t.Parallel()
	t.Run("Enabled", func(t *testing.T) {
		t.Parallel()
		client := coderdenttest.New(t, &coderdenttest.Options{
			BrowserOnly: true,
			Options: &coderdtest.Options{
				IncludeProvisionerDaemon: true,
			},
		})
		user := coderdtest.CreateFirstUser(t, client)
		coderdenttest.AddLicense(t, client, coderdenttest.LicenseOptions{
			BrowserOnly: true,
		})
		id := setupWorkspaceAgent(t, client, user)
		_, err := client.DialWorkspaceAgentTailnet(context.Background(), slog.Logger{}, id)
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusConflict, apiErr.StatusCode())
	})
	t.Run("Disabled", func(t *testing.T) {
		t.Parallel()
		client := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				IncludeProvisionerDaemon: true,
			},
		})
		user := coderdtest.CreateFirstUser(t, client)
		coderdenttest.AddLicense(t, client, coderdenttest.LicenseOptions{
			BrowserOnly: false,
		})
		id := setupWorkspaceAgent(t, client, user)
		conn, err := client.DialWorkspaceAgentTailnet(context.Background(), slog.Logger{}, id)
		require.NoError(t, err)
		_ = conn.Close()
	})
}

func setupWorkspaceAgent(t *testing.T, client *codersdk.Client, user codersdk.CreateFirstUserResponse) uuid.UUID {
	authToken := uuid.NewString()
	version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
		Parse: echo.ParseComplete,
		Provision: []*proto.Provision_Response{{
			Type: &proto.Provision_Response_Complete{
				Complete: &proto.Provision_Complete{
					Resources: []*proto.Resource{{
						Name: "example",
						Type: "aws_instance",
						Agents: []*proto.Agent{{
							Id:   uuid.NewString(),
							Name: "example",
							Auth: &proto.Agent_Token{
								Token: authToken,
							},
						}},
					}},
				},
			},
		}},
	})
	coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
	template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
	workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
	coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)
	agentClient := codersdk.New(client.URL)
	agentClient.SessionToken = authToken
	agentCloser := agent.New(agent.Options{
		FetchMetadata:     agentClient.WorkspaceAgentMetadata,
		CoordinatorDialer: agentClient.ListenWorkspaceAgentTailnet,
		Logger:            slogtest.Make(t, nil).Named("agent"),
	})
	defer func() {
		_ = agentCloser.Close()
	}()
	resources := coderdtest.AwaitWorkspaceAgents(t, client, workspace.ID)
	return resources[0].Agents[0].ID
}
