package chatd //nolint:testpackage // Uses unexported chatworker helpers.

import (
	"context"
	"database/sql"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	dbpubsub "github.com/coder/coder/v2/coderd/database/pubsub"
	coderdpubsub "github.com/coder/coder/v2/coderd/pubsub"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

type workerTestFixture struct {
	db     database.Store
	pubsub dbpubsub.Pubsub
	sqlDB  *sql.DB
	user   database.User
	org    database.Organization
	model  database.ChatModelConfig
	apiKey database.APIKey
}

type publishedEvent struct {
	channel string
	payload []byte
}

type recordingPubsub struct {
	// Embed the full pubsub so a recordingPubsub can also stand in for a
	// Server's pubsub, recording every published event.
	dbpubsub.Pubsub
	mu     sync.Mutex
	events []publishedEvent
}

func newRecordingPubsub(inner dbpubsub.Pubsub) *recordingPubsub {
	return &recordingPubsub{Pubsub: inner}
}

func (p *recordingPubsub) Publish(channel string, payload []byte) error {
	p.mu.Lock()
	p.events = append(p.events, publishedEvent{
		channel: channel,
		payload: append([]byte(nil), payload...),
	})
	p.mu.Unlock()
	return p.Pubsub.Publish(channel, payload)
}

func (p *recordingPubsub) ownershipMessages(t *testing.T) []coderdpubsub.ChatStateOwnershipMessage {
	t.Helper()
	p.mu.Lock()
	defer p.mu.Unlock()
	messages := make([]coderdpubsub.ChatStateOwnershipMessage, 0)
	for _, event := range p.events {
		if event.channel != coderdpubsub.ChatStateOwnershipChannel {
			continue
		}
		var msg coderdpubsub.ChatStateOwnershipMessage
		require.NoError(t, json.Unmarshal(event.payload, &msg))
		messages = append(messages, msg)
	}
	return messages
}

func (p *recordingPubsub) watchEvents(t *testing.T) []codersdk.ChatWatchEvent {
	t.Helper()
	p.mu.Lock()
	defer p.mu.Unlock()
	events := make([]codersdk.ChatWatchEvent, 0)
	for _, event := range p.events {
		var msg codersdk.ChatWatchEvent
		if err := json.Unmarshal(event.payload, &msg); err != nil {
			continue
		}
		if event.channel != coderdpubsub.ChatWatchEventChannel(msg.Chat.OwnerID) {
			continue
		}
		events = append(events, msg)
	}
	return events
}

func (p *recordingPubsub) stateUpdateMessages(t *testing.T, chatID uuid.UUID) []coderdpubsub.ChatStateUpdateMessage {
	t.Helper()
	p.mu.Lock()
	defer p.mu.Unlock()
	messages := make([]coderdpubsub.ChatStateUpdateMessage, 0)
	for _, event := range p.events {
		if event.channel != coderdpubsub.ChatStateUpdateChannel(chatID) {
			continue
		}
		var msg coderdpubsub.ChatStateUpdateMessage
		require.NoError(t, json.Unmarshal(event.payload, &msg))
		messages = append(messages, msg)
	}
	return messages
}

func newWorkerTestFixture(t *testing.T) *workerTestFixture {
	t.Helper()
	db, ps, sqlDB := dbtestutil.NewDBWithSQLDB(t)
	user := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})
	dbgen.OrganizationMember(t, db, database.OrganizationMember{
		UserID:         user.ID,
		OrganizationID: org.ID,
	})
	dbgen.ChatProvider(t, db, database.ChatProvider{
		Provider:    "openai",
		DisplayName: "openai",
		BaseUrl:     "http://example.invalid",
	})
	model := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{
		IsDefault:      true,
		OrganizationID: org.ID,
	})
	apiKey, _ := dbgen.APIKey(t, db, database.APIKey{UserID: user.ID})
	return &workerTestFixture{db: db, pubsub: ps, sqlDB: sqlDB, user: user, org: org, model: model, apiKey: apiKey}
}

