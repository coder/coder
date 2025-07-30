package usage

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	agplusage "github.com/coder/coder/v2/coderd/usage"
	"github.com/coder/quartz"
)

// Collector collects usage events and stores them in the database for
// publishing.
type Collector struct {
	clock quartz.Clock
}

var _ agplusage.Collector = &Collector{}

// NewCollector creates a new database-backed usage event collector.
func NewCollector(opts ...CollectorOption) *Collector {
	c := &Collector{
		clock: quartz.NewReal(),
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

type CollectorOption func(*Collector)

// CollectorWithClock sets the quartz clock to use for the collector.
func CollectorWithClock(clock quartz.Clock) CollectorOption {
	return func(c *Collector) {
		c.clock = clock
	}
}

// CollectDiscreteUsageEvent implements agplusage.Collector.
func (c *Collector) CollectDiscreteUsageEvent(ctx context.Context, db database.Store, event agplusage.DiscreteEvent) error {
	if !event.EventType().IsDiscrete() {
		return xerrors.Errorf("event type %q is not a discrete event", event.EventType())
	}
	if err := event.Valid(); err != nil {
		return xerrors.Errorf("invalid %q event: %w", event.EventType(), err)
	}

	jsonData, err := json.Marshal(event)
	if err != nil {
		return xerrors.Errorf("marshal event as JSON: %w", err)
	}

	// Duplicate events are ignored by the query, so we don't need to check the
	// error.
	return db.InsertUsageEvent(ctx, database.InsertUsageEventParams{
		// Always generate a new UUID for discrete events.
		ID:        uuid.New().String(),
		EventType: event.EventType(),
		EventData: jsonData,
		CreatedAt: dbtime.Time(c.clock.Now()),
	})
}
