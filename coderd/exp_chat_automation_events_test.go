package coderd_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/serpent"
)

// postChatAutomationEvent sends a webhook event without a Coder session,
// the way an external system does.
func postChatAutomationEvent(t *testing.T, client *codersdk.ExperimentalClient, automationID uuid.UUID, secret string, body []byte) (int, []byte) {
	t.Helper()
	ctx := testutil.Context(t, testutil.WaitLong)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		client.URL.JoinPath("api", "experimental", "chat-automations", automationID.String(), "events").String(),
		bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	if secret != "" {
		req.Header.Set("Authorization", "Bearer "+secret)
	}
	res, err := client.HTTPClient.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()
	resBody, err := io.ReadAll(res.Body)
	require.NoError(t, err)
	return res.StatusCode, resBody
}

func TestChatAutomationEvents(t *testing.T) {
	t.Parallel()
	event := []byte(`{"service":"api","status":"deployed"}`)

	t.Run("Accepted", func(t *testing.T) {
		t.Parallel()
		env := newChatAutomationTestEnv(t, nil, nil)
		ctx := testutil.Context(t, testutil.WaitLong)
		created, err := env.member.CreateChatAutomation(ctx, env.orgID, env.webhookRequest())
		require.NoError(t, err)

		status, body := postChatAutomationEvent(t, env.member, created.Automation.ID, created.WebhookSecret, event)
		require.Equal(t, http.StatusAccepted, status, string(body))
		var res codersdk.ChatAutomationEventResponse
		require.NoError(t, json.Unmarshal(body, &res))
		require.Equal(t, env.memberChat.ID, res.ChatID)
		require.NotEqual(t, uuid.Nil, res.InputID)

		messages, err := env.member.GetChatMessages(ctx, env.memberChat.ID, nil)
		require.NoError(t, err)
		require.Len(t, messages.Messages, 1)
		content := messages.Messages[0].Content
		require.Len(t, content, 2)
		require.Equal(t, "A deploy finished.", content[0].Text)
		require.Contains(t, content[1].Text, "untrusted event data")
		require.Contains(t, content[1].Text, string(event))
	})

	t.Run("Unauthorized", func(t *testing.T) {
		t.Parallel()
		env := newChatAutomationTestEnv(t, nil, nil)
		ctx := testutil.Context(t, testutil.WaitLong)
		created, err := env.member.CreateChatAutomation(ctx, env.orgID, env.webhookRequest())
		require.NoError(t, err)
		// A Coder session token is not a webhook secret.
		sessionToken := env.member.SessionToken()

		for _, tc := range []struct {
			name   string
			id     uuid.UUID
			secret string
		}{
			{"MissingSecret", created.Automation.ID, ""},
			{"WrongSecret", created.Automation.ID, created.WebhookSecret + "x"},
			{"SessionToken", created.Automation.ID, sessionToken},
			{"UnknownAutomation", uuid.New(), created.WebhookSecret},
		} {
			status, body := postChatAutomationEvent(t, env.member, tc.id, tc.secret, event)
			require.Equal(t, http.StatusUnauthorized, status, tc.name)
			require.Contains(t, string(body), "Invalid chat automation webhook secret.", tc.name)
		}
		// Unknown ids and wrong secrets are indistinguishable.
		_, wrong := postChatAutomationEvent(t, env.member, created.Automation.ID, created.WebhookSecret+"x", event)
		_, unknown := postChatAutomationEvent(t, env.member, uuid.New(), created.WebhookSecret, event)
		require.Equal(t, wrong, unknown)
		require.Empty(t, env.queuedMessageIDs(t, env.memberChat.ID))
	})

	t.Run("ExperimentOffForOwner", func(t *testing.T) {
		t.Parallel()
		env := newChatAutomationTestEnv(t, nil, nil)
		ctx := testutil.Context(t, testutil.WaitLong)
		created, err := env.member.CreateChatAutomation(ctx, env.orgID, env.webhookRequest())
		require.NoError(t, err)
		member, err := env.member.User(ctx, codersdk.Me)
		require.NoError(t, err)
		_, err = env.owner.PutExperimentRule(ctx, codersdk.ExperimentChatAutomations, codersdk.PutExperimentRuleRequest{
			Mode:      codersdk.ExperimentRuleModeCondition,
			Condition: fmt.Sprintf("user.username != %q", member.Username),
		})
		require.NoError(t, err)

		status, _ := postChatAutomationEvent(t, env.member, created.Automation.ID, created.WebhookSecret+"x", event)
		require.Equal(t, http.StatusUnauthorized, status, "the secret is checked before the experiment")
		status, _ = postChatAutomationEvent(t, env.member, created.Automation.ID, created.WebhookSecret, event)
		require.Equal(t, http.StatusNotFound, status)
	})

	t.Run("InvalidBody", func(t *testing.T) {
		t.Parallel()
		env := newChatAutomationTestEnv(t, nil, nil)
		ctx := testutil.Context(t, testutil.WaitLong)
		created, err := env.member.CreateChatAutomation(ctx, env.orgID, env.webhookRequest())
		require.NoError(t, err)

		status, _ := postChatAutomationEvent(t, env.member, created.Automation.ID, created.WebhookSecret, []byte("not json"))
		require.Equal(t, http.StatusBadRequest, status)
		large := []byte(`"` + strings.Repeat("a", 256*1024) + `"`)
		status, _ = postChatAutomationEvent(t, env.member, created.Automation.ID, created.WebhookSecret, large)
		require.Equal(t, http.StatusRequestEntityTooLarge, status)
	})

	t.Run("BusyChat", func(t *testing.T) {
		t.Parallel()
		// A queue of 2 leaves automations a share of 1.
		env := newChatAutomationTestEnv(t, nil, func(o *coderdtest.Options) {
			o.DeploymentValues.AI.Chat.MaxQueuedMessagesPerChat = serpent.Int64(2)
		})
		ctx := testutil.Context(t, testutil.WaitLong)
		busy := dbgen.Chat(t, env.db, database.Chat{
			OrganizationID:    env.orgID,
			OwnerID:           env.memberID,
			LastModelConfigID: env.modelConfig.ID,
			Status:            database.ChatStatusRunning,
		})
		create := func(whenBusy codersdk.ChatAutomationWhenBusy) codersdk.CreateChatAutomationResponse {
			req := env.webhookRequest()
			req.TargetChatID = &busy.ID
			req.WhenBusy = ptr.Ref(whenBusy)
			created, err := env.member.CreateChatAutomation(ctx, env.orgID, req)
			require.NoError(t, err)
			return created
		}

		skip := create(codersdk.ChatAutomationWhenBusySkip)
		status, body := postChatAutomationEvent(t, env.member, skip.Automation.ID, skip.WebhookSecret, event)
		require.Equal(t, http.StatusConflict, status, string(body))

		queue := create(codersdk.ChatAutomationWhenBusyQueue)
		status, body = postChatAutomationEvent(t, env.member, queue.Automation.ID, queue.WebhookSecret, event)
		require.Equal(t, http.StatusAccepted, status, string(body))
		status, body = postChatAutomationEvent(t, env.member, queue.Automation.ID, queue.WebhookSecret, event)
		require.Equal(t, http.StatusTooManyRequests, status, string(body))
		require.Len(t, env.queuedMessageIDs(t, busy.ID), 1)
	})

	t.Run("NewChatTarget", func(t *testing.T) {
		t.Parallel()
		env := newChatAutomationTestEnv(t, nil, nil)
		ctx := testutil.Context(t, testutil.WaitLong)
		req := env.webhookRequest()
		req.TargetMode = codersdk.ChatAutomationTargetModeNewChat
		req.TargetChatID = nil
		req.NewChatModelConfigID = &env.modelConfig.ID
		created, err := env.member.CreateChatAutomation(ctx, env.orgID, req)
		require.NoError(t, err)

		status, body := postChatAutomationEvent(t, env.member, created.Automation.ID, created.WebhookSecret, event)
		require.Equal(t, http.StatusNotImplemented, status, string(body))
	})
}
