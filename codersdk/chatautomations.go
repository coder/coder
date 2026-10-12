package codersdk

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
)

// ChatAutomationKind is what starts a chat automation.
type ChatAutomationKind string

const (
	ChatAutomationKindWebhook  ChatAutomationKind = "webhook"
	ChatAutomationKindSchedule ChatAutomationKind = "schedule"
)

// ChatAutomationTargetMode is where a chat automation delivers its prompt.
type ChatAutomationTargetMode string

const (
	ChatAutomationTargetModeExistingChat ChatAutomationTargetMode = "existing_chat"
	ChatAutomationTargetModeNewChat      ChatAutomationTargetMode = "new_chat"
)

// ChatAutomationWebhookUse is how many times a webhook automation can be
// triggered.
type ChatAutomationWebhookUse string

const (
	ChatAutomationWebhookUseSingle ChatAutomationWebhookUse = "single"
	ChatAutomationWebhookUseMulti  ChatAutomationWebhookUse = "multi"
)

// ChatAutomationWhenBusy is what an existing_chat automation does when the
// target chat is busy.
type ChatAutomationWhenBusy string

const (
	ChatAutomationWhenBusyQueue ChatAutomationWhenBusy = "queue"
	ChatAutomationWhenBusySkip  ChatAutomationWhenBusy = "skip"
)

// ChatAutomationPausedReason is a condition that stops an automation from
// running even while it is enabled.
type ChatAutomationPausedReason string

const (
	// ChatAutomationPausedReasonOwnerInactive means the owner is suspended,
	// dormant, or deleted.
	ChatAutomationPausedReasonOwnerInactive ChatAutomationPausedReason = "owner_inactive"
	// ChatAutomationPausedReasonExperimentDisabled means the owner's
	// chat-automations experiment is off.
	ChatAutomationPausedReasonExperimentDisabled ChatAutomationPausedReason = "experiment_disabled"
	// ChatAutomationPausedReasonTargetUnavailable means the target chat of an
	// existing_chat automation is deleted, archived, or is no longer a root
	// chat of the owner in the automation's organization.
	ChatAutomationPausedReasonTargetUnavailable ChatAutomationPausedReason = "target_unavailable"
	// ChatAutomationPausedReasonModelUnavailable means the model config of a
	// new_chat automation is unset, deleted, disabled, in another
	// organization, or not readable by the owner, or its provider is
	// disabled or deleted. It is not evaluated while the owner is inactive.
	ChatAutomationPausedReasonModelUnavailable ChatAutomationPausedReason = "model_unavailable"
	// ChatAutomationPausedReasonOwnerNotMember means the owner is no longer
	// a member of the automation's organization. It is not reported while
	// the owner is inactive.
	ChatAutomationPausedReasonOwnerNotMember ChatAutomationPausedReason = "owner_not_member"
)

// ChatAutomationRunOutcome is how the latest run of an automation ended.
type ChatAutomationRunOutcome string

const (
	// ChatAutomationRunOutcomeAccepted means the input was delivered or
	// queued.
	ChatAutomationRunOutcomeAccepted ChatAutomationRunOutcome = "accepted"
	// ChatAutomationRunOutcomeSkipped means the input was dropped because
	// of the target's state at that moment: a busy chat that skips, a full
	// queue, a missed schedule occurrence, or an earlier run that is still
	// active. Later runs can succeed without a change.
	ChatAutomationRunOutcomeSkipped ChatAutomationRunOutcome = "skipped"
	// ChatAutomationRunOutcomeRefused means a condition of the automation,
	// its owner, or the input refused the run. Later runs fail the same
	// way until someone fixes it.
	ChatAutomationRunOutcomeRefused ChatAutomationRunOutcome = "refused"
)

// ChatAutomationRunReason is the machine reason of a skipped or refused
// run.
type ChatAutomationRunReason string

