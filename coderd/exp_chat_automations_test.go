package coderd_test

import (
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd"
	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/serpent"
)

type chatAutomationTestEnv struct {
	api    *coderd.API
	db     database.Store
	owner  *codersdk.ExperimentalClient
	orgID  uuid.UUID
	member *codersdk.ExperimentalClient
	// memberID owns memberChat and can read modelConfig.
	memberID    uuid.UUID
	memberChat  database.Chat
	modelConfig database.ChatModelConfig
}

// newChatAutomationTestEnv starts coderd with the chat-automations
// experiment on (unless experiments is set) and a member who owns a root
// chat in the default organization.
func newChatAutomationTestEnv(t *testing.T, experiments []string, mutate func(*coderdtest.Options)) chatAutomationTestEnv {
	t.Helper()

	dv := coderdtest.DeploymentValues(t)
	dv.AI.BridgeConfig.Enabled = serpent.Bool(true)
	if experiments == nil {
		experiments = []string{string(codersdk.ExperimentChatAutomations)}
	}
	dv.Experiments = experiments
	opts := &coderdtest.Options{DeploymentValues: dv, ChatWorkerDisabled: true}
	if mutate != nil {
		mutate(opts)
	}
	client, _, api := coderdtest.NewWithAPI(t, opts)
	first := coderdtest.CreateFirstUser(t, client)
	memberClient, member := coderdtest.CreateAnotherUser(t, client, first.OrganizationID)

	modelConfig := dbgen.ChatModelConfig(t, api.Database, database.ChatModelConfig{OrganizationID: first.OrganizationID})
	chat := dbgen.Chat(t, api.Database, database.Chat{
		OrganizationID:    first.OrganizationID,
		OwnerID:           member.ID,
		LastModelConfigID: modelConfig.ID,
	})
	return chatAutomationTestEnv{
		api:         api,
		db:          api.Database,
		owner:       codersdk.NewExperimentalClient(client),
		orgID:       first.OrganizationID,
		member:      codersdk.NewExperimentalClient(memberClient),
		memberID:    member.ID,
		memberChat:  chat,
		modelConfig: modelConfig,
	}
}

func (e chatAutomationTestEnv) webhookRequest() codersdk.CreateChatAutomationRequest {
	return codersdk.CreateChatAutomationRequest{
		Name:         "Deploy hook",
		Kind:         codersdk.ChatAutomationKindWebhook,
		TargetMode:   codersdk.ChatAutomationTargetModeExistingChat,
		TargetChatID: &e.memberChat.ID,
		Prompt:       "A deploy finished.",
	}
}

func (e chatAutomationTestEnv) scheduleRequest() codersdk.CreateChatAutomationRequest {
	return codersdk.CreateChatAutomationRequest{
		Name:                 "Nightly report",
		Kind:                 codersdk.ChatAutomationKindSchedule,
		TargetMode:           codersdk.ChatAutomationTargetModeNewChat,
		NewChatModelConfigID: &e.modelConfig.ID,
		Prompt:               "Summarize yesterday.",
		ScheduleCron:         ptr.Ref("0 9 1 * *"),
		ScheduleTimeZone:     ptr.Ref("Europe/Berlin"),
	}
}

func rawGet(t *testing.T, client *codersdk.ExperimentalClient, path string) (int, string) {
	t.Helper()
	res, err := client.Request(testutil.Context(t, testutil.WaitShort), http.MethodGet, path, nil)
	require.NoError(t, err)
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	require.NoError(t, err)
	return res.StatusCode, string(body)
}

