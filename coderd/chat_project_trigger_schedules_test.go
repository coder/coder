package coderd_test

import (
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/serpent"
)

// yearlySchedule runs once a year, so the background runner, which polls
// with the real clock, never finds a test's trigger due.
const yearlySchedule = "CRON_TZ=Europe/Berlin 0 9 1 1 *"

func newChatProjectScheduleClient(t *testing.T) (*codersdk.ExperimentalClient, *coderd.API) {
	t.Helper()
	client, _, api := newChatClientWithAPIAndDatabase(t,
		func(options *coderdtest.Options) {
			options.DeploymentValues.Experiments = serpent.StringArray{
				string(codersdk.ExperimentChatProjects),
			}
		},
		withChatWorkerDisabled,
	)
	return client, api
}

func TestChatProjectScheduleTriggerValidation(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client, _ := newChatProjectScheduleClient(t)
	firstUser := coderdtest.CreateFirstUser(t, client.Client)
	project := createChatProject(t, client, firstUser.OrganizationID, "Reporter")

	for _, tc := range []struct {
		name   string
		req    codersdk.CreateChatProjectTriggerRequest
		field  string
		detail string
	}{
		{
			name:   "EveryMinute",
			req:    codersdk.CreateChatProjectTriggerRequest{Schedule: "* * * * *"},
			field:  "schedule",
			detail: "Schedule runs every minute; the minimum interval is 5 minutes.",
		},
		{
			name:   "EveryTwoMinutes",
			req:    codersdk.CreateChatProjectTriggerRequest{Schedule: "*/2 * * * *"},
			field:  "schedule",
			detail: "Schedule runs every 2 minutes; the minimum interval is 5 minutes.",
		},
		{
			name:   "ShortGapAcrossHours",
			req:    codersdk.CreateChatProjectTriggerRequest{Schedule: "0,58 * * * *"},
			field:  "schedule",
			detail: "Schedule runs every 2 minutes; the minimum interval is 5 minutes.",
		},
		{
			name:   "UnknownTimeZone",
			req:    codersdk.CreateChatProjectTriggerRequest{Schedule: "CRON_TZ=Mars/Olympus 0 9 * * *"},
			field:  "schedule",
			detail: `Schedule has an unknown time zone "Mars/Olympus". Use a name such as Europe/Berlin or America/New_York.`,
		},
		{
			name:   "FourFields",
			req:    codersdk.CreateChatProjectTriggerRequest{Schedule: "0 9 * *"},
			field:  "schedule",
			detail: `Schedule must have five fields: minute, hour, day of month, month, and day of week, such as "0 9 * * 1-5". It can start with CRON_TZ=<time zone>.`,
		},
		{
			name:   "InvalidField",
			req:    codersdk.CreateChatProjectTriggerRequest{Schedule: "0 25 * * *"},
			field:  "schedule",
			detail: "Schedule is not a valid cron expression: end of range (25) above maximum (23): 25.",
		},
		{
			name:   "NeverRuns",
			req:    codersdk.CreateChatProjectTriggerRequest{Schedule: "0 0 30 2 *"},
			field:  "schedule",
			detail: "Schedule never runs. Check the day of month and month fields.",
		},
		{
			name:   "Missing",
			req:    codersdk.CreateChatProjectTriggerRequest{},
			field:  "schedule",
			detail: "Schedule is required for schedule triggers.",
		},
		{
			name:   "Events",
			req:    codersdk.CreateChatProjectTriggerRequest{Schedule: yearlySchedule, Events: []string{"push"}},
			field:  "events",
			detail: "Schedule triggers start a chat on their schedule and have no events.",
		},
		{
			name: "TextPattern",
			req: codersdk.CreateChatProjectTriggerRequest{
				Schedule:                     yearlySchedule,
				ChatProjectTriggerTextFilter: codersdk.ChatProjectTriggerTextFilter{TextPattern: "deploy"},
			},
			field:  "text_pattern",
			detail: "Schedule triggers have no text to filter.",
		},
		{
			name:   "NoPrompt",
			req:    codersdk.CreateChatProjectTriggerRequest{Schedule: yearlySchedule, Prompt: " "},
			field:  "prompt",
			detail: "Prompt is required for schedule triggers. It tells the agent what to do on each run.",
		},
	} {
		req := tc.req
		req.Name = "Daily report"
		req.Source = codersdk.ChatProjectTriggerSourceSchedule
		if req.Prompt == "" {
			req.Prompt = "Summarize yesterday's issues."
		}
		_, err := client.CreateChatProjectTrigger(ctx, firstUser.OrganizationID, project.ID, req)
		sdkErr := coderdtest.SDKError(t, err)
		require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode(), tc.name)
		require.Equal(t, []codersdk.ValidationError{{Field: tc.field, Detail: tc.detail}}, sdkErr.Validations, tc.name)
	}

	// Only schedule triggers have a schedule.
	_, err := client.CreateChatProjectTrigger(ctx, firstUser.OrganizationID, project.ID, codersdk.CreateChatProjectTriggerRequest{
		Name:     "Webhook",
		Schedule: yearlySchedule,
	})
	requireChatProjectFieldError(t, err, "schedule", "Only schedule triggers have a schedule.")

	created, err := client.CreateChatProjectTrigger(ctx, firstUser.OrganizationID, project.ID, codersdk.CreateChatProjectTriggerRequest{
		Name:     "Weekday report",
		Prompt:   "Summarize yesterday's issues.",
		Source:   codersdk.ChatProjectTriggerSourceSchedule,
		Schedule: "  CRON_TZ=Europe/Berlin   30 8 * * 1-5 ",
	})
	require.NoError(t, err)
	require.Empty(t, created.WebhookURL)
	require.Equal(t, "CRON_TZ=Europe/Berlin 30 8 * * 1-5", created.Trigger.Schedule)
	require.NotNil(t, created.Trigger.NextRunAt)
	berlin, err := time.LoadLocation("Europe/Berlin")
	require.NoError(t, err)
	next := created.Trigger.NextRunAt.In(berlin)
	require.Equal(t, 8, next.Hour())
	require.Equal(t, 30, next.Minute())
	require.True(t, next.After(time.Now()))

	// Schedule triggers have no webhook URL to regenerate.
	_, err = client.RegenerateChatProjectTriggerWebhookURL(ctx, firstUser.OrganizationID, project.ID, created.Trigger.ID)
	require.Equal(t, http.StatusBadRequest, coderdtest.SDKError(t, err).StatusCode())

	_, err = client.UpdateChatProjectTrigger(ctx, firstUser.OrganizationID, project.ID, created.Trigger.ID, codersdk.UpdateChatProjectTriggerRequest{
		Schedule: ptr.Ref("*/3 * * * *"),
	})
	requireChatProjectFieldError(t, err, "schedule", "Schedule runs every 3 minutes; the minimum interval is 5 minutes.")

	// Disabling clears the next run; enabling schedules it again.
	disabled, err := client.UpdateChatProjectTrigger(ctx, firstUser.OrganizationID, project.ID, created.Trigger.ID, codersdk.UpdateChatProjectTriggerRequest{
		Enabled: ptr.Ref(false),
	})
	require.NoError(t, err)
	require.Nil(t, disabled.NextRunAt)
	enabled, err := client.UpdateChatProjectTrigger(ctx, firstUser.OrganizationID, project.ID, created.Trigger.ID, codersdk.UpdateChatProjectTriggerRequest{
		Enabled:  ptr.Ref(true),
		Schedule: ptr.Ref(yearlySchedule),
	})
	require.NoError(t, err)
	require.NotNil(t, enabled.NextRunAt)
	require.Equal(t, time.January, enabled.NextRunAt.In(berlin).Month())

	// Saving other settings keeps the next run.
	renamed, err := client.UpdateChatProjectTrigger(ctx, firstUser.OrganizationID, project.ID, created.Trigger.ID, codersdk.UpdateChatProjectTriggerRequest{
		Name: ptr.Ref("Yearly report"),
	})
	require.NoError(t, err)
	require.Equal(t, enabled.NextRunAt, renamed.NextRunAt)
}