const (
	ChatAutomationRunReasonChatBusy  ChatAutomationRunReason = "chat_busy"
	ChatAutomationRunReasonQueueFull ChatAutomationRunReason = "queue_full"
	ChatAutomationRunReasonMissed    ChatAutomationRunReason = "missed"
	// ChatAutomationRunReasonPreviousRunActive skips a scheduled run of a
	// new_chat automation while the chat of an earlier run is still
	// running, so a stalled model cannot pile up chats. Webhook deliveries
	// and Run now are not skipped for it.
	ChatAutomationRunReasonPreviousRunActive  ChatAutomationRunReason = "previous_run_active"
	ChatAutomationRunReasonHookDenied         ChatAutomationRunReason = "hook_denied"
	ChatAutomationRunReasonOwnerInactive      ChatAutomationRunReason = "owner_inactive"
	ChatAutomationRunReasonForbidden          ChatAutomationRunReason = "forbidden"
	ChatAutomationRunReasonModelUnavailable   ChatAutomationRunReason = "model_unavailable"
	ChatAutomationRunReasonTargetUnavailable  ChatAutomationRunReason = "target_unavailable"
	ChatAutomationRunReasonExperimentDisabled ChatAutomationRunReason = "experiment_disabled"
	ChatAutomationRunReasonDisabled           ChatAutomationRunReason = "disabled"
	ChatAutomationRunReasonSingleUseConsumed  ChatAutomationRunReason = "single_use_consumed"
	ChatAutomationRunReasonBodyTooLarge       ChatAutomationRunReason = "body_too_large"
	ChatAutomationRunReasonInvalidBody        ChatAutomationRunReason = "invalid_body"
	ChatAutomationRunReasonNoDefaultModel     ChatAutomationRunReason = "no_default_model"
	ChatAutomationRunReasonError              ChatAutomationRunReason = "error"
)

// ChatAutomationLastRun is the outcome of an automation's latest schedule
// occurrence, Run now, or authenticated webhook delivery. It is one record,
// not a run history: each run replaces it. Schedule occurrences that never
// start, because the automation is paused (see PausedReasons), leave it
// unchanged.
type ChatAutomationLastRun struct {
	At      time.Time                `json:"at" format:"date-time"`
	Outcome ChatAutomationRunOutcome `json:"outcome" enums:"accepted,skipped,refused"`
	// Reason is set for skipped and refused runs.
	Reason *ChatAutomationRunReason `json:"reason,omitempty" enums:"chat_busy,queue_full,missed,previous_run_active,hook_denied,owner_inactive,forbidden,model_unavailable,target_unavailable,experiment_disabled,disabled,single_use_consumed,body_too_large,invalid_body,no_default_model,error"`
	// ChatID is the chat the run went to, or was meant for when the target
	// chat was known. For previous_run_active it is the chat that is still
	// active. The chat can be deleted since.
	ChatID *uuid.UUID `json:"chat_id,omitempty" format:"uuid"`
}

// ChatAutomationChat identifies a chat that a chat automation refers to.
// Archived lets clients show an archived target, which refuses runs,
// without reading the chat.
type ChatAutomationChat struct {
	ID       uuid.UUID `json:"id" format:"uuid"`
	Title    string    `json:"title"`
	Archived bool      `json:"archived"`
}

