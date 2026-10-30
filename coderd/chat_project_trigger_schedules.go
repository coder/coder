package coderd

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"

	"github.com/coder/coder/v2/buildinfo"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/schedule/cron"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/codersdk"
)

// Schedule triggers start a chat on a cron schedule. Each enabled schedule
// trigger stores when it runs next in next_run_at. Every coderd replica
// polls for due triggers; the query locks the rows it returns and skips rows
// another replica has locked, and next_run_at moves to the next future run
// before the lock is released, so each run is claimed once. Runs missed
// while no replica was polling are skipped, not caught up. A claimed run
// whose chat fails to start is recorded as a rejected delivery and is not
// retried.

const (
	// chatProjectScheduleTriggerPollInterval is how often each replica looks
	// for due schedule triggers, which bounds how late a run starts.
	chatProjectScheduleTriggerPollInterval = 30 * time.Second
	// chatProjectScheduleTriggerBatchSize bounds how many due triggers one
	// claim locks.
	chatProjectScheduleTriggerBatchSize = 20
	// minChatProjectTriggerScheduleInterval is the shortest time allowed
	// between two runs of a schedule trigger.
	minChatProjectTriggerScheduleInterval = 5 * time.Minute
	// chatProjectTriggerScheduleCheckedRuns is how many upcoming runs are
	// checked against the minimum interval.
	chatProjectTriggerScheduleCheckedRuns = 1000
	// maxChatProjectTriggerScheduleChars bounds the stored cron expression.
	maxChatProjectTriggerScheduleChars = 256

	// chatProjectScheduleConversationKey is the one conversation a schedule
	// trigger that continues conversations sends every run to.
	chatProjectScheduleConversationKey = "schedule"
	// Delivery event names for scheduled and manual runs.
	chatProjectScheduleTriggerEventScheduled = "schedule"
	chatProjectScheduleTriggerEventRunNow    = "run_now"
)

// parseChatProjectTriggerSchedule validates the cron expression of a
// schedule trigger. It returns a message for people when the expression is
// not usable.
func parseChatProjectTriggerSchedule(raw string, now time.Time) (*cron.Schedule, string) {
	raw = strings.Join(strings.Fields(raw), " ")
	if raw == "" {
		return nil, "Schedule is required for schedule triggers."
	}
	if len(raw) > maxChatProjectTriggerScheduleChars {
		return nil, fmt.Sprintf("Schedule must be at most %d characters.", maxChatProjectTriggerScheduleChars)
	}
	fields := strings.Fields(raw)
	if tz, ok := strings.CutPrefix(fields[0], "CRON_TZ="); ok {
		if _, err := time.LoadLocation(tz); err != nil || tz == "" || tz == "Local" {
			return nil, fmt.Sprintf("Schedule has an unknown time zone %q. Use a name such as Europe/Berlin or America/New_York.", tz)
		}
		fields = fields[1:]
	}
	if len(fields) != 5 {
		return nil, "Schedule must have five fields: minute, hour, day of month, month, and day of week, such as \"0 9 * * 1-5\". It can start with CRON_TZ=<time zone>."
	}
	sched, err := cron.Standard(raw)
	if err != nil {
		return nil, fmt.Sprintf("Schedule is not a valid cron expression: %s.", strings.TrimPrefix(err.Error(), "parse schedule: "))
	}
	prev := sched.Next(now)
	if prev.IsZero() {
		return nil, "Schedule never runs. Check the day of month and month fields."
	}
	for range chatProjectTriggerScheduleCheckedRuns {
		next := sched.Next(prev)
		if next.IsZero() {
			break
		}
		if gap := next.Sub(prev); gap < minChatProjectTriggerScheduleInterval {
			if gap <= time.Minute {
				return nil, fmt.Sprintf("Schedule runs every minute; the minimum interval is %d minutes.", int(minChatProjectTriggerScheduleInterval.Minutes()))
			}
			return nil, fmt.Sprintf("Schedule runs every %d minutes; the minimum interval is %d minutes.", int(gap.Minutes()), int(minChatProjectTriggerScheduleInterval.Minutes()))
		}
		prev = next
	}
	return sched, ""
}

// chatProjectTriggerNextRunAt returns the first run of a schedule after now.
// A schedule that never runs again has no next run.
func chatProjectTriggerNextRunAt(schedule string, now time.Time) (sql.NullTime, error) {
	sched, err := cron.Standard(schedule)
	if err != nil {
		return sql.NullTime{}, xerrors.Errorf("parse trigger schedule: %w", err)
	}
	next := sched.Next(now)
	if next.IsZero() {
		return sql.NullTime{}, nil
	}
	return sql.NullTime{Time: next, Valid: true}, nil
}

