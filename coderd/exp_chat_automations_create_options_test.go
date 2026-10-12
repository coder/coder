package coderd_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestChatAutomationCreateEnabled(t *testing.T) {
	t.Parallel()
	env := newChatAutomationTestEnv(t, nil, nil)
	ctx := testutil.Context(t, testutil.WaitLong)

	// Omitted keeps today's behavior: the automation starts turned on with
	// a pending run.
	omitted, err := env.member.CreateChatAutomation(ctx, env.orgID, env.scheduleRequest())
	require.NoError(t, err)
	require.True(t, omitted.Automation.Enabled)
	require.NotNil(t, omitted.Automation.ScheduleNextRunAt)

	on := env.scheduleRequest()
	on.Enabled = ptr.Ref(true)
	created, err := env.member.CreateChatAutomation(ctx, env.orgID, on)
	require.NoError(t, err)
	require.True(t, created.Automation.Enabled)
	require.NotNil(t, created.Automation.ScheduleNextRunAt)
	require.Len(t, created.Automation.NextRunTimes, 5)

	// Off in one request: no pending run, so nothing can start before the
	// owner turns it on.
	for _, req := range []codersdk.CreateChatAutomationRequest{env.scheduleRequest(), env.webhookRequest()} {
		req.Enabled = ptr.Ref(false)
		off, err := env.member.CreateChatAutomation(ctx, env.orgID, req)
		require.NoError(t, err, req.Kind)
		require.False(t, off.Automation.Enabled, req.Kind)
		require.Nil(t, off.Automation.ScheduleNextRunAt, req.Kind)
		require.Empty(t, off.Automation.NextRunTimes, req.Kind)
		stored, err := env.db.GetChatAutomationByID(dbauthz.AsSystemRestricted(ctx), off.Automation.ID)
		require.NoError(t, err)
		require.False(t, stored.Enabled)
		require.False(t, stored.ScheduleNextRunAt.Valid)
		if req.Kind == codersdk.ChatAutomationKindWebhook {
			// The secret is still returned once, and a delivery is refused
			// while the automation is off.
			require.NotEmpty(t, off.WebhookSecret)
			status, body := postChatAutomationEvent(t, env.member, off.Automation.ID, off.WebhookSecret, []byte(`{}`))
			require.Equal(t, http.StatusForbidden, status, string(body))
		}

		// Turning it on computes the next run.
		enabled, err := env.member.UpdateChatAutomation(ctx, env.orgID, off.Automation.ID, codersdk.UpdateChatAutomationRequest{Enabled: ptr.Ref(true)})
		require.NoError(t, err)
		require.True(t, enabled.Enabled)
		if req.Kind == codersdk.ChatAutomationKindSchedule {
			require.NotNil(t, enabled.ScheduleNextRunAt)
		}
	}
}

func TestChatAutomationReasoningEffortMatchesModel(t *testing.T) {
	t.Parallel()
	env := newChatAutomationTestEnv(t, nil, nil)
	ctx := testutil.Context(t, testutil.WaitLong)

	// This model offers none, minimal, and low, the list the editor shows.
	options, err := json.Marshal(codersdk.ChatModelCallConfig{
		ReasoningEffort: &codersdk.ChatModelReasoningEffortConfig{Max: ptr.Ref("low")},
	})
	require.NoError(t, err)
	capped := dbgen.ChatModelConfig(t, env.db, database.ChatModelConfig{OrganizationID: env.orgID, Options: options})
	models, err := env.member.ChatModels(ctx, env.orgID)
	require.NoError(t, err)
	var offered []string
	for _, model := range models.Models {
		if model.ID == capped.ID {
			offered = model.ReasoningEfforts
		}
	}
	require.Equal(t, []string{"none", "minimal", "low"}, offered, "the list the editor offers")

	requireEffortRejected := func(t *testing.T, err error) {
		t.Helper()
		sdkErr := requireSDKError(t, err, http.StatusBadRequest)
		require.Len(t, sdkErr.Validations, 1)
		require.Equal(t, "reasoning_effort", sdkErr.Validations[0].Field)
		require.Contains(t, sdkErr.Validations[0].Detail, "none, minimal, low")
	}

	req := env.scheduleRequest()
	req.NewChatModelConfigID = &capped.ID
	req.ReasoningEffort = ptr.Ref("high")
	_, err = env.member.CreateChatAutomation(ctx, env.orgID, req)
	requireEffortRejected(t, err)

	req.ReasoningEffort = ptr.Ref("low")
	created, err := env.member.CreateChatAutomation(ctx, env.orgID, req)
	require.NoError(t, err)
	id := created.Automation.ID

	_, err = env.member.UpdateChatAutomation(ctx, env.orgID, id, codersdk.UpdateChatAutomationRequest{ReasoningEffort: ptr.Ref("high")})
	requireEffortRejected(t, err)

	// A model without an effort list, like the test environment's, takes
	// any effort, as the editor offers every effort for it.
	_, err = env.member.UpdateChatAutomation(ctx, env.orgID, id, codersdk.UpdateChatAutomationRequest{
		NewChatModelConfigID: &env.modelConfig.ID,
		ReasoningEffort:      ptr.Ref("max"),
	})
	require.NoError(t, err)

	// Moving back to the capped model while the stored effort is "max" is
	// refused too: the result must be a pair the model offers.
	_, err = env.member.UpdateChatAutomation(ctx, env.orgID, id, codersdk.UpdateChatAutomationRequest{NewChatModelConfigID: &capped.ID})
	requireEffortRejected(t, err)
	_, err = env.member.UpdateChatAutomation(ctx, env.orgID, id, codersdk.UpdateChatAutomationRequest{
		NewChatModelConfigID: &capped.ID,
		ReasoningEffort:      ptr.Ref(""),
	})
	require.NoError(t, err)

	// An edit that touches neither the model nor the effort is not
	// refused when an admin later lowers the model's maximum.
	_, err = env.member.UpdateChatAutomation(ctx, env.orgID, id, codersdk.UpdateChatAutomationRequest{ReasoningEffort: ptr.Ref("low")})
	require.NoError(t, err)
	lowered, err := json.Marshal(codersdk.ChatModelCallConfig{
		ReasoningEffort: &codersdk.ChatModelReasoningEffortConfig{Max: ptr.Ref("none")},
	})
	require.NoError(t, err)
	// An owner subject, as the system subject cannot update model configs.
	adminCtx := dbauthz.As(ctx, rbac.Subject{
		ID:     uuid.NewString(),
		Roles:  rbac.RoleIdentifiers{rbac.RoleOwner()},
		Groups: []string{},
		Scope:  rbac.ScopeAll,
	})
	_, err = env.db.UpdateChatModelConfig(adminCtx, database.UpdateChatModelConfigParams{
		ID: capped.ID, Model: capped.Model, DisplayName: capped.DisplayName, Enabled: capped.Enabled,
		IsDefault: capped.IsDefault, ContextLimit: capped.ContextLimit, CompressionThreshold: capped.CompressionThreshold,
		Options: lowered, AIProviderID: capped.AIProviderID,
	})
	require.NoError(t, err)
	_, err = env.member.UpdateChatAutomation(ctx, env.orgID, id, codersdk.UpdateChatAutomationRequest{Name: ptr.Ref("Renamed")})
	require.NoError(t, err)
}
