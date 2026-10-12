package chatd

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"

	"cdr.dev/slog/v3"
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

// recordAutomationRunResult records the outcome of a publish that ended
// with err as the automation's last run. chatID is the chat the input went
// to, uuid.Nil when it is not known.
func (p *Server) recordAutomationRunResult(ctx context.Context, automationID, chatID uuid.UUID, err error) {
	outcome, reason, record := classifyAutomationRun(err)
	if !record {
		return
	}
	// The skip names the chat that is still active.
	if active, ok := errors.AsType[*AutomationPreviousRunActiveError](err); ok {
		chatID = active.ChatID
	}
	p.recordAutomationRun(ctx, automationID, outcome, reason, chatID)
}

// RecordAutomationWebhookRefusal records an authenticated webhook delivery
// that coderd refused before it reached chatd, for example because its
// body was too large or not JSON. Callers must have verified the secret.
func (p *Server) RecordAutomationWebhookRefusal(ctx context.Context, automationID uuid.UUID, reason codersdk.ChatAutomationRunReason) {
	p.recordAutomationRun(ctx, automationID, database.ChatAutomationRunOutcomeRefused, reason, uuid.Nil)
}

// recordAutomationRun writes the automation's last run. It is best effort:
// the outcome is already decided, so a failed write is only logged, and
// it runs even when the caller's context is canceled, for example after a
// webhook sender disconnected.
func (p *Server) recordAutomationRun(ctx context.Context, automationID uuid.UUID, outcome database.ChatAutomationRunOutcome, reason codersdk.ChatAutomationRunReason, chatID uuid.UUID) {
	writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), automationLastRunWriteTimeout)
	defer cancel()
	//nolint:gocritic // Publishers have no user identity; chatd records the outcome of every owner's runs.
	_, err := p.db.UpdateChatAutomationLastRunByID(dbauthz.AsChatd(writeCtx), database.UpdateChatAutomationLastRunByIDParams{
		ID:             automationID,
		LastRunAt:      dbtime.Time(p.clock.Now()),
		LastRunOutcome: outcome,
		LastRunReason:  sql.NullString{String: string(reason), Valid: reason != ""},
		LastRunChatID:  uuid.NullUUID{UUID: chatID, Valid: chatID != uuid.Nil},
	})
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		p.logger.Warn(ctx, "record chat automation last run",
			slog.F("automation_id", automationID),
			slog.F("outcome", outcome),
			slog.F("reason", reason),
			slog.Error(err))
	}
}
