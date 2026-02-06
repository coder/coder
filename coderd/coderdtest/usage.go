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
	events []usagetypes.DiscreteEvent
}

func NewUsageInserter() *UsageInserter {
	return &UsageInserter{
		events: []usagetypes.DiscreteEvent{},
	}
}

func (u *UsageInserter) InsertDiscreteUsageEvent(_ context.Context, _ database.Store, event usagetypes.DiscreteEvent) error {
	u.Lock()
	defer u.Unlock()
	u.events = append(u.events, event)
	return nil
}

func (u *UsageInserter) GetEvents() []usagetypes.DiscreteEvent {
	u.Lock()
	defer u.Unlock()
	eventsCopy := make([]usagetypes.DiscreteEvent, len(u.events))
	copy(eventsCopy, u.events)
	return eventsCopy
}

func (u *UsageInserter) Reset() {
	u.Lock()
	defer u.Unlock()
	u.events = []usagetypes.DiscreteEvent{}
}
