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
	// AgentRuntimeInterval is the bucket size for hb_agent_runtime_v1 usage
	// events. Each event covers one UTC hour [H, H+1).
	AgentRuntimeInterval = time.Hour
	// AgentRuntimeWindow is the trailing window scanned for missing buckets.
	// Buckets that are still missing beyond this window (e.g. because the
	// deployment was down for longer) are forfeited, which can only ever
	// undercount usage.
	AgentRuntimeWindow = 7 * 24 * time.Hour
	// AgentRuntimeEligibilityLag is how long after a bucket closes before it
	// becomes eligible for generation. This gives replicas time to commit
	// in-flight chat messages with timestamps inside the bucket.
	AgentRuntimeEligibilityLag = 5 * time.Minute
	// agentRuntimeJitter is the maximum random delay added after each hour
	// boundary. It staggers replicas so one is likely to complete the work
	// before others attempt it (inserts are idempotent either way).
	agentRuntimeJitter = 4 * time.Minute
	// generatorTimerName tags the quartz timer so tests can trap it.
	generatorTimerName = "agent-runtime-generator"
)

// Generator reconciles hb_agent_runtime_v1 heartbeat usage events. Unlike
// Cron jobs, which sample state at the time they fire, the Generator derives
// events from data already persisted in the database (chat_messages), so it
// can deterministically backfill hours that were missed while the deployment
// was down.
//
// Every tick it scans the trailing AgentRuntimeWindow for missing hourly
// buckets and fills each one with the total chat message runtime recorded in
// that hour, inserting zero-valued events for idle hours. Deterministic event
// IDs plus the database's ON CONFLICT (id) DO NOTHING make concurrent
// replicas safe without locking.
//
// Events are generated unconditionally in enterprise builds; the
// publish_usage_data license flag only gates publishing to Tallyman.
type Generator struct {
	clock quartz.Clock
	log   slog.Logger
	db    database.Store
	ins   agplusage.Inserter

	// cancel cancels the context of the running goroutine. If the ctx passed
	// into Start is canceled, the goroutine also stops.
	cancel context.CancelFunc

	// wg ensures the goroutine has exited before Close returns.
	wg sync.WaitGroup

	// startOnce ensures Start is idempotent.
	startOnce sync.Once
}

// NewGenerator creates a Generator that reconciles agent runtime heartbeat
// events. The clock controls all timers so that tests can advance time
// deterministically via quartz.Mock.
func NewGenerator(clock quartz.Clock, log slog.Logger, db database.Store, ins agplusage.Inserter) *Generator {
	return &Generator{
		clock: clock,
		log:   log,
		db:    db,
		ins:   ins,
	}
}

// Start launches the reconciliation goroutine. Subsequent calls are no-ops.
// On daemon restart a new Generator should be created.
func (g *Generator) Start(ctx context.Context) {
	g.startOnce.Do(func() {
		ctx, g.cancel = context.WithCancel(ctx)
		g.wg.Add(1)
		pproflabel.Go(ctx, pproflabel.Service(pproflabel.ServiceUsageEventGenerator), func(ctx context.Context) {
			g.run(ctx)
		})
	})
}

// Close cancels the goroutine and waits for it to exit.
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

	// The first pass runs shortly after startup to heal any gaps that
	// accumulated while the deployment was down. The uniform random delay in
	// [1m, 5m) staggers replicas that start simultaneously.
	//nolint:gosec // Jitter does not need cryptographic randomness.
	delay := time.Minute + time.Duration(rand.Int63n(int64(4*time.Minute)))
	for {
		// Use a quartz timer so the wait honors ctx cancellation and tests
		// can advance time deterministically.
		timer := g.clock.NewTimer(delay, generatorTimerName)

		select {
		case <-ctx.Done():
			if !timer.Stop() {
				// Drain the channel if the timer already fired.
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
			// The next tick rescans the whole window, so failed ticks heal
			// automatically.
			g.log.Warn(ctx, "generate agent runtime usage events", slog.Error(err))
		}

		// Wake shortly after the next hour boundary, once the just-closed
		// bucket becomes eligible.
		_, delay = nextTick(g.clock.Now(), AgentRuntimeInterval, agentRuntimeJitter)
		delay += AgentRuntimeEligibilityLag
	}
}

// generateAgentRuntimeEvents scans the trailing window for missing hourly
// buckets and inserts one hb_agent_runtime_v1 event per missing bucket. Any
// error aborts the current tick; remaining buckets are retried on the next
// tick.
func (g *Generator) generateAgentRuntimeEvents(ctx context.Context) error {
	now := g.clock.Now().UTC()
	// The most recent bucket that is old enough to generate. Bucket [H, H+1)
	// becomes eligible at H + interval + lag.
	latestEligible := now.Add(-AgentRuntimeInterval - AgentRuntimeEligibilityLag).Truncate(AgentRuntimeInterval)
	// The oldest bucket we are willing to backfill.
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
	existing := make(map[time.Time]struct{}, len(existingTimes))
	for _, ts := range existingTimes {
		// Events of this type always have created_at set to the exact bucket
		// start; truncation just normalizes timezone and precision.
		existing[ts.UTC().Truncate(AgentRuntimeInterval)] = struct{}{}
	}

	var filled int
	for bucket := earliest; !bucket.After(latestEligible); bucket = bucket.Add(AgentRuntimeInterval) {
		if _, ok := existing[bucket]; ok {
			continue
		}

		runtimeMs, err := g.db.GetTotalChatMessageRuntimeMsInRange(ctx, database.GetTotalChatMessageRuntimeMsInRangeParams{
			StartTime: bucket,
			EndTime:   bucket.Add(AgentRuntimeInterval),
		})
		if err != nil {
			return xerrors.Errorf("sum chat message runtime for bucket %s: %w", bucket, err)
		}

		// The deterministic ID makes concurrent inserts of the same bucket
		// idempotent. created_at is the bucket start (not insertion time) so
		// daily rollups attribute backfilled hours to the correct day.
		id := string(usagetypes.UsageEventTypeHBAgentRuntimeV1) + ":" + bucket.Format(cronDateFormat)
		err = g.ins.InsertHeartbeatUsageEvent(ctx, g.db, id, bucket, usagetypes.HBAgentRuntime{RuntimeMs: runtimeMs})
		if err != nil {
			return xerrors.Errorf("insert agent runtime event for bucket %s: %w", bucket, err)
		}
		filled++
	}
	if filled > 0 {
		g.log.Info(ctx, "generated agent runtime usage events",
			slog.F("buckets_filled", filled),
			slog.F("window_start", earliest),
			slog.F("latest_eligible", latestEligible),
		)
	}
	return nil
}
