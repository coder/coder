package coderd_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/x/workspacedebug"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/provisioner/echo"
	"github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/testutil"
)

func TestWorkspaceDebugChat(t *testing.T) {
	t.Parallel()

	t.Run("GetOrCreate", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t, func(opts *coderdtest.Options) {
			opts.IncludeProvisionerDaemon = true
		})
		owner := coderdtest.CreateFirstUser(t, client.Client)
		_ = createChatModel(t, client)

		version := coderdtest.CreateTemplateVersion(t, client.Client, owner.OrganizationID, &echo.Responses{
			Parse:         echo.ParseComplete,
			ProvisionPlan: echo.PlanComplete,
			ProvisionApplyMap: map[proto.WorkspaceTransition][]*proto.Response{
				proto.WorkspaceTransition_START: echo.ApplyFailed,
			},
		})
		coderdtest.AwaitTemplateVersionJobCompleted(t, client.Client, version.ID)
		template := coderdtest.CreateTemplate(t, client.Client, owner.OrganizationID, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client.Client, template.ID)
		build := coderdtest.AwaitWorkspaceBuildJobCompleted(t, client.Client, workspace.LatestBuild.ID)
		require.Equal(t, codersdk.ProvisionerJobFailed, build.Job.Status)

		first, err := client.CreateWorkspaceDebugChat(ctx, build.ID)
		require.NoError(t, err)
		require.True(t, first.Created)
		require.Equal(t, "failed!", first.FailureSummary)
		// The chat is deliberately detached: a failed build has no agent to
		// dial, and binding it would advertise workspace tools that cannot work.
		require.Nil(t, first.Chat.WorkspaceID)
		require.Equal(t, workspacedebug.LabelKindValue, first.Chat.Labels[workspacedebug.LabelKind])
		require.Equal(t, build.ID.String(), first.Chat.Labels[workspacedebug.LabelBuildID])
		require.Equal(t, workspace.ID.String(), first.Chat.Labels[workspacedebug.LabelWorkspaceID])
		require.NotNil(t, first.Chat.LastReasoningEffort)
		require.Equal(t, "low", *first.Chat.LastReasoningEffort)

		messages, err := client.GetChatMessages(ctx, first.Chat.ID, nil)
		require.NoError(t, err)
		var sawPrompt bool
		for _, message := range messages.Messages {
			if message.Role != codersdk.ChatMessageRoleUser {
				continue
			}
			for _, part := range message.Content {
				if part.Type == codersdk.ChatMessagePartTypeText {
					require.Contains(t, part.Text, `failed to startup with "failed!" error`)
					sawPrompt = true
				}
			}
		}
		require.True(t, sawPrompt, "expected the seeded failure prompt as the first user message")

		second, err := client.CreateWorkspaceDebugChat(ctx, build.ID)
		require.NoError(t, err)
		require.False(t, second.Created)
		require.Equal(t, first.Chat.ID, second.Chat.ID)
		require.Equal(t, first.FailureSummary, second.FailureSummary)
	})

	t.Run("OtherUserCannotOpen", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t, func(opts *coderdtest.Options) {
			opts.IncludeProvisionerDaemon = true
		})
		owner := coderdtest.CreateFirstUser(t, client.Client)
		_ = createChatModel(t, client)

		version := coderdtest.CreateTemplateVersion(t, client.Client, owner.OrganizationID, &echo.Responses{
			Parse:          echo.ParseComplete,
			ProvisionPlan:  echo.PlanComplete,
			ProvisionApply: echo.ApplyFailed,
		})
		coderdtest.AwaitTemplateVersionJobCompleted(t, client.Client, version.ID)
		template := coderdtest.CreateTemplate(t, client.Client, owner.OrganizationID, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client.Client, template.ID)
		build := coderdtest.AwaitWorkspaceBuildJobCompleted(t, client.Client, workspace.LatestBuild.ID)

		memberClient, _ := coderdtest.CreateAnotherUser(t, client.Client, owner.OrganizationID)
		_, err := codersdk.NewExperimentalClient(memberClient).CreateWorkspaceDebugChat(ctx, build.ID)
		require.Error(t, err)
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, 404, sdkErr.StatusCode(), "members cannot see another user's workspace build")
	})
}