func (f *workerTestFixture) createRunningChat(t *testing.T) database.Chat {
	t.Helper()
	ctx := testutil.Context(t, testutil.WaitShort)
	res, err := chatstate.CreateChat(ctx, f.db, f.pubsub, chatstate.CreateChatInput{
		OrganizationID:    f.org.ID,
		OwnerID:           f.user.ID,
		LastModelConfigID: f.model.ID,
		Title:             "test",
		ClientType:        database.ChatClientTypeApi,
		InitialMessages: []chatstate.Message{
			userTextMessage(t, "hello", f.user.ID, f.model.ID, f.apiKey.ID),
		},
	})
	require.NoError(t, err)
	return res.Chat
}

func (f *workerTestFixture) createRunningSubagentChat(t *testing.T, parentID uuid.UUID) database.Chat {
	t.Helper()
	ctx := testutil.Context(t, testutil.WaitShort)
	res, err := chatstate.CreateChat(ctx, f.db, f.pubsub, chatstate.CreateChatInput{
		OrganizationID:    f.org.ID,
		OwnerID:           f.user.ID,
		LastModelConfigID: f.model.ID,
		Title:             "subagent",
		ClientType:        database.ChatClientTypeApi,
		ParentChatID:      uuid.NullUUID{UUID: parentID, Valid: true},
		RootChatID:        uuid.NullUUID{UUID: parentID, Valid: true},
		InitialMessages: []chatstate.Message{
			userTextMessage(t, "hello", f.user.ID, f.model.ID, f.apiKey.ID),
		},
	})
	require.NoError(t, err)
	return res.Chat
}

func (f *workerTestFixture) createRequiresActionChat(t *testing.T) database.Chat {
	t.Helper()
	ctx := testutil.Context(t, testutil.WaitShort)
	toolName := "dynamic_" + uuid.NewString()
	dynamicTools, err := json.Marshal([]codersdk.DynamicTool{{
		Name:        toolName,
		Description: "test tool",
		InputSchema: json.RawMessage(`{"type":"object"}`),
	}})
	require.NoError(t, err)
	res, err := chatstate.CreateChat(ctx, f.db, f.pubsub, chatstate.CreateChatInput{
		OrganizationID:    f.org.ID,
		OwnerID:           f.user.ID,
		LastModelConfigID: f.model.ID,
		Title:             "test",
		ClientType:        database.ChatClientTypeApi,
		DynamicTools: pqtype.NullRawMessage{
			RawMessage: dynamicTools,
			Valid:      true,
		},
		InitialMessages: []chatstate.Message{
			userTextMessage(t, "hello", f.user.ID, f.model.ID, f.apiKey.ID),
		},
	})
	require.NoError(t, err)
	machine := chatstate.NewChatMachine(f.db, f.pubsub, res.Chat.ID)
	require.NoError(t, machine.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
		_, err := tx.CommitStep(chatstate.CommitStepInput{
			Messages: []chatstate.Message{
				assistantToolCallMessage(t, f.model.ID, toolName),
			},
		})
		return err
	}))
	require.NoError(t, machine.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
		_, err := tx.EnterRequiresAction(chatstate.EnterRequiresActionInput{})
		return err
	}))
	chat, err := f.db.GetChatByID(ctx, res.Chat.ID)
	require.NoError(t, err)
	return chat
}

func userTextMessage(t *testing.T, text string, createdBy uuid.UUID, modelConfigID uuid.UUID, apiKeyID string) chatstate.Message {
	t.Helper()
	raw, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{codersdk.ChatMessageText(text)})
	require.NoError(t, err)
	return chatstate.Message{
		Role:           database.ChatMessageRoleUser,
		Content:        raw,
		Visibility:     database.ChatMessageVisibilityBoth,
		ContentVersion: chatprompt.CurrentContentVersion,
		CreatedBy:      uuid.NullUUID{UUID: createdBy, Valid: true},
		ModelConfigID:  uuid.NullUUID{UUID: modelConfigID, Valid: true},
	}
}

func assistantTextMessage(t *testing.T, text string, modelConfigID uuid.UUID) chatstate.Message {
	t.Helper()
	raw, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{codersdk.ChatMessageText(text)})
	require.NoError(t, err)
	return chatstate.Message{
		Role:           database.ChatMessageRoleAssistant,
		Content:        raw,
		Visibility:     database.ChatMessageVisibilityBoth,
		ContentVersion: chatprompt.CurrentContentVersion,
		ModelConfigID:  uuid.NullUUID{UUID: modelConfigID, Valid: true},
	}
}

