package chatd

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"maps"
	"net/http"
	"time"

	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/schedule/cron"
	"github.com/coder/coder/v2/coderd/x/chatd/chathooks"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/codersdk"
)

const (
	// automationScheduleInterval is how often every instance scans for
	// due schedule automations by default.
	automationScheduleInterval = 30 * time.Second
	// automationScheduleGrace is how late an occurrence may still be
	// accepted. Older occurrences are missed and never replayed.
	automationScheduleGrace = 60 * time.Second
	// automationScheduleClaimLease is how long an instance holds its claim
	// on an occurrence, which it takes before the prompt hooks run, so
	// instances that scan the same due row do not all run the hooks.
	//
	// The lease is longer than the grace window, so the claim keeps the
	// prompt hooks from running twice for one occurrence however skewed
	// the clocks of the instances are. An instance claims only an
	// occurrence it sees as due, at now_A >= cursor by its clock. Another
	// instance can claim the same occurrence only once the lease has
	// expired by its clock, at now_B >= now_A + lease > cursor + grace,
	// and its check before the hooks then refuses the occurrence as
	// expired. An instance that stops while it holds a claim loses that
	// occurrence: it is missed once the lease expires.
	automationScheduleClaimLease = 2 * automationScheduleGrace
	// automationScheduleConcurrency bounds the occurrences one scan
	// publishes at once, so an occurrence that waits for a lock or a slow
	// hook does not hold back the others past the grace window.
	automationScheduleConcurrency = 8
)

var (
	// ErrAutomationScheduleStale is returned when a scheduled occurrence
	// no longer matches the automation: its schedule revision or cursor
	// changed, or the occurrence is not due yet.
	ErrAutomationScheduleStale = xerrors.New("chat automation schedule occurrence is stale")
	// ErrAutomationOccurrenceExpired is returned when a scheduled
	// occurrence is older than the grace window once the locks are held.
	ErrAutomationOccurrenceExpired = xerrors.New("chat automation schedule occurrence expired")
)

// automationOccurrence is a scheduled run as the scanner observed it.
type automationOccurrence struct {
	revision int64
	cursor   time.Time
}

// checkAutomationOccurrence requires the locked automation to still have
// the observed schedule revision and cursor, and the occurrence to be due
// and within the grace window at now.
//
// now comes from the local clock of the instance that holds the locks,
// not from the database, so instances with skewed clocks judge time
// differently. Skew cannot cause a double acceptance or a replay: an
// acceptance commits only together with the cursor write, which is
// conditional on the observed revision and cursor under the automation
// lock, and every cursor write moves the cursor strictly forward (see
// advanceAutomationSchedule). So each occurrence is accepted at most
// once, and an occurrence behind the cursor can never be accepted again.
// A clock that is off by d only shifts the window in which an occurrence
// can be accepted to [cursor-d, cursor+grace+d] in true time, and a
// clock that runs ahead can skip the occurrences within d of true time.
func checkAutomationOccurrence(automation database.ChatAutomation, occurrence automationOccurrence, now time.Time) error {
	if automation.Kind != database.ChatAutomationKindSchedule ||
		automation.ScheduleRevision != occurrence.revision ||
		!automation.ScheduleNextRunAt.Valid ||
		!automation.ScheduleNextRunAt.Time.Equal(occurrence.cursor) ||
		occurrence.cursor.After(now) {
		return ErrAutomationScheduleStale
	}
	if now.Sub(occurrence.cursor) > automationScheduleGrace {
		return ErrAutomationOccurrenceExpired
	}
	return nil
}

// advanceAutomationSchedule moves the automation's cursor from the
// observed occurrence to next, or clears it when next is zero. It reports
// false when the schedule revision or cursor changed since they were
// observed. The cursor only moves forward: a next at or before the
// observed cursor, for example after the local clock stepped back, is
// refused.
func advanceAutomationSchedule(ctx context.Context, store database.Store, automationID uuid.UUID, occurrence automationOccurrence, next, now time.Time) (bool, error) {
	if !next.IsZero() && !next.After(occurrence.cursor) {
		return false, xerrors.Errorf("advance chat automation schedule: next run %s is not after the cursor %s", next, occurrence.cursor)
	}
	//nolint:gocritic // The scheduler moves the cursors of every owner's automations; chatd may update them.
	count, err := store.AdvanceChatAutomationScheduleCursor(dbauthz.AsChatd(ctx), database.AdvanceChatAutomationScheduleCursorParams{
		ID:                automationID,
		ScheduleRevision:  occurrence.revision,
		ObservedNextRunAt: occurrence.cursor,
		NextRunAt:         sql.NullTime{Time: dbtime.Time(next), Valid: !next.IsZero()},
		UpdatedAt:         now,
	})
	if err != nil {
		return false, xerrors.Errorf("advance chat automation schedule: %w", err)
	}
	return count == 1, nil
}

