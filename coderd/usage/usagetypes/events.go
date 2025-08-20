// Package usagetypes contains the types for usage events. These are kept in
// their own package to avoid importing any real code from coderd.
//
// Imports in this package should be limited to the standard library and the
// following packages ONLY:
//   - github.com/google/uuid
//   - golang.org/x/xerrors
//
// This package is imported by the Tallyman codebase.
package usagetypes

// Please read the package documentation before adding imports.
import (
	"bytes"
	"encoding/json"
	"strings"

	"golang.org/x/xerrors"
)

// UsageEventType is an enum of all usage event types. It mirrors the database
// type `usage_event_type`.
type UsageEventType string

const (
	UsageEventTypeDCManagedAgentsV1 UsageEventType = "dc_managed_agents_v1"
)

func (e UsageEventType) Valid() bool {
	switch e {
	case UsageEventTypeDCManagedAgentsV1:
		return true
	default:
		return false
	}
}

func (e UsageEventType) IsDiscrete() bool {
	return e.Valid() && strings.HasPrefix(string(e), "dc_")
}

func (e UsageEventType) IsHeartbeat() bool {
	return e.Valid() && strings.HasPrefix(string(e), "hb_")
}

// ParseEvent parses the raw event data into the specified Go type. It fails if
// there is any unknown fields or extra data after the event. The returned event
// is validated.
func ParseEvent[T Event](data json.RawMessage) (T, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()

	var event T
	err := dec.Decode(&event)
	if err != nil {
		return event, xerrors.Errorf("unmarshal %T event: %w", event, err)
	}
	if dec.More() {
		return event, xerrors.Errorf("extra data after %T event", event)
	}
	err = event.Valid()
	if err != nil {
		return event, xerrors.Errorf("invalid %T event: %w", event, err)
	}

	return event, nil
}

// ParseEventWithType parses the raw event data into the specified Go type. It
// fails if there is any unknown fields or extra data after the event. The
// returned event is validated.
func ParseEventWithType(eventType UsageEventType, data json.RawMessage) (Event, error) {
	switch eventType {
	case UsageEventTypeDCManagedAgentsV1:
		return ParseEvent[DCManagedAgentsV1](data)
	default:
		return nil, xerrors.Errorf("unknown event type: %s", eventType)
	}
}

// Event is a usage event that can be collected by the usage collector.
//
// Note that the following event types should not be updated once they are
// merged into the product. Please consult Dean before making any changes.
//
// This type cannot be implemented outside of this package as it this package
// is the source of truth for the coder/tallyman repo.
type Event interface {
	usageEvent() // to prevent external types from implementing this interface
	EventType() UsageEventType
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
func (DCManagedAgentsV1) EventType() UsageEventType {
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