func assistantToolCallMessage(t *testing.T, modelConfigID uuid.UUID, toolName string) chatstate.Message {
	t.Helper()
	raw, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{{
		Type:       codersdk.ChatMessagePartTypeToolCall,
		ToolCallID: "call_" + uuid.NewString(),
		ToolName:   toolName,
		Args:       json.RawMessage(`{}`),
	}})
	require.NoError(t, err)
	return chatstate.Message{
		Role:           database.ChatMessageRoleAssistant,
		Content:        raw,
		Visibility:     database.ChatMessageVisibilityBoth,
		ContentVersion: chatprompt.CurrentContentVersion,
		ModelConfigID:  uuid.NullUUID{UUID: modelConfigID, Valid: true},
	}
}

func testOptions(t *testing.T, f *workerTestFixture, starter chatWorkerTaskStarter) chatWorkerOptions {
	t.Helper()
	if starter == nil {
		starter = newRecordingTaskStarter()
	}
	return chatWorkerOptions{
		WorkerID:                   uuid.New(),
		Store:                      f.db,
		Pubsub:                     f.pubsub,
		Logger:                     testutil.Logger(t),
		TaskStarter:                starter,
		AcquisitionInterval:        time.Hour,
		AcquisitionBatchSize:       10,
		RunnerSyncInterval:         time.Hour,
		HeartbeatInterval:          time.Hour,
		HeartbeatCleanupInterval:   time.Hour,
		HeartbeatStaleSeconds:      30,
		StateChannelSize:           16,
		RunnerManagerChannelSize:   16,
		AcquisitionWakeChannelSize: 1,
	}
}

// newUnstartedServer builds a real Server backed by the given pubsub and
// store. The server is never started; it only provides the dependencies
// that workers and task starters dereference.
func newUnstartedServer(t *testing.T, ps dbpubsub.Pubsub, db database.Store) *Server {
	t.Helper()
	server := New(ps, Config{
		Logger:    testutil.Logger(t),
		Database:  db,
		ReplicaID: uuid.New(),
	})
	t.Cleanup(func() { _ = server.Close() })
	return server
}

func startWorker(t *testing.T, opts chatWorkerOptions) *chatWorker {
	t.Helper()
	ps, ok := opts.Pubsub.(dbpubsub.Pubsub)
	require.True(t, ok, "worker pubsub must implement the full pubsub interface")
	worker, err := newChatWorker(newUnstartedServer(t, ps, opts.Store), opts)
	require.NoError(t, err)
	require.NoError(t, worker.Start(context.Background()))
	t.Cleanup(func() { require.NoError(t, worker.Close()) })
	return worker
}

type taskCall struct {
	kind  taskKind
	input chatWorkerTaskStartInput
	ctx   context.Context
}

type releaseGate struct {
	once sync.Once
	ch   chan struct{}
}

type recordingTaskStarter struct {
	mu           sync.Mutex
	calls        []taskCall
	callCh       chan taskCall
	releases     []*releaseGate
	block        bool
	ignoreCancel bool
}

func newRecordingTaskStarter() *recordingTaskStarter {
	return &recordingTaskStarter{callCh: make(chan taskCall, 128)}
}

func newBlockingTaskStarter(ignoreCancel bool) *recordingTaskStarter {
	return &recordingTaskStarter{
		callCh:       make(chan taskCall, 128),
		block:        true,
		ignoreCancel: ignoreCancel,
	}
}

func (s *recordingTaskStarter) StartGeneration(ctx context.Context, input chatWorkerTaskStartInput) error {
	return s.start(ctx, taskKindGeneration, input)
}

func (s *recordingTaskStarter) StartInterrupt(ctx context.Context, input chatWorkerTaskStartInput) error {
	return s.start(ctx, taskKindInterrupt, input)
}

func (s *recordingTaskStarter) StartRequiresActionTimeout(ctx context.Context, input chatWorkerTaskStartInput) error {
	return s.start(ctx, taskKindRequiresActionTimeout, input)
}

func (s *recordingTaskStarter) StartAbandon(ctx context.Context, input chatWorkerTaskStartInput) error {
	return s.start(ctx, taskKindAbandon, input)
}

