package cli_test

import (
	"fmt"
	"testing"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/provisioner/echo"
	"github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/testutil"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestTaskCreate(t *testing.T) {
	t.Parallel()

	createAITemplate := func(t *testing.T, client *codersdk.Client, user codersdk.CreateFirstUserResponse) (codersdk.TemplateVersion, codersdk.Template) {
		t.Helper()

		taskAppID := uuid.New()
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
			Parse: echo.ParseComplete,
			ProvisionPlan: []*proto.Response{
				{
					Type: &proto.Response_Plan{
						Plan: &proto.PlanComplete{
							Parameters: []*proto.RichParameter{{Name: codersdk.AITaskPromptParameterName, Type: "string"}},
							HasAiTasks: true,
							AiTasks:    []*proto.AITask{},
						},
					},
				},
			},
			ProvisionApply: []*proto.Response{
				{
					Type: &proto.Response_Apply{
						Apply: &proto.ApplyComplete{
							Resources: []*proto.Resource{{
								Name: "example",
								Type: "aws_instance",
								Agents: []*proto.Agent{{
									Id:   uuid.NewString(),
									Name: "example",
									Apps: []*proto.App{
										{
											Id:          taskAppID.String(),
											Slug:        "task-sidebar",
											DisplayName: "Task Sidebar",
										},
									},
								}},
							}},
							Parameters: []*proto.RichParameter{{Name: codersdk.AITaskPromptParameterName, Type: "string"}},
							AiTasks: []*proto.AITask{{
								SidebarApp: &proto.AITaskSidebarApp{
									Id: taskAppID.String(),
								},
							}},
						},
					},
				},
			},
		})
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)

		return version, template
	}

	t.Run("CreateWithTemplateNameAndVersion", func(t *testing.T) {
		t.Parallel()

		var (
			ctx = testutil.Context(t, testutil.WaitShort)

			prompt = "Task prompt"
		)

		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		owner := coderdtest.CreateFirstUser(t, client)
		templateVersion, template := createAITemplate(t, client, owner)

		member, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
		expMember := codersdk.NewExperimentalClient(member)

		tasks, err := expMember.Tasks(ctx, nil)
		require.NoError(t, err)
		require.Empty(t, tasks)

		args := []string{
			"exp",
			"task",
			"create",
			fmt.Sprintf("%s@%s", template.Name, templateVersion.Name),
			"--input", prompt,
		}

		inv, root := clitest.New(t, args...)
		clitest.SetupConfig(t, member, root)

		err = inv.Run()
		require.NoError(t, err)

		workspaces, err := member.Workspaces(ctx, codersdk.WorkspaceFilter{FilterQuery: "has-ai-task:true"})
		require.NoError(t, err)
		require.Len(t, workspaces.Workspaces, 1)

		coderdtest.AwaitWorkspaceBuildJobCompleted(t, member, workspaces.Workspaces[0].LatestBuild.ID)

		tasks, err = expMember.Tasks(ctx, nil)
		require.NoError(t, err)
		require.Len(t, tasks, 1)

		require.Equal(t, prompt, tasks[0].InitialPrompt)
	})
}