// ChatAutomation is a webhook or scheduled automation that delivers a
// prompt to an agent chat. It never carries the webhook secret or its
// hash. ProjectID is the project of the chats a new_chat automation
// creates; it is cleared when the project is deleted.
type ChatAutomation struct {
	ID             uuid.UUID `json:"id" format:"uuid"`
	OrganizationID uuid.UUID `json:"organization_id" format:"uuid"`
	OwnerID        uuid.UUID `json:"owner_id" format:"uuid"`
	// Owner is the display data of the automation owner. It is set by the
	// chat automations API only when the caller can read the owner's
	// membership in the automation's organization, the same check as
	// GET /api/v2/organizations/{organization}/members/{user}. It is absent
	// otherwise, including for deleted owners and owners who left the
	// organization.
	Owner           *MinimalUser `json:"owner,omitempty"`
	Name            string       `json:"name"`
	CreatedByChatID *uuid.UUID   `json:"created_by_chat_id,omitempty" format:"uuid"`
	// CreatedByChat is the chat that created the automation. It is set by
	// the chat automations API only when the caller can read that chat, so
	// it is absent for automations created outside a chat, for deleted
	// chats, and for chats the caller cannot read.
	CreatedByChat *ChatAutomationChat      `json:"created_by_chat,omitempty"`
	Kind          ChatAutomationKind       `json:"kind" enums:"webhook,schedule"`
	Enabled       bool                     `json:"enabled"`
	TargetMode    ChatAutomationTargetMode `json:"target_mode" enums:"existing_chat,new_chat"`
	TargetChatID  *uuid.UUID               `json:"target_chat_id,omitempty" format:"uuid"`
	// TargetChat is the target chat of an existing_chat automation. Like
	// CreatedByChat, it is set only when the caller can read that chat.
	TargetChat           *ChatAutomationChat       `json:"target_chat,omitempty"`
	NewChatModelConfigID *uuid.UUID                `json:"new_chat_model_config_id,omitempty" format:"uuid"`
	ReasoningEffort      *string                   `json:"reasoning_effort,omitempty"`
	WhenBusy             *ChatAutomationWhenBusy   `json:"when_busy,omitempty" enums:"queue,skip"`
	ProjectID            *uuid.UUID                `json:"project_id,omitempty" format:"uuid"`
	WebhookUse           *ChatAutomationWebhookUse `json:"webhook_use,omitempty" enums:"single,multi"`
	WebhookSecretVersion int64                     `json:"webhook_secret_version"`
	WebhookConsumedAt    *time.Time                `json:"webhook_consumed_at,omitempty" format:"date-time"`
	Prompt               string                    `json:"prompt"`
	ScheduleCron         *string                   `json:"schedule_cron,omitempty"`
	ScheduleTimeZone     *string                   `json:"schedule_time_zone,omitempty"`
	// ScheduleNextRunAt is the scheduler cursor, not a promise that a run
	// happens then. While the automation is paused it can stay in the past,
	// or keep moving past occurrences that are refused, for example while
	// its model is unavailable.
	ScheduleNextRunAt *time.Time `json:"schedule_next_run_at,omitempty" format:"date-time"`
	// NextRunTimes lists up to five upcoming runs of an enabled schedule.
	// It is empty for webhooks, disabled schedules, and schedules with any
	// paused reason.
	NextRunTimes []time.Time `json:"next_run_times" format:"date-time"`
	// PausedReasons lists conditions that stop the automation from running
	// even while it is enabled: schedule occurrences, Run now, and webhook
	// deliveries are refused. It is set by the chat automations API and
	// absent when none was found. It is computed independently of Enabled.
	// It is an advisory snapshot: admission stays authoritative, and an
	// empty list does not guarantee that a run is accepted, because lost
	// chat permissions, busy chats, and full queues are not reported.
	PausedReasons []ChatAutomationPausedReason `json:"paused_reasons,omitempty" enums:"owner_inactive,experiment_disabled,target_unavailable,model_unavailable,owner_not_member"`
	// ScheduleIntervalBelowMinimum is true when the schedule runs more
	// often than the deployment's minimum interval between runs. Such
	// automations keep running, but a change to their schedule must meet
	// the minimum.
	ScheduleIntervalBelowMinimum bool `json:"schedule_interval_below_minimum"`
	// LastRun is the outcome of the latest run, null until the first one.
	LastRun   *ChatAutomationLastRun `json:"last_run"`
	CreatedAt time.Time              `json:"created_at" format:"date-time"`
	UpdatedAt time.Time              `json:"updated_at" format:"date-time"`
}

