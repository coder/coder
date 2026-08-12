package chatd

import (
	"context"
	"database/sql"
	"encoding/json"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	coderdpubsub "github.com/coder/coder/v2/coderd/pubsub"
)

const shutdownCleanupTimeout = 5 * time.Second

type runnerKey struct {
	ChatID   uuid.UUID
	RunnerID uuid.UUID
}

type runnerStateUpdate struct {
	ChatID                   uuid.UUID
	WorkerID                 *uuid.UUID
	RunnerID                 *uuid.UUID
	SnapshotVersion          int64
	HistoryVersion           int64
	QueueVersion             int64
	GenerationAttempt        int64
	Status                   database.ChatStatus
	Archived                 bool
	RequiresActionDeadlineAt sql.NullTime
}

type spawnRunnerRequest struct {
	ChatID   uuid.UUID
	WorkerID uuid.UUID
	RunnerID uuid.UUID
}

type runnerRecord struct {
	key      runnerKey
	workerID uuid.UUID
	cancel   context.CancelFunc
	done     <-chan struct{}
	stateCh  chan runnerStateUpdate

	mu             sync.Mutex
	unsubscribe    func()
	cleanupStarted bool
}

func (r *runnerRecord) setUnsubscribe(unsubscribe func()) bool {
	r.mu.Lock()
	if r.cleanupStarted {
		r.mu.Unlock()
		if unsubscribe != nil {
			unsubscribe()
		}
		return false
	}
	r.unsubscribe = unsubscribe
	r.mu.Unlock()
	return true
}

func (r *runnerRecord) startCleanup() {
	r.mu.Lock()
	if r.cleanupStarted {
		r.mu.Unlock()
		return
	}
	r.cleanupStarted = true
	unsubscribe := r.unsubscribe
	r.unsubscribe = nil
	r.mu.Unlock()
	if unsubscribe != nil {
		unsubscribe()
	}
	r.cancel()
}

type runnerManager struct {
	server *Server
	opts   chatWorkerOptions
	ctx    context.Context

	closed  bool
	spawnMu sync.Mutex

	mu            sync.Mutex
	spawnCh       chan spawnRunnerRequest
	cleanupReqCh  chan runnerKey
	cleanupDoneCh chan runnerKey
	runners       map[runnerKey]*runnerRecord
	runnersByChat map[uuid.UUID]map[uuid.UUID]*runnerRecord
	cleaning      map[runnerKey]*runnerRecord

	wg sync.WaitGroup
}

func newRunnerManager(ctx context.Context, server *Server, opts chatWorkerOptions) *runnerManager {
	return &runnerManager{
		server:        server,
		opts:          opts,
		ctx:           ctx,
		spawnCh:       make(chan spawnRunnerRequest, opts.RunnerManagerChannelSize),
		cleanupReqCh:  make(chan runnerKey, opts.RunnerManagerChannelSize),
		cleanupDoneCh: make(chan runnerKey, opts.RunnerManagerChannelSize),
		runners:       make(map[runnerKey]*runnerRecord),
		runnersByChat: make(map[uuid.UUID]map[uuid.UUID]*runnerRecord),
		cleaning:      make(map[runnerKey]*runnerRecord),
	}
}

func (m *runnerManager) start() {
	m.wg.Go(m.run)
	m.wg.Go(m.databaseSyncLoop)
	m.wg.Go(m.heartbeatLoop)
	m.wg.Go(m.heartbeatCleanupLoop)
}

func (m *runnerManager) wait() {
	m.wg.Wait()
}

func (m *runnerManager) idle() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.runners) == 0 && len(m.cleaning) == 0
}

func (m *runnerManager) Spawn(ctx context.Context, req spawnRunnerRequest) error {
	m.spawnMu.Lock()
	defer m.spawnMu.Unlock()
	if m.closed {
		return xerrors.New("chatworker: runner manager closed")
	}

	select {
	case m.spawnCh <- req:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-m.ctx.Done():
		return m.ctx.Err()
	}
}

func (m *runnerManager) requestCleanup(ctx context.Context, key runnerKey) {
	select {
	case m.cleanupReqCh <- key:
	case <-ctx.Done():
	case <-m.ctx.Done():
	}
}

