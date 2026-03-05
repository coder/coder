package usage

import (
	"context"
	"math/rand"
	"sync"
	"sync/atomic"
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

// HeartbeatFunc generates a heartbeat event and its stable ID.
// It is called periodically by the cron. Returning an error skips
// the insert for that tick and logs a warning.
type HeartbeatFunc func(ctx context.Context) (id string, event usagetypes.HeartbeatEvent, err error)

// CronJob defines a periodic heartbeat job.
type CronJob struct {
	// Name is a human-readable label used in logs.
	Name string
	// Interval is the base duration between ticks.
	Interval time.Duration
	// Jitter is the maximum amount the tick may fire early or late.
	// The actual offset is uniformly distributed in [-Jitter, +Jitter].
	// This is useful for expensive work in multi-replica deployments:
	// staggering ticks means one replica is likely to complete the work
	// before others attempt it, allowing them to skip redundant effort
	// (heartbeat inserts are idempotent). Particularly valuable for
	// work that might take a while to compute.
	Jitter time.Duration
	// Fn produces the heartbeat event and its stable ID.
	Fn HeartbeatFunc
}

// Cron runs registered CronJobs on the dbInserter's clock. Stopping
// the context passed to Start cancels all jobs. Daemon restarts
// naturally restart the timers since Start() creates them fresh —
// there is no state to persist or recover.
type Cron struct {
	clock quartz.Clock
	log   slog.Logger
	db    database.Store
	ins   agplusage.Inserter
	jobs  []CronJob

	// cancel cancels the context on all running jobs. If the ctx passed into `Start`
	// is canceled, the jobs will also stop.
	cancel context.CancelFunc

	// wg ensures all job goroutines have exited before Close returns.
	wg sync.WaitGroup

	// startOnce ensures Start is idempotent.
	startOnce sync.Once
	started   atomic.Bool
}

// NewCron creates a Cron that periodically generates and inserts
// heartbeat events. The clock controls all timers so that tests can
// advance time deterministically via quartz.Mock.
func NewCron(clock quartz.Clock, log slog.Logger, db database.Store, ins agplusage.Inserter) *Cron {
	return &Cron{
		clock: clock,
		log:   log,
		db:    db,
		ins:   ins,
	}
}

// Register adds a job. It must be called before Start; calling it
// after Start returns an error.
func (c *Cron) Register(job CronJob) error {
	if c.started.Load() {
		return xerrors.New("cannot register a job after Start has been called")
	}
	c.jobs = append(c.jobs, job)
	return nil
}

// Start launches a goroutine per job. Subsequent calls are no-ops.
// On daemon restart a new Cron should be created.
func (c *Cron) Start(ctx context.Context) {
	c.startOnce.Do(func() {
		c.started.Store(true)
		ctx, c.cancel = context.WithCancel(ctx)
		for _, job := range c.jobs {
			c.wg.Add(1)
			pproflabel.Go(ctx, pproflabel.Service(pproflabel.ServiceUsageEventCron, "job", job.Name), func(ctx context.Context) {
				c.run(ctx, job)
			})
		}
	})
}

// Close cancels all jobs and waits for goroutines to exit.
func (c *Cron) Close() error {
	if c.cancel != nil {
		c.cancel()
	}
	c.wg.Wait()
	return nil
}

// jitteredDuration returns interval ± a random offset within the
// jitter window. Each tick gets an independent random offset so that
// replicas naturally stagger their work.
func jitteredDuration(interval, jitter time.Duration) time.Duration {
	if jitter <= 0 {
		return interval
	}
	// offset in [-jitter, +jitter].
	//nolint:gosec // Jitter does not need cryptographic randomness.
	offset := time.Duration(rand.Int63n(int64(2*jitter+1))) - jitter
	d := interval + offset
	if d <= 0 {
		// Ensure we never return a non-positive duration.
		d = 1
	}
	return d
}

func (c *Cron) run(ctx context.Context, job CronJob) {
	defer c.wg.Done()
	for {
		delay := jitteredDuration(job.Interval, job.Jitter)
		timer := c.clock.NewTimer(delay, job.Name)

		select {
		case <-ctx.Done():
			if !timer.Stop() {
				// Drain the channel if the timer already fired.
				<-timer.C
			}
			return
		case <-timer.C:
		}

		id, event, err := job.Fn(ctx)
		if err != nil {
			c.log.Warn(ctx, "cron heartbeat func failed",
				slog.F("job", job.Name),
				slog.Error(err),
			)
			continue
		}

		if err := c.ins.InsertHeartbeatUsageEvent(ctx, c.db, id, event); err != nil {
			c.log.Warn(ctx, "cron heartbeat insert failed",
				slog.F("job", job.Name),
				slog.Error(err),
			)
		}
	}
}

const AISeatsInterval = 4 * time.Hour

// AISeatsHeartbeat returns a HeartbeatFunc that queries the active
// AI seat count and emits it as an HBAISeats heartbeat event. The
// ID is time-bucketed to the 4-hour interval so that duplicate
// inserts from multiple replicas within the same bucket are
// idempotent.
func AISeatsHeartbeat(db database.Store) HeartbeatFunc {
	return func(ctx context.Context) (string, usagetypes.HeartbeatEvent, error) {
		ctx = dbauthz.AsUsagePublisher(ctx)
		count, err := db.GetActiveAISeatCount(ctx)
		if err != nil {
			return "", nil, xerrors.Errorf("get active AI seat count: %w", err)
		}
		// Round to the nearest bucket boundary rather than truncating
		// (floor). With ±jitter, a replica may fire slightly before or
		// after a boundary — rounding ensures both sides map to the
		// same bucket, keeping heartbeat IDs identical across replicas.
		bucket := roundToNearest(time.Now(), AISeatsInterval).UTC().Format(time.RFC3339)
		id := "hb_ai_seats_v1:" + bucket
		return id, usagetypes.HBAISeats{Count: uint64(count)}, nil
	}
}

// roundToNearest rounds t to the nearest multiple of d (from the zero
// time). This is like time.Truncate but rounds to nearest instead of
// flooring, which is important when jitter can push a tick slightly
// before or after a bucket boundary.
func roundToNearest(t time.Time, d time.Duration) time.Time {
	floor := t.Truncate(d)
	if t.Sub(floor) >= d/2 {
		return floor.Add(d)
	}
	return floor
}
