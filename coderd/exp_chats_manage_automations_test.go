package coderd_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

// TestChatManageAutomationsSwitch covers who may set the interim
// manage_automations switch on POST and PATCH /api/v2/chats.
func TestChatManageAutomationsSwitch(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	admin, db := newChatClientWithDatabase(t)
	first := coderdtest.CreateFirstUser(t, admin.Client)
	model := createChatModel(t, admin)
	memberRaw, member := coderdtest.CreateAnotherUser(t, admin.Client, first.OrganizationID)
	memberClient := codersdk.NewExperimentalClient(memberRaw)
	offRaw, off := coderdtest.CreateAnotherUser(t, admin.Client, first.OrganizationID)
	offClient := codersdk.NewExperimentalClient(offRaw)
	// The experiment is on for everyone except off.
	_, err := admin.PutExperimentRule(ctx, codersdk.ExperimentChatAutomations, codersdk.PutExperimentRuleRequest{
		Mode:      codersdk.ExperimentRuleModeCondition,
		Condition: fmt.Sprintf("user.username != %q", off.Username),
	})
	require.NoError(t, err)

	createRequest := func(ownerID *uuid.UUID, enabled bool) codersdk.CreateChatRequest {
		return codersdk.CreateChatRequest{
			OrganizationID:           first.OrganizationID,
			OwnerID:                  ownerID,
			Content:                  []codersdk.ChatInputPart{{Type: codersdk.ChatInputPartTypeText, Text: "hello"}},
			ManageAutomationsEnabled: enabled,
		}
	}
	switchState := func(ctx context.Context, t *testing.T, chatID uuid.UUID) bool {
		t.Helper()
		chat, err := admin.GetChat(ctx, chatID)
		require.NoError(t, err)
		return chat.ManageAutomationsEnabled
	}

	t.Run("CreateDefaultsOff", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		chat, err := memberClient.CreateChat(ctx, createRequest(nil, false))
		require.NoError(t, err)
		require.False(t, switchState(ctx, t, chat.ID))
	})

	t.Run("CreateOn", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		chat, err := memberClient.CreateChat(ctx, createRequest(nil, true))
		require.NoError(t, err)
		require.True(t, chat.ManageAutomationsEnabled)
		require.True(t, switchState(ctx, t, chat.ID))
	})

	t.Run("CreateChecksOwnerExperiment", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		// The caller has the experiment, the owner does not.
		_, err := admin.CreateChat(ctx, createRequest(&off.ID, true))
		requireSDKError(t, err, http.StatusBadRequest)
		_, err = offClient.CreateChat(ctx, createRequest(nil, true))
		requireSDKError(t, err, http.StatusBadRequest)

		// A creator acting through owner_id may set it for an owner
		// who has the experiment.
		chat, err := admin.CreateChat(ctx, createRequest(&member.ID, true))
		require.NoError(t, err)
		require.Equal(t, member.ID, chat.OwnerID)
		require.True(t, switchState(ctx, t, chat.ID))
	})

	t.Run("PatchOwnerOnly", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		chat, err := memberClient.CreateChat(ctx, createRequest(nil, false))
		require.NoError(t, err)

		// An administrator can update the chat but not this switch,
		// in either direction.
		for _, enabled := range []bool{true, false} {
			err = admin.UpdateChat(ctx, chat.ID, codersdk.UpdateChatRequest{ManageAutomationsEnabled: ptr.Ref(enabled)})
			requireSDKError(t, err, http.StatusForbidden)
		}
		require.False(t, switchState(ctx, t, chat.ID))

		require.NoError(t, memberClient.UpdateChat(ctx, chat.ID, codersdk.UpdateChatRequest{ManageAutomationsEnabled: ptr.Ref(true)}))
		require.True(t, switchState(ctx, t, chat.ID))
		require.NoError(t, memberClient.UpdateChat(ctx, chat.ID, codersdk.UpdateChatRequest{ManageAutomationsEnabled: ptr.Ref(false)}))
		require.False(t, switchState(ctx, t, chat.ID))
	})

	t.Run("PatchEnableChecksExperimentDisableAlwaysAllowed", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		chat := dbgen.Chat(t, db, database.Chat{
			OrganizationID:           first.OrganizationID,
			OwnerID:                  off.ID,
			LastModelConfigID:        model.ID,
			ManageAutomationsEnabled: true,
		})
		err := offClient.UpdateChat(ctx, chat.ID, codersdk.UpdateChatRequest{ManageAutomationsEnabled: ptr.Ref(true)})
		requireSDKError(t, err, http.StatusBadRequest)
		require.NoError(t, offClient.UpdateChat(ctx, chat.ID, codersdk.UpdateChatRequest{ManageAutomationsEnabled: ptr.Ref(false)}))
		require.False(t, switchState(ctx, t, chat.ID))
	})

	t.Run("PatchEnableRejectsSubagentChat", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		parent, err := memberClient.CreateChat(ctx, createRequest(nil, false))
		require.NoError(t, err)
		child := dbgen.Chat(t, db, database.Chat{
			OrganizationID:    first.OrganizationID,
			OwnerID:           member.ID,
			LastModelConfigID: parent.LastModelConfigID,
			ParentChatID:      uuid.NullUUID{UUID: parent.ID, Valid: true},
			RootChatID:        uuid.NullUUID{UUID: parent.ID, Valid: true},
		})
		err = memberClient.UpdateChat(ctx, child.ID, codersdk.UpdateChatRequest{ManageAutomationsEnabled: ptr.Ref(true)})
		requireSDKError(t, err, http.StatusBadRequest)
		require.False(t, switchState(ctx, t, child.ID))
	})
}