// CreateChatAutomationRequest creates a chat automation owned by the
// caller. ProjectID applies to new_chat automations only, requires the
// chat-projects experiment, and puts the chats the automation creates into
// that project.
type CreateChatAutomationRequest struct {
	Name                 string                    `json:"name"`
	Kind                 ChatAutomationKind        `json:"kind" enums:"webhook,schedule"`
	TargetMode           ChatAutomationTargetMode  `json:"target_mode" enums:"existing_chat,new_chat"`
	TargetChatID         *uuid.UUID                `json:"target_chat_id,omitempty" format:"uuid"`
	NewChatModelConfigID *uuid.UUID                `json:"new_chat_model_config_id,omitempty" format:"uuid"`
	ReasoningEffort      *string                   `json:"reasoning_effort,omitempty"`
	WhenBusy             *ChatAutomationWhenBusy   `json:"when_busy,omitempty" enums:"queue,skip"`
	ProjectID            *uuid.UUID                `json:"project_id,omitempty" format:"uuid"`
	WebhookUse           *ChatAutomationWebhookUse `json:"webhook_use,omitempty" enums:"single,multi"`
	Prompt               string                    `json:"prompt"`
	// ScheduleCron is a standard five-field cron expression. As in standard
	// cron, when both day of month and day of week are restricted, a time
	// matches if either field matches.
	ScheduleCron     *string `json:"schedule_cron,omitempty"`
	ScheduleTimeZone *string `json:"schedule_time_zone,omitempty"`
}

// CreateChatAutomationResponse is returned when a chat automation is
// created. WebhookSecret is set only for webhook automations. Only this
// response and RotateChatAutomationSecretResponse ever contain a secret.
type CreateChatAutomationResponse struct {
	Automation    ChatAutomation `json:"automation"`
	WebhookSecret string         `json:"webhook_secret,omitempty"`
}

// UpdateChatAutomationRequest changes the set fields of a chat automation.
// The kind, target mode, and webhook use of an automation cannot change:
// the API rejects a request that sets one of them, or any unknown field,
// with 400. A request that changes nothing leaves updated_at unchanged.
type UpdateChatAutomationRequest struct {
	// Enabled disables or re-enables the automation. Disabling removes the
	// messages the automation queued that have not started. Re-enabling a
	// schedule resumes at its next future occurrence; occurrences missed
	// while it was disabled do not run.
	Enabled          *bool   `json:"enabled,omitempty"`
	Name             *string `json:"name,omitempty"`
	Prompt           *string `json:"prompt,omitempty"`
	ScheduleCron     *string `json:"schedule_cron,omitempty"`
	ScheduleTimeZone *string `json:"schedule_time_zone,omitempty"`
	// ReasoningEffort sets the effort of new chats. An empty string clears
	// the override, so new chats use the model's default.
	ReasoningEffort      *string                 `json:"reasoning_effort,omitempty"`
	WhenBusy             *ChatAutomationWhenBusy `json:"when_busy,omitempty" enums:"queue,skip"`
	TargetChatID         *uuid.UUID              `json:"target_chat_id,omitempty" format:"uuid"`
	NewChatModelConfigID *uuid.UUID              `json:"new_chat_model_config_id,omitempty" format:"uuid"`
	// ProjectID applies to new_chat automations only. A project ID moves the
	// chats the automation creates later into that project. The all-zero
	// UUID removes the project. Omitted or null leaves it unchanged. Chats
	// created earlier keep their project.
	ProjectID *uuid.UUID `json:"project_id,omitempty" format:"uuid"`
}

// RotateChatAutomationSecretResponse is returned when a webhook
// automation's secret is rotated. The previous secret stops working, and
// the new secret cannot be read again.
type RotateChatAutomationSecretResponse struct {
	WebhookSecret        string `json:"webhook_secret"`
	WebhookSecretVersion int64  `json:"webhook_secret_version"`
}

// ChatAutomationSchedulePreviewRequest is a schedule to preview. It
// follows the same rules as the schedule of a chat automation.
type ChatAutomationSchedulePreviewRequest struct {
	ScheduleCron     string `json:"schedule_cron"`
	ScheduleTimeZone string `json:"schedule_time_zone"`
}

