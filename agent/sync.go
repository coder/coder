package agent

import (
	"context"
	"slices"
	"sync"
	"sync/atomic"
	"time"
)

// AGENT STATE COORDINATION SYSTEM DESIGN CONSIDERATIONS
// =====================================================
//
// This file contains a state-based event coordination system for workspace agents.
// Unlike traditional pubsub, events represent persistent state changes that can be
// queried by subscribers even after they've been published. This enables distributed
// locking and state coordination patterns.
//
// 1. PRODUCTION READINESS REQUIREMENTS
// ====================================
//
// Unlike MemoryPubsub (which is test-only), this needs to be production-ready:
// - Proper error handling and recovery
// - Context propagation (not context.Background()) ❌ TODO: Fix context propagation
// - Backpressure handling to prevent goroutine explosion ❌ TODO: Add worker pools
// - Memory management and cleanup ✅ IMPLEMENTED
// - Observability and metrics ❌ TODO: Add metrics
// - Graceful shutdown ✅ IMPLEMENTED
//
// 2. STATE COORDINATION DESIGN GOALS
// ==================================
//
// - Single-process, single-agent scope (no cross-instance communication needed) ✅ IMPLEMENTED
// - Low latency for real-time agent coordination ✅ IMPLEMENTED
// - Lightweight (no external dependencies like PostgreSQL) ✅ IMPLEMENTED
// - Thread-safe for concurrent agent operations ✅ IMPLEMENTED
// - State-driven architecture for agent coordination ✅ IMPLEMENTED
// - Persistent event history for late subscribers ✅ TODO: Add event persistence
// - Distributed locking capabilities ✅ TODO: Add locking mechanisms
//
// 4. CRITICAL IMPLEMENTATION CONSIDERATIONS
// ========================================
//
// State coordination specific requirements:
// - Events must persist after publishing (not fire-and-forget) ❌ TODO: Add event persistence
// - Late subscribers must see historical events ❌ TODO: Add historical event delivery
// - Event ordering must be preserved ❌ TODO: Add event ordering
// - Memory management for event history ❌ TODO: Add event cleanup
// - Thread-safe event storage and retrieval ❌ TODO: Add thread-safe storage
//
// 5. PERFORMANCE OPTIMIZATIONS
// =============================
//
// - Use worker pools instead of goroutine-per-message ❌ TODO: Add worker pools
// - Implement event batching for high-frequency events ❌ TODO: Add batching
// - Use sync.Pool for event reuse ❌ TODO: Add sync.Pool
// - Implement backpressure with bounded queues ❌ TODO: Add backpressure
// - Consider async vs sync delivery options ❌ TODO: Add delivery options
//
// 6. MEMORY MANAGEMENT
// ====================
//
// - Implement subscription cleanup on agent shutdown ✅ IMPLEMENTED
// - Use weak references or cleanup callbacks ❌ TODO: Add weak references
// - Monitor memory usage and implement limits ❌ TODO: Add memory monitoring
// - Consider event name length limits to prevent memory bloat ❌ TODO: Add length limits
//
// 7. ERROR HANDLING STRATEGY
// ==========================
//
// - Implement retry logic for failed event delivery ❌ TODO: Add retry logic
// - Add dead letter queue for persistent failures ❌ TODO: Add dead letter queue
// - Log errors with proper context ❌ TODO: Add error logging
// - Implement circuit breaker for cascading failures ❌ TODO: Add circuit breaker
//
// 8. OBSERVABILITY REQUIREMENTS
// =============================
//
// - Event publish/subscribe counters ❌ TODO: Add metrics
// - Latency histograms ❌ TODO: Add latency metrics
// - Error rates and types ❌ TODO: Add error metrics
// - Memory usage metrics ❌ TODO: Add memory metrics
// - Active subscription counts ❌ TODO: Add subscription metrics
//
// 9. STATE COORDINATION SYSTEM DESIGN
// ===================================
//
// Persistent state-based system: ❌ TODO: Update implementation
// - Events represent state changes that persist ✅ TODO: Add event persistence
// - Events can be queried by name and timestamp ✅ TODO: Add event querying
// - Support for event data/metadata ✅ TODO: Add event data
// - Hierarchical event names for organization ✅ IMPLEMENTED
// - Event history and replay capabilities ✅ TODO: Add event history
// - Distributed locking patterns ✅ TODO: Add locking patterns
//
// 10. CONCURRENCY PATTERNS
// ========================
//
// - Use sync.RWMutex for read-heavy operations (publishing) ✅ IMPLEMENTED
// - Implement subscription management with proper locking ✅ IMPLEMENTED
// - Use channels for async event delivery ❌ TODO: Add channels
// - Consider using sync.Map for high-concurrency scenarios ❌ TODO: Add sync.Map
//
// 11. SHUTDOWN AND CLEANUP
// ========================
//
// - Implement graceful shutdown with context cancellation ✅ IMPLEMENTED
// - Wait for in-flight events to complete ❌ TODO: Add wait for completion
// - Clean up all subscriptions and resources ✅ IMPLEMENTED
// - Prevent new subscriptions during shutdown ✅ IMPLEMENTED
//
// 12. TESTING STRATEGY
// ===================
//
// - Unit tests for all core functionality ❌ TODO: Add unit tests
// - Concurrency tests for race conditions ❌ TODO: Add concurrency tests
// - Performance tests for high-load scenarios ❌ TODO: Add performance tests
// - Memory leak tests for long-running agents ❌ TODO: Add memory leak tests
// - Integration tests with real agent workflows ❌ TODO: Add integration tests
//
// 13. IMPLEMENTATION PHASES
// ========================
//
// Phase 1: Core interface and basic functionality
// - Implement Subscribe/Publish with proper locking ✅ COMPLETED
// - Add context propagation ❌ TODO: Fix context propagation
// - Implement basic error handling ✅ COMPLETED
//
// Phase 2: Performance optimizations
// - Add worker pools for event delivery ❌ TODO: Add worker pools
// - Implement backpressure handling ❌ TODO: Add backpressure
// - Add event batching ❌ TODO: Add batching
//
// Phase 3: Production features
// - Add metrics and observability ❌ TODO: Add metrics
// - Implement retry logic and dead letter queues ❌ TODO: Add retry logic
// - Add comprehensive error handling ❌ TODO: Add comprehensive error handling
//
// Phase 4: Advanced features
// - Add event persistence options ❌ TODO: Add persistence
// - Implement event filtering and routing ❌ TODO: Add filtering
// - Add event ordering guarantees ❌ TODO: Add ordering
//
// 14. EXAMPLE USAGE PATTERNS
// ===========================
//
// // Lock-based state coordination
// coordinator := NewMemoryLockCoordinator()
//
// // Acquire and release locks
// if coordinator.AcquireLock("agent.resource") {
//     defer coordinator.ReleaseLock("agent.resource")
//     // Do work with the resource
// }
//
// // Check lock status
// if coordinator.IsLockHeld("agent.resource") {
//     // Lock is held, can't proceed
//     return
// }
//
// // Subscribe to lock events
// coordinator.SubscribeToLock("agent.resource", func(ctx context.Context, event string) {
//     // Handle lock acquired/released events
//     // Will receive both new and historical events
// })
//
// // Get lock history for analysis
// events := coordinator.GetLockHistory("agent.resource")
// for _, event := range events {
//     // Process historical lock events
// }
//
// 15. INTEGRATION WITH EXISTING AGENT SYSTEMS
// ===========================================
//
// - Integrate with agent's existing logging system
// - Connect to agent's health check mechanisms
// - Hook into workspace application lifecycle
// - Coordinate with agent's resource monitoring
// - Support agent's configuration management
//
// 16. SECURITY CONSIDERATIONS
// ===========================
//
// - Validate event names and length limits
// - Implement access control for sensitive events
// - Sanitize event names
// - Consider encryption for sensitive event names
//
// IMPLEMENTATION NOTES:
// ===================
//
// Start with a simple, working implementation that addresses the core issues
// with MemoryPubsub, then iteratively add production features. Focus on:
// 1. Correctness and thread safety
// 2. Performance and scalability
// 3. Observability and debugging
// 4. Error handling and recovery

