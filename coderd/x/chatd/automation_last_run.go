package chatd

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/x/chatd/chathooks"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/codersdk"
)

// automationLastRunWriteTimeout bounds the last-run write, which runs after
// the outcome is known and must not hold up the caller for long.
const automationLastRunWriteTimeout = 5 * time.Second

// classifyAutomationRun maps a publish result to the last-run outcome and
// reason. record is false for results that are not a run of the stored
// automation: the automation changed or disappeared under the publish, the
// secret was rotated, or the server is stopping.
func classifyAutomationRun(err error) (outcome database.ChatAutomationRunOutcome, reason codersdk.ChatAutomationRunReason, record bool) {
	var denied *chathooks.UserPromptDeniedError
	skipped := func(r codersdk.ChatAutomationRunReason) (database.ChatAutomationRunOutcome, codersdk.ChatAutomationRunReason, bool) {
		return database.ChatAutomationRunOutcomeSkipped, r, true
	}
	refused := func(r codersdk.ChatAutomationRunReason) (database.ChatAutomationRunOutcome, codersdk.ChatAutomationRunReason, bool) {
		return database.ChatAutomationRunOutcomeRefused, r, true
	}
	switch {
	case err == nil:
		return database.ChatAutomationRunOutcomeAccepted, "", true
	case errors.Is(err, ErrAutomationScheduleStale),
		errors.Is(err, ErrAutomationNotFound),
		errors.Is(err, ErrAutomationSecretChanged),
		errors.Is(err, context.Canceled),
		errors.Is(err, context.DeadlineExceeded):
		return "", "", false
	case errors.Is(err, ErrAutomationChatBusy):
		return skipped(codersdk.ChatAutomationRunReasonChatBusy)
	case errors.Is(err, ErrAutomationQueueShareFull), errors.Is(err, chatstate.ErrMessageQueueFull):
		return skipped(codersdk.ChatAutomationRunReasonQueueFull)
	case errors.Is(err, ErrAutomationOccurrenceExpired):
		return skipped(codersdk.ChatAutomationRunReasonMissed)
	case errors.Is(err, ErrAutomationPreviousRunActive):
		return skipped(codersdk.ChatAutomationRunReasonPreviousRunActive)
	case errors.As(err, &denied):
		return refused(codersdk.ChatAutomationRunReasonHookDenied)
	case errors.Is(err, ErrAutomationOwnerInactive):
		return refused(codersdk.ChatAutomationRunReasonOwnerInactive)
	case errors.Is(err, ErrAutomationForbidden), dbauthz.IsNotAuthorizedError(err):
		return refused(codersdk.ChatAutomationRunReasonForbidden)
	case errors.Is(err, ErrAutomationModelUnavailable):
		return refused(codersdk.ChatAutomationRunReasonModelUnavailable)
	case errors.Is(err, ErrAutomationTargetUnavailable):
		return refused(codersdk.ChatAutomationRunReasonTargetUnavailable)
	case errors.Is(err, ErrAutomationsExperimentDisabled):
		return refused(codersdk.ChatAutomationRunReasonExperimentDisabled)
	case errors.Is(err, ErrAutomationDisabled):
		return refused(codersdk.ChatAutomationRunReasonDisabled)
	case errors.Is(err, ErrAutomationWebhookConsumed):
		return refused(codersdk.ChatAutomationRunReasonSingleUseConsumed)
	case errors.Is(err, ErrAutomationEventTooLarge):
		return refused(codersdk.ChatAutomationRunReasonBodyTooLarge)
	case errors.Is(err, ErrNoDefaultChatModelConfig):
		return refused(codersdk.ChatAutomationRunReasonNoDefaultModel)
	default:
		return refused(codersdk.ChatAutomationRunReasonError)
	}
}

// automationRunTrigger names what started a run, for the run's audit
// entry and metrics.
type automationRunTrigger string

const (
	automationRunTriggerSchedule automationRunTrigger = "schedule"
	automationRunTriggerWebhook  automationRunTrigger = "webhook"
	automationRunTriggerRunNow   automationRunTrigger = "run_now"
)

// automationRun is the decided outcome of one run.
type automationRun struct {
	automationID uuid.UUID
	trigger      automationRunTrigger
	outcome      database.ChatAutomationRunOutcome
	reason       codersdk.ChatAutomationRunReason
	// chatID is the chat the input went to or was meant for, uuid.Nil
	// when it is not known.
	chatID uuid.UUID
	// inputID identifies an accepted input, uuid.Nil otherwise.
	inputID uuid.UUID
	request AutomationRequest
}

// recordAutomationRunResult records the outcome of a publish that ended
// with err: the automation's last run and the run's audit entry.
func (p *Server) recordAutomationRunResult(ctx context.Context, trigger automationRunTrigger, request AutomationRequest, automationID uuid.UUID, result PublishAutomationResult, err error) {
	outcome, reason, record := classifyAutomationRun(err)
	if !record {
		return
	}
	chatID := result.ChatID
	// The skip names the chat that is still active.
	if active, ok := errors.AsType[*AutomationPreviousRunActiveError](err); ok {
		chatID = active.ChatID
	}
	p.recordAutomationRun(ctx, automationRun{
		automationID: automationID,
		trigger:      trigger,
		outcome:      outcome,
		reason:       reason,
		chatID:       chatID,
		inputID:      result.InputID,
		request:      request,
	})
}

