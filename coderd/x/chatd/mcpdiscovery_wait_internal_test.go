package chatd

import (
	"context"
	"database/sql"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

// TestPrepareGenerationMCPDiscoveryBudget verifies preparation spends the
// discovery budget once for a pending current-run snapshot and that later
// preparations for the same agent process do not spend it again.
func TestPrepareGenerationMCPDiscoveryBudget(t *testing.T) {
	t.Parallel()
	db, ps := dbtestutil.NewDB(t)
	ctx := chatdTestContext(t)
	user := dbgen.User(t, db, database.User{})
	ws, build, agent := seedWorkspaceBinding(t, db, user.ID)
	dbgen.OrganizationMember(t, db, database.OrganizationMember{UserID: user.ID, OrganizationID: ws.OrganizationID})
	dbgen.AIProviderWithOptionalKey(t, db, database.AIProvider{Type: database.AIProviderTypeOpenai}, "test-key")
	model := insertInternalChatModelConfig(t, db, ws.OrganizationID, "gpt-4o-mini", true)
	chat := dbgen.Chat(t, db, database.Chat{
		OwnerID: user.ID, OrganizationID: ws.OrganizationID, LastModelConfigID: model.ID,
		WorkspaceID: uuid.NullUUID{UUID: ws.ID, Valid: true},
		BuildID:     uuid.NullUUID{UUID: build.ID, Valid: true},
		AgentID:     uuid.NullUUID{UUID: agent.ID, Valid: true},
	})
	require.NoError(t, db.UpdateWorkspaceAgentStartupByID(ctx, database.UpdateWorkspaceAgentStartupByIDParams{
		ID: agent.ID, Version: "v1.0.0", APIVersion: "2.0", Subsystems: []database.WorkspaceAgentSubsystem{}, AgentRunID: "run-a",
	}))
	_, err := db.UpsertWorkspaceAgentContextSnapshot(ctx, database.UpsertWorkspaceAgentContextSnapshotParams{
		WorkspaceAgentID: agent.ID, Version: 1, AggregateHash: []byte("pending"),
		ReceivedAt: dbtime.Now(), AgentRunID: "run-a",
		McpDiscoveryPhase: database.WorkspaceAgentMcpDiscoveryPhasePending,
	})
	require.NoError(t, err)
	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{}, withInternalTestServerTransportFactory(&aibridgeTestFactory{}))
	prepare := func(ctx context.Context) {
		t.Helper()
		result, err := server.prepareGeneration(ctx, generationPrepareInput{Chat: chat})
		require.NoError(t, err)
		result.Cleanup()
	}
	start := time.Now()
	prepare(ctx)
	require.GreaterOrEqual(t, time.Since(start), 15*time.Second, "first preparation must spend the pending discovery budget")
	// Each call reconstructs preparation state, as steps and later user turns do.
	for range 2 {
		nextCtx, cancel := context.WithTimeout(ctx, testutil.WaitShort)
		prepare(nextCtx)
		require.NoError(t, nextCtx.Err(), "later preparation must not spend another discovery budget")
		cancel()
	}
}

// mcpDiscoveryProbeStore counts snapshot reads and serves a configurable
// agent run id so attempt tests can observe when a wait is (re)armed.
type mcpDiscoveryProbeStore struct {
	database.Store
	calls      atomic.Int32
	agentRunID atomic.Pointer[string]
	// agentErr, when set, fails the agent row lookup.
	agentErr atomic.Pointer[error]
	read     func(context.Context, uuid.UUID) (database.WorkspaceAgentContextSnapshot, error)
}

func (s *mcpDiscoveryProbeStore) GetWorkspaceAgentByID(_ context.Context, id uuid.UUID) (database.WorkspaceAgent, error) {
	if p := s.agentErr.Load(); p != nil {
		return database.WorkspaceAgent{}, *p
	}
	runID := ""
	if p := s.agentRunID.Load(); p != nil {
		runID = *p
	}
	return database.WorkspaceAgent{ID: id, AgentRunID: runID}, nil
}

func (s *mcpDiscoveryProbeStore) GetLatestWorkspaceAgentContextSnapshot(ctx context.Context, id uuid.UUID) (database.WorkspaceAgentContextSnapshot, error) {
	s.calls.Add(1)
	return s.read(ctx, id)
}

func (*mcpDiscoveryProbeStore) ListWorkspaceAgentContextResources(context.Context, uuid.UUID) ([]database.WorkspaceAgentContextResource, error) {
	return nil, nil
}