// runChatProjectScheduleTriggers polls for due schedule triggers until ctx
// is canceled.
func (api *API) runChatProjectScheduleTriggers(ctx context.Context) {
	ticker := api.Clock.TickerFunc(ctx, chatProjectScheduleTriggerPollInterval, func() error {
		if api.chatDaemon == nil {
			return nil
		}
		if !buildinfo.IsDev() && !api.Experiments.Enabled(codersdk.ExperimentChatProjects) {
			return nil
		}
		api.runDueChatProjectScheduleTriggers(ctx, api.Clock.Now())
		return nil
	}, "chat_project_schedule_triggers")
	_ = ticker.Wait()
}

// runDueChatProjectScheduleTriggers claims every schedule trigger due at
// now and runs it once.
func (api *API) runDueChatProjectScheduleTriggers(ctx context.Context, now time.Time) {
	//nolint:gocritic // The schedule runner spans every organization's triggers.
	sysCtx := dbauthz.AsSystemRestricted(ctx)
	for ctx.Err() == nil {
		var claimed []database.ChatProjectTrigger
		err := api.Database.InTx(func(tx database.Store) error {
			claimed = nil
			due, err := tx.GetDueChatProjectScheduleTriggers(sysCtx, database.GetDueChatProjectScheduleTriggersParams{
				Now:      now,
				LimitOpt: chatProjectScheduleTriggerBatchSize,
			})
			if err != nil {
				return xerrors.Errorf("get due schedule triggers: %w", err)
			}
			for _, trigger := range due {
				// Missed runs are skipped: the next run is the first one
				// after now, not after the run that was due.
				next, err := chatProjectTriggerNextRunAt(trigger.Schedule, now)
				if err != nil {
					api.Logger.Warn(ctx, "schedule trigger has an invalid schedule", slog.F("trigger_id", trigger.ID), slog.Error(err))
				}
				if err := tx.UpdateChatProjectTriggerNextRunAt(sysCtx, database.UpdateChatProjectTriggerNextRunAtParams{
					ID:        trigger.ID,
					NextRunAt: next,
				}); err != nil {
					return xerrors.Errorf("advance schedule trigger: %w", err)
				}
				claimed = append(claimed, trigger)
			}
			return nil
		}, nil)
		if err != nil {
			if ctx.Err() == nil {
				api.Logger.Warn(ctx, "claim due schedule triggers", slog.Error(err))
			}
			return
		}
		for _, trigger := range claimed {
			_, err := api.runChatProjectScheduleTrigger(ctx, trigger, chatProjectScheduleTriggerEventScheduled, trigger.NextRunAt.Time)
			if err != nil {
				api.Logger.Debug(ctx, "scheduled trigger run did not start a chat", slog.F("trigger_id", trigger.ID), slog.Error(err))
			}
		}
		if len(claimed) < chatProjectScheduleTriggerBatchSize {
			return
		}
	}
}

// runChatProjectScheduleTrigger starts or continues the chat for one run of
// a schedule trigger and records the run as a delivery. event names the
// delivery; scheduledAt is the time the run was due, or now for manual
// runs.
func (api *API) runChatProjectScheduleTrigger(ctx context.Context, trigger database.ChatProjectTrigger, event string, scheduledAt time.Time) (chatProjectTriggerOutcome, error) {
	//nolint:gocritic // Triggered runs have no session; the chat runs as the agent's user.
	sysCtx := dbauthz.AsSystemRestricted(ctx)
	delivery := database.InsertChatProjectTriggerDeliveryParams{
		TriggerID: trigger.ID,
		Status:    string(codersdk.ChatProjectTriggerDeliveryStatusRejected),
		Event:     event,
		Message:   "Internal error.",
		ChatID:    uuid.NullUUID{},
	}
	defer api.recordChatProjectTriggerDelivery(sysCtx, &delivery)

	outcome, err := func() (chatProjectTriggerOutcome, error) {
		if !trigger.Enabled {
			return chatProjectTriggerOutcome{}, &chatProjectTriggerRejectError{
				status: http.StatusConflict,
				resp:   codersdk.Response{Message: "Trigger is disabled."},
			}
		}
		project, ownerCtx, agentCtx, err := api.chatProjectTriggerContexts(ctx, sysCtx, trigger)
		if err != nil {
			return chatProjectTriggerOutcome{}, err
		}
		run := chatProjectTriggerRun{
			api:          api,
			trigger:      trigger,
			project:      project,
			agentCtx:     agentCtx,
			sysCtx:       sysCtx,
			event:        chatProjectTriggerEvent{name: event},
			startMessage: chatProjectTriggerScheduleMessage(trigger, event, scheduledAt, false),
			nextMessage:  chatProjectTriggerScheduleMessage(trigger, event, scheduledAt, true),
		}
		var outcome chatProjectTriggerOutcome
		if trigger.ContinueConversations {
			run.event.conversationKey = chatProjectScheduleConversationKey
			outcome, err = run.continueOrStart(func() string { return "" })
		} else {
			outcome, err = run.start()
		}
		if err != nil {
			return outcome, err
		}
		if err := api.Database.UpdateChatProjectTriggerLastTriggeredAt(ownerCtx, database.UpdateChatProjectTriggerLastTriggeredAtParams{
			ID:              trigger.ID,
			LastTriggeredAt: dbtime.Now(),
		}); err != nil {
			api.Logger.Warn(ctx, "failed to record trigger run time", slog.F("trigger_id", trigger.ID), slog.Error(err))
		}
		return outcome, nil
	}()
	if err != nil {
		_, resp := chatProjectScheduleTriggerFailure(err, outcome.continued)
		delivery.Message = resp.Message
		return outcome, err
	}
	delivery.Status = string(codersdk.ChatProjectTriggerDeliveryStatusStarted)
	if outcome.continued {
		delivery.Status = string(codersdk.ChatProjectTriggerDeliveryStatusContinued)
	}
	delivery.Message = ""
	delivery.ChatID = uuid.NullUUID{UUID: outcome.chat.ID, Valid: true}
	return outcome, nil
}

