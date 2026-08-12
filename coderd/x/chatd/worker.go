package chatd

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	coderdpubsub "github.com/coder/coder/v2/coderd/pubsub"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
)

// chatWorker owns chat acquisition and runner lifecycle for one process.
type chatWorker struct {
	server *Server
	opts   chatWorkerOptions

	mu          sync.Mutex
	started     bool
	ctx         context.Context
	cancel      context.CancelFunc
	manager     *runnerManager
	unsubscribe func()
	wakeCh      chan struct{}
	wg          sync.WaitGroup
}

// newChatWorker constructs a chat worker. The worker is idle until Start is
// called.
func newChatWorker(server *Server, opts chatWorkerOptions) (*chatWorker, error) {
	if server == nil {
		return nil, xerrors.New("chatworker: server is required")
	}
	withDefaults, err := opts.withDefaults()
	if err != nil {
		return nil, err
	}
	return &chatWorker{server: server, opts: withDefaults}, nil
}

// chatWorkerID returns this worker's configured worker ID.
func (w *chatWorker) chatWorkerID() uuid.UUID {
	return w.opts.WorkerID
}

// Start starts the acquisition and runner manager loops.
func (w *chatWorker) Start(ctx context.Context) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.started {
		return xerrors.New("chatworker: worker already started")
	}
	workerID := w.opts.WorkerID
	workerCtx, cancel := context.WithCancel(ctx)
	manager := newRunnerManager(workerCtx, w.server, w.opts)
	if manager.opts.TaskStarter == nil {
		starter, err := newTaskStarter(manager.server, manager.opts, manager.RouteStateHint, manager.requestCleanup)
		if err != nil {
			cancel()
			return err
		}
		manager.opts.TaskStarter = starter
	}
	wakeCh := make(chan struct{}, w.opts.AcquisitionWakeChannelSize)

	unsubscribe, err := w.opts.Pubsub.SubscribeWithErr(
		coderdpubsub.ChatStateOwnershipChannel,
		coderdpubsub.HandleChatStateOwnership(func(ctx context.Context, _ coderdpubsub.ChatStateOwnershipMessage, err error) {
			if err != nil {
				w.opts.Logger.Warn(ctx, "chatworker ownership hint decode failed", slogError(err))
				return
			}
			wake(wakeCh)
		}),
	)
	if err != nil {
		cancel()
		return xerrors.Errorf("subscribe ownership hints: %w", err)
	}

	w.started = true
	w.ctx = workerCtx
	w.cancel = cancel
	w.manager = manager
	w.unsubscribe = unsubscribe
	w.wakeCh = wakeCh

	manager.start()
	w.wg.Go(func() {
		w.acquisitionLoop(workerCtx, workerID, manager, wakeCh)
	})
	w.wg.Go(func() {
		w.archiveLoop(workerCtx)
	})
	wake(wakeCh)
	return nil
}

// Wake requests an immediate acquisition pass.
func (w *chatWorker) Wake() {
	w.mu.Lock()
	wakeCh := w.wakeCh
	w.mu.Unlock()
	if wakeCh != nil {
		wake(wakeCh)
	}
}

// WaitIdle waits until the worker has no active or cleaning runners.
func (w *chatWorker) WaitIdle(ctx context.Context) error {
	for {
		w.mu.Lock()
		manager := w.manager
		w.mu.Unlock()
		if manager == nil || manager.idle() {
			return nil
		}
		timer := w.opts.Clock.NewTimer(10*time.Millisecond, "chatworker", "wait-idle")
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		}
		timer.Stop()
	}
}

// Close stops the worker and waits for its loops to exit.
func (w *chatWorker) Close() error {
	w.mu.Lock()
	if !w.started {
		w.mu.Unlock()
		return nil
	}
	cancel := w.cancel
	unsubscribe := w.unsubscribe
	manager := w.manager
	w.started = false
	w.cancel = nil
	w.unsubscribe = nil
	w.manager = nil
	w.wakeCh = nil
	w.mu.Unlock()

	if unsubscribe != nil {
		unsubscribe()
	}
	cancel()
	w.wg.Wait()
	if manager != nil {
		manager.wait()
	}
	return nil
}

func wake(ch chan<- struct{}) {
	select {
	case ch <- struct{}{}:
	default:
	}
}

func (w *chatWorker) acquisitionLoop(
	ctx context.Context,
	workerID uuid.UUID,
	manager *runnerManager,
	wakeCh <-chan struct{},
) {
	ticker := w.opts.Clock.NewTicker(w.opts.AcquisitionInterval, "chatworker", "acquisition")
	defer ticker.Stop()
	for {
		select {
		case <-wakeCh:
			w.acquireOnce(ctx, workerID, manager)
		case <-ticker.C:
			w.acquireOnce(ctx, workerID, manager)
		case <-ctx.Done():
			return
		}
	}
}

