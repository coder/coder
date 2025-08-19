package usage

import (
	"strings"

	"golang.org/x/xerrors"
)

// EventType is an enum of all usage event types. It mirrors the check
// constraint on the `event_type` column in the `usage_events` table.
type EventType string //nolint:revive

const (
	UsageEventTypeDCManagedAgentsV1 EventType = "dc_managed_agents_v1"
)

func (e EventType) Valid() bool {
	switch e {
	case UsageEventTypeDCManagedAgentsV1:
		return true
	default:
		return false
	}
}

func (e EventType) IsDiscrete() bool {
	return e.Valid() && strings.HasPrefix(string(e), "dc_")
}

func (e EventType) IsHeartbeat() bool {
	return e.Valid() && strings.HasPrefix(string(e), "hb_")
}

// Event is a usage event that can be collected by the usage collector.
//
// Note that the following event types should not be updated once they are
// merged into the product. Please consult Dean before making any changes.
//
// Event types cannot be implemented outside of this package, as they are
// imported by the coder/tallyman repository.
type Event interface {
	usageEvent() // to prevent external types from implementing this interface
	EventType() EventType
	Valid() error
	Fields() map[string]any // fields to be marshaled and sent to tallyman/Metronome
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
func (DCManagedAgentsV1) EventType() EventType {
	return UsageEventTypeDCManagedAgentsV1
}

func (e DCManagedAgentsV1) Valid() error {
	if e.Count == 0 {
		return xerrors.New("count must be greater than 0")
	}
	return nil
}

func (e DCManagedAgentsV1) Fields() map[string]any {
	return map[string]any{
		"count": e.Count,
	}
}
