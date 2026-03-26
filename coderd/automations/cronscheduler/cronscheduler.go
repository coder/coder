package cronscheduler

import (
	"context"
	"encoding/json"
	"io"
	"time"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/automations"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/pproflabel"
	"github.com/coder/coder/v2/coderd/schedule/cron"
	"github.com/coder/quartz"
)

const tickInterval = time.Minute

// New creates a background scheduler that evaluates cron-based
// automation triggers every minute. Only one replica runs the
// scheduler at a time via an advisory lock.
func New(ctx context.Context, logger slog.Logger, db database.Store, clk quartz.Clock) io.Closer {
	closed := make(chan struct{})
	ctx, cancelFunc := context.WithCancel(ctx)
	//nolint:gocritic // System-level background job needs broad read access.
	ctx = dbauthz.AsSystemRestricted(ctx)

	inst := &instance{
		cancel: cancelFunc,
		closed: closed,
		logger: logger,
		db:     db,
		clk:    clk,
	}

	ticker := clk.NewTicker(tickInterval)
	doTick := func(ctx context.Context, now time.Time) {
		defer ticker.Reset(tickInterval)
		inst.tick(ctx, now)
	}

	pproflabel.Go(ctx, pproflabel.Service("automation-cron"), func(ctx context.Context) {
		defer close(closed)
		defer ticker.Stop()
		// Force an initial tick.
		doTick(ctx, dbtime.Time(clk.Now()).UTC())
		for {
			select {
			case <-ctx.Done():
				return
			case t := <-ticker.C:
				ticker.Stop()
				doTick(ctx, dbtime.Time(t).UTC())
			}
		}
	})
	return inst
}

type instance struct {
	cancel context.CancelFunc
	closed chan struct{}
	logger slog.Logger
	db     database.Store
	clk    quartz.Clock
}

func (i *instance) Close() error {
	i.cancel()
	<-i.closed
	return nil
}

// tick runs a single scheduler iteration. It acquires an advisory
// lock so that only one replica processes cron triggers at a time.
func (i *instance) tick(ctx context.Context, now time.Time) {
	err := i.db.InTx(func(tx database.Store) error {
		// Only one replica should evaluate cron triggers.
		ok, err := tx.TryAcquireLock(ctx, database.LockIDAutomationCron)
		if err != nil {
			return err
		}
		if !ok {
			i.logger.Debug(ctx, "unable to acquire automation cron lock, skipping")
			return nil
		}

		triggers, err := tx.GetActiveCronTriggers(ctx)
		if err != nil {
			return err
		}

		for _, t := range triggers {
			if err := ctx.Err(); err != nil {
				return err
			}
			i.processTrigger(ctx, tx, t, now)
		}
		return nil
	}, database.DefaultTXOptions().WithID("automation_cron"))
	if err != nil && ctx.Err() == nil {
		i.logger.Error(ctx, "automation cron tick failed", slog.Error(err))
	}
}

// processTrigger evaluates a single cron trigger and fires it if
// the schedule indicates it is due.
func (i *instance) processTrigger(ctx context.Context, db database.Store, trigger database.GetActiveCronTriggersRow, now time.Time) {
	logger := i.logger.With(
		slog.F("trigger_id", trigger.ID),
		slog.F("automation_id", trigger.AutomationID),
	)

	if !trigger.CronSchedule.Valid {
		return
	}

	sched, err := cron.Standard(trigger.CronSchedule.String)
	if err != nil {
		logger.Warn(ctx, "invalid cron schedule on trigger",
			slog.F("schedule", trigger.CronSchedule.String),
			slog.Error(err),
		)
		return
	}

	// Determine the reference time for computing "next fire".
	// If the trigger has never fired, use its creation time.
	ref := trigger.CreatedAt
	if trigger.LastTriggeredAt.Valid {
		ref = trigger.LastTriggeredAt.Time
	}

	next := sched.Next(ref)
	if next.After(now) {
		// Not yet due.
		return
	}

	// Build a synthetic payload for the cron event.
	payload, _ := json.Marshal(map[string]any{
		"trigger":      "cron",
		"schedule":     trigger.CronSchedule.String,
		"scheduled_at": next.UTC().Format(time.RFC3339),
		"fired_at":     now.UTC().Format(time.RFC3339),
	})

	// Resolve labels against the synthetic payload if configured.
	var resolvedLabelsJSON pqtype.NullRawMessage
	if trigger.LabelPaths.Valid {
		var labelPaths map[string]string
		if jsonErr := json.Unmarshal(trigger.LabelPaths.RawMessage, &labelPaths); jsonErr == nil && len(labelPaths) > 0 {
			resolved := automations.ResolveLabels(string(payload), labelPaths)
			if j, jErr := json.Marshal(resolved); jErr == nil {
				resolvedLabelsJSON = pqtype.NullRawMessage{RawMessage: j, Valid: true}
			}
		}
	}

	status := "created"
	if trigger.AutomationStatus == "preview" {
		status = "preview"
	}

	_, insertErr := db.InsertAutomationEvent(ctx, database.InsertAutomationEventParams{
		AutomationID:   trigger.AutomationID,
		TriggerID:      uuid.NullUUID{UUID: trigger.ID, Valid: true},
		Payload:        payload,
		FilterMatched:  true,
		ResolvedLabels: resolvedLabelsJSON,
		Status:         status,
	})
	if insertErr != nil {
		logger.Error(ctx, "failed to insert cron automation event", slog.Error(insertErr))
		return
	}

	// Update last_triggered_at so this trigger is not re-fired
	// until the next scheduled time.
	updateErr := db.UpdateAutomationTriggerLastTriggeredAt(ctx, database.UpdateAutomationTriggerLastTriggeredAtParams{
		LastTriggeredAt: now,
		ID:              trigger.ID,
	})
	if updateErr != nil {
		logger.Error(ctx, "failed to update last_triggered_at", slog.Error(updateErr))
	}

	logger.Info(ctx, "fired cron automation trigger",
		slog.F("status", status),
		slog.F("schedule", trigger.CronSchedule.String),
		slog.F("next_after_ref", next),
	)
}
