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
	discreteEvents  []usagetypes.DiscreteEvent
	heartbeatEvents []usagetypes.HeartbeatEvent
	seenHeartbeats  map[string]struct{}
}

func NewUsageInserter() *UsageInserter {
	return &UsageInserter{
		discreteEvents:  []usagetypes.DiscreteEvent{},
		seenHeartbeats:  map[string]struct{}{},
		heartbeatEvents: []usagetypes.HeartbeatEvent{},
	}
}

func (u *UsageInserter) InsertDiscreteUsageEvent(_ context.Context, _ database.Store, event usagetypes.DiscreteEvent) error {
	u.Lock()
	defer u.Unlock()
	u.discreteEvents = append(u.discreteEvents, event)
	return nil
}

func (u *UsageInserter) InsertHeartbeatUsageEvent(_ context.Context, _ database.Store, id string, event usagetypes.HeartbeatEvent) error {
	u.Lock()
	defer u.Unlock()
	if _, seen := u.seenHeartbeats[id]; seen {
		return nil
	}

	u.seenHeartbeats[id] = struct{}{}
	u.heartbeatEvents = append(u.heartbeatEvents, event)
	return nil
}

func (u *UsageInserter) GetHeartbeatEvents() []usagetypes.HeartbeatEvent {
	u.Lock()
	defer u.Unlock()
	eventsCopy := make([]usagetypes.HeartbeatEvent, len(u.heartbeatEvents))
	copy(eventsCopy, u.heartbeatEvents)
	return eventsCopy
}

func (u *UsageInserter) GetDiscreteEvents() []usagetypes.DiscreteEvent {
	u.Lock()
	defer u.Unlock()
	eventsCopy := make([]usagetypes.DiscreteEvent, len(u.discreteEvents))
	copy(eventsCopy, u.discreteEvents)
	return eventsCopy
}

func (u *UsageInserter) TotalEventCount() int {
	u.Lock()
	defer u.Unlock()
	return len(u.discreteEvents) + len(u.heartbeatEvents)
}

func (u *UsageInserter) Reset() {
	u.Lock()
	defer u.Unlock()
	u.seenHeartbeats = map[string]struct{}{}
	u.discreteEvents = []usagetypes.DiscreteEvent{}
	u.heartbeatEvents = []usagetypes.HeartbeatEvent{}
}
