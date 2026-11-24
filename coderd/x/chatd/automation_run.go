package chatd

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/x/agenthooks/dispatch"
	"github.com/coder/coder/v2/coderd/x/chatd/chatautomation"
	"github.com/coder/coder/v2/coderd/x/chatd/chathooks"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/codersdk"
)

// automationTrigger is what started an automation run.
type automationTrigger string

const (
	automationTriggerSchedule automationTrigger = "schedule"
	automationTriggerRunNow   automationTrigger = "run_now"
)

// runReason says why a run was not accepted. It is stored as the reason of
// the last run.
type runReason string

const (
	runReasonChatBusy           runReason = "chat_busy"
	runReasonMissed             runReason = "missed"
	runReasonOverlap            runReason = "overlap"
	runReasonOwnerInactive      runReason = "owner_inactive"
	runReasonExperimentDisabled runReason = "experiment_disabled"
	runReasonTargetUnavailable  runReason = "target_unavailable"
	runReasonTargetForbidden    runReason = "target_forbidden"
	runReasonModelUnavailable   runReason = "model_unavailable"
	runReasonPromptHookDenied   runReason = "prompt_hook_denied"
)

// runReasonOutcome is skipped when a later run can succeed without a change,
// and refused when a person must correct something first.
var runReasonOutcome = map[runReason]database.ChatAutomationRunOutcome{
	runReasonChatBusy:           database.ChatAutomationRunOutcomeSkipped,
	runReasonMissed:             database.ChatAutomationRunOutcomeSkipped,
	runReasonOverlap:            database.ChatAutomationRunOutcomeSkipped,
	runReasonOwnerInactive:      database.ChatAutomationRunOutcomeRefused,
	runReasonExperimentDisabled: database.ChatAutomationRunOutcomeRefused,
	runReasonTargetUnavailable:  database.ChatAutomationRunOutcomeRefused,
	runReasonTargetForbidden:    database.ChatAutomationRunOutcomeRefused,
	runReasonModelUnavailable:   database.ChatAutomationRunOutcomeRefused,
	runReasonPromptHookDenied:   database.ChatAutomationRunOutcomeRefused,
}

// ErrAutomationChanged means that the automation was deleted, disabled or
// changed after the run read it. The run sends nothing and records nothing.
var ErrAutomationChanged = xerrors.New("the chat automation changed during the run")

var (
	errAutomationChatBusy      = xerrors.New("the target chat is busy")
	errAutomationOverlap       = xerrors.New("a chat of the automation is still active")
	errAutomationTargetMissing = xerrors.New("the target chat no longer exists")
	errAutomationProjectDenied = xerrors.New("the owner can no longer use the project")
)

// AutomationRunResult is the outcome of one run.
type AutomationRunResult struct {
	Outcome database.ChatAutomationRunOutcome
	// Reason says why a run was skipped or refused. It is empty for an
	// accepted run.
	Reason string
	// ChatID is the target chat, or the chat that a new_chat run created.
	ChatID uuid.NullUUID
}

// RunAutomationNow runs a schedule automation once, as its owner, and
// records the outcome. The caller has authorized the run and passes the
// automation as it read it. ErrAutomationChanged means that the automation
// was disabled or changed since then.
func (p *Server) RunAutomationNow(ctx context.Context, automation database.ChatAutomation) (AutomationRunResult, error) {
	if !automation.Enabled {
		return AutomationRunResult{}, ErrAutomationChanged
	}
	return p.runAutomation(ctx, automation, automationTriggerRunNow)
}

// runAutomation sends the automation's prompt as its owner and records the
// outcome. An error means the run has no outcome and records nothing.
func (p *Server) runAutomation(ctx context.Context, automation database.ChatAutomation, trigger automationTrigger) (AutomationRunResult, error) {
	//nolint:gocritic // A run reads and records automations as chatd and sends as the owner.
	ctx = dbauthz.AsChatd(ctx)
	result := AutomationRunResult{Outcome: database.ChatAutomationRunOutcomeAccepted}
	chatID, err := p.deliverAutomation(ctx, automation, trigger)
	if err != nil {
		reason, ok := runReasonForError(err)
		if !ok {
			return AutomationRunResult{}, err
		}
		result.Outcome = runReasonOutcome[reason]
		result.Reason = string(reason)
	}
	result.ChatID = chatID
	p.recordAutomationRun(ctx, automation.ID, result)
	return result, nil
}

