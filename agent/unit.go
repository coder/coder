package agent

import (
	"context"
	"slices"
	"sync"
	"sync/atomic"
	"time"
)

// A Unit (named to represent its resemblance to the systemd concept) is a kind of node in a dependency graph. It can depend
// on other units and it can be depended on by other units. Units are primarily meant to encapsulate sections of processes
// such as coder scripts to coordinate access to a contended resource, such as a database lock or a socket that is used or
// provided by the script.
//
// In most cases, `coder_script` resources will create and manage units by invocations of:
// * `coder agent unit start <unit> [--wants <unit>]`
// * `coder agent unit complete <unit>`
// * `coder agent unit fail <unit>`
// * `coder agent unit lock <unit>`
//
// Those CLI command examples are implemented elsewhere and are only shown here as a convenient example of the functionality
// provided by Units. This file contains analogous methods to be used by the CLI implementations.
type Unit struct {
	Name    string
	history []Event
	Wants   []*Unit
}

// Events provide a coarse grained record of the lifecycle and history of a unit.
type Event struct {
	Type      UnitEventType
	Timestamp time.Time
}

type UnitEventType string

const (
	UnitEventTypeAcquired  UnitEventType = "acquired"
	UnitEventTypeReleased  UnitEventType = "released"
	UnitEventTypeCompleted UnitEventType = "completed"
	UnitEventTypeFailed    UnitEventType = "failed"
)

// Listener represents an event handler
type Listener func(ctx context.Context, event UnitEventType)

// UnitCoordinator is the core interface for agent state coordination
type UnitCoordinator interface {
	StartUnit(unitName string) bool         // Returns true if acquired, false if already held
	StopUnit(unitName string) bool          // Releases the unit
	IsUnitHeld(unitName string) bool        // Checks if unit is currently held
	GetUnitHistory(unitName string) []Event // Get all events for a unit
	Close() error
}

// memoryLockCoordinator is the core implementation
type memoryUnitCoordinator struct {
	mu        sync.RWMutex
	listeners map[string]map[uint64]Listener
	units     map[string]Unit // Persistent event history
	nextID    uint64
	closed    int32
}

// NewMemoryLockCoordinator creates a new state coordinator instance
func NewMemoryUnitCoordinator() UnitCoordinator {
	return &memoryUnitCoordinator{
		listeners: make(map[string]map[uint64]Listener),
		units:     make(map[string]Unit),
		nextID:    0,
		closed:    0,
	}
}

// SubscribeToUnit adds a listener for unit events (acquired/released)
func (s *memoryUnitCoordinator) SubscribeToUnit(unitName string, listener Listener) (cancel func(), err error) {
	if atomic.LoadInt32(&s.closed) == 1 {
		return nil, context.Canceled
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Subscribe to both acquired and released events
	if s.listeners[unitName] == nil {
		s.listeners[unitName] = make(map[uint64]Listener)
	}

	id := atomic.AddUint64(&s.nextID, 1)
	s.listeners[unitName][id] = listener

	// Deliver historical events to new subscriber
	if events, exists := s.units[unitName]; exists {
		for _, eventData := range events.history {
			go listener(context.Background(), eventData.Type) // TODO: Fix context propagation
		}
	}

	return func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.listeners[unitName] != nil {
			delete(s.listeners[unitName], id)
		}
	}, nil
}

// GetLockHistory returns all events for a given lock (both acquired and released)
func (s *memoryUnitCoordinator) GetUnitHistory(unitName string) []Event {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var allEvents []Event

	if events, exists := s.units[unitName]; exists {
		allEvents = append(allEvents, events.history...)
	}

	// Sort by timestamp (acquired and released events interleaved by time)
	// For now, just return them in the order they were added
	slices.SortFunc(allEvents, func(a, b Event) int {
		return a.Timestamp.Compare(b.Timestamp)
	})
	return allEvents
}

// AcquireUnit attempts to acquire a unit, returns true if successful
func (s *memoryUnitCoordinator) StartUnit(unitName string) bool {
	return s.acquireUnitInternal(unitName, nil)
}

// acquireUnitInternal is the internal implementation for unit acquisition
func (s *memoryUnitCoordinator) acquireUnitInternal(unitName string, ttl *time.Duration) bool {
	if atomic.LoadInt32(&s.closed) == 1 {
		return false
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if unit is already held
	if s.hasUnitHeld(unitName) {
		return false
	}

	if _, exists := s.units[unitName]; !exists {
		s.units[unitName] = Unit{
			Name:    unitName,
			history: []Event{},
		}
	}

	now := time.Now()
	unit := s.units[unitName]
	unit.history = append(unit.history, Event{
		Type:      UnitEventTypeAcquired,
		Timestamp: now,
	})

	s.units[unitName] = unit

	// Notify listeners
	if listeners, exists := s.listeners[unitName]; exists {
		for _, listener := range listeners {
			go listener(context.Background(), UnitEventTypeAcquired)
		}
	}

	return true
}

// ReleaseLock releases a previously acquired lock
func (s *memoryUnitCoordinator) StopUnit(unitName string) bool {
	if atomic.LoadInt32(&s.closed) == 1 {
		return false
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Publish release event
	if _, exists := s.units[unitName]; !exists {
		return false
	}
	lock := s.units[unitName]

	lock.history = append(lock.history, Event{
		Type:      UnitEventTypeReleased,
		Timestamp: time.Now(),
	})
	s.units[unitName] = lock

	// Notify listeners
	if listeners, exists := s.listeners[unitName]; exists {
		for _, listener := range listeners {
			go listener(context.Background(), UnitEventTypeReleased)
		}
	}

	return true
}

// IsUnitHeld checks if a unit is currently held
func (s *memoryUnitCoordinator) IsUnitHeld(unitName string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if !s.hasUnitHeld(unitName) {
		return false
	}

	return true
}

// hasUnitHeld is a helper method to check unit state (must be called with unit held)
func (s *memoryUnitCoordinator) hasUnitHeld(unitName string) bool {
	unit, exists := s.units[unitName]
	if !exists {
		return false
	}

	if len(unit.history) == 0 {
		return false
	}

	// Check if the last event was an acquisition
	lastEvent := unit.history[len(unit.history)-1]
	return lastEvent.Type == UnitEventTypeAcquired
}

// Close shuts down the state coordinator
func (s *memoryUnitCoordinator) Close() error {
	atomic.StoreInt32(&s.closed, 1)

	s.mu.Lock()
	defer s.mu.Unlock()

	// Clear all listeners and events
	s.listeners = make(map[string]map[uint64]Listener)
	s.units = make(map[string]Unit)

	return nil
}
