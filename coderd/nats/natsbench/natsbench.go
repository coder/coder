package main

import (
	"context"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
)

const (
	// DefaultMessages is the default total message count for a run.
	DefaultMessages = 100000
	// Payload8KB and Payload64KB are the standard matrix payload sizes.
	Payload8KB  = 8 << 10
	Payload64KB = 64 << 10
)

// Config describes one benchmark run. Run validates a fully populated
// config and applies no defaults of its own (the CLI fills defaults
// before calling Run), so every required field must be set. The
// optional tuning fields below are passed straight through to
// nats.Options, where the nats package applies its own zero-value
// defaults. The benchmark deliberately does NOT autotune these knobs:
// it measures the pubsub as Coder configures it in production, so any
// dropped messages are reported as a metric rather than tuned away.
type Config struct {
	// Messages is the TOTAL number of messages across all publishers,
	// split evenly with the remainder assigned to publisher 0. Must be
	// at least 1.
	Messages int
	// PayloadSize is the benchmark message size in bytes.
	PayloadSize int
	// Subjects is the number of distinct subjects ("bench.<i>").
	Subjects int
	// Publishers is the number of concurrent publishers. Publisher i
	// publishes to subject i % Subjects from node i % Replicas.
	Publishers int
	// Subscribers is the number of subscribers. Subscriber j listens on
	// subject j % Subjects from node j % Replicas.
	Subscribers int
	// Replicas is the number of embedded pubsub nodes. 1 runs a single
	// node; greater values run a fully meshed cluster.
	Replicas int
	// Seed selects the pseudorandom node placement. The same seed
	// reproduces the same placement for a given shape; change it to
	// sample a different placement. Zero is a valid, deterministic
	// seed.
	Seed int64

	// InProcess uses in-process server connections instead of TCP
	// loopback.
	InProcess bool
	// PublishConns and SubscribeConns configure the pubsub connection
	// pools. Zero passes through to nats.Options, which defaults each
	// pool to a single connection (the production default).
	PublishConns   int
	SubscribeConns int
	// LocalQueueMsgs overrides the per-subscription NATS pending
	// message limit. Zero uses the nats package production default;
	// set it only for sensitivity analysis.
	LocalQueueMsgs int
	// LocalQueueBytes overrides the per-subscription NATS pending byte
	// limit. Zero uses the nats package production default.
	LocalQueueBytes int
	// MaxPending overrides the embedded server's per-client outbound
	// pending byte budget. Zero uses the nats package production
	// default.
	MaxPending int64
	// Timeout bounds each phase (readiness, publish, deliver). It must
	// be positive.
	Timeout time.Duration
}

// Result reports one run's exact accounting and throughput.
type Result struct {
	// Config is the fully resolved configuration the run used,
	// including derived sizing.
	Config Config

	// Published is the number of successfully published messages.
	Published int64
	// Delivered is the number of benchmark messages observed across all
	// subscribers. Fan-out makes this exceed Published whenever a
	// subject has multiple subscribers; it is logical throughput, not
	// physical bandwidth.
	Delivered int64
	// Expected is the total number of benchmark deliveries the plan
	// requires: the sum of every subscriber's expected count. It is the
	// exact denominator for the drop rate.
	Expected int64
	// Drops is Expected minus Delivered: the number of deliveries that
	// never arrived. It is the authoritative, complete loss count, since
	// the ErrDroppedMessages signal coalesces and cross-node routed loss
	// is silent. Drops are a reported metric, not a failure: a run with
	// drops still produces trustworthy throughput numbers for what it did
	// deliver.
	Drops int64

	// ConvergenceDuration is how long the readiness gate took to
	// propagate subscription interest across the cluster, measured from
	// the first probe (right after all subscriptions are registered) to
	// full propagation. Zero for single-node runs, which need no gate.
	ConvergenceDuration time.Duration

	// PublishDuration spans the hot start to the last publisher
	// finishing, including the final flush. DeliverDuration spans the
	// hot start to the last delivery: for a zero-drop run that is the
	// last subscriber reaching its expected count; for a run that dropped
	// messages it is the last observed counter progress before the
	// settle window elapsed, so the settle delay stays out of the rate.
	PublishDuration time.Duration
	DeliverDuration time.Duration

	// PubsPerSec is Published / PublishDuration. DeliveriesPerSec is
	// Delivered / DeliverDuration.
	PubsPerSec       float64
	DeliveriesPerSec float64
}

// validate rejects configurations the engine cannot run.
func (c Config) validate() error {
	if c.Messages < 1 {
		return xerrors.Errorf("messages must be at least 1, got %d", c.Messages)
	}
	if c.PayloadSize < 1 {
		return xerrors.Errorf("payload size must be at least 1, got %d", c.PayloadSize)
	}
	if c.PayloadSize > natsserver.MAX_PAYLOAD_SIZE {
		return xerrors.Errorf("payload size %d exceeds the NATS max payload %d", c.PayloadSize, natsserver.MAX_PAYLOAD_SIZE)
	}
	if c.Subjects < 1 {
		return xerrors.Errorf("subjects must be at least 1, got %d", c.Subjects)
	}
	if c.Publishers < 1 {
		return xerrors.Errorf("publishers must be at least 1, got %d", c.Publishers)
	}
	if c.Subscribers < 1 {
		return xerrors.Errorf("subscribers must be at least 1, got %d", c.Subscribers)
	}
	if c.Replicas < 1 {
		return xerrors.Errorf("replicas must be at least 1, got %d", c.Replicas)
	}
	if c.Timeout <= 0 {
		return xerrors.Errorf("timeout must be positive, got %s", c.Timeout)
	}
	return nil
}

// Run executes one benchmark run: build the deterministic plan, start
// the topology, and drive the workload. The config must be fully
// populated; Run applies no defaults. On failure it returns any partial
// Result alongside the error for diagnostics. Dropped messages are not
// a failure: a successful Result may still report a nonzero Drops count.
func Run(ctx context.Context, logger slog.Logger, cfg Config) (*Result, error) {
	if err := cfg.validate(); err != nil {
		return nil, xerrors.Errorf("validate config: %w", err)
	}

	pl := buildPlan(cfg)

	top, err := buildTopology(ctx, logger, cfg)
	if err != nil {
		return nil, xerrors.Errorf("build topology: %w", err)
	}
	defer top.closeAll()

	return runWorkload(ctx, logger, top, pl, cfg)
}