// recordAutomationRun writes the last run. It runs after the run's
// transaction, so a run never writes an automation row while it holds a
// chat row. A failed write does not change the outcome.
func (p *Server) recordAutomationRun(ctx context.Context, id uuid.UUID, result AutomationRunResult) {
	err := p.db.UpdateChatAutomationLastRun(ctx, database.UpdateChatAutomationLastRunParams{
		ID:             id,
		LastRunAt:      dbtime.Time(p.clock.Now("chatd", "automation-run")),
		LastRunOutcome: result.Outcome,
		LastRunReason:  sql.NullString{String: result.Reason, Valid: result.Reason != ""},
		LastRunChatID:  result.ChatID,
	})
	if err != nil && ctx.Err() == nil {
		p.logger.Warn(ctx, "record chat automation run", slog.F("automation_id", id), slog.Error(err))
	}
}

// deliverAutomation checks the owner and sends the prompt to the target. It
// returns the target chat with any error.
func (p *Server) deliverAutomation(ctx context.Context, automation database.ChatAutomation, trigger automationTrigger) (uuid.NullUUID, error) {
	owner, status, err := httpmw.UserRBACSubject(ctx, p.db, automation.OwnerID, rbac.ScopeAll)
	if err != nil {
		return automation.TargetChatID, xerrors.Errorf("load automation owner: %w", err)
	}
	switch {
	case status != database.UserStatusActive:
		return automation.TargetChatID, errAutomationOwnerInactive
	case p.automationsEnabled == nil || !p.automationsEnabled(ctx, automation.OwnerID):
		return automation.TargetChatID, errAutomationExperimentOff
	}
	ownerCtx := dbauthz.As(ctx, owner)
	content := automationMessageContent(automation, owner.FriendlyName, trigger)
	if automation.TargetMode == database.ChatAutomationTargetModeNewChat {
		return p.deliverToNewChat(ctx, ownerCtx, automation, trigger, content)
	}
	return automation.TargetChatID, p.deliverToChat(ctx, ownerCtx, automation, content)
}

var (
	errAutomationOwnerInactive = xerrors.New("the automation owner is not active")
	errAutomationExperimentOff = xerrors.New("chat automations are off for the owner")
)

// deliverToChat sends the prompt to an idle target chat. ctx acts as chatd
// and ownerCtx as the automation's owner.
func (p *Server) deliverToChat(ctx, ownerCtx context.Context, automation database.ChatAutomation, content []codersdk.ChatMessagePart) error {
	if !automation.TargetChatID.Valid {
		return errAutomationTargetMissing
	}
	chatID := automation.TargetChatID.UUID
	chat, err := p.db.GetChatByID(ctx, chatID)
	if errors.Is(err, sql.ErrNoRows) {
		return errAutomationTargetMissing
	}
	if err != nil {
		return xerrors.Errorf("get target chat: %w", err)
	}
	if chat.Archived {
		return ErrChatArchived
	}
	queue, err := chatstate.LoadQueueState(ctx, p.db, chatID)
	if err != nil {
		return xerrors.Errorf("load queue state: %w", err)
	}
	// Checking before the hooks keeps a busy chat from dispatching them.
	if !automationCanSend(chatstate.ClassifyExecutionState(chat, queue, true)) {
		return errAutomationChatBusy
	}

	opts := SendMessageOptions{
		ChatID:       chatID,
		CreatedBy:    automation.OwnerID,
		Content:      content,
		BusyBehavior: SendMessageBusyBehaviorQueue,
		AutomationID: uuid.NullUUID{UUID: automation.ID, Valid: true},
	}
	prepared, err := p.prepareSendMessage(ownerCtx, opts)
	if err != nil {
		return err
	}
	var (
		result           SendMessageResult
		promotedQueuedAt time.Time
	)
	refreshed, err := p.updateChat(ownerCtx, chatID, func(tx *chatstate.Tx, store database.Store, snapshot *chatstate.ChatSnapshot) error {
		if err := lockAutomationForRun(ctx, store, automation); err != nil {
			return err
		}
		if !automationCanSend(snapshot.State()) {
			return errAutomationChatBusy
		}
		var err error
		result, promotedQueuedAt, err = p.sendMessageLocked(ownerCtx, tx, store, snapshot, opts, prepared)
		return err
	})
	if err != nil {
		return err
	}
	result.Chat = refreshed
	p.finishSendMessage(ownerCtx, result, promotedQueuedAt)
	return nil
}

