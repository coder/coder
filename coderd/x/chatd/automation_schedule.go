package chatd

import (
	"context"
	"errors"
	"time"

	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/x/chatd/chatautomation"
)

const (
	// scheduleClaimBatch bounds the rows that one claim locks.
	scheduleClaimBatch = 100
	// scheduleMissedAfter is how late a claimed occurrence can be and still
	// run. Each replica scans every automationTickInterval, so only an
	// occurrence that came due while no replica scanned, such as during
	// downtime, is later than this.
	scheduleMissedAfter = 2 * automationTickInterval
)

// tickSchedules runs every schedule occurrence that is due at now.
func (p *Server) tickSchedules(ctx context.Context, now time.Time) {
	//nolint:gocritic // The scheduler claims the schedules of every owner.
	ctx = dbauthz.AsChatd(ctx)
	for {
		claimed, err := p.claimDueSchedules(ctx, now)
		if err != nil {
			if ctx.Err() == nil {
				p.logger.Warn(ctx, "claim due chat automation schedules", slog.Error(err))
			}
			return
		}
		for _, automation := range claimed {
			p.runScheduleOccurrence(ctx, automation, now)
		}
		if len(claimed) < scheduleClaimBatch {
			return
		}
	}
}

// claimDueSchedules locks the schedules that are due at now and moves each
// one to its first occurrence after now, so that no other claim returns it
// for the same occurrence. It returns the rows as they were before the move.
func (p *Server) claimDueSchedules(ctx context.Context, now time.Time) ([]database.ChatAutomation, error) {
	var claimed []database.ChatAutomation
	err := p.db.InTx(func(tx database.Store) error {
		rows, err := tx.ClaimDueChatAutomationSchedules(ctx, database.ClaimDueChatAutomationSchedulesParams{
			Now:        now,
			LimitCount: scheduleClaimBatch,
		})
		if err != nil {
			return xerrors.Errorf("claim schedules: %w", err)
		}
		for _, row := range rows {
			if err := tx.AdvanceChatAutomationNextRunAt(ctx, database.AdvanceChatAutomationNextRunAtParams{
				ID:                row.ID,
				ScheduleNextRunAt: chatautomation.NextRunAt(row, now),
			}); err != nil {
				return xerrors.Errorf("advance schedule %s: %w", row.ID, err)
			}
		}
		claimed = rows
		return nil
	}, nil)
	return claimed, err
}

// runScheduleOccurrence runs one claimed occurrence. An occurrence claimed
// too late is skipped as missed and not replayed.
func (p *Server) runScheduleOccurrence(ctx context.Context, automation database.ChatAutomation, now time.Time) {
	if now.Sub(automation.ScheduleNextRunAt.Time) > scheduleMissedAfter {
		p.recordAutomationRun(ctx, automation, AutomationRunResult{
			Outcome: runReasonOutcome[runReasonMissed],
			Reason:  string(runReasonMissed),
			ChatID:  automation.TargetChatID,
		})
		return
	}
	_, err := p.runAutomation(ctx, automation, automationRun{trigger: automationTriggerSchedule})
	if err != nil && !errors.Is(err, ErrAutomationChanged) && ctx.Err() == nil {
		p.logger.Warn(ctx, "run chat automation schedule",
			slog.F("automation_id", automation.ID),
			slog.Error(err),
		)
	}
}