// claimAutomationOccurrence claims the observed occurrence of the
// automation until claimedUntil, judging other claims at now. It reports
// false when another instance holds an unexpired claim on it or the
// schedule revision or cursor changed since they were observed. Moving
// the cursor drops the claim.
func claimAutomationOccurrence(ctx context.Context, store database.Store, automationID uuid.UUID, occurrence automationOccurrence, now, claimedUntil time.Time) (bool, error) {
	//nolint:gocritic // The scheduler claims the occurrences of every owner's automations; chatd may update them.
	count, err := store.ClaimChatAutomationScheduleOccurrence(dbauthz.AsChatd(ctx), database.ClaimChatAutomationScheduleOccurrenceParams{
		ID:                automationID,
		ScheduleRevision:  occurrence.revision,
		ObservedNextRunAt: occurrence.cursor,
		ClaimedUntil:      claimedUntil,
		Now:               now,
	})
	if err != nil {
		return false, xerrors.Errorf("claim chat automation schedule occurrence: %w", err)
	}
	return count == 1, nil
}

// releaseAutomationOccurrence drops the claim claimAutomationOccurrence
// took until claimedUntil, so a later scan can retry the occurrence. It
// leaves a claim another instance took after this one expired.
func releaseAutomationOccurrence(ctx context.Context, store database.Store, automationID uuid.UUID, occurrence automationOccurrence, claimedUntil time.Time) error {
	//nolint:gocritic // The scheduler releases the claims it took on every owner's automations.
	_, err := store.ReleaseChatAutomationScheduleClaim(dbauthz.AsChatd(ctx), database.ReleaseChatAutomationScheduleClaimParams{
		ID:                automationID,
		ScheduleRevision:  occurrence.revision,
		ObservedNextRunAt: occurrence.cursor,
		ClaimedUntil:      claimedUntil,
	})
	if err != nil {
		return xerrors.Errorf("release chat automation schedule claim: %w", err)
	}
	return nil
}

// automationScheduleLoop scans for due schedule automations at start and
// then every AutomationScheduleInterval.
func (w *chatWorker) automationScheduleLoop(ctx context.Context) {
	w.server.scanAutomationSchedules(ctx, w.opts.AutomationScheduleBatchSize)

	ticker := w.opts.Clock.NewTicker(w.opts.AutomationScheduleInterval, "chatworker", "automation-schedules")
	defer ticker.Stop("chatworker", "automation-schedules")
	for {
		select {
		case <-ticker.C:
			ticker.Stop("chatworker", "automation-schedules")
			w.server.scanAutomationSchedules(ctx, w.opts.AutomationScheduleBatchSize)
			ticker.Reset(w.opts.AutomationScheduleInterval, "chatworker", "automation-schedules")
		case <-ctx.Done():
			return
		}
	}
}

