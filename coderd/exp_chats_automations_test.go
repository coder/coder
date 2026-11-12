package coderd_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

// TestChatManageAutomationsSwitch covers the chat switch of the
// manage_automations tool: its starting value, who can change it, and what the
// experiment changes.
func TestChatManageAutomationsSwitch(t *testing.T) {
	t.Parallel()

	newClient := func(t *testing.T, experiment bool) (*codersdk.ExperimentalClient, database.Store, codersdk.CreateFirstUserResponse) {
		t.Helper()
		client, db := newChatClientWithDatabase(t, func(o *coderdtest.Options) {
			if experiment {
				o.DeploymentValues.Experiments = append(o.DeploymentValues.Experiments, string(codersdk.ExperimentChatAutomations))
			}
		})
		first := coderdtest.CreateFirstUser(t, client.Client)
		_ = createChatModel(t, client)
		return client, db, first
	}
	getChat := func(ctx context.Context, t *testing.T, client *codersdk.ExperimentalClient, id uuid.UUID) codersdk.Chat {
		t.Helper()
		chat, err := client.GetChat(ctx, id)
		require.NoError(t, err)
		return chat
	}
	// hasField reports whether the raw response of a chat holds the switch. The
	// field is left out while the switch is off.
	hasField := func(ctx context.Context, t *testing.T, client *codersdk.ExperimentalClient, id uuid.UUID) bool {
		t.Helper()
		res, err := client.Request(ctx, http.MethodGet, "/api/v2/chats/"+id.String(), nil)
		require.NoError(t, err)
		defer res.Body.Close()
		require.Equal(t, http.StatusOK, res.StatusCode)
		var fields map[string]json.RawMessage
		require.NoError(t, json.NewDecoder(res.Body).Decode(&fields))
		_, ok := fields["manage_automations_enabled"]
		return ok
	}
	create := func(t *testing.T, client *codersdk.ExperimentalClient, first codersdk.CreateFirstUserResponse) codersdk.Chat {
		t.Helper()
		chat, err := client.CreateChat(testutil.Context(t, testutil.WaitLong), codersdk.CreateChatRequest{
			OrganizationID: first.OrganizationID,
			Content:        []codersdk.ChatInputPart{{Type: codersdk.ChatInputPartTypeText, Text: "hello"}},
		})
		require.NoError(t, err)
		return chat
	}

	t.Run("OwnerExperimentOn", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		client, _, first := newClient(t, true)
		chat := create(t, client, first)
		require.True(t, chat.ManageAutomationsEnabled)

		for _, enabled := range []bool{false, true} {
			require.NoError(t, client.UpdateChat(ctx, chat.ID, codersdk.UpdateChatRequest{ManageAutomationsEnabled: ptr.Ref(enabled)}))
			require.Equal(t, enabled, getChat(ctx, t, client, chat.ID).ManageAutomationsEnabled)
			require.Equal(t, enabled, hasField(ctx, t, client, chat.ID))
		}
	})

	t.Run("AdministratorForbidden", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		client, db, first := newClient(t, true)
		_, member := coderdtest.CreateAnotherUser(t, client.Client, first.OrganizationID)
		chat := dbgen.Chat(t, db, database.Chat{
			OrganizationID:           first.OrganizationID,
			OwnerID:                  member.ID,
			Title:                    "member chat",
			LastModelConfigID:        createChatModel(t, client).ID,
			ManageAutomationsEnabled: true,
		})

		// The deployment owner can update every chat, and still cannot
		// change the switch of another user's chat. The whole request fails:
		// the title of the same request is not written either.
		err := client.UpdateChat(ctx, chat.ID, codersdk.UpdateChatRequest{
			Title:                    ptr.Ref("renamed by the deployment owner"),
			ManageAutomationsEnabled: ptr.Ref(false),
		})

		sdkErr := requireSDKError(t, err, http.StatusForbidden)
		require.Equal(t, "Only the chat owner can change its automations switch.", sdkErr.Message)
		got := getChat(ctx, t, client, chat.ID)
		require.True(t, got.ManageAutomationsEnabled)
		require.Equal(t, "member chat", got.Title)
	})

	t.Run("OwnerExperimentOff", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		client, db, first := newClient(t, false)
		chat := create(t, client, first)
		require.False(t, chat.ManageAutomationsEnabled)

		// Turning it on is refused. Turning it off works, so that a stale
		// switch can be cleared.
		err := client.UpdateChat(ctx, chat.ID, codersdk.UpdateChatRequest{ManageAutomationsEnabled: ptr.Ref(true)})
		sdkErr := requireSDKError(t, err, http.StatusBadRequest)
		require.Equal(t, "Chat automations are not enabled for you.", sdkErr.Message)
		require.False(t, getChat(ctx, t, client, chat.ID).ManageAutomationsEnabled)

		//nolint:gocritic // Seed a switch that was on before the experiment went off.
		_, err = db.UpdateChatManageAutomationsEnabledByID(dbauthz.AsSystemRestricted(ctx), database.UpdateChatManageAutomationsEnabledByIDParams{
			ID: chat.ID, ManageAutomationsEnabled: true,
		})
		require.NoError(t, err)
		require.NoError(t, client.UpdateChat(ctx, chat.ID, codersdk.UpdateChatRequest{ManageAutomationsEnabled: ptr.Ref(false)}))
		require.False(t, getChat(ctx, t, client, chat.ID).ManageAutomationsEnabled)
	})
}