func TestMCPDiscoveryAttemptRearm(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	db := &mcpDiscoveryProbeStore{read: func(ctx context.Context, _ uuid.UUID) (database.WorkspaceAgentContextSnapshot, error) {
		<-ctx.Done()
		return database.WorkspaceAgentContextSnapshot{}, ctx.Err()
	}}
	runA := "run-a"
	db.agentRunID.Store(&runA)
	server := &Server{db: db, logger: testutil.Logger(t)}
	chatID, agentID := uuid.New(), uuid.New()
	attempt := func(chatID, agentID uuid.UUID) {
		t.Helper()
		attemptCtx, cancel := context.WithCancel(ctx)
		cancel()
		server.waitForMCPDiscovery(attemptCtx, chatID, agentID)
	}
	attempt(chatID, agentID)
	require.EqualValues(t, 1, db.calls.Load())
	start := time.Now()
	got := server.waitForMCPDiscovery(ctx, chatID, agentID)
	require.EqualValues(t, 2, db.calls.Load(), "a timed-out attempt is refreshed with one read, not repeated, for the same agent process")
	require.Less(t, time.Since(start), 5*time.Second, "the refresh read is bounded, not a second wait")
	require.True(t, got.WaitTimedOut)
	runB := "run-b"
	db.agentRunID.Store(&runB)
	attempt(chatID, agentID)
	require.EqualValues(t, 3, db.calls.Load(), "a new agent process on the same agent row rearms")
	attempt(chatID, uuid.New())
	require.EqualValues(t, 4, db.calls.Load(), "a new agent binding rearms")
	attempt(uuid.New(), agentID)
	require.EqualValues(t, 5, db.calls.Load(), "each chat gets an attempt")
}

func TestMCPDiscoveryAttemptTimedOutExpires(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	db := &mcpDiscoveryProbeStore{read: func(ctx context.Context, _ uuid.UUID) (database.WorkspaceAgentContextSnapshot, error) {
		<-ctx.Done()
		return database.WorkspaceAgentContextSnapshot{}, ctx.Err()
	}}
	runA := "run-a"
	db.agentRunID.Store(&runA)
	mClock := quartz.NewMock(t)
	server := &Server{db: db, logger: testutil.Logger(t), clock: mClock}
	attempt := func(chatID, agentID uuid.UUID) {
		t.Helper()
		attemptCtx, cancel := context.WithCancel(ctx)
		cancel()
		server.waitForMCPDiscovery(attemptCtx, chatID, agentID)
	}
	chatID, agentID := uuid.New(), uuid.New()
	attempt(chatID, agentID)
	require.EqualValues(t, 1, db.calls.Load())
	mClock.Advance(mcpDiscoveryAttemptRetention - time.Second).MustWait(ctx)
	attempt(chatID, agentID)
	require.EqualValues(t, 1, db.calls.Load(), "a timed-out attempt is reused within retention")
	// Two other chats time out and are then left alone past retention.
	attempt(uuid.New(), agentID)
	attempt(uuid.New(), agentID)
	server.mcpDiscoveryMu.Lock()
	require.Len(t, server.mcpDiscoveryAttempts, 3)
	server.mcpDiscoveryMu.Unlock()
	mClock.Advance(mcpDiscoveryAttemptRetention).MustWait(ctx)
	attempt(chatID, agentID)
	require.EqualValues(t, 4, db.calls.Load(), "an expired attempt is spent again")
	server.mcpDiscoveryMu.Lock()
	require.Len(t, server.mcpDiscoveryAttempts, 1, "expired attempts for other chats are swept on rearm")
	server.mcpDiscoveryMu.Unlock()
}

func TestMCPDiscoveryAttemptKeptWhenRunIDLookupFails(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	db := &mcpDiscoveryProbeStore{read: func(ctx context.Context, _ uuid.UUID) (database.WorkspaceAgentContextSnapshot, error) {
		<-ctx.Done()
		return database.WorkspaceAgentContextSnapshot{}, ctx.Err()
	}}
	runA := "run-a"
	db.agentRunID.Store(&runA)
	server := &Server{db: db, logger: testutil.Logger(t)}
	chatID, agentID := uuid.New(), uuid.New()
	attemptCtx, cancel := context.WithCancel(ctx)
	cancel()
	server.waitForMCPDiscovery(attemptCtx, chatID, agentID)
	require.EqualValues(t, 1, db.calls.Load())
	server.mcpDiscoveryMu.Lock()
	cached := server.mcpDiscoveryAttempts[chatID]
	server.mcpDiscoveryMu.Unlock()
	require.NotNil(t, cached)

	// The run id lookup fails (a canceled turn, a database hiccup): the
	// cached attempt for the same agent must be kept, not replaced by an
	// attempt built from the failed context.
	lookupErr := xerrors.New("lookup failed")
	db.agentErr.Store(&lookupErr)
	got := server.waitForMCPDiscovery(attemptCtx, chatID, agentID)
	require.True(t, got.WaitTimedOut, "the cached outcome is reused")
	require.EqualValues(t, 1, db.calls.Load(), "no new attempt is started")
	server.mcpDiscoveryMu.Lock()
	require.Same(t, cached, server.mcpDiscoveryAttempts[chatID])
	server.mcpDiscoveryMu.Unlock()

	// Without a cached attempt, a failed lookup fails open without
	// caching anything.
	otherChat := uuid.New()
	got = server.waitForMCPDiscovery(ctx, otherChat, agentID)
	require.False(t, got.WaitTimedOut)
	require.EqualValues(t, 1, db.calls.Load())
	server.mcpDiscoveryMu.Lock()
	require.Nil(t, server.mcpDiscoveryAttempts[otherChat])
	server.mcpDiscoveryMu.Unlock()
}

