package usage

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	agplusage "github.com/coder/coder/v2/coderd/usage"
)

const (
	historicalCatchupBatchSize  = 7 * 24 * time.Hour
	historicalCatchupBatchDelay = 100 * time.Millisecond
	historicalCatchupRetryDelay = time.Minute
	historicalCatchupTimerName  = "agent-runtime-history-catchup"
)

type historicalCatchupBatchResult struct {
	lockAcquired bool
	initialized  bool
	complete     bool

	start          time.Time
	end            time.Time
	next           time.Time
	endExclusive   time.Time
	inserted       int
	existing       int
	invalidBuckets []time.Time
	duration       time.Duration
}

func (g *Generator) runHistoricalCatchup(ctx context.Context) {
	//nolint:gocritic // We are a publisher in this function.
	ctx = dbauthz.AsUsagePublisher(ctx)
	defer g.wg.Done()

	for {
		result, err := g.generateHistoricalCatchupBatch(ctx)
		if ctx.Err() != nil {
			return
		}

		delay := historicalCatchupBatchDelay
		switch {
		case err != nil:
			g.log.Warn(ctx, "generate historical agent runtime usage events",
				slog.F("batch_start", result.start),
				slog.F("batch_end", result.end),
				slog.F("next_bucket", result.next),
				slog.F("end_exclusive", result.endExclusive),
				slog.F("duration", result.duration),
				slog.Error(err),
			)
			delay = historicalCatchupRetryDelay
		case !result.lockAcquired:
			g.log.Debug(ctx, "historical agent runtime catch-up lock unavailable")
			delay = historicalCatchupRetryDelay
		case result.complete:
			return
		}

		timer := g.clock.NewTimer(delay, historicalCatchupTimerName)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return
		case <-timer.C:
		}
	}
}

func (g *Generator) generateHistoricalCatchupBatch(ctx context.Context) (historicalCatchupBatchResult, error) {
	startedAt := g.clock.Now()
	result := historicalCatchupBatchResult{}

	err := g.db.InTx(func(tx database.Store) error {
		// InTx can retry a transaction, so discard counters from an aborted
		// attempt before recording the committed attempt.
		result = historicalCatchupBatchResult{}
		locked, err := tx.TryAcquireLock(ctx, database.LockIDAgentRuntimeHistoricalBackfill)
		if err != nil {
			return xerrors.Errorf("acquire historical catch-up lock: %w", err)
		}
		result.lockAcquired = locked
		if !locked {
			return nil
		}

		checkpoint, err := tx.GetAgentRuntimeBackfillCheckpoint(ctx)
		if err != nil {
			return xerrors.Errorf("read historical catch-up checkpoint: %w", err)
		}
		if !checkpoint.Present {
			return xerrors.New("historical catch-up checkpoint is missing")
		}
		state, err := agplusage.ParseAgentRuntimeBackfillState(checkpoint.Value)
		if err != nil {
			return xerrors.Errorf("parse historical catch-up checkpoint: %w", err)
		}
		if state.Status == agplusage.AgentRuntimeBackfillStatusComplete {
			result.complete = true
			result.next = *state.NextBucket
			result.endExclusive = *state.EndExclusive
			return nil
		}

		if state.Status == agplusage.AgentRuntimeBackfillStatusPending {
			state, err = g.initializeHistoricalCatchup(ctx, tx)
			if err != nil {
				return err
			}
			result.initialized = true
			result.next = *state.NextBucket
			result.endExclusive = *state.EndExclusive
			if state.Status == agplusage.AgentRuntimeBackfillStatusComplete {
				if err := updateHistoricalCatchupCheckpoint(ctx, tx, state); err != nil {
					return err
				}
				result.complete = true
				return nil
			}
		}

		result.start = *state.NextBucket
		result.endExclusive = *state.EndExclusive
		result.end = minTime(result.start.Add(historicalCatchupBatchSize), result.endExclusive)
		if !result.start.Before(result.end) {
			completedAt := g.clock.Now().UTC()
			state.Status = agplusage.AgentRuntimeBackfillStatusComplete
			state.NextBucket = state.EndExclusive
			state.CompletedAt = &completedAt
			if err := updateHistoricalCatchupCheckpoint(ctx, tx, state); err != nil {
				return err
			}
			result.complete = true
			result.next = result.endExclusive
			return nil
		}

		buckets, err := tx.ListMissingChatMessageRuntimeBuckets(ctx, database.ListMissingChatMessageRuntimeBucketsParams{
			StartTime: result.start,
			EndTime:   result.end,
		})
		if err != nil {
			return xerrors.Errorf("list historical runtime buckets: %w", err)
		}
		result.existing = int(result.end.Sub(result.start)/AgentRuntimeInterval) - len(buckets)
		for _, bucket := range buckets {
			bucketTime := bucket.Bucket.UTC()
			if bucket.RuntimeMs < 0 {
				result.invalidBuckets = append(result.invalidBuckets, bucketTime)
				continue
			}
			inserted, err := g.insertAgentRuntimeEvent(ctx, tx, bucketTime, bucket.RuntimeMs)
			if err != nil {
				return xerrors.Errorf("insert historical bucket %s: %w", bucketTime, err)
			}
			if inserted {
				result.inserted++
			} else {
				result.existing++
			}
		}

		state.NextBucket = &result.end
		result.next = result.end
		if result.end.Equal(result.endExclusive) {
			completedAt := g.clock.Now().UTC()
			state.Status = agplusage.AgentRuntimeBackfillStatusComplete
			state.CompletedAt = &completedAt
			result.complete = true
		}
		return updateHistoricalCatchupCheckpoint(ctx, tx, state)
	}, database.DefaultTXOptions().WithID("agent_runtime_historical_backfill"))

	result.duration = g.clock.Since(startedAt)
	if err != nil {
		return result, err
	}
	if !result.lockAcquired {
		return result, nil
	}

	if result.initialized {
		g.log.Info(ctx, "initialized historical agent runtime catch-up",
			slog.F("next_bucket", result.next),
			slog.F("end_exclusive", result.endExclusive),
			slog.F("completed", result.complete),
		)
	}
	for _, bucket := range result.invalidBuckets {
		g.log.Warn(ctx, "skip negative historical agent runtime bucket",
			slog.F("bucket", bucket),
		)
	}
	if !result.start.IsZero() {
		g.log.Info(ctx, "committed historical agent runtime catch-up batch",
			slog.F("batch_start", result.start),
			slog.F("batch_end", result.end),
			slog.F("buckets_inserted", result.inserted),
			slog.F("buckets_existing", result.existing),
			slog.F("buckets_failed", len(result.invalidBuckets)),
			slog.F("next_bucket", result.next),
			slog.F("duration", result.duration),
			slog.F("completed", result.complete),
		)
	}
	if result.complete {
		g.log.Info(ctx, "completed historical agent runtime catch-up",
			slog.F("end_exclusive", result.endExclusive),
			slog.F("completed_at", g.clock.Now().UTC()),
		)
	}
	return result, nil
}