// scanAutomationSchedules runs every due schedule occurrence once. The
// due rows are read without locks; each publish rechecks its occurrence
// under the chat and automation locks, so concurrent scans on several
// instances accept each occurrence at most once. Each instance claims an
// occurrence before its prompt hooks run, so the hooks also run once.
//
// The due rows are read in pages of batchSize until a page comes back
// short. Rows that stay due, such as those of owners with the experiment
// off, therefore never hide the rows behind them. Occurrences are
// published concurrently, at most automationScheduleConcurrency at a
// time, and the scan returns once all of them are done.
func (p *Server) scanAutomationSchedules(ctx context.Context, batchSize int32) {
	var publishes errgroup.Group
	publishes.SetLimit(automationScheduleConcurrency)
	defer func() { _ = publishes.Wait() }()
	now := dbtime.Time(p.clock.Now())
	// The experiment is decided once per owner per scan. Owners with it
	// off keep their cursors; a cursor that falls behind is missed once
	// the experiment is on again.
	enabled := make(map[uuid.UUID]bool)
	after := database.GetDueChatAutomationSchedulesParams{Now: now, LimitCount: batchSize, AfterID: uuid.Nil}
	for {
		//nolint:gocritic // The scheduler reads every owner's due schedules; each publish runs as the owner.
		rows, err := p.db.GetDueChatAutomationSchedules(dbauthz.AsChatd(ctx), after)
		if err != nil {
			if ctx.Err() == nil {
				p.logger.Warn(ctx, "read due chat automation schedules", slog.Error(err))
			}
			return
		}
		for _, row := range rows {
			if ctx.Err() != nil {
				return
			}
			on, ok := enabled[row.OwnerID]
			if !ok {
				on = AutomationsEnabled(ctx, p.experimentEvaluator, row.OwnerID)
				enabled[row.OwnerID] = on
			}
			if on {
				publishes.Go(func() error {
					p.runAutomationOccurrence(ctx, row, now)
					return nil
				})
			}
		}
		if len(rows) < int(batchSize) {
			return
		}
		last := rows[len(rows)-1]
		after.AfterNextRunAt, after.AfterID = last.ScheduleNextRunAt.Time, last.ID
	}
}

// runAutomationOccurrence publishes the occurrence at the cursor of a due
// schedule automation that the scan read at now, or moves the cursor past
// an occurrence that is missed or refused.
//
// A missed occurrence, one older than the grace window, is not replayed.
// The cursor moves to the first cron time within the grace window instead,
// and when that time is already due it is published in the same call, so
// a late scan still runs an occurrence that is on time.
//
// An occurrence is claimed before it is published. When another instance
// holds the claim, this call leaves the occurrence to it. Moving the
// cursor drops the claim, and a publish that fails with a retryable error
// releases it, so the next scan retries within the grace window.
func (p *Server) runAutomationOccurrence(ctx context.Context, row database.ChatAutomation, now time.Time) {
	occurrence := automationOccurrence{revision: row.ScheduleRevision, cursor: row.ScheduleNextRunAt.Time}
	occurrenceLogger := func(occurrence automationOccurrence) slog.Logger {
		return p.logger.With(
			slog.F("automation_id", row.ID),
			slog.F("schedule_revision", occurrence.revision),
			slog.F("scheduled_at", occurrence.cursor),
		)
	}
	logger := occurrenceLogger(occurrence)
	sched, err := cron.Standard(row.ScheduleCron.String, row.ScheduleTimeZone.String)
	if err != nil {
		logger.Warn(ctx, "parse chat automation schedule", slog.Error(err))
		return
	}
	// skip moves the cursor past the occurrence to the first cron time
	// after a fresh clock read minus lookback, unless someone else moved
	// it first. It returns the new cursor and whether it moved.
	skip := func(lookback time.Duration, msg string, fields ...slog.Field) (time.Time, bool) {
		skippedAt := dbtime.Time(p.clock.Now())
		next := sched.Next(skippedAt.Add(-lookback))
		moved, err := advanceAutomationSchedule(ctx, p.db, row.ID, occurrence, next, skippedAt)
		if err != nil {
			logger.Warn(ctx, "skip chat automation schedule occurrence", slog.Error(err))
			return time.Time{}, false
		}
		if moved {
			logger.Info(ctx, msg, append(fields, slog.F("next_run_at", next))...)
		}
		return next, moved
	}

	if now.Sub(occurrence.cursor) > automationScheduleGrace {
		// The occurrence is older than the grace window, so the first
		// cron time within the window is after the cursor.
		next, moved := skip(automationScheduleGrace, "chat automation schedule occurrence missed")
		if !moved || next.IsZero() || next.After(dbtime.Time(p.clock.Now())) {
			return
		}
		// The new cursor is within the grace window by construction, so
		// this continues at most once.
		occurrence = automationOccurrence{revision: occurrence.revision, cursor: next}
		logger = occurrenceLogger(occurrence)
	}
	claimedAt := dbtime.Time(p.clock.Now())
	claimedUntil := claimedAt.Add(automationScheduleClaimLease)
	claimed, err := claimAutomationOccurrence(ctx, p.db, row.ID, occurrence, claimedAt, claimedUntil)
	if err != nil {
		if ctx.Err() == nil {
			logger.Warn(ctx, "claim chat automation schedule occurrence", slog.Error(err))
		}
		return
	}
	if !claimed {
		// Another instance publishes it, or the automation changed since
		// the scan.
		logger.Debug(ctx, "chat automation schedule occurrence not claimed")
		return
	}
	result, err := p.publishAutomation(ctx, automationPublish{
		automationID: row.ID,
		content: func(automation database.ChatAutomation) []codersdk.ChatMessagePart {
			return []codersdk.ChatMessagePart{codersdk.ChatMessageText(automation.Prompt)}
		},
		occurrence: &occurrence,
	})
	var denied *chathooks.UserPromptDeniedError
	switch {
	case err == nil:
		logger.Info(ctx, "chat automation schedule occurrence accepted",
			slog.F("chat_id", result.ChatID), slog.F("input_id", result.InputID))
		if row.TargetMode == database.ChatAutomationTargetModeNewChat {
			p.auditAutomationCreatedChat(ctx, logger, row, result, nil)
		}
	case errors.Is(err, ErrAutomationOccurrenceExpired):
		// The occurrence expired while waiting for the locks. A later
		// cron time may still be within the grace window.
		skip(automationScheduleGrace, "chat automation schedule occurrence skipped", slog.F("reason", err.Error()))
	case errors.Is(err, ErrAutomationChatBusy),
		errors.Is(err, ErrAutomationQueueShareFull),
		errors.Is(err, chatstate.ErrMessageQueueFull),
		errors.As(err, &denied),
		errors.Is(err, ErrAutomationForbidden),
		errors.Is(err, ErrAutomationModelUnavailable):
		skip(0, "chat automation schedule occurrence skipped", slog.F("reason", err.Error()))
	case errors.Is(err, ErrAutomationScheduleStale),
		errors.Is(err, ErrAutomationDisabled),
		errors.Is(err, ErrAutomationNotFound):
		// The automation changed since the scan; the next scan reads it
		// again.
	default:
		// The cursor stays, so the next scan retries while the occurrence
		// is within the grace window. This includes
		// ErrAutomationsExperimentDisabled: the evaluator also reports a
		// failed read as off, and the next scan drops an owner whose
		// experiment really is off.
		if ctx.Err() != nil {
			// The server is stopping, so a release would fail. The claim
			// expires on its own and the occurrence is missed.
			return
		}
		logger.Warn(ctx, "publish chat automation schedule occurrence", slog.Error(err))
		if err := releaseAutomationOccurrence(ctx, p.db, row.ID, occurrence, claimedUntil); err != nil {
			logger.Warn(ctx, "release chat automation schedule occurrence", slog.Error(err))
		}
	}
}