// ChatAutomationSchedulePreviewResponse lists the next runs of a previewed
// schedule.
type ChatAutomationSchedulePreviewResponse struct {
	NextRunTimes []time.Time `json:"next_run_times" format:"date-time"`
	// ClockChangeNote explains how the schedule runs across daylight
	// saving changes. It is set when the schedule's time zone changes its
	// UTC offset within a year.
	ClockChangeNote string `json:"clock_change_note,omitempty"`
	// IntervalWarning is set when the schedule runs more often than the
	// deployment's minimum interval. Creating such a schedule, or changing
	// an automation's schedule to it, fails, but an automation that
	// already has it keeps running.
	IntervalWarning string `json:"interval_warning,omitempty"`
}

// ChatAutomationEventResponse is returned when a webhook automation
// accepts an event. InputID identifies the accepted input on the message
// it created in ChatID.
type ChatAutomationEventResponse struct {
	InputID uuid.UUID `json:"input_id" format:"uuid"`
	ChatID  uuid.UUID `json:"chat_id" format:"uuid"`
}

// ChatAutomationRunResponse is returned when a schedule automation runs
// now. InputID identifies the accepted input on the message it created in
// ChatID.
type ChatAutomationRunResponse struct {
	InputID uuid.UUID `json:"input_id" format:"uuid"`
	ChatID  uuid.UUID `json:"chat_id" format:"uuid"`
}

// ChatAutomationReference names an automation that delivered into a chat.
// Anyone who can read the chat can read its references, so it carries no
// other automation fields.
type ChatAutomationReference struct {
	ID   uuid.UUID          `json:"id" format:"uuid"`
	Name string             `json:"name"`
	Kind ChatAutomationKind `json:"kind" enums:"webhook,schedule"`
}

func chatAutomationsPath(organizationID uuid.UUID) string {
	return fmt.Sprintf("/api/experimental/organizations/%s/chat-automations", organizationID)
}

// ChatAutomationsFilter narrows the chat automation list.
type ChatAutomationsFilter struct {
	// Owner keeps only the automations of one owner: "me", a username, or
	// a user ID. Empty lists every automation the caller can read. The
	// filter never shows an automation the caller cannot read.
	Owner string `json:"owner,omitempty"`
}

// ChatAutomations lists the chat automations in an organization that the
// caller can read, narrowed by filter.
func (c *ExperimentalClient) ChatAutomations(ctx context.Context, organizationID uuid.UUID, filter ChatAutomationsFilter) ([]ChatAutomation, error) {
	var opts []RequestOption
	if filter.Owner != "" {
		opts = append(opts, WithQueryParam("owner", filter.Owner))
	}
	res, err := c.Request(ctx, http.MethodGet, chatAutomationsPath(organizationID), nil, opts...)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, ReadBodyAsError(res)
	}
	var automations []ChatAutomation
	return automations, ReadBodyAsJSON(res, &automations)
}