func (s *recordingTaskStarter) start(ctx context.Context, kind taskKind, input chatWorkerTaskStartInput) error {
	call := taskCall{kind: kind, input: input, ctx: ctx}
	var gate *releaseGate
	s.mu.Lock()
	if s.block {
		gate = &releaseGate{ch: make(chan struct{})}
		s.releases = append(s.releases, gate)
	}
	s.calls = append(s.calls, call)
	s.mu.Unlock()
	s.callCh <- call
	if gate == nil {
		return nil
	}
	if s.ignoreCancel {
		<-gate.ch
		return nil
	}
	select {
	case <-gate.ch:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *recordingTaskStarter) waitCall(t *testing.T, kind taskKind, chatID uuid.UUID) taskCall {
	t.Helper()
	deadline := time.After(testutil.WaitLong)
	for {
		select {
		case call := <-s.callCh:
			if (kind == "" || call.kind == kind) && (chatID == uuid.Nil || call.input.ChatID == chatID) {
				return call
			}
		case <-deadline:
			t.Fatalf("timed out waiting for task call kind=%q chat_id=%s", kind, chatID)
			return taskCall{}
		}
	}
}

func (s *recordingTaskStarter) assertNoCall(t *testing.T) {
	t.Helper()
	select {
	case call := <-s.callCh:
		t.Fatalf("unexpected task call: %s for chat %s", call.kind, call.input.ChatID)
	case <-time.After(100 * time.Millisecond):
	}
}

func (s *recordingTaskStarter) release(t *testing.T, index int) {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	require.Less(t, index, len(s.releases))
	s.releases[index].once.Do(func() { close(s.releases[index].ch) })
}

func (s *recordingTaskStarter) releaseAll() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, gate := range s.releases {
		gate.once.Do(func() { close(gate.ch) })
	}
}

func finishTurn(t *testing.T, f *workerTestFixture, chatID uuid.UUID) database.Chat {
	t.Helper()
	ctx := testutil.Context(t, testutil.WaitShort)
	machine := chatstate.NewChatMachine(f.db, f.pubsub, chatID)
	require.NoError(t, machine.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
		_, err := tx.FinishTurn(chatstate.FinishTurnInput{})
		return err
	}))
	chat, err := f.db.GetChatByID(ctx, chatID)
	require.NoError(t, err)
	return chat
}

func commitAssistantStep(t *testing.T, f *workerTestFixture, chatID uuid.UUID, text string) database.Chat {
	t.Helper()
	ctx := testutil.Context(t, testutil.WaitShort)
	machine := chatstate.NewChatMachine(f.db, f.pubsub, chatID)
	require.NoError(t, machine.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
		_, err := tx.CommitStep(chatstate.CommitStepInput{
			Messages: []chatstate.Message{assistantTextMessage(t, text, f.model.ID)},
		})
		return err
	}))
	chat, err := f.db.GetChatByID(ctx, chatID)
	require.NoError(t, err)
	return chat
}

func interruptChat(t *testing.T, f *workerTestFixture, chatID uuid.UUID) database.Chat {
	t.Helper()
	ctx := testutil.Context(t, testutil.WaitShort)
	machine := chatstate.NewChatMachine(f.db, f.pubsub, chatID)
	require.NoError(t, machine.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
		_, err := tx.SendMessage(chatstate.SendMessageInput{
			Message:      userTextMessage(t, "interrupt", f.user.ID, f.model.ID, f.apiKey.ID),
			BusyBehavior: chatstate.BusyBehaviorInterrupt,
		})
		return err
	}))
	chat, err := f.db.GetChatByID(ctx, chatID)
	require.NoError(t, err)
	return chat
}

func acquireChat(t *testing.T, f *workerTestFixture, chatID uuid.UUID, workerID uuid.UUID, runnerID uuid.UUID) database.Chat {
	t.Helper()
	ctx := testutil.Context(t, testutil.WaitShort)
	machine := chatstate.NewChatMachine(f.db, f.pubsub, chatID)
	require.NoError(t, machine.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
		_, err := tx.Acquire(chatstate.AcquireInput{WorkerID: workerID, RunnerID: runnerID})
		return err
	}))
	chat, err := f.db.GetChatByID(ctx, chatID)
	require.NoError(t, err)
	return chat
}