func TestChatAutomations(t *testing.T) {
	t.Parallel()

	t.Run("CreateAndRead", func(t *testing.T) {
		t.Parallel()
		sink := testutil.NewFakeSink(t)
		logger := sink.Logger()
		auditor := audit.NewMock()
		env := newChatAutomationTestEnv(t, nil, func(o *coderdtest.Options) {
			o.Logger = &logger
			o.Auditor = auditor
		})
		ctx := testutil.Context(t, testutil.WaitLong)

		webhook, err := env.member.CreateChatAutomation(ctx, env.orgID, env.webhookRequest())
		require.NoError(t, err)
		require.True(t, strings.HasPrefix(webhook.WebhookSecret, "coder_automation_"), "secret %q", webhook.WebhookSecret)
		require.Greater(t, len(webhook.WebhookSecret), len("coder_automation_")+40)
		require.Equal(t, env.memberID, webhook.Automation.OwnerID)
		require.True(t, webhook.Automation.Enabled)
		require.Equal(t, int64(1), webhook.Automation.WebhookSecretVersion)
		require.Equal(t, codersdk.ChatAutomationWhenBusyQueue, *webhook.Automation.WhenBusy)
		require.Equal(t, codersdk.ChatAutomationWebhookUseMulti, *webhook.Automation.WebhookUse)
		require.Empty(t, webhook.Automation.NextRunTimes)

		// Only the SHA-256 hash of the full secret is stored.
		stored, err := env.db.GetChatAutomationByID(dbauthz.AsSystemRestricted(ctx), webhook.Automation.ID)
		require.NoError(t, err)
		wantHash := sha256.Sum256([]byte(webhook.WebhookSecret))
		require.Equal(t, wantHash[:], stored.WebhookSecretHash)

		before := time.Now()
		schedule, err := env.member.CreateChatAutomation(ctx, env.orgID, env.scheduleRequest())
		require.NoError(t, err)
		require.Empty(t, schedule.WebhookSecret)
		require.Nil(t, schedule.Automation.WhenBusy)
		require.Nil(t, schedule.Automation.WebhookUse)
		require.NotNil(t, schedule.Automation.ScheduleNextRunAt)
		require.True(t, schedule.Automation.ScheduleNextRunAt.After(before))
		require.Len(t, schedule.Automation.NextRunTimes, 5)
		require.True(t, schedule.Automation.NextRunTimes[0].Equal(*schedule.Automation.ScheduleNextRunAt))
		berlin, err := time.LoadLocation("Europe/Berlin")
		require.NoError(t, err)
		for i, run := range schedule.Automation.NextRunTimes {
			local := run.In(berlin)
			require.Equal(t, 1, local.Day())
			require.Equal(t, 9, local.Hour())
			if i > 0 {
				require.True(t, run.After(schedule.Automation.NextRunTimes[i-1]))
			}
		}

		// Existing-chat schedules skip a busy chat by default.
		scheduleExisting := env.webhookRequest()
		scheduleExisting.Kind = codersdk.ChatAutomationKindSchedule
		scheduleExisting.ScheduleCron = ptr.Ref("*/5 * * * *")
		scheduleExisting.ScheduleTimeZone = ptr.Ref("UTC")
		skip, err := env.member.CreateChatAutomation(ctx, env.orgID, scheduleExisting)
		require.NoError(t, err)
		require.Equal(t, codersdk.ChatAutomationWhenBusySkip, *skip.Automation.WhenBusy)

		// The secret and its hash never leave the create response.
		basePath := fmt.Sprintf("/api/experimental/organizations/%s/chat-automations", env.orgID)
		for _, path := range []string{basePath, basePath + "/" + webhook.Automation.ID.String()} {
			status, body := rawGet(t, env.member, path)
			require.Equal(t, http.StatusOK, status, body)
			require.NotContains(t, body, webhook.WebhookSecret)
			require.NotContains(t, body, "hash")
		}
		list, err := env.member.ChatAutomations(ctx, env.orgID)
		require.NoError(t, err)
		require.Len(t, list, 3)
		require.Equal(t, skip.Automation.ID, list[0].ID, "newest first")

		creates := 0
		for _, entry := range auditor.AuditLogs() {
			if entry.ResourceType == database.ResourceTypeChatAutomation && entry.Action == database.AuditActionCreate {
				creates++
			}
		}
		require.Equal(t, 3, creates)
		for _, entry := range sink.Entries() {
			require.NotContains(t, fmt.Sprintf("%s %+v", entry.Message, entry.Fields), webhook.WebhookSecret)
		}
	})

	t.Run("PerOwnerLimitCountsAllOrganizations", func(t *testing.T) {
		t.Parallel()
		env := newChatAutomationTestEnv(t, nil, func(o *coderdtest.Options) {
			o.DeploymentValues.AI.Chat.MaxAutomationsPerOwner = 2
		})
		ctx := testutil.Context(t, testutil.WaitLong)

		// One automation the member owns in an organization they are not
		// a member of still counts.
		otherOrg := dbgen.Organization(t, env.db, database.Organization{})
		dbgen.ChatAutomation(t, env.db, database.ChatAutomation{OrganizationID: otherOrg.ID, OwnerID: env.memberID})

		_, err := env.member.CreateChatAutomation(ctx, env.orgID, env.webhookRequest())
		require.NoError(t, err)
		_, err = env.member.CreateChatAutomation(ctx, env.orgID, env.scheduleRequest())
		requireSDKError(t, err, http.StatusConflict)
	})

	t.Run("OnlyOwnerCanUpdate", func(t *testing.T) {
		t.Parallel()
		env := newChatAutomationTestEnv(t, nil, nil)
		ctx := testutil.Context(t, testutil.WaitLong)
		orgAdminRaw, _ := coderdtest.CreateAnotherUser(t, env.owner.Client, env.orgID, rbac.ScopedRoleOrgAdmin(env.orgID))
		orgAdmin := codersdk.NewExperimentalClient(orgAdminRaw)

		created, err := env.member.CreateChatAutomation(ctx, env.orgID, env.scheduleRequest())
		require.NoError(t, err)
		id := created.Automation.ID

		rename := codersdk.UpdateChatAutomationRequest{Name: ptr.Ref("Renamed")}
		for name, client := range map[string]*codersdk.ExperimentalClient{"OrgAdmin": orgAdmin, "SiteOwner": env.owner} {
			_, err := client.UpdateChatAutomation(ctx, env.orgID, id, rename)
			requireSDKError(t, err, http.StatusForbidden)

			list, err := client.ChatAutomations(ctx, env.orgID)
			require.NoError(t, err, name)
			require.Len(t, list, 1, name)
			_, err = client.ChatAutomation(ctx, env.orgID, id)
			require.NoError(t, err, name)
		}

		updated, err := env.member.UpdateChatAutomation(ctx, env.orgID, id, rename)
		require.NoError(t, err)
		require.Equal(t, "Renamed", updated.Name)

		// The automation is not found under another organization's path,
		// even for a caller who can read it.
		otherOrg := dbgen.Organization(t, env.db, database.Organization{})
		_, err = env.owner.ChatAutomation(ctx, otherOrg.ID, id)
		requireSDKError(t, err, http.StatusNotFound)

		require.NoError(t, orgAdmin.DeleteChatAutomation(ctx, env.orgID, id))
		_, err = env.member.ChatAutomation(ctx, env.orgID, id)
		requireSDKError(t, err, http.StatusNotFound)
	})

	t.Run("CreateValidation", func(t *testing.T) {
		t.Parallel()
		env := newChatAutomationTestEnv(t, nil, nil)
		otherOrg := dbgen.Organization(t, env.db, database.Organization{})
		foreignModel := dbgen.ChatModelConfig(t, env.db, database.ChatModelConfig{OrganizationID: otherOrg.ID})
		childChat := dbgen.Chat(t, env.db, database.Chat{
			OrganizationID:    env.orgID,
			OwnerID:           env.memberID,
			LastModelConfigID: env.modelConfig.ID,
			ParentChatID:      uuid.NullUUID{UUID: env.memberChat.ID, Valid: true},
			RootChatID:        uuid.NullUUID{UUID: env.memberChat.ID, Valid: true},
		})

		// The site owner can read the member's chat and the foreign model
		// config, so these requests reach the ownership and organization
		// checks rather than failing the lookup.
		ctx := testutil.Context(t, testutil.WaitLong)
		_, err := env.owner.CreateChatAutomation(ctx, env.orgID, env.webhookRequest())
		sdkErr := requireSDKError(t, err, http.StatusBadRequest)
		require.Len(t, sdkErr.Validations, 1, sdkErr.Error())
		require.Equal(t, "target_chat_id", sdkErr.Validations[0].Field)
		require.Contains(t, sdkErr.Validations[0].Detail, "owned by the automation owner")
		foreignModelReq := env.scheduleRequest()
		foreignModelReq.NewChatModelConfigID = &foreignModel.ID
		_, err = env.owner.CreateChatAutomation(ctx, env.orgID, foreignModelReq)
		sdkErr = requireSDKError(t, err, http.StatusBadRequest)
		require.Len(t, sdkErr.Validations, 1, sdkErr.Error())
		require.Equal(t, "new_chat_model_config_id", sdkErr.Validations[0].Field)
		require.Contains(t, sdkErr.Validations[0].Detail, "not in the automation's organization")

		for _, tc := range []struct {
			name   string
			field  string
			mutate func(*codersdk.CreateChatAutomationRequest)
		}{
			{"BlankName", "name", func(r *codersdk.CreateChatAutomationRequest) { r.Name = "  " }},
			{"LongName", "name", func(r *codersdk.CreateChatAutomationRequest) { r.Name = strings.Repeat("a", 129) }},
			{"BlankPrompt", "prompt", func(r *codersdk.CreateChatAutomationRequest) { r.Prompt = " \n" }},
			{"BadKind", "kind", func(r *codersdk.CreateChatAutomationRequest) { r.Kind = "email" }},
			{"BadTargetMode", "target_mode", func(r *codersdk.CreateChatAutomationRequest) { r.TargetMode = "any" }},
			{"CronOnWebhook", "schedule_cron", func(r *codersdk.CreateChatAutomationRequest) { r.ScheduleCron = ptr.Ref("0 9 * * *") }},
			{"BadWebhookUse", "webhook_use", func(r *codersdk.CreateChatAutomationRequest) {
				r.WebhookUse = ptr.Ref(codersdk.ChatAutomationWebhookUse("twice"))
			}},
			{"BadWhenBusy", "when_busy", func(r *codersdk.CreateChatAutomationRequest) {
				r.WhenBusy = ptr.Ref(codersdk.ChatAutomationWhenBusy("wait"))
			}},
			{"MissingTarget", "target_chat_id", func(r *codersdk.CreateChatAutomationRequest) { r.TargetChatID = nil }},
			{"UnknownTarget", "target_chat_id", func(r *codersdk.CreateChatAutomationRequest) { r.TargetChatID = ptr.Ref(uuid.New()) }},
			{"ChildTarget", "target_chat_id", func(r *codersdk.CreateChatAutomationRequest) { r.TargetChatID = &childChat.ID }},
			{"ModelOnExistingChat", "new_chat_model_config_id", func(r *codersdk.CreateChatAutomationRequest) { r.NewChatModelConfigID = &env.modelConfig.ID }},
			{"EffortOnExistingChat", "reasoning_effort", func(r *codersdk.CreateChatAutomationRequest) { r.ReasoningEffort = ptr.Ref("high") }},
			{"ScheduleWithoutCron", "schedule_cron", func(r *codersdk.CreateChatAutomationRequest) {
				r.Kind = codersdk.ChatAutomationKindSchedule
				r.ScheduleTimeZone = ptr.Ref("UTC")
			}},
			{"ScheduleSixFields", "schedule_cron", func(r *codersdk.CreateChatAutomationRequest) {
				r.Kind = codersdk.ChatAutomationKindSchedule
				r.ScheduleCron, r.ScheduleTimeZone = ptr.Ref("0 0 9 * * *"), ptr.Ref("UTC")
			}},
			{"ScheduleBadTimeZone", "schedule_time_zone", func(r *codersdk.CreateChatAutomationRequest) {
				r.Kind = codersdk.ChatAutomationKindSchedule
				r.ScheduleCron, r.ScheduleTimeZone = ptr.Ref("0 9 * * *"), ptr.Ref("Mars/Olympus")
			}},
			{"ScheduleWebhookUse", "webhook_use", func(r *codersdk.CreateChatAutomationRequest) {
				r.Kind = codersdk.ChatAutomationKindSchedule
				r.ScheduleCron, r.ScheduleTimeZone = ptr.Ref("0 9 * * *"), ptr.Ref("UTC")
				r.WebhookUse = ptr.Ref(codersdk.ChatAutomationWebhookUseSingle)
			}},
			{"NewChatWithTarget", "target_chat_id", func(r *codersdk.CreateChatAutomationRequest) {
				r.TargetMode = codersdk.ChatAutomationTargetModeNewChat
				r.NewChatModelConfigID = &env.modelConfig.ID
			}},
			{"NewChatWhenBusy", "when_busy", func(r *codersdk.CreateChatAutomationRequest) {
				r.TargetMode, r.TargetChatID = codersdk.ChatAutomationTargetModeNewChat, nil
				r.NewChatModelConfigID = &env.modelConfig.ID
				r.WhenBusy = ptr.Ref(codersdk.ChatAutomationWhenBusyQueue)
			}},
			{"NewChatWithoutModel", "new_chat_model_config_id", func(r *codersdk.CreateChatAutomationRequest) {
				r.TargetMode, r.TargetChatID = codersdk.ChatAutomationTargetModeNewChat, nil
			}},
			{"NewChatForeignModel", "new_chat_model_config_id", func(r *codersdk.CreateChatAutomationRequest) {
				r.TargetMode, r.TargetChatID = codersdk.ChatAutomationTargetModeNewChat, nil
				r.NewChatModelConfigID = &foreignModel.ID
			}},
			{"NewChatBadEffort", "reasoning_effort", func(r *codersdk.CreateChatAutomationRequest) {
				r.TargetMode, r.TargetChatID = codersdk.ChatAutomationTargetModeNewChat, nil
				r.NewChatModelConfigID = &env.modelConfig.ID
				r.ReasoningEffort = ptr.Ref("extreme")
			}},
		} {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				ctx := testutil.Context(t, testutil.WaitLong)
				req := env.webhookRequest()
				tc.mutate(&req)
				_, err := env.member.CreateChatAutomation(ctx, env.orgID, req)
				sdkErr := requireSDKError(t, err, http.StatusBadRequest)
				require.Len(t, sdkErr.Validations, 1, sdkErr.Error())
				require.Equal(t, tc.field, sdkErr.Validations[0].Field, sdkErr.Error())
			})
		}
	})

	t.Run("UpdateValidation", func(t *testing.T) {
		t.Parallel()
		env := newChatAutomationTestEnv(t, nil, nil)
		ctx := testutil.Context(t, testutil.WaitLong)
		_, otherUser := coderdtest.CreateAnotherUser(t, env.owner.Client, env.orgID)
		othersChat := dbgen.Chat(t, env.db, database.Chat{OrganizationID: env.orgID, OwnerID: otherUser.ID, LastModelConfigID: env.modelConfig.ID})
		childChat := dbgen.Chat(t, env.db, database.Chat{
			OrganizationID:    env.orgID,
			OwnerID:           env.memberID,
			LastModelConfigID: env.modelConfig.ID,
			ParentChatID:      uuid.NullUUID{UUID: env.memberChat.ID, Valid: true},
			RootChatID:        uuid.NullUUID{UUID: env.memberChat.ID, Valid: true},
		})
		disabledModel := dbgen.ChatModelConfig(t, env.db, database.ChatModelConfig{OrganizationID: env.orgID}, func(p *database.InsertChatModelConfigParams) {
			p.Enabled = false
		})

		webhook, err := env.member.CreateChatAutomation(ctx, env.orgID, env.webhookRequest())
		require.NoError(t, err)
		schedule, err := env.member.CreateChatAutomation(ctx, env.orgID, env.scheduleRequest())
		require.NoError(t, err)

		for _, tc := range []struct {
			name   string
			id     uuid.UUID
			field  string
			req    codersdk.UpdateChatAutomationRequest
			rawReq map[string]any
		}{
			{name: "BlankName", id: webhook.Automation.ID, field: "name", req: codersdk.UpdateChatAutomationRequest{Name: ptr.Ref("")}},
			{name: "BlankPrompt", id: webhook.Automation.ID, field: "prompt", req: codersdk.UpdateChatAutomationRequest{Prompt: ptr.Ref("  ")}},
			{name: "CronOnWebhook", id: webhook.Automation.ID, field: "schedule_cron", req: codersdk.UpdateChatAutomationRequest{ScheduleCron: ptr.Ref("0 9 * * *")}},
			{name: "BadCron", id: schedule.Automation.ID, field: "schedule_cron", req: codersdk.UpdateChatAutomationRequest{ScheduleCron: ptr.Ref("0 25 * * *")}},
			{name: "BadTimeZone", id: schedule.Automation.ID, field: "schedule_time_zone", req: codersdk.UpdateChatAutomationRequest{ScheduleTimeZone: ptr.Ref("Local")}},
			{name: "WhenBusyOnNewChat", id: schedule.Automation.ID, field: "when_busy", req: codersdk.UpdateChatAutomationRequest{WhenBusy: ptr.Ref(codersdk.ChatAutomationWhenBusyQueue)}},
			{name: "TargetOnNewChat", id: schedule.Automation.ID, field: "target_chat_id", req: codersdk.UpdateChatAutomationRequest{TargetChatID: &env.memberChat.ID}},
			{name: "EffortOnExistingChat", id: webhook.Automation.ID, field: "reasoning_effort", req: codersdk.UpdateChatAutomationRequest{ReasoningEffort: ptr.Ref("low")}},
			{name: "ChildTarget", id: webhook.Automation.ID, field: "target_chat_id", req: codersdk.UpdateChatAutomationRequest{TargetChatID: &childChat.ID}},
			{name: "OthersChat", id: webhook.Automation.ID, field: "target_chat_id", req: codersdk.UpdateChatAutomationRequest{TargetChatID: &othersChat.ID}},
			{name: "DisabledModel", id: schedule.Automation.ID, field: "new_chat_model_config_id", req: codersdk.UpdateChatAutomationRequest{NewChatModelConfigID: &disabledModel.ID}},
			{name: "Kind", id: webhook.Automation.ID, field: "kind", rawReq: map[string]any{"kind": "schedule"}},
			{name: "TargetMode", id: webhook.Automation.ID, field: "target_mode", rawReq: map[string]any{"target_mode": "new_chat"}},
		} {
			var body any = tc.req
			if tc.rawReq != nil {
				body = tc.rawReq
			}
			res, err := env.member.Request(ctx, http.MethodPatch, fmt.Sprintf("/api/experimental/organizations/%s/chat-automations/%s", env.orgID, tc.id), body)
			require.NoError(t, err)
			sdkErr := requireSDKError(t, codersdk.ReadBodyAsError(res), http.StatusBadRequest)
			_ = res.Body.Close()
			require.Len(t, sdkErr.Validations, 1, tc.name)
			require.Equal(t, tc.field, sdkErr.Validations[0].Field, tc.name)
		}

		// Rejected updates change nothing.
		stored, err := env.db.GetChatAutomationByID(dbauthz.AsSystemRestricted(ctx), schedule.Automation.ID)
		require.NoError(t, err)
		require.Equal(t, int64(1), stored.ScheduleRevision)

		// A schedule change bumps the revision and moves the cursor.
		updated, err := env.member.UpdateChatAutomation(ctx, env.orgID, schedule.Automation.ID, codersdk.UpdateChatAutomationRequest{
			ScheduleCron:     ptr.Ref("30 6 * * *"),
			ScheduleTimeZone: ptr.Ref("UTC"),
		})
		require.NoError(t, err)
		require.Equal(t, "30 6 * * *", *updated.ScheduleCron)
		require.Equal(t, 6, updated.ScheduleNextRunAt.UTC().Hour())
		require.Equal(t, 30, updated.ScheduleNextRunAt.UTC().Minute())
		require.True(t, updated.NextRunTimes[0].Equal(*updated.ScheduleNextRunAt))
		stored, err = env.db.GetChatAutomationByID(dbauthz.AsSystemRestricted(ctx), schedule.Automation.ID)
		require.NoError(t, err)
		require.Equal(t, int64(2), stored.ScheduleRevision)

		// A name change alone keeps the revision.
		_, err = env.member.UpdateChatAutomation(ctx, env.orgID, schedule.Automation.ID, codersdk.UpdateChatAutomationRequest{Name: ptr.Ref("Morning report")})
		require.NoError(t, err)
		stored, err = env.db.GetChatAutomationByID(dbauthz.AsSystemRestricted(ctx), schedule.Automation.ID)
		require.NoError(t, err)
		require.Equal(t, int64(2), stored.ScheduleRevision)
		require.Equal(t, "Morning report", stored.Name)

		// A prompt change bumps the revision.
		_, err = env.member.UpdateChatAutomation(ctx, env.orgID, schedule.Automation.ID, codersdk.UpdateChatAutomationRequest{Prompt: ptr.Ref("Summarize today.")})
		require.NoError(t, err)
		stored, err = env.db.GetChatAutomationByID(dbauthz.AsSystemRestricted(ctx), schedule.Automation.ID)
		require.NoError(t, err)
		require.Equal(t, int64(3), stored.ScheduleRevision)

		// An empty reasoning effort clears the override.
		withEffort, err := env.member.UpdateChatAutomation(ctx, env.orgID, schedule.Automation.ID, codersdk.UpdateChatAutomationRequest{ReasoningEffort: ptr.Ref("high")})
		require.NoError(t, err)
		require.Equal(t, ptr.Ref("high"), withEffort.ReasoningEffort)
		cleared, err := env.member.UpdateChatAutomation(ctx, env.orgID, schedule.Automation.ID, codersdk.UpdateChatAutomationRequest{ReasoningEffort: ptr.Ref("")})
		require.NoError(t, err)
		require.Nil(t, cleared.ReasoningEffort)
	})
}

