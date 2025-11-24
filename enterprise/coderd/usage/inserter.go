package usage

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	agplusage "github.com/coder/coder/v2/coderd/usage"
	"github.com/coder/coder/v2/coderd/usage/usagetypes"
	"github.com/coder/quartz"
)

// dbInserter collects usage events and stores them in the database for
// publishing.
type dbInserter struct {
	clock quartz.Clock
}

var _ agplusage.Inserter = &dbInserter{}

// NewDBInserter creates a new database-backed usage event inserter.
func NewDBInserter(opts ...InserterOption) agplusage.Inserter {
	c := &dbInserter{
		clock: quartz.NewReal(),
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

type InserterOption func(*dbInserter)

// InserterWithClock sets the quartz clock to use for the inserter.
func InserterWithClock(clock quartz.Clock) InserterOption {
	return func(c *dbInserter) {
		c.clock = clock
	}
}

// InsertDiscreteUsageEvent implements agplusage.Inserter.
func (i *dbInserter) InsertDiscreteUsageEvent(ctx context.Context, tx database.Store, event usagetypes.DiscreteEvent) error {
	if !event.EventType().IsDiscrete() {
		return xerrors.Errorf("event type %q is not a discrete event", event.EventType())
	}
	if err := event.Valid(); err != nil {
		return xerrors.Errorf("invalid %q event: %w", event.EventType(), err)
	}

	jsonData, err := json.Marshal(event.Fields())
	if err != nil {
		return xerrors.Errorf("marshal event as JSON: %w", err)
	}

	// Duplicate events are ignored by the query, so we don't need to check the
	// error.
	return tx.InsertUsageEvent(ctx, database.InsertUsageEventParams{
		// Always generate a new UUID for discrete events.
		ID:        uuid.New().String(),
		EventType: string(event.EventType()),
		EventData: jsonData,
		CreatedAt: dbtime.Time(i.clock.Now()),
	})
}
