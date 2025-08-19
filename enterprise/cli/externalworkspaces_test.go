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
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
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
						HasExternalAgents: true,
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
		client, owner := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				IncludeProvisionerDaemon: true,
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureWorkspaceExternalAgent: 1,
				},
			},
		})
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
		inv, root := newCLI(t, args...)
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
		pty.ExpectMatch("Please run the following command to attach external agent")
		pty.ExpectRegexMatch("curl -fsSL .* | CODER_AGENT_TOKEN=.* sh")

		ctx := testutil.Context(t, testutil.WaitLong)
		testutil.TryReceive(ctx, t, doneChan)

		// Verify the workspace was created
		ws, err := member.WorkspaceByOwnerAndName(context.Background(), codersdk.Me, "my-external-workspace", codersdk.WorkspaceOptions{})
		require.NoError(t, err)
		assert.Equal(t, template.Name, ws.TemplateName)
	})

	t.Run("CreateWithoutTemplate", func(t *testing.T) {
		t.Parallel()
		client, owner := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				IncludeProvisionerDaemon: true,
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureWorkspaceExternalAgent: 1,
				},
			},
		})
		member, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)

		args := []string{
			"external-workspaces",
			"create",
			"my-external-workspace",
		}
		inv, root := newCLI(t, args...)
		clitest.SetupConfig(t, member, root)

		err := inv.Run()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "Missing values for the required flags: template")
	})

	t.Run("CreateWithRegularTemplate", func(t *testing.T) {
		t.Parallel()
		client, owner := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				IncludeProvisionerDaemon: true,
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureWorkspaceExternalAgent: 1,
				},
			},
		})
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
		inv, root := newCLI(t, args...)
		clitest.SetupConfig(t, member, root)

		err := inv.Run()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "does not have an external agent")
	})

	t.Run("List", func(t *testing.T) {
		t.Parallel()
		client, owner := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				IncludeProvisionerDaemon: true,
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureWorkspaceExternalAgent: 1,
				},
			},
		})
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
		inv, root := newCLI(t, args...)
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
		client, owner := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				IncludeProvisionerDaemon: true,
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureWorkspaceExternalAgent: 1,
				},
			},
		})
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
		inv, root := newCLI(t, args...)
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
		client, owner := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				IncludeProvisionerDaemon: true,
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureWorkspaceExternalAgent: 1,
				},
			},
		})
		member, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)

		args := []string{
			"external-workspaces",
			"list",
		}
		inv, root := newCLI(t, args...)
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
		client, owner := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				IncludeProvisionerDaemon: true,
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureWorkspaceExternalAgent: 1,
				},
			},
		})
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
		}
		inv, root := newCLI(t, args...)
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
		pty.ExpectMatch("Please run the following command to attach external agent to the workspace")
		pty.ExpectRegexMatch("curl -fsSL .* | CODER_AGENT_TOKEN=.* sh")
		cancelFunc()

		ctx = testutil.Context(t, testutil.WaitLong)
		testutil.TryReceive(ctx, t, done)
	})

	t.Run("AgentInstructionsJSON", func(t *testing.T) {
		t.Parallel()
		client, owner := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				IncludeProvisionerDaemon: true,
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureWorkspaceExternalAgent: 1,
				},
			},
		})
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
			"--output=json",
		}
		inv, root := newCLI(t, args...)
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
		assert.NotEmpty(t, agentInfo["init_script"])
	})

	t.Run("AgentInstructionsNonExistentWorkspace", func(t *testing.T) {
		t.Parallel()
		client, owner := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				IncludeProvisionerDaemon: true,
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureWorkspaceExternalAgent: 1,
				},
			},
		})
		member, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)

		args := []string{
			"external-workspaces",
			"agent-instructions",
			"non-existent-workspace",
		}
		inv, root := newCLI(t, args...)
		clitest.SetupConfig(t, member, root)

		err := inv.Run()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "Resource not found")
	})

	t.Run("AgentInstructionsNonExistentAgent", func(t *testing.T) {
		t.Parallel()
		client, owner := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				IncludeProvisionerDaemon: true,
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureWorkspaceExternalAgent: 1,
				},
			},
		})
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
			ws.Name + ".non-existent-agent",
		}
		inv, root := newCLI(t, args...)
		clitest.SetupConfig(t, member, root)

		err := inv.Run()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "agent not found by name")
	})

	t.Run("CreateWithTemplateVersion", func(t *testing.T) {
		t.Parallel()
		client, owner := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				IncludeProvisionerDaemon: true,
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureWorkspaceExternalAgent: 1,
				},
			},
		})
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
		inv, root := newCLI(t, args...)
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
		pty.ExpectMatch("Please run the following command to attach external agent")
		pty.ExpectRegexMatch("curl -fsSL .* | CODER_AGENT_TOKEN=.* sh")

		ctx := testutil.Context(t, testutil.WaitLong)
		testutil.TryReceive(ctx, t, doneChan)

		// Verify the workspace was created
		ws, err := member.WorkspaceByOwnerAndName(context.Background(), codersdk.Me, "my-external-workspace", codersdk.WorkspaceOptions{})
		require.NoError(t, err)
		assert.Equal(t, template.Name, ws.TemplateName)
	})
}