func TestMCPDiscoveryTimedOutAttemptKeyedToObservedRun(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	// The agent restarts while the first wait is in flight: the cache key
	// was read as run A, the wait spends its budget on run B.
	runA, runB := "run-a", "run-b"
	db := &mcpDiscoveryProbeStore{}
	db.agentRunID.Store(&runA)
	db.read = func(context.Context, uuid.UUID) (database.WorkspaceAgentContextSnapshot, error) {
		db.agentRunID.Store(&runB)
		return database.WorkspaceAgentContextSnapshot{AgentRunID: runB, McpDiscoveryPhase: database.WorkspaceAgentMcpDiscoveryPhasePending}, nil
	}
	server := &Server{db: db, logger: testutil.Logger(t)}
	chatID, agentID := uuid.New(), uuid.New()
	// The first poll already observes run B; the caller's deadline then
	// ends the wait without spending the full budget.
	waitCtx, cancel := context.WithTimeout(ctx, testutil.IntervalMedium)
	defer cancel()
	require.True(t, server.waitForMCPDiscovery(waitCtx, chatID, agentID).WaitTimedOut)
	calls := db.calls.Load()

	// The next turn reads run B and must reuse the spent attempt rather
	// than spend another budget on the same process.
	got := server.waitForMCPDiscovery(ctx, chatID, agentID)
	require.True(t, got.WaitTimedOut)
	require.LessOrEqual(t, db.calls.Load()-calls, int32(1), "one refresh read, no new wait")
	server.mcpDiscoveryMu.Lock()
	require.Equal(t, runB, server.mcpDiscoveryAttempts[chatID].agentRunID)
	server.mcpDiscoveryMu.Unlock()
}

func TestMCPDiscoveryTimedOutAttemptRefreshedOnceComplete(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	var complete atomic.Bool
	db := &mcpDiscoveryProbeStore{read: func(context.Context, uuid.UUID) (database.WorkspaceAgentContextSnapshot, error) {
		phase := database.WorkspaceAgentMcpDiscoveryPhasePending
		if complete.Load() {
			phase = database.WorkspaceAgentMcpDiscoveryPhaseComplete
		}
		return database.WorkspaceAgentContextSnapshot{AgentRunID: "run-a", McpDiscoveryPhase: phase}, nil
	}}
	runA := "run-a"
	db.agentRunID.Store(&runA)
	server := &Server{db: db, logger: testutil.Logger(t)}
	chatID, agentID := uuid.New(), uuid.New()
	attemptCtx, cancel := context.WithCancel(ctx)
	cancel()
	require.True(t, server.waitForMCPDiscovery(attemptCtx, chatID, agentID).WaitTimedOut)
	calls := db.calls.Load()

	// Still pending: the cached timeout is reused after one read.
	got := server.waitForMCPDiscovery(ctx, chatID, agentID)
	require.True(t, got.WaitTimedOut)

	// Discovery finished after the budget was spent: a later caller gets
	// the current state from one read, and the spent attempt stays cached.
	complete.Store(true)
	got = server.waitForMCPDiscovery(ctx, chatID, agentID)
	require.False(t, got.WaitTimedOut)
	require.Equal(t, codersdk.ChatContextMCPDiscoveryPhaseComplete, got.Phase)
	require.LessOrEqual(t, db.calls.Load()-calls, int32(2), "one read per reuse, no new wait")
	server.mcpDiscoveryMu.Lock()
	require.NotNil(t, server.mcpDiscoveryAttempts[chatID])
	server.mcpDiscoveryMu.Unlock()
}