// auditAutomationCreatedChat records the chat that a new_chat automation
// created on its schedule or through the manage_automations tool, as chat
// creation through the chat API and webhook deliveries do. The entry names
// the automation owner, whose authority created the chat, and carries the
// automation and input ids plus extraFields.
func (p *Server) auditAutomationCreatedChat(ctx context.Context, logger slog.Logger, automation database.ChatAutomation, result PublishAutomationResult, extraFields map[string]string) {
	if p.chatWorker == nil || p.chatWorker.opts.Auditor == nil {
		return
	}
	auditor := p.chatWorker.opts.Auditor.Load()
	if auditor == nil {
		return
	}
	//nolint:gocritic // The scheduler has no user identity; the entry needs the chat the owner's automation created.
	chat, err := p.db.GetChatByID(dbauthz.AsChatd(ctx), result.ChatID)
	if err != nil {
		logger.Warn(ctx, "load chat created by automation for audit", slog.F("chat_id", result.ChatID), slog.Error(err))
		return
	}
	additional := map[string]string{
		"automation_id": automation.ID.String(),
		"input_id":      result.InputID.String(),
	}
	maps.Copy(additional, extraFields)
	fields, err := json.Marshal(additional)
	if err != nil {
		fields = nil
	}
	audit.BackgroundAudit(ctx, &audit.BackgroundAuditParams[database.Chat]{
		Audit:            *auditor,
		Log:              logger,
		UserID:           automation.OwnerID,
		OrganizationID:   automation.OrganizationID,
		Action:           database.AuditActionCreate,
		New:              chat,
		Status:           http.StatusAccepted,
		AdditionalFields: fields,
	})
}
