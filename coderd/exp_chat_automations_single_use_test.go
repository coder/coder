package coderd_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

// TestChatAutomationSingleUseWebhookDisablesItself checks that a
// single-use webhook turns itself off when it accepts its event, refuses
// later events as used, cannot be turned on again, and no longer counts
// toward the per-owner cap.
func TestChatAutomationSingleUseWebhookDisablesItself(t *testing.T) {
	t.Parallel()
	env := newChatAutomationTestEnv(t, nil, func(o *coderdtest.Options) {
		o.DeploymentValues.AI.Chat.MaxAutomationsPerOwner = 1
	})
	ctx := testutil.Context(t, testutil.WaitLong)
	req := env.webhookRequest()
	req.WebhookUse = ptr.Ref(codersdk.ChatAutomationWebhookUseSingle)
	created, err := env.member.CreateChatAutomation(ctx, env.orgID, req)
	require.NoError(t, err)
	id := created.Automation.ID

	// The owner is at the cap of one.
	_, err = env.member.CreateChatAutomation(ctx, env.orgID, env.scheduleRequest())
	requireSDKError(t, err, http.StatusConflict)

	status, body := postChatAutomationEvent(t, env.member, id, created.WebhookSecret, []byte(`{"n":1}`))
	require.Equal(t, http.StatusAccepted, status, string(body))
	got, err := env.member.ChatAutomation(ctx, env.orgID, id)
	require.NoError(t, err)
	require.False(t, got.Enabled, "a used single-use webhook turns itself off")
	require.NotNil(t, got.WebhookConsumedAt)

	// A later event is refused as used, not as disabled.
	status, body = postChatAutomationEvent(t, env.member, id, created.WebhookSecret, []byte(`{"n":2}`))
	require.Equal(t, http.StatusConflict, status, string(body))
	require.Contains(t, string(body), "already used")
	got, err = env.member.ChatAutomation(ctx, env.orgID, id)
	require.NoError(t, err)
	require.NotNil(t, got.LastRun)
	require.Equal(t, ptr.Ref(codersdk.ChatAutomationRunReasonSingleUseConsumed), got.LastRun.Reason)
	require.Len(t, env.userMessages(t, env.memberChat.ID), 1)

	// Turning it on again is refused with a clear message.
	_, err = env.member.UpdateChatAutomation(ctx, env.orgID, id, codersdk.UpdateChatAutomationRequest{Enabled: ptr.Ref(true)})
	sdkErr := requireSDKError(t, err, http.StatusConflict)
	require.Contains(t, sdkErr.Message, "already used")
	require.Contains(t, sdkErr.Detail, "Create a new")

	// Rotating keeps its current refusal.
	_, err = env.member.RotateChatAutomationSecret(ctx, env.orgID, id)
	requireSDKError(t, err, http.StatusConflict)

	// A used single-use webhook no longer counts toward the cap.
	_, err = env.member.CreateChatAutomation(ctx, env.orgID, env.scheduleRequest())
	require.NoError(t, err)
}