func (m *runnerManager) RouteStateHint(ctx context.Context, state runnerStateUpdate) {
	m.mu.Lock()
	byRunner := m.runnersByChat[state.ChatID]
	targets := make([]*runnerRecord, 0, len(byRunner))
	for _, rec := range byRunner {
		targets = append(targets, rec)
	}
	m.mu.Unlock()

	for _, rec := range targets {
		select {
		case rec.stateCh <- state:
		case <-rec.done:
			// Only this runner exited; keep fanning out to the rest.
			continue
		case <-ctx.Done():
			return
		case <-m.ctx.Done():
			return
		default:
			// stateCh is full; drop the hint for this runner.
		}
	}
}

func (m *runnerManager) run() {
	for {
		select {
		case req := <-m.spawnCh:
			m.handleSpawn(req)
		case key := <-m.cleanupReqCh:
			m.handleCleanupRequest(key)
		case key := <-m.cleanupDoneCh:
			m.handleCleanupDone(key)
		case <-m.ctx.Done():
			queued := m.closeAndDrainQueues()
			m.cancelAll()
			m.releaseOwnershipHints(queued)
			return
		}
	}
}

func (m *runnerManager) handleSpawn(req spawnRunnerRequest) {
	key := runnerKey{ChatID: req.ChatID, RunnerID: req.RunnerID}
	m.mu.Lock()
	if _, ok := m.runners[key]; ok {
		// A duplicate spawn for a live runner indicates a logic error
		// in the sync loop.
		m.opts.Logger.Error(m.ctx, "invalid spawn request: chat runner already spawned", slog.F("key", key))
		m.mu.Unlock()
		return
	}
	if _, ok := m.cleaning[key]; ok {
		// A duplicate spawn for a live runner indicates a logic error
		// in the sync loop.
		m.opts.Logger.Error(m.ctx, "invalid spawn request: chat runner in cleanup", slog.F("key", key))
		m.mu.Unlock()
		return
	}
	runnerCtx, cancel := context.WithCancel(m.ctx)
	done := make(chan struct{})
	rec := &runnerRecord{
		key:      key,
		workerID: req.WorkerID,
		cancel:   cancel,
		done:     done,
		stateCh:  make(chan runnerStateUpdate, m.opts.StateChannelSize),
	}
	m.runners[key] = rec
	if m.runnersByChat[req.ChatID] == nil {
		m.runnersByChat[req.ChatID] = make(map[uuid.UUID]*runnerRecord)
	}
	m.runnersByChat[req.ChatID][req.RunnerID] = rec
	m.mu.Unlock()

	r := newRunner(runnerCtx, m, rec, m.opts)
	m.wg.Go(func() {
		defer close(done)
		r.run()
	})
}

func (m *runnerManager) closeAndDrainQueues() []runnerKey {
	m.spawnMu.Lock()
	defer m.spawnMu.Unlock()

	m.closed = true
	return m.drainQueues()
}

func (m *runnerManager) drainQueues() []runnerKey {
	queued := make([]runnerKey, 0)
	for {
		select {
		case req := <-m.spawnCh:
			queued = append(queued, runnerKey{ChatID: req.ChatID, RunnerID: req.RunnerID})
		case key := <-m.cleanupReqCh:
			m.handleCleanupRequest(key)
		case key := <-m.cleanupDoneCh:
			m.handleCleanupDone(key)
		default:
			return queued
		}
	}
}

func (m *runnerManager) handleCleanupRequest(key runnerKey) {
	m.mu.Lock()
	rec, ok := m.runners[key]
	if !ok {
		m.mu.Unlock()
		return
	}
	delete(m.runners, key)
	if byChat := m.runnersByChat[key.ChatID]; byChat != nil {
		delete(byChat, key.RunnerID)
		if len(byChat) == 0 {
			delete(m.runnersByChat, key.ChatID)
		}
	}
	m.cleaning[key] = rec
	m.mu.Unlock()

	rec.startCleanup()
	m.registerCleanupWaiter(key, rec)
}