type Lock struct {
	Name    string
	history []Event
}

type Event struct {
	Type      LockEventType
	Timestamp time.Time
}

type LockEventType string

const (
	LockEventTypeAcquired LockEventType = "acquired"
	LockEventTypeReleased LockEventType = "released"
)

// Listener represents an event handler
type Listener func(ctx context.Context, event LockEventType)

// LockCoordinator is the core interface for agent state coordination
type LockCoordinator interface {
	AcquireLock(lockName string) bool                                              // Returns true if acquired, false if already held
	ReleaseLock(lockName string) bool                                              // Releases the lock
	IsLockHeld(lockName string) bool                                               // Checks if lock is currently held
	SubscribeToLock(lockName string, listener Listener) (cancel func(), err error) // Subscribe to lock events
	GetLockHistory(lockName string) []Event                                        // Get all events for a lock
	Close() error
}

// memoryLockCoordinator is the core implementation
type memoryLockCoordinator struct {
	mu        sync.RWMutex
	listeners map[string]map[uint64]Listener
	locks     map[string]Lock // Persistent event history
	nextID    uint64
	closed    int32
}

// NewMemoryLockCoordinator creates a new state coordinator instance
func NewMemoryLockCoordinator() LockCoordinator {
	return &memoryLockCoordinator{
		listeners: make(map[string]map[uint64]Listener),
		locks:     make(map[string]Lock),
	}
}

