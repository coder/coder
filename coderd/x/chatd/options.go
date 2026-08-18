package chatd

import (
	"context"
	"database/sql"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	dbpubsub "github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/coderd/notifications"
	"github.com/coder/coder/v2/coderd/x/chatd/messagepartbuffer"
	"github.com/coder/quartz"
)

const (
	defaultAcquisitionInterval   = 30 * time.Second
	defaultAcquisitionBatchSize  = int32(10)
	defaultRunnerSyncInterval    = 15 * time.Second
	defaultHeartbeatInterval     = 9 * time.Second
	defaultHeartbeatCleanupEvery = 30 * time.Second
	defaultHeartbeatStaleSeconds = int32(30)
	// The archive cutoff is based on UTC start-of-day and only moves
	// once per day, so hourly runs are more than enough to keep up
	// while still catching chats that cross the threshold shortly
	// after midnight.
	defaultArchiveInterval         = time.Hour
	defaultArchiveBatchSize        = int32(1000)
	defaultStateChannelSize        = 64
	defaultTaskRetryInitialBackoff = 100 * time.Millisecond
	defaultTaskRetryMaxBackoff     = 5 * time.Second
)

// chatWorkerPubsub is the chat worker pubsub dependency.
type chatWorkerPubsub interface {
	Publish(event string, message []byte) error
	SubscribeWithErr(event string, listener dbpubsub.ListenerWithErr) (func(), error)
}

// chatWorkerTaskStarter starts runner-owned side-effect tasks.
type chatWorkerTaskStarter interface {
	StartGeneration(context.Context, chatWorkerTaskStartInput) error
	StartInterrupt(context.Context, chatWorkerTaskStartInput) error
	StartRequiresActionTimeout(context.Context, chatWorkerTaskStartInput) error
	StartAbandon(context.Context, chatWorkerTaskStartInput) error
}

// chatWorkerTaskStartInput describes one runner task invocation.
type chatWorkerTaskStartInput struct {
	TaskID uuid.UUID
	ChatID uuid.UUID
	// TurnID is a process-local correlation ID minted per generation
	// task run. It groups the run's hook events; it is best-effort only
	// and never persisted.
	TurnID                   uuid.UUID
	WorkerID                 uuid.UUID
	RunnerID                 uuid.UUID
	HistoryVersion           int64
	GenerationAttempt        int64
	Status                   database.ChatStatus
	RequiresActionDeadlineAt sql.NullTime
	DebugTurn                *runnerDebugTurn
	SessionStart             *sessionStartTracker
	StopNudges               *stopNudgeTracker
	// InterruptSnapshot, set for interrupt tasks, is shared by every
	// retry attempt of the task instance so the first attempt's episode
	// snapshot survives buffer eviction during a stalled attempt. Nil
	// makes each attempt re-read the buffer.
	InterruptSnapshot *interruptEpisodeSnapshot
}

func (i chatWorkerTaskStartInput) hookTurnID() *uuid.UUID {
	if i.TurnID == uuid.Nil {
		return nil
	}
	turnID := i.TurnID
	return &turnID
}

// stopNudgeTracker allows at most one stop-hook nudge continuation per
// turn. Turns are keyed by the last user prompt's message ID so the
// claim survives task restarts, which mint fresh process-local turn
// IDs.
type stopNudgeTracker struct {
	mu      sync.Mutex
	turnKey int64
	claimed bool
	pending bool
}

// stopNudgeKey identifies the current turn by its prompt row. Model
// visibility user rows are hook context, not prompts.
func stopNudgeKey(messages []database.ChatMessage) int64 {
	index := lastUserPromptIndex(messages)
	if index == -1 {
		return 0
	}
	return messages[index].ID
}

func (t *stopNudgeTracker) claim(turnKey int64) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.turnKey != turnKey {
		t.turnKey = turnKey
		t.claimed = false
	}
	if t.claimed {
		return false
	}
	t.claimed = true
	t.pending = true
	return true
}

func (t *stopNudgeTracker) consume(turnKey int64) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.turnKey != turnKey || !t.pending {
		return false
	}
	t.pending = false
	return true
}

func (t *stopNudgeTracker) cancel(turnKey int64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.turnKey != turnKey || !t.pending {
		return
	}
	t.pending = false
	t.claimed = false
}

func (t *stopNudgeTracker) reset() {
	t.mu.Lock()
	t.turnKey = 0
	t.claimed = false
	t.pending = false
	t.mu.Unlock()
}

type sessionStartTracker struct {
	mu        sync.Mutex
	completed bool
	inFlight  chan struct{}
}