func (w *chatWorker) acquireOnce(ctx context.Context, workerID uuid.UUID, manager *runnerManager) {
	attempted := make(map[uuid.UUID]struct{})
	for {
		rows, err := w.opts.Store.GetChatWorkerAcquisitionCandidates(ctx, database.GetChatWorkerAcquisitionCandidatesParams{
			StaleSeconds: w.opts.HeartbeatStaleSeconds,
			LimitCount:   w.opts.AcquisitionBatchSize,
		})
		if err != nil {
			if ctx.Err() == nil {
				w.opts.Logger.Warn(ctx, "chatworker acquisition query failed", slogError(err))
			}
			return
		}
		if len(rows) == 0 {
			return
		}
		newRows := 0
		for _, row := range rows {
			if _, ok := attempted[row.ID]; ok {
				continue
			}
			attempted[row.ID] = struct{}{}
			newRows++
			if err := w.acquireCandidateSafely(ctx, workerID, manager, row.ID); err != nil {
				if ctx.Err() != nil {
					return
				}
				w.opts.Logger.Warn(ctx, "chatworker acquisition candidate failed", slogError(err))
			}
		}
		if len(rows) < int(w.opts.AcquisitionBatchSize) || newRows == 0 {
			return
		}
	}
}

var errSkipAcquire = xerrors.New("skip acquire")

func (w *chatWorker) acquireCandidateSafely(
	ctx context.Context,
	workerID uuid.UUID,
	manager *runnerManager,
	chatID uuid.UUID,
) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = xerrors.Errorf("chatworker acquisition panic: %v", recovered)
		}
	}()
	return w.acquireCandidate(ctx, workerID, manager, chatID)
}

func (w *chatWorker) acquireCandidate(
	ctx context.Context,
	workerID uuid.UUID,
	manager *runnerManager,
	chatID uuid.UUID,
) error {
	runnerID := uuid.New()
	machine := chatstate.NewChatMachine(w.opts.Store, w.opts.Pubsub, chatID)
	err := machine.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
		chat, err := store.GetChatByID(ctx, chatID)
		if errors.Is(err, sql.ErrNoRows) {
			return errSkipAcquire
		}
		if err != nil {
			return xerrors.Errorf("load chat: %w", err)
		}
		queueCount, err := store.CountChatQueuedMessages(ctx, chatID)
		if err != nil {
			return xerrors.Errorf("count queue: %w", err)
		}
		if !chatstate.ClassifyExecutionState(chat, queueCount > 0, true).IsRunnable() || chat.Archived {
			return errSkipAcquire
		}
		if chat.WorkerID.Valid && chat.RunnerID.Valid {
			stale, err := store.IsChatHeartbeatStale(ctx, database.IsChatHeartbeatStaleParams{
				ChatID:       chat.ID,
				RunnerID:     chat.RunnerID.UUID,
				StaleSeconds: w.opts.HeartbeatStaleSeconds,
			})
			if err != nil {
				return xerrors.Errorf("check heartbeat stale: %w", err)
			}
			if !stale {
				return errSkipAcquire
			}
		}
		_, err = tx.Acquire(chatstate.AcquireInput{WorkerID: workerID, RunnerID: runnerID})
		return err
	})
	if errors.Is(err, errSkipAcquire) || errors.Is(err, chatstate.ErrChatNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := manager.Spawn(ctx, spawnRunnerRequest{ChatID: chatID, WorkerID: workerID, RunnerID: runnerID}); err != nil {
		if errAbandon := w.abandonAcquiredChat(ctx, workerID, runnerID, chatID); errAbandon != nil {
			return errors.Join(err, errAbandon)
		}
		return err
	}
	return nil
}

func (w *chatWorker) abandonAcquiredChat(ctx context.Context, workerID uuid.UUID, runnerID uuid.UUID, chatID uuid.UUID) error {
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownCleanupTimeout)
	defer cancel()
	machine := chatstate.NewChatMachine(w.opts.Store, w.opts.Pubsub, chatID)
	err := machine.Update(cleanupCtx, func(tx *chatstate.Tx, store database.Store) error {
		chat, err := store.GetChatByID(cleanupCtx, chatID)
		if errors.Is(err, sql.ErrNoRows) {
			return errSkipAcquire
		}
		if err != nil {
			return xerrors.Errorf("load chat: %w", err)
		}
		if !chat.WorkerID.Valid || chat.WorkerID.UUID != workerID || !chat.RunnerID.Valid || chat.RunnerID.UUID != runnerID {
			return errSkipAcquire
		}
		_, err = tx.Abandon(chatstate.AbandonInput{})
		return err
	})
	if errors.Is(err, errSkipAcquire) || errors.Is(err, chatstate.ErrChatNotFound) {
		return nil
	}
	return err
}