func TestMCPDiscoveryAttemptCompleteNotCached(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	db := &mcpDiscoveryProbeStore{read: func(context.Context, uuid.UUID) (database.WorkspaceAgentContextSnapshot, error) {
		return database.WorkspaceAgentContextSnapshot{AgentRunID: "run-a", McpDiscoveryPhase: database.WorkspaceAgentMcpDiscoveryPhaseComplete}, nil
	}}
	runA := "run-a"
	db.agentRunID.Store(&runA)
	server := &Server{db: db, logger: testutil.Logger(t)}
	chatID, agentID := uuid.New(), uuid.New()
	got := server.waitForMCPDiscovery(ctx, chatID, agentID)
	require.Equal(t, codersdk.ChatContextMCPDiscoveryPhaseComplete, got.Phase)
	require.False(t, got.WaitTimedOut)
	server.waitForMCPDiscovery(ctx, chatID, agentID)
	require.EqualValues(t, 2, db.calls.Load(), "a completed wait is re-checked cheaply on the next turn")
	server.mcpDiscoveryMu.Lock()
	require.Empty(t, server.mcpDiscoveryAttempts)
	server.mcpDiscoveryMu.Unlock()
}

func TestMCPDiscoveryConcurrentAttempt(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	entered := make(chan struct{})
	release := make(chan struct{})
	var enteredOnce sync.Once
	db := &mcpDiscoveryProbeStore{read: func(ctx context.Context, _ uuid.UUID) (database.WorkspaceAgentContextSnapshot, error) {
		enteredOnce.Do(func() { close(entered) })
		select {
		case <-release:
		case <-ctx.Done():
		}
		return database.WorkspaceAgentContextSnapshot{}, sql.ErrNoRows
	}}
	runA := "run-a"
	db.agentRunID.Store(&runA)
	server := &Server{db: db, logger: testutil.Logger(t)}
	chatID, agentID := uuid.New(), uuid.New()
	firstCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		server.waitForMCPDiscovery(firstCtx, chatID, agentID)
	}()
	testutil.TryReceive(ctx, t, entered)
	joinedCtx, joinedCancel := context.WithCancel(ctx)
	joinedCancel()
	server.waitForMCPDiscovery(joinedCtx, chatID, agentID)
	require.EqualValues(t, 1, db.calls.Load(), "joiners must not start an independent wait")
	cancel()
	close(release)
	testutil.TryReceive(ctx, t, done)
	// Reusing the spent attempt refreshes the outcome with one read and
	// starts no new wait.
	server.waitForMCPDiscovery(ctx, chatID, agentID)
	require.EqualValues(t, 2, db.calls.Load())
}

// TestMCPDiscoveryCanceledJoinerRace runs a canceled joiner concurrently
// with the owner's completion; under the race detector the joiner must not
// touch the outcome the owner is still writing.
func TestMCPDiscoveryCanceledJoinerRace(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	entered := make(chan struct{})
	var enteredOnce sync.Once
	release := make(chan struct{})
	db := &mcpDiscoveryProbeStore{read: func(ctx context.Context, _ uuid.UUID) (database.WorkspaceAgentContextSnapshot, error) {
		enteredOnce.Do(func() { close(entered) })
		select {
		case <-release:
		case <-ctx.Done():
		}
		return database.WorkspaceAgentContextSnapshot{AgentRunID: "run-a", McpDiscoveryPhase: database.WorkspaceAgentMcpDiscoveryPhaseComplete}, nil
	}}
	runA := "run-a"
	db.agentRunID.Store(&runA)
	server := &Server{db: db, logger: testutil.Logger(t)}
	chatID, agentID := uuid.New(), uuid.New()
	ownerDone := make(chan struct{})
	go func() {
		defer close(ownerDone)
		server.waitForMCPDiscovery(ctx, chatID, agentID)
	}()
	testutil.TryReceive(ctx, t, entered)
	joinedCtx, joinedCancel := context.WithCancel(ctx)
	joinedCancel()
	joinerDone := make(chan struct{})
	go func() {
		defer close(joinerDone)
		server.waitForMCPDiscovery(joinedCtx, chatID, agentID)
	}()
	close(release)
	testutil.TryReceive(ctx, t, ownerDone)
	testutil.TryReceive(ctx, t, joinerDone)
}

func TestAppendWorkspaceMCPNote(t *testing.T) {
	t.Parallel()
	const summary = "Workspace MCP discovery is still initializing."
	require.Equal(t, "<workspace-context>\nOperating System: linux\n</workspace-context>",
		appendWorkspaceMCPNote("<workspace-context>\nOperating System: linux\n</workspace-context>", ""),
		"an empty summary leaves the block untouched")
	require.Equal(t, "<workspace-context>\nOperating System: linux\n\nWorkspace MCP: "+summary+"\n</workspace-context>",
		appendWorkspaceMCPNote("<workspace-context>\nOperating System: linux\n</workspace-context>", summary))
	require.Equal(t, "<workspace-context>\nWorkspace MCP: "+summary+"\n</workspace-context>",
		appendWorkspaceMCPNote("", summary), "a chat without instruction files still gets the note")
}
