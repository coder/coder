package coderdtest

import (
	"context"
	"sync"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/usage"
	"github.com/coder/coder/v2/coderd/usage/usagetypes"
)

var _ usage.Inserter = (*UsageInserter)(nil)

type UsageInserter struct {
	sync.Mutex
	Events []usagetypes.DiscreteEvent
}

func NewUsageInserter() *UsageInserter {
	return &UsageInserter{
		Events: []usagetypes.DiscreteEvent{},
	}
}

func (u *UsageInserter) InsertDiscreteUsageEvent(_ context.Context, _ database.Store, event usagetypes.DiscreteEvent) error {
	u.Lock()
	defer u.Unlock()
	u.Events = append(u.Events, event)
	return nil
}

func (u *UsageInserter) Reset() {
	u.Lock()
	defer u.Unlock()
	u.Events = []usagetypes.DiscreteEvent{}
}