func (t *sessionStartTracker) claim(ctx context.Context) (bool, func(bool), error) {
	for {
		t.mu.Lock()
		if t.completed {
			t.mu.Unlock()
			return false, nil, nil
		}
		if t.inFlight == nil {
			t.inFlight = make(chan struct{})
			t.mu.Unlock()
			return true, func(completed bool) {
				t.mu.Lock()
				t.completed = completed
				close(t.inFlight)
				t.inFlight = nil
				t.mu.Unlock()
			}, nil
		}
		inFlight := t.inFlight
		t.mu.Unlock()
		select {
		case <-inFlight:
		case <-ctx.Done():
			return false, nil, ctx.Err()
		}
	}
}

// chatWorkerOptions configures a chatWorker.
type chatWorkerOptions struct {
	WorkerID uuid.UUID

	Store             database.Store
	Pubsub            chatWorkerPubsub
	Logger            slog.Logger
	Clock             quartz.Clock
	TaskStarter       chatWorkerTaskStarter
	MessagePartBuffer *messagepartbuffer.Buffer

	NotificationsEnqueuer notifications.Enqueuer
	Auditor               *atomic.Pointer[audit.Auditor]
	AutoArchiveRecords    prometheus.Counter

	AcquisitionInterval        time.Duration
	AcquisitionBatchSize       int32
	ArchiveInterval            time.Duration
	ArchiveBatchSize           int32
	RunnerSyncInterval         time.Duration
	HeartbeatInterval          time.Duration
	HeartbeatCleanupInterval   time.Duration
	HeartbeatStaleSeconds      int32
	StateChannelSize           int
	RunnerManagerChannelSize   int
	AcquisitionWakeChannelSize int
	TaskRetryInitialBackoff    time.Duration
	TaskRetryMaxBackoff        time.Duration
}

func (o chatWorkerOptions) withDefaults() (chatWorkerOptions, error) {
	if o.Store == nil {
		return chatWorkerOptions{}, xerrors.New("chatworker: store is required")
	}
	if o.Pubsub == nil {
		return chatWorkerOptions{}, xerrors.New("chatworker: pubsub is required")
	}
	if o.TaskStarter == nil && o.MessagePartBuffer == nil {
		return chatWorkerOptions{}, xerrors.New("chatworker: task starter or message part buffer is required")
	}
	if o.WorkerID == uuid.Nil {
		return chatWorkerOptions{}, xerrors.New("chatworker: worker ID is required")
	}
	if o.Clock == nil {
		o.Clock = quartz.NewReal()
	}
	if o.AcquisitionInterval <= 0 {
		o.AcquisitionInterval = defaultAcquisitionInterval
	}
	if o.AcquisitionBatchSize <= 0 {
		o.AcquisitionBatchSize = defaultAcquisitionBatchSize
	}
	if o.ArchiveInterval <= 0 {
		o.ArchiveInterval = defaultArchiveInterval
	}
	if o.ArchiveBatchSize <= 0 {
		o.ArchiveBatchSize = defaultArchiveBatchSize
	}
	if o.NotificationsEnqueuer == nil {
		o.NotificationsEnqueuer = notifications.NewNoopEnqueuer()
	}
	if o.RunnerSyncInterval <= 0 {
		o.RunnerSyncInterval = defaultRunnerSyncInterval
	}
	if o.HeartbeatInterval <= 0 {
		o.HeartbeatInterval = defaultHeartbeatInterval
	}
	if o.HeartbeatCleanupInterval <= 0 {
		o.HeartbeatCleanupInterval = defaultHeartbeatCleanupEvery
	}
	if o.HeartbeatStaleSeconds <= 0 {
		o.HeartbeatStaleSeconds = defaultHeartbeatStaleSeconds
	}
	if o.StateChannelSize <= 0 {
		o.StateChannelSize = defaultStateChannelSize
	}
	if o.RunnerManagerChannelSize <= 0 {
		o.RunnerManagerChannelSize = defaultStateChannelSize
	}
	if o.AcquisitionWakeChannelSize <= 0 {
		o.AcquisitionWakeChannelSize = 1
	}
	if o.TaskRetryInitialBackoff <= 0 {
		o.TaskRetryInitialBackoff = defaultTaskRetryInitialBackoff
	}
	if o.TaskRetryMaxBackoff <= 0 {
		o.TaskRetryMaxBackoff = defaultTaskRetryMaxBackoff
	}
	if o.TaskRetryMaxBackoff < o.TaskRetryInitialBackoff {
		o.TaskRetryMaxBackoff = o.TaskRetryInitialBackoff
	}
	return o, nil
}