// chatProjectScheduleTriggerFailure describes why a schedule trigger run did
// not start or continue a chat.
func chatProjectScheduleTriggerFailure(err error, continued bool) (int, codersdk.Response) {
	var rejectErr *chatProjectTriggerRejectError
	switch {
	case errors.As(err, &rejectErr):
		return rejectErr.status, rejectErr.resp
	case errors.Is(err, chatstate.ErrMessageQueueFull):
		return http.StatusTooManyRequests, codersdk.Response{Message: "The trigger's chat has too many queued messages. The run was skipped."}
	case continued:
		return http.StatusInternalServerError, codersdk.Response{Message: "Sending the run to the trigger's chat failed.", Detail: err.Error()}
	default:
		return http.StatusInternalServerError, codersdk.Response{Message: "Chat creation failed.", Detail: err.Error()}
	}
}

// chatProjectTriggerScheduleMessage builds the user message of a schedule
// trigger run. followUp is set when the run continues the trigger's chat.
func chatProjectTriggerScheduleMessage(trigger database.ChatProjectTrigger, event string, scheduledAt time.Time, followUp bool) string {
	if sched, err := cron.Standard(trigger.Schedule); err == nil {
		scheduledAt = scheduledAt.In(sched.Location())
	}
	at := scheduledAt.Format("Monday, 2 January 2006 15:04 MST")
	var intro string
	switch {
	case event == chatProjectScheduleTriggerEventRunNow && followUp:
		intro = fmt.Sprintf("The %q schedule trigger was run manually again in this chat at %s.", trigger.Name, at)
	case event == chatProjectScheduleTriggerEventRunNow:
		intro = fmt.Sprintf("This chat was started by running the %q schedule trigger manually at %s.", trigger.Name, at)
	case followUp:
		intro = fmt.Sprintf("The %q schedule trigger ran again in this chat for its run at %s.", trigger.Name, at)
	default:
		intro = fmt.Sprintf("This chat was started by the %q schedule trigger for its run at %s.", trigger.Name, at)
	}
	if trigger.Prompt == "" {
		return intro
	}
	return trigger.Prompt + "\n\n" + intro
}

// @Summary Run a schedule trigger now
// @Description Starts a run of a schedule trigger outside its schedule. The
// @Description run does not change when the trigger runs next.
// @ID run-chat-project-schedule-trigger
// @Security CoderSessionToken
// @Tags Chats
// @Produce json
// @Param organization path string true "Organization ID" format(uuid)
// @Param project path string true "Chat project ID" format(uuid)
// @Param trigger path string true "Trigger ID" format(uuid)
// @Success 202 {object} codersdk.ChatProjectTriggerWebhookResponse
// @Router /api/experimental/organizations/{organization}/chats/projects/{project}/triggers/{trigger}/run [post]
// @x-apidocgen {"skip": true}
func (api *API) postChatProjectTriggerRun(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !api.requireChatDaemon(ctx, rw) {
		return
	}
	trigger, ok := api.chatProjectTriggerParam(rw, r)
	if !ok {
		return
	}
	if trigger.Source != string(codersdk.ChatProjectTriggerSourceSchedule) {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Only schedule triggers can be run from Coder. Other triggers run when their webhook is called.",
		})
		return
	}
	outcome, err := api.runChatProjectScheduleTrigger(ctx, trigger, chatProjectScheduleTriggerEventRunNow, api.Clock.Now())
	if err != nil {
		if writeChatHookErr(ctx, rw, err, "Chat message denied by lifecycle hook.") {
			return
		}
		status, resp := chatProjectScheduleTriggerFailure(err, outcome.continued)
		httpapi.Write(ctx, rw, status, resp)
		return
	}
	httpapi.Write(ctx, rw, http.StatusAccepted, codersdk.ChatProjectTriggerWebhookResponse{
		ChatID:    outcome.chat.ID,
		Continued: outcome.continued,
	})
}