// deliverToNewChat creates a chat with the prompt. A schedule skips while an
// earlier chat of the automation is still active.
func (p *Server) deliverToNewChat(ctx, ownerCtx context.Context, automation database.ChatAutomation, trigger automationTrigger, content []codersdk.ChatMessagePart) (uuid.NullUUID, error) {
	checkOverlap := func(store database.Store) error {
		if trigger != automationTriggerSchedule {
			return nil
		}
		active, err := store.CountActiveChatsByAutomationID(ctx, automation.ID)
		if err != nil {
			return xerrors.Errorf("count active chats: %w", err)
		}
		if active > 0 {
			return errAutomationOverlap
		}
		return nil
	}
	// Checking before the hooks keeps an overlapping run from dispatching
	// them; the transaction checks again under the automation lock.
	if err := checkOverlap(p.db); err != nil {
		return uuid.NullUUID{}, err
	}
	if automation.ProjectID.Valid {
		project, err := p.db.GetChatProjectByID(ctx, automation.ProjectID.UUID)
		if err != nil {
			return uuid.NullUUID{}, xerrors.Errorf("get project: %w", err)
		}
		usable, err := ChatProjectUsableBy(ctx, p.db, project, automation.OwnerID)
		if err != nil {
			return uuid.NullUUID{}, err
		}
		if !usable {
			return uuid.NullUUID{}, errAutomationProjectDenied
		}
	}
	chat, err := p.CreateChat(ownerCtx, CreateOptions{
		OrganizationID:     automation.OrganizationID,
		OwnerID:            automation.OwnerID,
		ProjectID:          automation.ProjectID,
		Title:              automation.Name,
		ModelConfigID:      automation.NewChatModelConfigID.UUID,
		InitialUserContent: content,
		AutomationID:       uuid.NullUUID{UUID: automation.ID, Valid: true},
		BeforeInsert: func(store database.Store) error {
			if err := lockAutomationForRun(ctx, store, automation); err != nil {
				return err
			}
			return checkOverlap(store)
		},
	})
	if err != nil {
		return uuid.NullUUID{}, err
	}
	return uuid.NullUUID{UUID: chat.ID, Valid: true}, nil
}

// lockAutomationForRun takes the automation lock for the rest of the
// transaction, then checks that the automation is still the one that the run
// read. A disable, change or delete that commits first stops the run.
func lockAutomationForRun(ctx context.Context, store database.Store, read database.ChatAutomation) error {
	if err := store.AcquireLock(ctx, chatautomation.LockID(read.ID)); err != nil {
		return xerrors.Errorf("lock chat automation: %w", err)
	}
	current, err := store.GetChatAutomationByID(ctx, read.ID)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrAutomationChanged
	}
	if err != nil {
		return xerrors.Errorf("get chat automation: %w", err)
	}
	if !current.Enabled || !current.UpdatedAt.Equal(read.UpdatedAt) {
		return ErrAutomationChanged
	}
	return nil
}

// automationCanSend reports whether a run may send to a chat in state: only
// an idle chat with an empty queue, so that automations never queue, steer
// or interrupt.
func automationCanSend(state chatstate.ExecutionState) bool {
	return state == chatstate.StateW || state == chatstate.StateE0
}

// runReasonForError maps the error of a run step to the reason that the run
// was not accepted. ok is false for an error that is not an outcome.
func runReasonForError(err error) (reason runReason, ok bool) {
	_, denied := errors.AsType[*chathooks.UserPromptDeniedError](err)
	_, dispatchFailed := errors.AsType[*dispatch.Error](err)
	switch {
	case errors.Is(err, errAutomationChatBusy), errors.Is(err, chatstate.ErrMessageQueueFull):
		return runReasonChatBusy, true
	case errors.Is(err, errAutomationOverlap):
		return runReasonOverlap, true
	case errors.Is(err, errAutomationOwnerInactive):
		return runReasonOwnerInactive, true
	case errors.Is(err, errAutomationExperimentOff):
		return runReasonExperimentDisabled, true
	case errors.Is(err, errAutomationTargetMissing), errors.Is(err, chatstate.ErrChatNotFound), errors.Is(err, ErrChatArchived):
		return runReasonTargetUnavailable, true
	case errors.Is(err, errAutomationProjectDenied), dbauthz.IsNotAuthorizedError(err):
		return runReasonTargetForbidden, true
	case errors.Is(err, ErrInvalidModelConfigID), errors.Is(err, ErrNoDefaultChatModelConfig):
		return runReasonModelUnavailable, true
	case denied, dispatchFailed:
		return runReasonPromptHookDenied, true
	}
	return "", false
}

// automationMessageContent is the message of one run: a line that tells the
// agent which automation sent it, why and for whom, then the saved prompt.
func automationMessageContent(automation database.ChatAutomation, ownerUsername string, trigger automationTrigger) []codersdk.ChatMessagePart {
	cause := "its schedule came due"
	if trigger == automationTriggerRunNow {
		cause = "its owner ran it now"
	}
	return []codersdk.ChatMessagePart{
		codersdk.ChatMessageText(fmt.Sprintf(
			"The chat automation %q (%s) of @%s sent this message because %s. Perform the task below.",
			automation.Name, automation.ID, ownerUsername, cause,
		)),
		codersdk.ChatMessageText(automation.Prompt),
	}
}
