package usage

import (
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
)

// Event is a usage event that can be collected by the usage collector.
//
// Note that the following event types should not be updated once they are
// merged into the product. Please consult Dean before making any changes.
type Event interface {
	usageEvent() // to prevent external types from implementing this interface
	EventType() database.UsageEventType
	Valid() error
}

// DiscreteEvent is a usage event that is collected as a discrete event.
type DiscreteEvent interface {
	Event
	discreteUsageEvent() // marker method, also prevents external types from implementing this interface
}

// DCManagedAgentsV1 is a discrete usage event for the number of managed agents.
// This event is sent in the following situations:
//   - Once on first startup after usage tracking is added to the product with
//     the count of all existing managed agents (count=N)
//   - A new managed agent is created (count=1)
type DCManagedAgentsV1 struct {
	Count uint64 `json:"count"`
}

var _ DiscreteEvent = DCManagedAgentsV1{}

func (DCManagedAgentsV1) usageEvent()         {}
func (DCManagedAgentsV1) discreteUsageEvent() {}
func (DCManagedAgentsV1) EventType() database.UsageEventType {
	return database.UsageEventTypeDcManagedAgentsV1
}

func (e DCManagedAgentsV1) Valid() error {
	if e.Count == 0 {
		return xerrors.New("count must be greater than 0")
	}
	return nil
}
