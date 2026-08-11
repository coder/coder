package usage

import (
	"context"
	"math/rand"
	"sync"
	"time"

	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/pproflabel"
	agplusage "github.com/coder/coder/v2/coderd/usage"
	"github.com/coder/coder/v2/coderd/usage/usagetypes"
	"github.com/coder/quartz"
)

const (
	// AgentRuntimeInterval is the bucket size of hb_agent_runtime_v1 events.
	AgentRuntimeInterval = time.Hour
	// AgentRuntimeWindow is the trailing window scanned for missing buckets.
	// Buckets still missing beyond this window (e.g. because the deployment
	// was down for longer) are forfeited, which can only ever undercount
	// usage.
	AgentRuntimeWindow = 7 * 24 * time.Hour
	// AgentRuntimeEligibilityLag is how long after a bucket closes before it
	// becomes eligible for generation, giving replicas time to commit
	// in-flight chat messages with timestamps inside the bucket. This
	// assumes chat message inserts commit within the lag of their
	// statement-time created_at; a message committing later than the lag
	// after its bucket closes lands in an already-sealed bucket and its
	// runtime is dropped (undercount-only).
	AgentRuntimeEligibilityLag = 5 * time.Minute
	// agentRuntimeJitter staggers replicas after each hour boundary so one
	// is likely to complete the work before others attempt it.
	agentRuntimeJitter = 4 * time.Minute
	// agentRuntimeStartupDelay is the floor on the first pass after start,
	// giving the deployment time to finish booting before the generator
	// competes for database work. Jitter is added on top of it.
	agentRuntimeStartupDelay = time.Minute
	// generatorTimerName tags the quartz timer so tests can trap it.
	generatorTimerName = "agent-runtime-generator"
)

// Generator reconciles hb_agent_runtime_v1 heartbeat usage events. Unlike
// Cron jobs, which sample live state when they fire, the Generator derives
// events from data already persisted in the database, so it can
// deterministically backfill hours missed while the deployment was down,
// zero-filling idle hours. Deterministic event IDs make concurrent replicas
// safe without locking: a re-insert of a committed bucket is a no-op via the
// insert's ON CONFLICT (id) arbiter, and two replicas racing an uncommitted
// bucket surface a unique violation that generateBucket recognizes as the
// other replica winning.
//
// Events are generated unconditionally in enterprise builds; the
// publish_usage_data license flag only gates publishing to Tallyman.
type Generator struct {
	clock quartz.Clock
	log   slog.Logger
	db    database.Store
	ins   agplusage.Inserter

	cancel    context.CancelFunc
	wg        sync.WaitGroup
	startOnce sync.Once
}

// NewGenerator creates an unstarted Generator.
func NewGenerator(clock quartz.Clock, log slog.Logger, db database.Store, ins agplusage.Inserter) *Generator {
	return &Generator{
		clock: clock,
		log:   log,
		db:    db,
		ins:   ins,
	}
}

// Start launches the reconciliation goroutine. Subsequent calls are no-ops;
// a closed Generator cannot be restarted.
func (g *Generator) Start(ctx context.Context) {
	g.startOnce.Do(func() {
		ctx, g.cancel = context.WithCancel(ctx)
		g.wg.Add(1)
		pproflabel.Go(ctx, pproflabel.Service(pproflabel.ServiceUsageEventGenerator), func(ctx context.Context) {
			g.run(ctx)
		})
	})
}

// Close stops the Generator and waits for its goroutine to exit.
// It always returns nil; the error return exists to satisfy io.Closer, as the
// Generator is registered with the server's closer list.
func (g *Generator) Close() error {
	if g.cancel != nil {
		g.cancel()
	}
	g.wg.Wait()
	return nil
}

func (g *Generator) run(ctx context.Context) {
	//nolint:gocritic // We are a publisher in this function.
	ctx = dbauthz.AsUsagePublisher(ctx)
	defer g.wg.Done()

	// The random initial delay staggers replicas that start simultaneously.
	//nolint:gosec // Jitter does not need cryptographic randomness.
	delay := agentRuntimeStartupDelay + time.Duration(rand.Int63n(int64(agentRuntimeJitter)))
	for {
		timer := g.clock.NewTimer(delay, generatorTimerName)

		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return
		case <-timer.C:
		}

		err := g.generateAgentRuntimeEvents(ctx)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			g.log.Warn(ctx, "generate agent runtime usage events", slog.Error(err))
		}

		// Wake at the next eligibility instant (hour boundary + lag), not
		// the next hour boundary. Computing the tick against the
		// lag-shifted clock keeps a bucket whose eligibility is still
		// pending in this hour (e.g. a pass that ran just before HH:05)
		// from waiting a whole extra hour.
		_, delay = nextTick(g.clock.Now().Add(-AgentRuntimeEligibilityLag), AgentRuntimeInterval, agentRuntimeJitter)
	}
}

