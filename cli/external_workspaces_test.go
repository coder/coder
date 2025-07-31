package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/provisioner/echo"
	"github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/pty/ptytest"
	"github.com/coder/coder/v2/testutil"
)

// completeWithExternalAgent creates a template version with an external agent resource
func completeWithExternalAgent() *echo.Responses {
	return &echo.Responses{
		Parse: echo.ParseComplete,
		ProvisionPlan: []*proto.Response{
			{
				Type: &proto.Response_Plan{
					Plan: &proto.PlanComplete{
						Resources: []*proto.Resource{
							{
								Type: "coder_external_agent",
								Name: "main",
								Agents: []*proto.Agent{
									{
										Name:            "external-agent",
										OperatingSystem: "linux",
										Architecture:    "amd64",
									},
								},
							},
						},
					},
				},
			},
		},
		ProvisionApply: []*proto.Response{
			{
				Type: &proto.Response_Apply{
					Apply: &proto.ApplyComplete{
						Resources: []*proto.Resource{
							{
								Type: "coder_external_agent",
								Name: "main",
								Agents: []*proto.Agent{
									{
										Name:            "external-agent",
										OperatingSystem: "linux",
										Architecture:    "amd64",
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

// completeWithRegularAgent creates a template version with a regular agent (no external agent)
func completeWithRegularAgent() *echo.Responses {
	return &echo.Responses{
		Parse: echo.ParseComplete,
		ProvisionPlan: []*proto.Response{
			{
				Type: &proto.Response_Plan{
					Plan: &proto.PlanComplete{
						Resources: []*proto.Resource{
							{
								Type: "compute",
								Name: "main",
								Agents: []*proto.Agent{
									{
										Name:            "regular-agent",
										OperatingSystem: "linux",
										Architecture:    "amd64",
									},
								},
							},
						},
					},
				},
			},
		},
		ProvisionApply: []*proto.Response{
			{
				Type: &proto.Response_Apply{
					Apply: &proto.ApplyComplete{
						Resources: []*proto.Resource{
							{
								Type: "compute",
								Name: "main",
								Agents: []*proto.Agent{
									{
										Name:            "regular-agent",
										OperatingSystem: "linux",
										Architecture:    "amd64",
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

func TestExternalWorkspaces(t *testing.T) {
	t.Parallel()

	t.Run("Create", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		owner := coderdtest.CreateFirstUser(t, client)
		member, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
		version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, completeWithExternalAgent())
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID)

		args := []string{
			"external-workspaces",
			"create",
			"my-external-workspace",
			"--template", template.Name,
		}
		inv, root := clitest.New(t, args...)
		clitest.SetupConfig(t, member, root)
		doneChan := make(chan struct{})
		pty := ptytest.New(t).Attach(inv)
		go func() {
			defer close(doneChan)
			err := inv.Run()
			assert.NoError(t, err)
		}()

		// Expect the workspace creation confirmation
		pty.ExpectMatch("coder_external_agent.main")
		pty.ExpectMatch("external-agent (linux, amd64)")
		pty.ExpectMatch("Confirm create")
		pty.WriteLine("yes")

		// Expect the external agent instructions
		pty.ExpectMatch("Please run the following commands to attach external agent")
		pty.ExpectMatch("export CODER_AGENT_TOKEN=")
		pty.ExpectMatch("curl -fsSL")

		<-doneChan

		// Verify the workspace was created
		ws, err := member.WorkspaceByOwnerAndName(context.Background(), codersdk.Me, "my-external-workspace", codersdk.WorkspaceOptions{})
		require.NoError(t, err)
		assert.Equal(t, template.Name, ws.TemplateName)
	})

	t.Run("CreateWithoutTemplate", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		owner := coderdtest.CreateFirstUser(t, client)
		member, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)

		args := []string{
			"external-workspaces",
			"create",
			"my-external-workspace",
		}
		inv, root := clitest.New(t, args...)
		clitest.SetupConfig(t, member, root)

		err := inv.Run()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "template name is required for external workspace creation")
	})

	t.Run("CreateWithRegularTemplate", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		owner := coderdtest.CreateFirstUser(t, client)
		member, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
		version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, completeWithRegularAgent())
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID)

		args := []string{
			"external-workspaces",
			"create",
			"my-external-workspace",
			"--template", template.Name,
		}
		inv, root := clitest.New(t, args...)
		clitest.SetupConfig(t, member, root)

		err := inv.Run()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "does not have an external agent")
	})

	t.Run("List", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		owner := coderdtest.CreateFirstUser(t, client)
		member, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
		version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, completeWithExternalAgent())
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID)

		// Create an external workspace
		ws := coderdtest.CreateWorkspace(t, member, template.ID)
		coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, ws.LatestBuild.ID)

		args := []string{
			"external-workspaces",
			"list",
		}
		inv, root := clitest.New(t, args...)
		clitest.SetupConfig(t, member, root)
		pty := ptytest.New(t).Attach(inv)

		ctx, cancelFunc := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancelFunc()
		done := make(chan any)
		go func() {
			errC := inv.WithContext(ctx).Run()
			assert.NoError(t, errC)
			close(done)
		}()
		pty.ExpectMatch(ws.Name)
		pty.ExpectMatch(template.Name)
		cancelFunc()
		<-done
	})

	t.Run("ListJSON", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		owner := coderdtest.CreateFirstUser(t, client)
		member, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
		version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, completeWithExternalAgent())
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID)

		// Create an external workspace
		ws := coderdtest.CreateWorkspace(t, member, template.ID)
		coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, ws.LatestBuild.ID)

		args := []string{
			"external-workspaces",
			"list",
			"--output=json",
		}
		inv, root := clitest.New(t, args...)
		clitest.SetupConfig(t, member, root)

		ctx, cancelFunc := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancelFunc()

		out := bytes.NewBuffer(nil)
		inv.Stdout = out
		err := inv.WithContext(ctx).Run()
		require.NoError(t, err)

		var workspaces []codersdk.Workspace
		require.NoError(t, json.Unmarshal(out.Bytes(), &workspaces))
		require.Len(t, workspaces, 1)
		assert.Equal(t, ws.Name, workspaces[0].Name)
	})

	t.Run("ListNoWorkspaces", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		owner := coderdtest.CreateFirstUser(t, client)
		member, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)

		args := []string{
			"external-workspaces",
			"list",
		}
		inv, root := clitest.New(t, args...)
		clitest.SetupConfig(t, member, root)
		pty := ptytest.New(t).Attach(inv)

		ctx, cancelFunc := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancelFunc()
		done := make(chan any)
		go func() {
			errC := inv.WithContext(ctx).Run()
			assert.NoError(t, errC)
			close(done)
		}()
		pty.ExpectMatch("No workspaces found!")
		pty.ExpectMatch("coder external-workspaces create")
		cancelFunc()
		<-done
	})

	t.Run("AgentInstructions", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		owner := coderdtest.CreateFirstUser(t, client)
		member, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
		version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, completeWithExternalAgent())
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID)

		// Create an external workspace
		ws := coderdtest.CreateWorkspace(t, member, template.ID)
		coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, ws.LatestBuild.ID)

		args := []string{
			"external-workspaces",
			"agent-instructions",
			ws.Name,
			"external-agent",
		}
		inv, root := clitest.New(t, args...)
		clitest.SetupConfig(t, member, root)
		pty := ptytest.New(t).Attach(inv)

		ctx, cancelFunc := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancelFunc()
		done := make(chan any)
		go func() {
			errC := inv.WithContext(ctx).Run()
			assert.NoError(t, errC)
			close(done)
		}()
		pty.ExpectMatch("Please run the following commands to attach agent external-agent:")
		pty.ExpectMatch("export CODER_AGENT_TOKEN=")
		pty.ExpectMatch("curl -fsSL")
		cancelFunc()
		<-done
	})

	t.Run("AgentInstructionsJSON", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		owner := coderdtest.CreateFirstUser(t, client)
		member, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
		version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, completeWithExternalAgent())
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID)

		// Create an external workspace
		ws := coderdtest.CreateWorkspace(t, member, template.ID)
		coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, ws.LatestBuild.ID)

		args := []string{
			"external-workspaces",
			"agent-instructions",
			ws.Name,
			"external-agent",
			"--output=json",
		}
		inv, root := clitest.New(t, args...)
		clitest.SetupConfig(t, member, root)

		ctx, cancelFunc := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancelFunc()

		out := bytes.NewBuffer(nil)
		inv.Stdout = out
		err := inv.WithContext(ctx).Run()
		require.NoError(t, err)

		var agentInfo map[string]interface{}
		require.NoError(t, json.Unmarshal(out.Bytes(), &agentInfo))
		assert.Equal(t, "token", agentInfo["auth_type"])
		assert.NotEmpty(t, agentInfo["auth_token"])
		assert.Contains(t, agentInfo["init_script"], "/api/v2/init-script")
	})

	t.Run("AgentInstructionsNonExistentWorkspace", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		owner := coderdtest.CreateFirstUser(t, client)
		member, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)

		args := []string{
			"external-workspaces",
			"agent-instructions",
			"non-existent-workspace",
			"external-agent",
		}
		inv, root := clitest.New(t, args...)
		clitest.SetupConfig(t, member, root)

		err := inv.Run()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "get workspace by name")
	})

	t.Run("AgentInstructionsNonExistentAgent", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		owner := coderdtest.CreateFirstUser(t, client)
		member, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
		version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, completeWithExternalAgent())
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID)

		// Create an external workspace
		ws := coderdtest.CreateWorkspace(t, member, template.ID)
		coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, ws.LatestBuild.ID)

		args := []string{
			"external-workspaces",
			"agent-instructions",
			ws.Name,
			"non-existent-agent",
		}
		inv, root := clitest.New(t, args...)
		clitest.SetupConfig(t, member, root)

		err := inv.Run()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "get external agent token for agent")
	})

	t.Run("CreateWithTemplateVersion", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		owner := coderdtest.CreateFirstUser(t, client)
		member, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
		version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, completeWithExternalAgent())
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID)

		args := []string{
			"external-workspaces",
			"create",
			"my-external-workspace",
			"--template", template.Name,
			"--template-version", version.Name,
			"-y",
		}
		inv, root := clitest.New(t, args...)
		clitest.SetupConfig(t, member, root)
		doneChan := make(chan struct{})
		pty := ptytest.New(t).Attach(inv)
		go func() {
			defer close(doneChan)
			err := inv.Run()
			assert.NoError(t, err)
		}()

		// Expect the workspace creation confirmation
		pty.ExpectMatch("coder_external_agent.main")
		pty.ExpectMatch("external-agent (linux, amd64)")

		// Expect the external agent instructions
		pty.ExpectMatch("Please run the following commands to attach external agent")
		pty.ExpectMatch("export CODER_AGENT_TOKEN=")
		pty.ExpectMatch("curl -fsSL")

		<-doneChan

		// Verify the workspace was created
		ws, err := member.WorkspaceByOwnerAndName(context.Background(), codersdk.Me, "my-external-workspace", codersdk.WorkspaceOptions{})
		require.NoError(t, err)
		assert.Equal(t, template.Name, ws.TemplateName)
	})
}
