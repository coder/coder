package coderdtest

import (
	"context"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/usage"
	"github.com/coder/coder/v2/coderd/usage/usagetypes"
)

var _ usage.Inserter = (*UsageInserter)(nil)

type UsageInserter struct {
	Events []usagetypes.DiscreteEvent
}

func NewUsageInserter() *UsageInserter {
	return &UsageInserter{
		Events: []usagetypes.DiscreteEvent{},
	}
}

func (u *UsageInserter) InsertDiscreteUsageEvent(_ context.Context, _ database.Store, event usagetypes.DiscreteEvent) error {
	u.Events = append(u.Events, event)
	return nil
}

func (u *UsageInserter) Reset() {
	u.Events = []usagetypes.DiscreteEvent{}
}