func forceExecutionState(
	t *testing.T,
	f *workerTestFixture,
	chatID uuid.UUID,
	status database.ChatStatus,
	archived bool,
) database.Chat {
	t.Helper()
	ctx := testutil.Context(t, testutil.WaitShort)
	var updated database.Chat
	require.NoError(t, f.db.InTx(func(store database.Store) error {
		if _, err := store.LockChatAndBumpSnapshotVersion(ctx, chatID); err != nil {
			return err
		}
		chat, err := store.GetChatByID(ctx, chatID)
		if err != nil {
			return err
		}
		updated, err = store.UpdateChatExecutionState(ctx, database.UpdateChatExecutionStateParams{
			ID:                       chat.ID,
			Status:                   status,
			Archived:                 archived,
			WorkerID:                 chat.WorkerID,
			RunnerID:                 chat.RunnerID,
			LastError:                chat.LastError,
			RequiresActionDeadlineAt: chat.RequiresActionDeadlineAt,
		})
		return err
	}, nil))
	return updated
}

func forceExecutionStateAndPublish(
	t *testing.T,
	f *workerTestFixture,
	chatID uuid.UUID,
	status database.ChatStatus,
	archived bool,
) database.Chat {
	t.Helper()
	updated := forceExecutionState(t, f, chatID, status, archived)
	publishChatUpdate(t, f, updated)
	return updated
}

func publishChatUpdate(t *testing.T, f *workerTestFixture, chat database.Chat) {
	t.Helper()
	msg := coderdpubsub.ChatStateUpdateMessage{
		SnapshotVersion:   chat.SnapshotVersion,
		HistoryVersion:    chat.HistoryVersion,
		QueueVersion:      chat.QueueVersion,
		RetryStateVersion: chat.RetryStateVersion,
		GenerationAttempt: chat.GenerationAttempt,
		Status:            string(chat.Status),
		Archived:          chat.Archived,
	}
	if chat.WorkerID.Valid {
		id := chat.WorkerID.UUID
		msg.WorkerID = &id
	}
	if chat.RunnerID.Valid {
		id := chat.RunnerID.UUID
		msg.RunnerID = &id
	}
	payload, err := json.Marshal(msg)
	require.NoError(t, err)
	require.NoError(t, f.pubsub.Publish(coderdpubsub.ChatStateUpdateChannel(chat.ID), payload))
}

func makeHeartbeatStale(t *testing.T, f *workerTestFixture, chatID uuid.UUID, runnerID uuid.UUID) time.Time {
	t.Helper()
	_, err := f.sqlDB.ExecContext(
		testutil.Context(t, testutil.WaitShort),
		`UPDATE chat_heartbeats SET heartbeat_at = NOW() - INTERVAL '1 hour' WHERE chat_id = $1 AND runner_id = $2`,
		chatID,
		runnerID,
	)
	require.NoError(t, err)
	heartbeat, err := f.db.GetChatHeartbeat(testutil.Context(t, testutil.WaitShort), database.GetChatHeartbeatParams{
		ChatID:   chatID,
		RunnerID: runnerID,
	})
	require.NoError(t, err)
	return heartbeat.HeartbeatAt
}

// runnerRefreshStore passes through only the initial bootstrap read. Each
// subsequent read waits for an explicit response or context cancellation.
type runnerRefreshStore struct {
	database.Store
	bootstrap *database.Chat
	once      sync.Once
	reads     chan runnerRefreshRead
}

type runnerRefreshRead struct {
	ctx   context.Context
	reply chan runnerRefreshResult
}

func (s *runnerRefreshStore) GetChatByID(ctx context.Context, _ uuid.UUID) (database.Chat, error) {
	var bootstrap *database.Chat
	s.once.Do(func() { bootstrap = s.bootstrap })
	if bootstrap != nil {
		return *bootstrap, nil
	}
	read := runnerRefreshRead{ctx: ctx, reply: make(chan runnerRefreshResult, 1)}
	select {
	case s.reads <- read:
	case <-ctx.Done():
		return database.Chat{}, ctx.Err()
	}
	select {
	case result := <-read.reply:
		return result.chat, result.err
	case <-ctx.Done():
		return database.Chat{}, ctx.Err()
	}
}