func (g *Generator) initializeHistoricalCatchup(ctx context.Context, tx database.Store) (agplusage.AgentRuntimeBackfillState, error) {
	endExclusive := g.initialRecentWindowStart
	if endExclusive.IsZero() {
		endExclusive = g.clock.Now().UTC().Truncate(AgentRuntimeInterval).Add(-AgentRuntimeWindow)
	}
	earliest, err := tx.GetEarliestChatMessageRuntimeBucket(ctx)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return agplusage.AgentRuntimeBackfillState{}, xerrors.Errorf("find earliest historical runtime bucket: %w", err)
	}

	earliest = earliest.UTC().Truncate(AgentRuntimeInterval)
	state := agplusage.AgentRuntimeBackfillState{
		Version:      agplusage.AgentRuntimeBackfillVersion,
		Status:       agplusage.AgentRuntimeBackfillStatusRunning,
		NextBucket:   &earliest,
		EndExclusive: &endExclusive,
	}
	if errors.Is(err, sql.ErrNoRows) || !earliest.Before(endExclusive) {
		completedAt := g.clock.Now().UTC()
		state.Status = agplusage.AgentRuntimeBackfillStatusComplete
		state.NextBucket = &endExclusive
		state.CompletedAt = &completedAt
	}
	return state, nil
}

func updateHistoricalCatchupCheckpoint(ctx context.Context, tx database.Store, state agplusage.AgentRuntimeBackfillState) error {
	value, err := agplusage.MarshalAgentRuntimeBackfillState(state)
	if err != nil {
		return xerrors.Errorf("marshal historical catch-up checkpoint: %w", err)
	}
	updated, err := tx.UpdateAgentRuntimeBackfillCheckpoint(ctx, value)
	if err != nil {
		return xerrors.Errorf("update historical catch-up checkpoint: %w", err)
	}
	if updated != 1 {
		return xerrors.Errorf("update historical catch-up checkpoint: expected 1 row, updated %d", updated)
	}
	return nil
}

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}
