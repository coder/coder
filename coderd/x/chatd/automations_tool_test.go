package chatd_test

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/x/chatd"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

// TestManageAutomationsToolOffering checks which root chats the model is
// offered the manage_automations tool in.
func TestManageAutomationsToolOffering(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name          string
		switchOn      bool
		planMode      bool
		experimentOff bool
		offered       bool
	}{
		{name: "SwitchOn", switchOn: true, offered: true},
		{name: "SwitchOff"},
		{name: "PlanMode", switchOn: true, planMode: true},
		{name: "ExperimentOff", switchOn: true, experimentOff: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			db, ps := dbtestutil.NewDB(t)
			ctx := testutil.Context(t, testutil.WaitLong)
			openAIURL, recordedCalls := newToolRecordingOpenAI(t)
			user, org, model := seedChatDependenciesWithProvider(t, db, "openai-compat", openAIURL)
			server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
				cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(chattest.NewMockAIBridgeTransport(t, openAIURL))
				if tc.experimentOff {
					cfg.Experiments = slices.DeleteFunc(slices.Clone(cfg.Experiments), func(experiment codersdk.Experiment) bool {
						return experiment == codersdk.ExperimentChatAutomations
					})
				}
			})

			opts := chatd.CreateOptions{
				OrganizationID:           org.ID,
				OwnerID:                  user.ID,
				Title:                    "manage-automations-offer",
				ModelConfigID:            model.ID,
				InitialUserContent:       []codersdk.ChatMessagePart{codersdk.ChatMessageText("hello")},
				ManageAutomationsEnabled: tc.switchOn,
			}
			if tc.planMode {
				opts.PlanMode = database.NullChatPlanMode{ChatPlanMode: database.ChatPlanModePlan, Valid: true}
			}
			chat, err := server.CreateChat(ctx, opts)
			require.NoError(t, err)
			waitForChatProcessed(ctx, t, db, chat.ID, server)

			calls := recordedCalls()
			require.NotEmpty(t, calls)
			if tc.offered {
				require.Contains(t, calls[0], "manage_automations")
			} else {
				require.NotContains(t, calls[0], "manage_automations")
			}
		})
	}
}