func TestChatAutomationsExperimentGate(t *testing.T) {
	t.Parallel()

	t.Run("Off", func(t *testing.T) {
		t.Parallel()
		env := newChatAutomationTestEnv(t, []string{}, nil)
		ctx := testutil.Context(t, testutil.WaitLong)
		existing := dbgen.ChatAutomation(t, env.db, database.ChatAutomation{
			OrganizationID: env.orgID,
			OwnerID:        env.memberID,
			TargetChatID:   uuid.NullUUID{UUID: env.memberChat.ID, Valid: true},
		})

		_, err := env.member.ChatAutomations(ctx, env.orgID)
		requireSDKError(t, err, http.StatusNotFound)
		_, err = env.member.CreateChatAutomation(ctx, env.orgID, env.webhookRequest())
		requireSDKError(t, err, http.StatusNotFound)
		_, err = env.member.ChatAutomation(ctx, env.orgID, existing.ID)
		requireSDKError(t, err, http.StatusNotFound)
		_, err = env.member.UpdateChatAutomation(ctx, env.orgID, existing.ID, codersdk.UpdateChatAutomationRequest{Name: ptr.Ref("x")})
		requireSDKError(t, err, http.StatusNotFound)
		err = env.member.DeleteChatAutomation(ctx, env.orgID, existing.ID)
		requireSDKError(t, err, http.StatusNotFound)

		_, err = env.db.GetChatAutomationByID(dbauthz.AsSystemRestricted(ctx), existing.ID)
		require.NoError(t, err, "the gated delete must not remove the row")
	})

	t.Run("RuntimeRuleIsPerUser", func(t *testing.T) {
		t.Parallel()
		env := newChatAutomationTestEnv(t, nil, nil)
		ctx := testutil.Context(t, testutil.WaitLong)
		otherRaw, _ := coderdtest.CreateAnotherUser(t, env.owner.Client, env.orgID)
		other := codersdk.NewExperimentalClient(otherRaw)
		member, err := env.member.User(ctx, codersdk.Me)
		require.NoError(t, err)

		_, err = env.member.ChatAutomations(ctx, env.orgID)
		require.NoError(t, err)

		_, err = env.owner.PutExperimentRule(ctx, codersdk.ExperimentChatAutomations, codersdk.PutExperimentRuleRequest{
			Mode:      codersdk.ExperimentRuleModeCondition,
			Condition: fmt.Sprintf("user.username != %q", member.Username),
		})
		require.NoError(t, err)

		_, err = env.member.ChatAutomations(ctx, env.orgID)
		requireSDKError(t, err, http.StatusNotFound)
		_, err = other.ChatAutomations(ctx, env.orgID)
		require.NoError(t, err)
	})

	t.Run("NoV2Alias", func(t *testing.T) {
		t.Parallel()
		env := newChatAutomationTestEnv(t, nil, nil)
		status, body := rawGet(t, env.member, fmt.Sprintf("/api/v2/organizations/%s/chat-automations", env.orgID))
		require.Equal(t, http.StatusNotFound, status, body)
	})
}