func TestChatProjectScheduleTriggerRuns(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client, api := newChatProjectScheduleClient(t)
	firstUser := coderdtest.CreateFirstUser(t, client.Client)
	// Created first so the next model is the organization default.
	agentModel := createChatModel(t, client)
	defaultModel := createChatModel(t, client)
	project := createChatProject(t, client, firstUser.OrganizationID, "Reporter")
	_, err := client.UpdateChatProject(ctx, firstUser.OrganizationID, project.ID, codersdk.UpdateChatProjectRequest{
		ModelConfigID: ptr.Ref(agentModel.ID.String()),
	})
	require.NoError(t, err)

	created, err := client.CreateChatProjectTrigger(ctx, firstUser.OrganizationID, project.ID, codersdk.CreateChatProjectTriggerRequest{
		Name:     "Yearly report",
		Prompt:   "Summarize last year's issues.",
		Source:   codersdk.ChatProjectTriggerSourceSchedule,
		Schedule: yearlySchedule,
	})
	require.NoError(t, err)
	trigger := created.Trigger
	require.NotNil(t, trigger.NextRunAt)
	due := *trigger.NextRunAt

	// Not due yet: nothing runs.
	coderd.RunDueChatProjectScheduleTriggers(api, ctx, due.Add(-time.Second))
	runs, err := client.ListChatProjectRuns(ctx, firstUser.OrganizationID, project.ID)
	require.NoError(t, err)
	require.Empty(t, runs)

	// Two replicas polling at once run the trigger once.
	now := due.Add(time.Second)
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			coderd.RunDueChatProjectScheduleTriggers(api, ctx, now)
		}()
	}
	wg.Wait()
	// Polling again at the same time finds nothing due.
	coderd.RunDueChatProjectScheduleTriggers(api, ctx, now)

	runs, err = client.ListChatProjectRuns(ctx, firstUser.OrganizationID, project.ID)
	require.NoError(t, err)
	require.Len(t, runs, 1)
	require.Equal(t, &trigger.ID, runs[0].TriggerID)

	chat, err := client.GetChat(ctx, runs[0].ChatID)
	require.NoError(t, err)
	// The agent's model applies to scheduled chats.
	require.Equal(t, agentModel.ID, chat.LastModelConfigID)
	require.NotEqual(t, defaultModel.ID, chat.LastModelConfigID)
	text := triggerChatUserText(ctx, t, client, chat.ID)
	require.Contains(t, text, "Summarize last year's issues.")
	require.Contains(t, text, `"Yearly report" schedule trigger`)

	triggers, err := client.ListChatProjectTriggers(ctx, firstUser.OrganizationID, project.ID)
	require.NoError(t, err)
	require.Len(t, triggers, 1)
	require.NotNil(t, triggers[0].LastTriggeredAt)
	require.NotNil(t, triggers[0].NextRunAt)
	// The next run moved a year ahead.
	require.True(t, triggers[0].NextRunAt.After(now))
	require.Equal(t, due.Year()+1, triggers[0].NextRunAt.Year())
	require.NotNil(t, triggers[0].LastDelivery)
	require.Equal(t, codersdk.ChatProjectTriggerDeliveryStatusStarted, triggers[0].LastDelivery.Status)
	require.Equal(t, "schedule", triggers[0].LastDelivery.Event)

	// Running it manually starts another chat and keeps the next run.
	manual, err := client.RunChatProjectTrigger(ctx, firstUser.OrganizationID, project.ID, trigger.ID)
	require.NoError(t, err)
	require.False(t, manual.Continued)
	require.NotEqual(t, chat.ID, manual.ChatID)
	require.Contains(t, triggerChatUserText(ctx, t, client, manual.ChatID), "manually")
	deliveries, err := client.ChatProjectTriggerDeliveries(ctx, firstUser.OrganizationID, project.ID, trigger.ID)
	require.NoError(t, err)
	require.Len(t, deliveries, 2)
	require.Equal(t, "run_now", deliveries[0].Event)
	afterManual, err := client.ListChatProjectTriggers(ctx, firstUser.OrganizationID, project.ID)
	require.NoError(t, err)
	require.Equal(t, triggers[0].NextRunAt, afterManual[0].NextRunAt)

	// A disabled trigger does not run, on schedule or manually.
	_, err = client.UpdateChatProjectTrigger(ctx, firstUser.OrganizationID, project.ID, trigger.ID, codersdk.UpdateChatProjectTriggerRequest{
		Enabled: ptr.Ref(false),
	})
	require.NoError(t, err)
	coderd.RunDueChatProjectScheduleTriggers(api, ctx, now.AddDate(5, 0, 0))
	_, err = client.RunChatProjectTrigger(ctx, firstUser.OrganizationID, project.ID, trigger.ID)
	require.Equal(t, http.StatusConflict, coderdtest.SDKError(t, err).StatusCode())
	runs, err = client.ListChatProjectRuns(ctx, firstUser.OrganizationID, project.ID)
	require.NoError(t, err)
	require.Len(t, runs, 2)

	// Only schedule triggers can be run manually.
	webhook, err := client.CreateChatProjectTrigger(ctx, firstUser.OrganizationID, project.ID, codersdk.CreateChatProjectTriggerRequest{
		Name: "Webhook",
	})
	require.NoError(t, err)
	_, err = client.RunChatProjectTrigger(ctx, firstUser.OrganizationID, project.ID, webhook.Trigger.ID)
	require.Equal(t, http.StatusBadRequest, coderdtest.SDKError(t, err).StatusCode())

	// Other members cannot run the owner's trigger.
	memberClient, _ := coderdtest.CreateAnotherUser(t, client.Client, firstUser.OrganizationID)
	member := codersdk.NewExperimentalClient(memberClient)
	_, err = member.RunChatProjectTrigger(ctx, firstUser.OrganizationID, project.ID, trigger.ID)
	require.Equal(t, http.StatusNotFound, coderdtest.SDKError(t, err).StatusCode())
}