// generateAgentRuntimeEvents inserts one hb_agent_runtime_v1 event per
// missing hourly bucket in the trailing window. Per-bucket errors skip only
// that bucket; the next tick rescans the whole window, so transient failures
// self-heal.
func (g *Generator) generateAgentRuntimeEvents(ctx context.Context) error {
	now := g.clock.Now().UTC()
	// Bucket [H, H+1) becomes eligible at H + interval + lag.
	latestEligible := now.Add(-AgentRuntimeInterval - AgentRuntimeEligibilityLag).Truncate(AgentRuntimeInterval)
	earliest := now.Truncate(AgentRuntimeInterval).Add(-AgentRuntimeWindow)
	if latestEligible.Before(earliest) {
		return nil
	}

	existingTimes, err := g.db.ListUsageEventCreatedAtsByTypeSince(ctx, database.ListUsageEventCreatedAtsByTypeSinceParams{
		EventType: string(usagetypes.UsageEventTypeHBAgentRuntimeV1),
		Since:     earliest,
	})
	if err != nil {
		return xerrors.Errorf("list existing agent runtime events: %w", err)
	}
	// A row marks its bucket complete regardless of publish outcome, so a
	// bucket whose event Tallyman permanently rejected is never
	// regenerated (re-inserting under the deterministic ID is a no-op via
	// the insert's ON CONFLICT (id) arbiter).
	//
	// The runtime is not lost locally: the row still holds it, and the
	// event can be re-queued for publishing with
	//
	//	UPDATE usage_events
	//	SET published_at = NULL, publish_started_at = NULL, failure_message = NULL
	//	WHERE id = 'hb_agent_runtime_v1:<bucket start, e.g. 2026-07-15_14:00:00>';
	//
	// That re-arm only has an effect while the bucket is inside the
	// publisher's 30-day cutoff: SelectUsageEventsForPublishing also
	// filters created_at > now - INTERVAL '30 days', and created_at is the
	// bucket start, so past that the UPDATE reports success but the row is
	// never picked up again. The release gate (Tallyman must accept this
	// event type before coderd ships it) is what keeps permanent
	// rejections exceptional.
	existing := make(map[time.Time]struct{}, len(existingTimes))
	for _, ts := range existingTimes {
		// created_at is always the exact bucket start for this event type;
		// truncation just normalizes timezone and precision.
		existing[ts.UTC().Truncate(AgentRuntimeInterval)] = struct{}{}
	}

	var filled, failed int
	for bucket := earliest; !bucket.After(latestEligible); bucket = bucket.Add(AgentRuntimeInterval) {
		if _, ok := existing[bucket]; ok {
			continue
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}

		err := g.generateBucket(ctx, bucket)
		if err != nil {
			if ctx.Err() != nil {
				// A cancel landing mid-query surfaces as a bucket error.
				// Report the cancellation instead of logging shutdown as
				// a bucket failure.
				return ctx.Err()
			}
			// Skip only the failed bucket so one bad bucket (e.g. an
			// invalid runtime sum) cannot stall every later bucket until
			// it ages out of the window.
			g.log.Warn(ctx, "generate agent runtime usage event for bucket",
				slog.F("bucket", bucket),
				slog.Error(err),
			)
			failed++
			continue
		}
		filled++
	}
	if filled > 0 || failed > 0 {
		g.log.Info(ctx, "generated agent runtime usage events",
			slog.F("buckets_filled", filled),
			slog.F("buckets_failed", failed),
			slog.F("window_start", earliest),
			slog.F("latest_eligible", latestEligible),
		)
	}
	return nil
}

// generateBucket computes and inserts the event for a single hourly bucket.
func (g *Generator) generateBucket(ctx context.Context, bucket time.Time) error {
	runtimeMs, err := g.db.GetTotalChatMessageRuntimeMsInRange(ctx, database.GetTotalChatMessageRuntimeMsInRangeParams{
		StartTime: bucket,
		EndTime:   bucket.Add(AgentRuntimeInterval),
	})
	if err != nil {
		return xerrors.Errorf("sum chat message runtime: %w", err)
	}

	// The deterministic ID makes concurrent inserts of the same bucket
	// idempotent, and created_at is the bucket start (not the insertion
	// time) so daily rollups attribute backfilled hours to the correct day.
	stableID := string(usagetypes.UsageEventTypeHBAgentRuntimeV1) + ":" + bucket.Format(usageEventIDTimeFormat)
	err = g.ins.InsertHeartbeatUsageEvent(ctx, g.db, stableID, bucket, usagetypes.HBAgentRuntime{RuntimeMs: runtimeMs})
	if database.IsUniqueViolation(err, database.UniqueIndexUsageEventsAgentRuntime) {
		// The insert's ON CONFLICT (id) arbiter only sees committed rows, so
		// a concurrent replica inserting the same bucket can trip the bucket
		// unique index instead. Either way a row for this bucket already
		// exists, which is all generateBucket needs.
		return nil
	}
	if err != nil {
		return xerrors.Errorf("insert usage event: %w", err)
	}
	return nil
}