// RecordAutomationWebhookRefusal records an authenticated webhook delivery
// that coderd refused before it reached chatd, for example because its
// body was too large or not JSON. Callers must have verified the secret.
func (p *Server) RecordAutomationWebhookRefusal(ctx context.Context, automationID uuid.UUID, reason codersdk.ChatAutomationRunReason, request AutomationRequest) {
	p.recordAutomationRun(ctx, automationRun{
		automationID: automationID,
		trigger:      automationRunTriggerWebhook,
		outcome:      database.ChatAutomationRunOutcomeRefused,
		reason:       reason,
		request:      request,
	})
}

// recordAutomationRun writes the automation's last run and the run's audit
// entry. It is best effort: the outcome is already decided, so a failed
// write is only logged, and it runs even when the caller's context is
// canceled, for example after a webhook sender disconnected.
func (p *Server) recordAutomationRun(ctx context.Context, run automationRun) {
	writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), automationLastRunWriteTimeout)
	defer cancel()
	logger := p.logger.With(
		slog.F("automation_id", run.automationID),
		slog.F("trigger", run.trigger),
		slog.F("outcome", run.outcome),
		slog.F("reason", run.reason),
	)
	//nolint:gocritic // Publishers have no user identity; chatd records the outcome of every owner's runs.
	chatdCtx := dbauthz.AsChatd(writeCtx)
	_, err := p.db.UpdateChatAutomationLastRunByID(chatdCtx, database.UpdateChatAutomationLastRunByIDParams{
		ID:             run.automationID,
		LastRunAt:      dbtime.Time(p.clock.Now()),
		LastRunOutcome: run.outcome,
		LastRunReason:  sql.NullString{String: string(run.reason), Valid: run.reason != ""},
		LastRunChatID:  uuid.NullUUID{UUID: run.chatID, Valid: run.chatID != uuid.Nil},
	})
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		logger.Warn(ctx, "record chat automation last run", slog.Error(err))
	}
	p.auditAutomationRun(chatdCtx, logger, run)
}

// auditAutomationRun records one start entry for the automation per run,
// attributed to the owner, whose authority the run used. A deleted
// automation is not audited: there is no row to name.
func (p *Server) auditAutomationRun(ctx context.Context, logger slog.Logger, run automationRun) {
	if p.chatWorker == nil || p.chatWorker.opts.Auditor == nil {
		return
	}
	auditor := p.chatWorker.opts.Auditor.Load()
	if auditor == nil {
		return
	}
	automation, err := p.db.GetChatAutomationByID(ctx, run.automationID)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			logger.Warn(ctx, "load chat automation for run audit", slog.Error(err))
		}
		return
	}
	additional := map[string]string{
		"trigger": string(run.trigger),
		"outcome": string(run.outcome),
		"reason":  string(run.reason),
	}
	if run.inputID != uuid.Nil {
		additional["input_id"] = run.inputID.String()
	}
	if run.chatID != uuid.Nil {
		additional["chat_id"] = run.chatID.String()
	}
	// Marshaling a map[string]string cannot fail.
	fields, _ := json.Marshal(additional)
	audit.BackgroundAudit(ctx, &audit.BackgroundAuditParams[database.ChatAutomation]{
		Audit:          *auditor,
		Log:            logger,
		UserID:         automation.OwnerID,
		OrganizationID: automation.OrganizationID,
		RequestID:      run.request.RequestID,
		IP:             run.request.IP,
		UserAgent:      run.request.UserAgent,
		Action:         database.AuditActionStart,
		Status:         automationRunStatus(run.outcome, run.reason),
		// A run changes no configuration, so the entry has no diff.
		Old:              automation,
		New:              automation,
		AdditionalFields: fields,
	})
}

// automationRunStatus is the HTTP status of a run: the status the webhook
// and Run now routes return for the outcome, and the same status for a
// scheduled run, which has no response.
func automationRunStatus(outcome database.ChatAutomationRunOutcome, reason codersdk.ChatAutomationRunReason) int {
	if outcome == database.ChatAutomationRunOutcomeAccepted {
		return http.StatusAccepted
	}
	switch reason {
	case codersdk.ChatAutomationRunReasonQueueFull:
		return http.StatusTooManyRequests
	case codersdk.ChatAutomationRunReasonHookDenied,
		codersdk.ChatAutomationRunReasonOwnerInactive,
		codersdk.ChatAutomationRunReasonForbidden,
		codersdk.ChatAutomationRunReasonDisabled:
		return http.StatusForbidden
	case codersdk.ChatAutomationRunReasonExperimentDisabled:
		return http.StatusNotFound
	case codersdk.ChatAutomationRunReasonBodyTooLarge:
		return http.StatusRequestEntityTooLarge
	case codersdk.ChatAutomationRunReasonInvalidBody,
		codersdk.ChatAutomationRunReasonNoDefaultModel:
		return http.StatusBadRequest
	case codersdk.ChatAutomationRunReasonError:
		return http.StatusInternalServerError
	default:
		// chat_busy, missed, previous_run_active, single_use_consumed,
		// target_unavailable, model_unavailable.
		return http.StatusConflict
	}
}
