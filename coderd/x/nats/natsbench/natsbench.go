package natsbench

import (
	"context"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
)

const (
	// DefaultMessages is the default total message count for a run.
	DefaultMessages = 100_000
	// Payload8KB and Payload64KB are the standard matrix payload sizes.
	Payload8KB  = 8 << 10
	Payload64KB = 64 << 10

	// defaultTimeout bounds each workload phase when Config.Timeout is
	// unset.
	defaultTimeout = 60 * time.Second
)

// Config describes one benchmark run.
type Config struct {
	// Messages is the TOTAL number of messages across all publishers,
	// split evenly with the remainder assigned to publisher 0. Zero
	// means DefaultMessages.
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

	// InProcess uses in-process server connections instead of TCP
	// loopback.
	InProcess bool
	// PublishConns and SubscribeConns configure the pubsub connection
	// pools. Zero means the production default of 1 each.
	PublishConns   int
	SubscribeConns int
	// LocalQueueMsgs sets the per-listener queue capacity. Zero derives
	// it from the workload so the busiest subscriber cannot overflow.
	LocalQueueMsgs int
	// LocalQueueBytes sets the per-subscription NATS pending byte
	// limit. Zero derives it from the busiest subject's full burst.
	LocalQueueBytes int
	// MaxPending sets the embedded server's per-client outbound pending
	// byte budget. Zero derives it from the workload.
	MaxPending int64
	// Timeout bounds each phase (readiness, publish, deliver). Zero
	// means 60s.
	Timeout time.Duration
}

// Result reports one run's exact accounting and throughput.
type Result struct {
	// Config is the fully resolved configuration the run used,
	// including defaults and derived sizing.
	Config Config

	// Published is the number of successfully published messages.
	Published int64
	// Delivered is the number of benchmark messages observed across all
	// subscribers. Fan-out makes this exceed Published whenever a
	// subject has multiple subscribers; it is logical throughput, not
	// physical bandwidth.
	Delivered int64
	// Drops counts dropped-message signals. Any nonzero value
	// invalidates the run: signals coalesce, so this is a lower bound
	// on actual loss, never an exact count.
	Drops int64

	// PublishDuration spans the start barrier to the last publisher
	// finishing, including the final flush. DeliverDuration spans the
	// start barrier to the last subscriber reaching its expected count.
	PublishDuration time.Duration
	DeliverDuration time.Duration

	// PubsPerSec is Published / PublishDuration. DeliveriesPerSec is
	// Delivered / DeliverDuration.
	PubsPerSec       float64
	DeliveriesPerSec float64
}

// withDefaults fills unset optional fields.
func (c Config) withDefaults() Config {
	if c.Messages == 0 {
		c.Messages = DefaultMessages
	}
	if c.Timeout <= 0 {
		c.Timeout = defaultTimeout
	}
	return c
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
	return nil
}

// Run executes one benchmark run: build the deterministic plan, start
// the topology, and drive the workload. On failure it returns any
// partial Result alongside the error for diagnostics; such a result
// must never be reported as a valid measurement.
func Run(ctx context.Context, logger slog.Logger, cfg Config) (*Result, error) {
	cfg = cfg.withDefaults()
	if err := cfg.validate(); err != nil {
		return nil, xerrors.Errorf("validate config: %w", err)
	}

	pl := buildPlan(cfg)
	cfg = applySizing(ctx, logger, cfg, pl)

	top, err := buildTopology(ctx, logger, cfg)
	if err != nil {
		return nil, xerrors.Errorf("build topology: %w", err)
	}
	defer top.closeAll()

	return runWorkload(ctx, logger, top, pl, cfg)
}
