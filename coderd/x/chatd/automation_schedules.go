package chatd

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
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
	// automationScheduleBatchSize bounds the due automations one scan
	// reads.
	automationScheduleBatchSize = 500
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

// automationScheduleLoop scans for due schedule automations at start and
// then every AutomationScheduleInterval.
func (w *chatWorker) automationScheduleLoop(ctx context.Context) {
	w.server.scanAutomationSchedules(ctx)

	ticker := w.opts.Clock.NewTicker(w.opts.AutomationScheduleInterval, "chatworker", "automation-schedules")
	defer ticker.Stop("chatworker", "automation-schedules")
	for {
		select {
		case <-ticker.C:
			ticker.Stop("chatworker", "automation-schedules")
			w.server.scanAutomationSchedules(ctx)
			ticker.Reset(w.opts.AutomationScheduleInterval, "chatworker", "automation-schedules")
		case <-ctx.Done():
			return
		}
	}
}

// scanAutomationSchedules runs every due schedule occurrence once. The
// due rows are read without locks; each publish rechecks its occurrence
// under the chat and automation locks, so concurrent scans on several
// instances accept each occurrence at most once.
func (p *Server) scanAutomationSchedules(ctx context.Context) {
	now := dbtime.Time(p.clock.Now())
	//nolint:gocritic // The scheduler reads every owner's due schedules; each publish runs as the owner.
	rows, err := p.db.GetDueChatAutomationSchedules(dbauthz.AsChatd(ctx), database.GetDueChatAutomationSchedulesParams{
		Now:        now,
		LimitCount: automationScheduleBatchSize,
	})
	if err != nil {
		if ctx.Err() == nil {
			p.logger.Warn(ctx, "read due chat automation schedules", slog.Error(err))
		}
		return
	}
	// The experiment is decided once per owner per scan. Owners with it
	// off keep their cursors; a cursor that falls behind is missed once
	// the experiment is on again.
	enabled := make(map[uuid.UUID]bool)
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
			p.runAutomationOccurrence(ctx, row, now)
		}
	}
}

// runAutomationOccurrence publishes the occurrence at the cursor of a due
// schedule automation that the scan read at now, or moves the cursor past
// an occurrence that is missed or refused.
func (p *Server) runAutomationOccurrence(ctx context.Context, row database.ChatAutomation, now time.Time) {
	occurrence := automationOccurrence{revision: row.ScheduleRevision, cursor: row.ScheduleNextRunAt.Time}
	logger := p.logger.With(
		slog.F("automation_id", row.ID),
		slog.F("schedule_revision", occurrence.revision),
		slog.F("scheduled_at", occurrence.cursor),
	)
	sched, err := cron.Standard(row.ScheduleCron.String, row.ScheduleTimeZone.String)
	if err != nil {
		logger.Warn(ctx, "parse chat automation schedule", slog.Error(err))
		return
	}
	// skip moves the cursor to the first cron time after a fresh clock
	// read, unless someone else moved it first.
	skip := func(msg string, fields ...slog.Field) {
		skippedAt := dbtime.Time(p.clock.Now())
		next := sched.Next(skippedAt)
		moved, err := advanceAutomationSchedule(ctx, p.db, row.ID, occurrence, next, skippedAt)
		if err != nil {
			logger.Warn(ctx, "skip chat automation schedule occurrence", slog.Error(err))
			return
		}
		if moved {
			logger.Info(ctx, msg, append(fields, slog.F("next_run_at", next))...)
		}
	}

	if now.Sub(occurrence.cursor) > automationScheduleGrace {
		skip("chat automation schedule occurrence missed")
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
	case errors.Is(err, ErrAutomationChatBusy),
		errors.Is(err, ErrAutomationQueueShareFull),
		errors.Is(err, chatstate.ErrMessageQueueFull),
		errors.As(err, &denied),
		errors.Is(err, ErrAutomationForbidden),
		errors.Is(err, ErrAutomationsExperimentDisabled),
		errors.Is(err, ErrAutomationOccurrenceExpired):
		skip("chat automation schedule occurrence skipped", slog.F("reason", err.Error()))
	case errors.Is(err, ErrAutomationScheduleStale),
		errors.Is(err, ErrAutomationDisabled),
		errors.Is(err, ErrAutomationNotFound):
		// The automation changed since the scan; the next scan reads it
		// again.
	default:
		// The cursor stays, so the next scan retries while the occurrence
		// is within the grace window.
		if ctx.Err() == nil {
			logger.Warn(ctx, "publish chat automation schedule occurrence", slog.Error(err))
		}
	}
}