// CreateChatAutomation creates a chat automation owned by the caller.
func (c *ExperimentalClient) CreateChatAutomation(ctx context.Context, organizationID uuid.UUID, req CreateChatAutomationRequest) (CreateChatAutomationResponse, error) {
	res, err := c.Request(ctx, http.MethodPost, chatAutomationsPath(organizationID), req)
	if err != nil {
		return CreateChatAutomationResponse{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		return CreateChatAutomationResponse{}, ReadBodyAsError(res)
	}
	var resp CreateChatAutomationResponse
	return resp, ReadBodyAsJSON(res, &resp)
}

// ChatAutomation returns a chat automation.
func (c *ExperimentalClient) ChatAutomation(ctx context.Context, organizationID, automationID uuid.UUID) (ChatAutomation, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("%s/%s", chatAutomationsPath(organizationID), automationID), nil)
	if err != nil {
		return ChatAutomation{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return ChatAutomation{}, ReadBodyAsError(res)
	}
	var automation ChatAutomation
	return automation, ReadBodyAsJSON(res, &automation)
}

// UpdateChatAutomation changes a chat automation. Only its owner can
// update it, except that anyone allowed to update it can disable it.
func (c *ExperimentalClient) UpdateChatAutomation(ctx context.Context, organizationID, automationID uuid.UUID, req UpdateChatAutomationRequest) (ChatAutomation, error) {
	res, err := c.Request(ctx, http.MethodPatch, fmt.Sprintf("%s/%s", chatAutomationsPath(organizationID), automationID), req)
	if err != nil {
		return ChatAutomation{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return ChatAutomation{}, ReadBodyAsError(res)
	}
	var automation ChatAutomation
	return automation, ReadBodyAsJSON(res, &automation)
}

// DeleteChatAutomation deletes a chat automation.
func (c *ExperimentalClient) DeleteChatAutomation(ctx context.Context, organizationID, automationID uuid.UUID) error {
	res, err := c.Request(ctx, http.MethodDelete, fmt.Sprintf("%s/%s", chatAutomationsPath(organizationID), automationID), nil)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		return ReadBodyAsError(res)
	}
	return nil
}

// RotateChatAutomationSecret replaces the webhook secret of a webhook
// automation and returns the new secret. Only its owner can rotate it.
func (c *ExperimentalClient) RotateChatAutomationSecret(ctx context.Context, organizationID, automationID uuid.UUID) (RotateChatAutomationSecretResponse, error) {
	res, err := c.Request(ctx, http.MethodPost, fmt.Sprintf("%s/%s/secret/rotate", chatAutomationsPath(organizationID), automationID), nil)
	if err != nil {
		return RotateChatAutomationSecretResponse{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return RotateChatAutomationSecretResponse{}, ReadBodyAsError(res)
	}
	var resp RotateChatAutomationSecretResponse
	return resp, ReadBodyAsJSON(res, &resp)
}

// RunChatAutomation sends the saved prompt of an enabled schedule
// automation to its target now. The schedule's next run does not change.
// Only its owner can run it.
func (c *ExperimentalClient) RunChatAutomation(ctx context.Context, organizationID, automationID uuid.UUID) (ChatAutomationRunResponse, error) {
	res, err := c.Request(ctx, http.MethodPost, fmt.Sprintf("%s/%s/runs", chatAutomationsPath(organizationID), automationID), nil)
	if err != nil {
		return ChatAutomationRunResponse{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusAccepted {
		return ChatAutomationRunResponse{}, ReadBodyAsError(res)
	}
	var resp ChatAutomationRunResponse
	return resp, ReadBodyAsJSON(res, &resp)
}

// ChatAutomationSchedulePreview returns the next runs of a schedule
// without creating an automation.
func (c *ExperimentalClient) ChatAutomationSchedulePreview(ctx context.Context, organizationID uuid.UUID, req ChatAutomationSchedulePreviewRequest) (ChatAutomationSchedulePreviewResponse, error) {
	res, err := c.Request(ctx, http.MethodPost, chatAutomationsPath(organizationID)+"/schedule-preview", req)
	if err != nil {
		return ChatAutomationSchedulePreviewResponse{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return ChatAutomationSchedulePreviewResponse{}, ReadBodyAsError(res)
	}
	var resp ChatAutomationSchedulePreviewResponse
	return resp, ReadBodyAsJSON(res, &resp)
}

// ChatAutomationReferences lists the automations that delivered into a
// chat. The caller needs only read access to the chat.
func (c *ExperimentalClient) ChatAutomationReferences(ctx context.Context, chatID uuid.UUID) ([]ChatAutomationReference, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/experimental/chats/%s/automations", chatID), nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, ReadBodyAsError(res)
	}
	var references []ChatAutomationReference
	return references, ReadBodyAsJSON(res, &references)
}