func (m *runnerManager) registerCleanupWaiter(key runnerKey, rec *runnerRecord) {
	m.wg.Go(func() {
		<-rec.done
		if m.ctx.Err() != nil {
			m.mu.Lock()
			delete(m.cleaning, key)
			m.mu.Unlock()
			return
		}
		select {
		case m.cleanupDoneCh <- key:
		case <-m.ctx.Done():
			m.mu.Lock()
			delete(m.cleaning, key)
			m.mu.Unlock()
		}
	})
}

func (m *runnerManager) handleCleanupDone(key runnerKey) {
	m.mu.Lock()
	delete(m.cleaning, key)
	m.mu.Unlock()
}

func (m *runnerManager) cancelAll() {
	type cleanupTarget struct {
		key runnerKey
		rec *runnerRecord
	}

	m.mu.Lock()
	active := make([]cleanupTarget, 0, len(m.runners))
	cleaning := make([]*runnerRecord, 0, len(m.cleaning))
	for _, rec := range m.cleaning {
		cleaning = append(cleaning, rec)
	}
	for key, rec := range m.runners {
		delete(m.runners, key)
		m.cleaning[key] = rec
		active = append(active, cleanupTarget{key: key, rec: rec})
	}
	clear(m.runnersByChat)
	m.mu.Unlock()

	keys := make([]runnerKey, 0, len(cleaning)+len(active))
	for _, rec := range cleaning {
		rec.startCleanup()
		keys = append(keys, rec.key)
	}
	for _, target := range active {
		target.rec.startCleanup()
		m.registerCleanupWaiter(target.key, target.rec)
		keys = append(keys, target.key)
	}
	m.releaseOwnershipHints(keys)
}

func (m *runnerManager) releaseOwnershipHints(keys []runnerKey) {
	if len(keys) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(m.ctx), shutdownCleanupTimeout)
	defer cancel()

	chatIDs := make([]uuid.UUID, 0, len(keys))
	runnerIDs := make([]uuid.UUID, 0, len(keys))
	uniqueChatIDs := make(map[uuid.UUID]struct{}, len(keys))
	for _, key := range keys {
		chatIDs = append(chatIDs, key.ChatID)
		runnerIDs = append(runnerIDs, key.RunnerID)
		uniqueChatIDs[key.ChatID] = struct{}{}
	}
	if _, err := m.opts.Store.BatchDeleteChatHeartbeats(ctx, database.BatchDeleteChatHeartbeatsParams{
		ChatIds:   chatIDs,
		RunnerIds: runnerIDs,
	}); err != nil {
		m.opts.Logger.Warn(ctx, "chatworker shutdown heartbeat cleanup failed", slogError(err))
	}

	syncIDs := make([]uuid.UUID, 0, len(uniqueChatIDs))
	for id := range uniqueChatIDs {
		syncIDs = append(syncIDs, id)
	}
	chats, err := m.opts.Store.GetChatsByIDsForRunnerSync(ctx, syncIDs)
	if err != nil {
		m.opts.Logger.Warn(ctx, "chatworker shutdown ownership lookup failed", slogError(err))
	}
	snapshotByChat := make(map[uuid.UUID]int64, len(chats))
	for _, chat := range chats {
		snapshotByChat[chat.ID] = chat.SnapshotVersion
	}
	for _, key := range keys {
		payload, err := json.Marshal(coderdpubsub.ChatStateOwnershipMessage{
			ChatID:          key.ChatID,
			SnapshotVersion: snapshotByChat[key.ChatID],
		})
		if err != nil {
			m.opts.Logger.Warn(ctx, "chatworker shutdown ownership marshal failed", slogError(err))
			continue
		}
		if err := m.opts.Pubsub.Publish(coderdpubsub.ChatStateOwnershipChannel, payload); err != nil {
			m.opts.Logger.Warn(ctx, "chatworker shutdown ownership publish failed", slogError(err))
		}
	}
}

func (m *runnerManager) snapshotRunnerKeys() []runnerKey {
	m.mu.Lock()
	defer m.mu.Unlock()
	keys := make([]runnerKey, 0, len(m.runners))
	for key := range m.runners {
		keys = append(keys, key)
	}
	return keys
}

