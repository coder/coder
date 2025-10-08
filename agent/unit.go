package agent

import (
	"context"
	"slices"
	"sync"
	"sync/atomic"
	"time"
)

// A Unit (named to represent its resemblance to the systemd concept) is a kind of lock that encodes metadata
// about the state of a resource. Units are primarilymeant to be sections of processes such as coder scripts
// that encapsulate a contended resource, such as a database lock or a socket.
//
// In most cases, `coder_script` resources will create and manage units by invocation of `coder agent lock <unit>`.
// Locks may be acquired with no intention of releasing them as a signal to other scripts that
// a contended resource has been provided and is available. For example, a script that installs curl
// might acquire a lock called "curl-install" to signal to other scripts that curl has been installed
// and is available. In this case, the lock will be released when the agent is stopped.
type Unit struct {
	Name    string
	history []Event
}

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

// LockCoordinator is the core interface for agent state coordination
type UnitCoordinator interface {
	AcquireUnit(unitName string) bool                                              // Returns true if acquired, false if already held
	ReleaseUnit(unitName string) bool                                              // Releases the unit
	IsUnitHeld(unitName string) bool                                               // Checks if unit is currently held
	SubscribeToUnit(unitName string, listener Listener) (cancel func(), err error) // Subscribe to unit events
	GetUnitHistory(unitName string) []Event                                        // Get all events for a unit
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
func (s *memoryUnitCoordinator) AcquireUnit(unitName string) bool {
	return s.acquireUnitInternal(unitName, nil)
}

// acquireUnitInternal is the internal implementation for unit acquisition
func (s *memoryUnitCoordinator) acquireUnitInternal(unitName string, ttl *time.Duration) bool {
	if atomic.LoadInt32(&s.closed) == 1 {
		return false
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if lock is already held and not expired
	if s.hasUnitHeld(unitName) && !s.isUnitExpired(unitName) {
		return false
	}

	// Clean up expired lock if it exists
	if s.hasUnitHeld(unitName) && s.isUnitExpired(unitName) {
		s.expireUnitInternal(unitName)
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
func (s *memoryUnitCoordinator) ReleaseUnit(unitName string) bool {
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

	// Check if unit has expired
	if s.isUnitExpired(unitName) {
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

// isLockExpired checks if a lock has expired based on its TTL (must be called with lock held)
func (s *memoryUnitCoordinator) isUnitExpired(unitName string) bool {
	unit, exists := s.units[unitName]
	if !exists {
		return false
	}

	// No TTL means no expiration
	if unit.ttl == nil || unit.acquiredAt == nil {
		return false
	}

	// Check if TTL has passed
	return time.Since(*unit.acquiredAt) > *unit.ttl
}

// expireLockInternal marks a lock as expired (must be called with write lock held)
func (s *memoryUnitCoordinator) expireUnitInternal(unitName string) {
	unit, exists := s.units[unitName]
	if !exists {
		return
	}

	// Cancel timer if it exists
	if unit.timer != nil {
		unit.timer.Stop()
		unit.timer = nil
	}

	// Add expiration event to history
	unit.history = append(unit.history, Event{
		Type:      UnitEventTypeExpired,
		Timestamp: time.Now(),
	})
	// Clear TTL and acquiredAt to prevent further expiration checks
	unit.ttl = nil
	unit.acquiredAt = nil
	s.units[unitName] = unit

	// Notify listeners of expiration
	if listeners, exists := s.listeners[unitName]; exists {
		for _, listener := range listeners {
			go listener(context.Background(), UnitEventTypeExpired)
		}
	}
}

// expireUnitWithTimer is called by the TTL timer to expire a unit
func (s *memoryUnitCoordinator) expireUnitWithTimer(unitName string) {
	if atomic.LoadInt32(&s.closed) == 1 {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Double-check the unit still exists and hasn't been released
	if !s.hasUnitHeld(unitName) {
		return
	}

	// Expire the unit
	s.expireUnitInternal(unitName)
}

// Close shuts down the state coordinator
func (s *memoryUnitCoordinator) Close() error {
	atomic.StoreInt32(&s.closed, 1)

	s.mu.Lock()
	defer s.mu.Unlock()

	// Stop all TTL timers before clearing locks
	for _, unit := range s.units {
		if unit.timer != nil {
			unit.timer.Stop()
		}
	}

	// Clear all listeners and events
	s.listeners = make(map[string]map[uint64]Listener)
	s.units = make(map[string]Unit)

	return nil
}