func TestChatProjectScheduleTriggerContinuesConversation(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client, api := newChatProjectScheduleClient(t)
	firstUser := coderdtest.CreateFirstUser(t, client.Client)
	_ = createChatModel(t, client)
	project := createChatProject(t, client, firstUser.OrganizationID, "Reporter")

	created, err := client.CreateChatProjectTrigger(ctx, firstUser.OrganizationID, project.ID, codersdk.CreateChatProjectTriggerRequest{
		Name:                  "Yearly report",
		Prompt:                "Add this year's numbers.",
		Source:                codersdk.ChatProjectTriggerSourceSchedule,
		Schedule:              yearlySchedule,
		ContinueConversations: ptr.Ref(true),
	})
	require.NoError(t, err)
	require.True(t, created.Trigger.ContinueConversations)
	due := *created.Trigger.NextRunAt

	coderd.RunDueChatProjectScheduleTriggers(api, ctx, due.Add(time.Second))
	coderd.RunDueChatProjectScheduleTriggers(api, ctx, due.AddDate(1, 0, 1))

	runs, err := client.ListChatProjectRuns(ctx, firstUser.OrganizationID, project.ID)
	require.NoError(t, err)
	require.Len(t, runs, 1)
	deliveries, err := client.ChatProjectTriggerDeliveries(ctx, firstUser.OrganizationID, project.ID, created.Trigger.ID)
	require.NoError(t, err)
	require.Len(t, deliveries, 2)
	require.Equal(t, codersdk.ChatProjectTriggerDeliveryStatusContinued, deliveries[0].Status)
	require.Equal(t, codersdk.ChatProjectTriggerDeliveryStatusStarted, deliveries[1].Status)
	require.Equal(t, deliveries[0].ChatID, deliveries[1].ChatID)
	require.NotNil(t, deliveries[0].ChatID)
	require.NotEqual(t, uuid.Nil, *deliveries[0].ChatID)
}