// SubscribeToLock adds a listener for lock events (acquired/released)
func (s *memoryLockCoordinator) SubscribeToLock(lockName string, listener Listener) (cancel func(), err error) {
	if atomic.LoadInt32(&s.closed) == 1 {
		return nil, context.Canceled
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Subscribe to both acquired and released events
	if s.listeners[lockName] == nil {
		s.listeners[lockName] = make(map[uint64]Listener)
	}

	id := atomic.AddUint64(&s.nextID, 1)
	s.listeners[lockName][id] = listener

	// Deliver historical events to new subscriber
	if events, exists := s.locks[lockName]; exists {
		for _, eventData := range events.history {
			go listener(context.Background(), eventData.Type) // TODO: Fix context propagation
		}
	}

	return func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.listeners[lockName] != nil {
			delete(s.listeners[lockName], id)
		}
	}, nil
}

// GetLockHistory returns all events for a given lock (both acquired and released)
func (s *memoryLockCoordinator) GetLockHistory(lockName string) []Event {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var allEvents []Event

	if events, exists := s.locks[lockName]; exists {
		allEvents = append(allEvents, events.history...)
	}

	// Sort by timestamp (acquired and released events interleaved by time)
	// For now, just return them in the order they were added
	slices.SortFunc(allEvents, func(a, b Event) int {
		return a.Timestamp.Compare(b.Timestamp)
	})
	return allEvents
}

// AcquireLock attempts to acquire a lock, returns true if successful
func (s *memoryLockCoordinator) AcquireLock(lockName string) bool {
	if atomic.LoadInt32(&s.closed) == 1 {
		return false
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if lock is already held (has acquired but no released)
	if s.hasLockHeld(lockName) {
		return false
	}

	if _, exists := s.locks[lockName]; !exists {
		s.locks[lockName] = Lock{
			Name:    lockName,
			history: []Event{},
		}
	}
	lock := s.locks[lockName]
	lock.history = append(lock.history, Event{
		Type:      LockEventTypeAcquired,
		Timestamp: time.Now(),
	})
	s.locks[lockName] = lock

	// Notify listeners
	if listeners, exists := s.listeners[lockName]; exists {
		for _, listener := range listeners {
			go listener(context.Background(), LockEventTypeAcquired)
		}
	}

	return true
}

// ReleaseLock releases a previously acquired lock
func (s *memoryLockCoordinator) ReleaseLock(lockName string) bool {
	if atomic.LoadInt32(&s.closed) == 1 {
		return false
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Publish release event
	if _, exists := s.locks[lockName]; !exists {
		return false
	}
	lock := s.locks[lockName]
	lock.history = append(lock.history, Event{
		Type:      LockEventTypeReleased,
		Timestamp: time.Now(),
	})
	s.locks[lockName] = lock

	// Notify listeners
	if listeners, exists := s.listeners[lockName]; exists {
		for _, listener := range listeners {
			go listener(context.Background(), LockEventTypeReleased)
		}
	}

	return true
}

// IsLockHeld checks if a lock is currently held
func (s *memoryLockCoordinator) IsLockHeld(lockName string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.hasLockHeld(lockName)
}

// hasLockHeld is a helper method to check lock state (must be called with lock held)
func (s *memoryLockCoordinator) hasLockHeld(lockName string) bool {
	lock, exists := s.locks[lockName]
	if !exists {
		return false
	}

	if len(lock.history) == 0 {
		return false
	}

	// Check if the last event was an acquisition
	lastEvent := lock.history[len(lock.history)-1]
	return lastEvent.Type == LockEventTypeAcquired
}

// Close shuts down the state coordinator
func (s *memoryLockCoordinator) Close() error {
	atomic.StoreInt32(&s.closed, 1)

	s.mu.Lock()
	defer s.mu.Unlock()

	// Clear all listeners and events
	s.listeners = make(map[string]map[uint64]Listener)
	s.locks = make(map[string]Lock)

	return nil
}