func newRunnerRecoveryTest(t *testing.T) (*runner, *recordingTaskStarter, *runnerRefreshStore, *quartz.Mock, database.Chat) {
	t.Helper()
	ctx, cancel := context.WithCancel(testutil.Context(t, testutil.WaitLong))
	clock := quartz.NewMock(t).WithLogger(quartz.NoOpLogger)
	starter := newBlockingTaskStarter(false)
	store := &runnerRefreshStore{reads: make(chan runnerRefreshRead, 1)}
	opts, err := (chatWorkerOptions{
		WorkerID: uuid.New(), Store: store, Pubsub: dbpubsub.NewInMemory(),
		TaskStarter: starter, Clock: clock, Logger: testutil.Logger(t),
		TaskRetryInitialBackoff: 100 * time.Millisecond,
		TaskRetryMaxBackoff:     400 * time.Millisecond,
	}).withDefaults()
	require.NoError(t, err)
	rec := &runnerRecord{
		key:      runnerKey{ChatID: uuid.New(), RunnerID: uuid.New()},
		workerID: opts.WorkerID, cancel: cancel,
		stateCh: make(chan runnerStateUpdate, 16),
	}
	mgr := newRunnerManager(ctx, nil, opts)
	r := newRunner(ctx, mgr, rec, opts)
	chat := database.Chat{
		ID: rec.key.ChatID, WorkerID: uuid.NullUUID{UUID: rec.workerID, Valid: true},
		RunnerID:        uuid.NullUUID{UUID: rec.key.RunnerID, Valid: true},
		SnapshotVersion: 10, HistoryVersion: 5, Status: database.ChatStatusRunning,
	}
	t.Cleanup(func() {
		cancel()
		if rec.done != nil {
			testutil.TryReceive(testutil.Context(t, testutil.WaitLong), t, rec.done)
		}
		r.stopRecoveryTimer()
		r.waitForTasks()
		r.refreshWG.Wait()
		if rec.unsubscribe != nil {
			rec.unsubscribe()
		}
	})
	return r, starter, store, clock, chat
}

func runRecoveryTestRunner(t *testing.T, r *runner, store *runnerRefreshStore, chat database.Chat) {
	t.Helper()
	store.bootstrap = &chat
	done := make(chan struct{})
	r.rec.done = done
	go func() {
		defer close(done)
		r.run()
	}()
}

func finishRecoveryTestTask(t *testing.T, r *runner, starter *recordingTaskStarter, index int) taskInstanceID {
	t.Helper()
	id := r.activeTaskID
	starter.release(t, index)
	testutil.TryReceive(testutil.Context(t, testutil.WaitLong), t, r.tasks[id].done)
	return id
}

// runnerExitTaskStarter makes a released generation take the retry wrapper's
// expected-exit path without changing the recording starter's nil semantics.
type runnerExitTaskStarter struct {
	chatWorkerTaskStarter
}

func (s runnerExitTaskStarter) StartGeneration(ctx context.Context, input chatWorkerTaskStartInput) error {
	if err := s.chatWorkerTaskStarter.StartGeneration(ctx, input); err != nil {
		return err
	}
	return errTaskExpectedExit
}

// runnerGenerationRefreshStore observes only the runner's reads. Tasks and
// the manager retain the real store, so their queries cannot satisfy the
// completion-refresh assertion.
type runnerGenerationRefreshStore struct {
	database.Store
	bootstrap bool
	refreshed chan database.Chat
}

func (s *runnerGenerationRefreshStore) GetChatByID(ctx context.Context, chatID uuid.UUID) (database.Chat, error) {
	chat, err := s.Store.GetChatByID(ctx, chatID)
	if !s.bootstrap {
		s.bootstrap = true
		return chat, err
	}
	if err == nil {
		select {
		case s.refreshed <- chat:
		case <-ctx.Done():
			return database.Chat{}, ctx.Err()
		}
	}
	return chat, err
}

// runnerBootstrapStore controls the bootstrap result without replacing the
// runner loop or its shutdown path.
type runnerBootstrapStore struct {
	database.Store
	getChat func(context.Context, uuid.UUID) (database.Chat, error)
}

func (s runnerBootstrapStore) GetChatByID(ctx context.Context, chatID uuid.UUID) (database.Chat, error) {
	return s.getChat(ctx, chatID)
}

type runnerSubscribeErrorPubsub struct {
	chatWorkerPubsub
	err error
}

func (p runnerSubscribeErrorPubsub) SubscribeWithErr(string, dbpubsub.ListenerWithErr) (func(), error) {
	return nil, p.err
}