func (m *runnerManager) databaseSyncLoop() {
	ticker := m.opts.Clock.NewTicker(m.opts.RunnerSyncInterval, "chatworker", "runner-sync")
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if err := m.syncOnce(m.ctx); err != nil {
				m.opts.Logger.Warn(m.ctx, "chatworker runner sync failed", slogError(err))
			}
		case <-m.ctx.Done():
			return
		}
	}
}

func (m *runnerManager) syncOnce(ctx context.Context) error {
	keys := m.snapshotRunnerKeys()
	if len(keys) == 0 {
		return nil
	}
	idsByChat := make(map[uuid.UUID]struct{}, len(keys))
	for _, key := range keys {
		idsByChat[key.ChatID] = struct{}{}
	}
	chatIDs := make([]uuid.UUID, 0, len(idsByChat))
	for id := range idsByChat {
		chatIDs = append(chatIDs, id)
	}
	chats, err := m.opts.Store.GetChatsByIDsForRunnerSync(ctx, chatIDs)
	if err != nil {
		return xerrors.Errorf("get chats for runner sync: %w", err)
	}
	seen := make(map[uuid.UUID]struct{}, len(chats))
	for _, chat := range chats {
		seen[chat.ID] = struct{}{}
		m.RouteStateHint(ctx, stateUpdateFromChat(chat))
	}
	for _, key := range keys {
		if _, ok := seen[key.ChatID]; !ok {
			m.requestCleanup(ctx, key)
		}
	}
	return nil
}

func (m *runnerManager) heartbeatLoop() {
	ticker := m.opts.Clock.NewTicker(m.opts.HeartbeatInterval, "chatworker", "heartbeat")
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if err := m.heartbeatOnce(m.ctx); err != nil {
				m.opts.Logger.Warn(m.ctx, "chatworker heartbeat failed", slogError(err))
			}
		case <-m.ctx.Done():
			return
		}
	}
}

func (m *runnerManager) heartbeatOnce(ctx context.Context) error {
	keys := m.snapshotRunnerKeys()
	if len(keys) == 0 {
		return nil
	}
	chatIDs := make([]uuid.UUID, 0, len(keys))
	runnerIDs := make([]uuid.UUID, 0, len(keys))
	for _, key := range keys {
		chatIDs = append(chatIDs, key.ChatID)
		runnerIDs = append(runnerIDs, key.RunnerID)
	}
	return m.opts.Store.BatchUpsertChatHeartbeats(ctx, database.BatchUpsertChatHeartbeatsParams{
		ChatIds:   chatIDs,
		RunnerIds: runnerIDs,
	})
}

func (m *runnerManager) heartbeatCleanupLoop() {
	ticker := m.opts.Clock.NewTicker(m.opts.HeartbeatCleanupInterval, "chatworker", "heartbeat-cleanup")
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if err := m.heartbeatCleanupOnce(m.ctx); err != nil {
				m.opts.Logger.Warn(m.ctx, "chatworker heartbeat cleanup failed", slogError(err))
			}
		case <-m.ctx.Done():
			return
		}
	}
}

func (m *runnerManager) heartbeatCleanupOnce(ctx context.Context) error {
	_, err := m.opts.Store.DeleteStaleChatHeartbeats(ctx, m.opts.HeartbeatStaleSeconds)
	return err
}

func stateUpdateFromChat(chat database.Chat) runnerStateUpdate {
	var workerID *uuid.UUID
	if chat.WorkerID.Valid {
		id := chat.WorkerID.UUID
		workerID = &id
	}
	var runnerID *uuid.UUID
	if chat.RunnerID.Valid {
		id := chat.RunnerID.UUID
		runnerID = &id
	}
	return runnerStateUpdate{
		ChatID:                   chat.ID,
		WorkerID:                 workerID,
		RunnerID:                 runnerID,
		SnapshotVersion:          chat.SnapshotVersion,
		HistoryVersion:           chat.HistoryVersion,
		QueueVersion:             chat.QueueVersion,
		GenerationAttempt:        chat.GenerationAttempt,
		Status:                   chat.Status,
		Archived:                 chat.Archived,
		RequiresActionDeadlineAt: chat.RequiresActionDeadlineAt,
	}
}

func slogError(err error) slog.Field {
	return slog.Error(err)
}
