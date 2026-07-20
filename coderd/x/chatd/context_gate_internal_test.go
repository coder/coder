package chatd

import (
	"context"
	"database/sql"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/structpb"

	"cdr.dev/slog/v3/sloggers/slogtest"
	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	coderdpubsub "github.com/coder/coder/v2/coderd/pubsub"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

// newGateServer builds a minimal Server for exercising the context-report
// gate directly against a real database, with a caller-provided clock and
// wait ceiling. The pubsub carries the context_ready watch events the gate
// publishes at gate-exit pinning and the context_error events it publishes
// when a turn degrades.
func newGateServer(t *testing.T, fix rebindFixture, clk quartz.Clock, ceiling time.Duration) *Server {
	t.Helper()
	return &Server{
		db:                             fix.db,
		pubsub:                         fix.ps,
		logger:                         slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}),
		clock:                          clk,
		contextReportTimeout:           ceiling,
		agentInactiveDisconnectTimeout: 10 * time.Minute,
	}
}

// subscribeChatWatchEvents collects the owner's chat watch events published
// via pubsub so tests can assert on gate-exit context_ready publishes.
func subscribeChatWatchEvents(t *testing.T, fix rebindFixture) <-chan codersdk.ChatWatchEvent {
	t.Helper()
	events := make(chan codersdk.ChatWatchEvent, 16)
	cancel, err := fix.ps.Subscribe(
		coderdpubsub.ChatWatchEventChannel(fix.user.ID),
		func(_ context.Context, message []byte) {
			var event codersdk.ChatWatchEvent
			if err := json.Unmarshal(message, &event); err != nil {
				return
			}
			events <- event
		})
	require.NoError(t, err)
	t.Cleanup(cancel)
	return events
}

func newGateTurnContext(t *testing.T, server *Server, chat database.Chat) *turnWorkspaceContext {
	t.Helper()
	cur := chat
	wc := &turnWorkspaceContext{
		server:           server,
		chatStateMu:      &sync.Mutex{},
		currentChat:      &cur,
		loadChatSnapshot: server.db.GetChatByID,
	}
	t.Cleanup(wc.close)
	return wc
}

// requireChatDegraded asserts the degrade contract on the chat row: the
// reason is recorded on context_error, the pinned hash stays NULL (so a
// later push can still hydrate), and the chat is not in an error state.
func requireChatDegraded(t *testing.T, fix rebindFixture, chatID uuid.UUID, message string) {
	t.Helper()
	post, err := fix.db.GetChatByID(fix.ctx, chatID)
	require.NoError(t, err)
	require.Contains(t, post.ContextError, message,
		"the degrade reason must land on chats.context_error")
	require.Nil(t, post.ContextAggregateHash,
		"a degrade must leave the pinned hash NULL so a later push still hydrates")
	require.NotEqual(t, database.ChatStatusError, post.Status,
		"a degrade must not put the chat in an error state")
}

// requireContextErrorEvent asserts the degrade published a context_error
// watch event carrying the degraded chat's waiting state and error.
func requireContextErrorEvent(t *testing.T, fix rebindFixture, events <-chan codersdk.ChatWatchEvent, chatID uuid.UUID, message string) {
	t.Helper()
	event := testutil.RequireReceive(fix.ctx, t, events)
	require.Equal(t, codersdk.ChatWatchEventKindContextError, event.Kind,
		"a degrade publishes a context_error event")
	require.Equal(t, chatID, event.Chat.ID)
	require.NotNil(t, event.Chat.Context)
	require.Equal(t, codersdk.ChatContextStateWaiting, event.Chat.Context.State)
	require.Contains(t, event.Chat.Context.Error, message)
}

type gateAwaitResult struct {
	agent database.WorkspaceAgent
	err   error
}

// awaitGateAsync runs awaitChatContextReported in a goroutine so the test can
// drive the mock clock while the gate waits.
func awaitGateAsync(ctx context.Context, server *Server, wc *turnWorkspaceContext) <-chan gateAwaitResult {
	results := make(chan gateAwaitResult, 1)
	go func() {
		agent, err := server.awaitChatContextReported(ctx, wc, server.logger)
		results <- gateAwaitResult{agent: agent, err: err}
	}()
	return results
}

// seedStopBuild appends a stop-transition build so the workspace's latest
// build is no longer a start.
func seedStopBuild(t *testing.T, db database.Store, fix rebindFixture) {
	t.Helper()
	tv, err := db.GetTemplateVersionByID(fix.ctx, mustLatestBuild(t, db, fix).TemplateVersionID)
	require.NoError(t, err)
	job := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
		OrganizationID: fix.org.ID,
		CompletedAt:    sql.NullTime{Valid: true, Time: dbtime.Now()},
	})
	dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
		WorkspaceID:       fix.ws.ID,
		TemplateVersionID: tv.ID,
		JobID:             job.ID,
		BuildNumber:       mustLatestBuild(t, db, fix).BuildNumber + 1,
		Transition:        database.WorkspaceTransitionStop,
	})
}

// seedStartBuildWithAgent appends a start-transition build carrying a single
// fresh agent, simulating a workspace rebuild.
func seedStartBuildWithAgent(t *testing.T, db database.Store, fix rebindFixture) (database.WorkspaceBuild, database.WorkspaceAgent) {
	t.Helper()
	tv, err := db.GetTemplateVersionByID(fix.ctx, mustLatestBuild(t, db, fix).TemplateVersionID)
	require.NoError(t, err)
	job := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
		OrganizationID: fix.org.ID,
		CompletedAt:    sql.NullTime{Valid: true, Time: dbtime.Now()},
	})
	build := dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
		WorkspaceID:       fix.ws.ID,
		TemplateVersionID: tv.ID,
		JobID:             job.ID,
		BuildNumber:       mustLatestBuild(t, db, fix).BuildNumber + 1,
		Transition:        database.WorkspaceTransitionStart,
	})
	res := dbgen.WorkspaceResource(t, db, database.WorkspaceResource{
		Transition: database.WorkspaceTransitionStart,
		JobID:      job.ID,
	})
	agent := dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{ResourceID: res.ID})
	return build, agent
}

func mustLatestBuild(t *testing.T, db database.Store, fix rebindFixture) database.WorkspaceBuild {
	t.Helper()
	build, err := db.GetLatestWorkspaceBuildByWorkspaceID(fix.ctx, fix.ws.ID)
	require.NoError(t, err)
	return build
}

func gateChat(t *testing.T, fix rebindFixture, agentID uuid.UUID, buildID uuid.UUID) database.Chat {
	t.Helper()
	seed := database.Chat{
		OwnerID:           fix.user.ID,
		OrganizationID:    fix.org.ID,
		LastModelConfigID: fix.model.ID,
		WorkspaceID:       uuid.NullUUID{UUID: fix.ws.ID, Valid: true},
		Status:            database.ChatStatusRunning,
	}
	if agentID != uuid.Nil {
		seed.AgentID = uuid.NullUUID{UUID: agentID, Valid: true}
	}
	if buildID != uuid.Nil {
		seed.BuildID = uuid.NullUUID{UUID: buildID, Valid: true}
	}
	return dbgen.Chat(t, fix.db, seed)
}

func TestAwaitChatContextReported(t *testing.T) {
	t.Parallel()

	// SnapshotAlreadyExists: the bound agent already pushed a snapshot, so
	// the gate exits on its pre-wait check without any ticker interaction
	// (the mock clock is never advanced), pins the chat, and announces the
	// pin with a context_ready watch event.
	t.Run("SnapshotAlreadyExists", func(t *testing.T) {
		t.Parallel()
		fix := newRebindFixture(t)
		server := newGateServer(t, fix, quartz.NewMock(t), defaultContextReportTimeout)
		chat := gateChat(t, fix, fix.agentA, fix.buildID)
		require.Nil(t, chat.ContextAggregateHash)
		events := subscribeChatWatchEvents(t, fix)

		wc := newGateTurnContext(t, server, chat)
		agent, err := server.awaitChatContextReported(fix.ctx, wc, server.logger)
		require.NoError(t, err)
		require.Equal(t, fix.agentA, agent.ID)

		post, err := fix.db.GetChatByID(fix.ctx, chat.ID)
		require.NoError(t, err)
		require.Equal(t, fix.hashA, post.ContextAggregateHash, "gate exit pins the chat to the reported snapshot")

		event := testutil.RequireReceive(fix.ctx, t, events)
		require.Equal(t, codersdk.ChatWatchEventKindContextReady, event.Kind,
			"gate-exit pinning publishes a context_ready event")
		require.Equal(t, chat.ID, event.Chat.ID)
		require.NotNil(t, event.Chat.Context)
		require.Equal(t, codersdk.ChatContextStateReady, event.Chat.Context.State)
	})

	// PinnedSameBuild: a pinned chat whose binding matches the latest start
	// build skips the gate entirely, even though its agent has no snapshot
	// row (an entered gate could never exit here).
	t.Run("PinnedSameBuild", func(t *testing.T) {
		t.Parallel()
		fix := newRebindFixture(t)
		server := newGateServer(t, fix, quartz.NewMock(t), defaultContextReportTimeout)
		chat := gateChat(t, fix, fix.agentNoSnap, fix.buildID)
		require.NoError(t, fix.db.SetChatContextSnapshot(fix.ctx, database.SetChatContextSnapshotParams{
			ID:            chat.ID,
			AggregateHash: []byte{0xfe},
		}))
		chat, err := fix.db.GetChatByID(fix.ctx, chat.ID)
		require.NoError(t, err)
		require.NotNil(t, chat.ContextAggregateHash)

		wc := newGateTurnContext(t, server, chat)
		agent, err := server.awaitChatContextReported(fix.ctx, wc, server.logger)
		require.NoError(t, err)
		require.Equal(t, fix.agentNoSnap, agent.ID)
	})

	// PinnedStopBuild: a pinned chat on a workspace whose latest build is a
	// stop transition keeps its binding and proceeds from pinned context, as
	// before the gate existed.
	t.Run("PinnedStopBuild", func(t *testing.T) {
		t.Parallel()
		fix := newRebindFixture(t)
		server := newGateServer(t, fix, quartz.NewMock(t), defaultContextReportTimeout)
		chat := gateChat(t, fix, fix.agentA, fix.buildID)
		_, hydrateErr := fix.db.HydrateAgentChatsContext(fix.ctx, database.HydrateAgentChatsContextParams{
			AgentID:       fix.agentA,
			AggregateHash: fix.hashA,
		})
		require.NoError(t, hydrateErr)
		seedStopBuild(t, fix.db, fix)

		chat, err := fix.db.GetChatByID(fix.ctx, chat.ID)
		require.NoError(t, err)
		wc := newGateTurnContext(t, server, chat)
		agent, err := server.awaitChatContextReported(fix.ctx, wc, server.logger)
		require.NoError(t, err)
		require.Equal(t, fix.agentA, agent.ID, "the stop build does not steal the binding")

		post, err := fix.db.GetChatByID(fix.ctx, chat.ID)
		require.NoError(t, err)
		require.Equal(t, fix.hashA, post.ContextAggregateHash, "pinned context survives the stop build")
	})

	// StopBuildUnpinned: an unpinned chat cannot receive a context report
	// from a workspace whose latest build is not a start, so the gate
	// degrades immediately: the turn proceeds without error, the reason is
	// recorded on chats.context_error, and a context_error watch event is
	// published. Covered both when the old agent binding still resolves and
	// when no agent is bound at all.
	t.Run("StopBuildUnpinned", func(t *testing.T) {
		t.Parallel()
		for _, bound := range []bool{true, false} {
			name := "Bound"
			if !bound {
				name = "Unbound"
			}
			t.Run(name, func(t *testing.T) {
				t.Parallel()
				fix := newRebindFixture(t)
				server := newGateServer(t, fix, quartz.NewMock(t), defaultContextReportTimeout)
				agentID := uuid.Nil
				buildID := uuid.Nil
				if bound {
					agentID = fix.agentNoSnap
					buildID = fix.buildID
				}
				chat := gateChat(t, fix, agentID, buildID)
				seedStopBuild(t, fix.db, fix)
				events := subscribeChatWatchEvents(t, fix)

				wc := newGateTurnContext(t, server, chat)
				agent, err := server.awaitChatContextReported(fix.ctx, wc, server.logger)
				require.NoError(t, err, "the gate must not fail the turn")
				require.Equal(t, uuid.Nil, agent.ID, "a degraded turn runs with the zero-value agent")

				requireChatDegraded(t, fix, chat.ID,
					"workspace must be started to report chat context")
				requireContextErrorEvent(t, fix, events, chat.ID,
					"workspace must be started to report chat context")
			})
		}
	})

	// AgentTooOld: an agent that connected with an Agent API below 2.10 can
	// never push context, so the gate degrades immediately instead of
	// waiting out the ceiling.
	t.Run("AgentTooOld", func(t *testing.T) {
		t.Parallel()
		fix := newRebindFixture(t)
		server := newGateServer(t, fix, quartz.NewMock(t), defaultContextReportTimeout)
		require.NoError(t, fix.db.UpdateWorkspaceAgentStartupByID(fix.ctx, database.UpdateWorkspaceAgentStartupByIDParams{
			ID:         fix.agentNoSnap,
			Version:    "v2.0.0",
			APIVersion: "2.0",
			Subsystems: []database.WorkspaceAgentSubsystem{},
		}))
		chat := gateChat(t, fix, fix.agentNoSnap, fix.buildID)
		events := subscribeChatWatchEvents(t, fix)

		wc := newGateTurnContext(t, server, chat)
		agent, err := server.awaitChatContextReported(fix.ctx, wc, server.logger)
		require.NoError(t, err, "the gate must not fail the turn")
		require.Equal(t, uuid.Nil, agent.ID)

		requireChatDegraded(t, fix, chat.ID,
			"workspace agent is too old to report chat context")
		requireContextErrorEvent(t, fix, events, chat.ID,
			"workspace agent is too old to report chat context")
	})

	// DisconnectedAgentDegradesImmediately: an agent that connected once and
	// then disconnected cannot push, so the gate degrades immediately
	// instead of waiting out the ceiling. This mirrors the dial path's
	// disconnect derivation; a never-connected agent stays waitable.
	t.Run("DisconnectedAgentDegradesImmediately", func(t *testing.T) {
		t.Parallel()
		fix := newRebindFixture(t)
		server := newGateServer(t, fix, quartz.NewMock(t), defaultContextReportTimeout)
		connectedAt := dbtime.Now().Add(-time.Hour)
		require.NoError(t, fix.db.UpdateWorkspaceAgentConnectionByID(fix.ctx, database.UpdateWorkspaceAgentConnectionByIDParams{
			ID:               fix.agentNoSnap,
			FirstConnectedAt: sql.NullTime{Valid: true, Time: connectedAt},
			LastConnectedAt:  sql.NullTime{Valid: true, Time: connectedAt},
			DisconnectedAt:   sql.NullTime{Valid: true, Time: connectedAt.Add(time.Minute)},
			UpdatedAt:        dbtime.Now(),
		}))
		chat := gateChat(t, fix, fix.agentNoSnap, fix.buildID)
		events := subscribeChatWatchEvents(t, fix)

		wc := newGateTurnContext(t, server, chat)
		agent, err := server.awaitChatContextReported(fix.ctx, wc, server.logger)
		require.NoError(t, err, "the gate must not fail the turn")
		require.Equal(t, uuid.Nil, agent.ID)

		requireChatDegraded(t, fix, chat.ID,
			"workspace agent is not connected, so it cannot report chat context")
		requireContextErrorEvent(t, fix, events, chat.ID,
			"workspace agent is not connected, so it cannot report chat context")
	})

	// DegradeMemorySkipsWait: an unpinned chat whose context_error is
	// already non-empty degraded on a previous turn (that is the only way an
	// unpinned chat gains one), so the gate degrades again immediately,
	// keeping the message, without any ticker interaction (the mock clock is
	// never advanced).
	t.Run("DegradeMemorySkipsWait", func(t *testing.T) {
		t.Parallel()
		fix := newRebindFixture(t)
		server := newGateServer(t, fix, quartz.NewMock(t), defaultContextReportTimeout)
		chat := gateChat(t, fix, fix.agentNoSnap, fix.buildID)
		require.NoError(t, fix.db.UpdateChatContextError(fix.ctx, database.UpdateChatContextErrorParams{
			ID:           chat.ID,
			ContextError: "workspace agent did not report chat context within 15m0s",
		}))
		chat, err := fix.db.GetChatByID(fix.ctx, chat.ID)
		require.NoError(t, err)
		events := subscribeChatWatchEvents(t, fix)

		// The workspace is started and the agent is waitable, so without
		// the degrade memory this call would stall on the ticker; the mock
		// clock is never advanced, so an immediate return proves the memory
		// short-circuits the wait.
		wc := newGateTurnContext(t, server, chat)
		agent, err := server.awaitChatContextReported(fix.ctx, wc, server.logger)
		require.NoError(t, err, "the gate must not fail the turn")
		require.Equal(t, uuid.Nil, agent.ID)

		requireChatDegraded(t, fix, chat.ID,
			"workspace agent did not report chat context within 15m0s")
		requireContextErrorEvent(t, fix, events, chat.ID,
			"workspace agent did not report chat context within 15m0s")
	})

	// PushAfterDegradeHydratesAndClearsError: a degrade leaves the pinned
	// hash NULL, so a later agent push still hydrates the chat, overwrites
	// the context error, and the next gate call proceeds pinned. This is the
	// self-heal path.
	t.Run("PushAfterDegradeHydratesAndClearsError", func(t *testing.T) {
		t.Parallel()
		fix := newRebindFixture(t)
		server := newGateServer(t, fix, quartz.NewMock(t), defaultContextReportTimeout)
		// A too-old agent degrades the first turn immediately.
		require.NoError(t, fix.db.UpdateWorkspaceAgentStartupByID(fix.ctx, database.UpdateWorkspaceAgentStartupByIDParams{
			ID:         fix.agentNoSnap,
			Version:    "v2.0.0",
			APIVersion: "2.0",
			Subsystems: []database.WorkspaceAgentSubsystem{},
		}))
		chat := gateChat(t, fix, fix.agentNoSnap, fix.buildID)

		wc := newGateTurnContext(t, server, chat)
		_, err := server.awaitChatContextReported(fix.ctx, wc, server.logger)
		require.NoError(t, err)
		requireChatDegraded(t, fix, chat.ID,
			"workspace agent is too old to report chat context")

		// The agent's push arrives later: the NULL-gated hydrate stamps the
		// hash and replaces the degrade error in one statement.
		hash := []byte{0xee}
		seedAgentContext(fix.ctx, t, fix.db, fix.agentNoSnap, "/home/coder/AGENTS.md", hash,
			database.WorkspaceAgentContextBodyKindInstructionFile,
			json.RawMessage(`{"instruction_file":{"content":"healed"}}`))
		hydrated, err := fix.db.HydrateAgentChatsContext(fix.ctx, database.HydrateAgentChatsContextParams{
			AgentID:       fix.agentNoSnap,
			AggregateHash: hash,
		})
		require.NoError(t, err)
		require.Contains(t, hydrated, chat.ID, "the degraded chat is still NULL-hash hydratable")

		post, err := fix.db.GetChatByID(fix.ctx, chat.ID)
		require.NoError(t, err)
		require.Equal(t, hash, post.ContextAggregateHash, "the push pins the chat")
		require.Empty(t, post.ContextError, "the push overwrites the degrade error")

		// The next gate call proceeds pinned, with the agent resolved.
		wc2 := newGateTurnContext(t, server, post)
		agent, err := server.awaitChatContextReported(fix.ctx, wc2, server.logger)
		require.NoError(t, err)
		require.Equal(t, fix.agentNoSnap, agent.ID)
	})

	// WaitsForPush: an unpinned chat whose agent has not pushed waits on the
	// gate ticker; a snapshot that appears mid-wait is picked up on the next
	// tick, the chat is pinned to it, and the turn proceeds.
	t.Run("WaitsForPush", func(t *testing.T) {
		t.Parallel()
		fix := newRebindFixture(t)
		mClock := quartz.NewMock(t)
		trap := mClock.Trap().NewTicker(contextReportTimerTagGroup, contextReportTickerTag)
		defer trap.Close()
		server := newGateServer(t, fix, mClock, defaultContextReportTimeout)
		chat := gateChat(t, fix, fix.agentNoSnap, fix.buildID)

		wc := newGateTurnContext(t, server, chat)
		results := awaitGateAsync(fix.ctx, server, wc)

		// The gate found no snapshot and started waiting.
		trap.MustWait(fix.ctx).MustRelease(fix.ctx)

		// One empty tick: still nothing to report.
		mClock.Advance(contextReportPollInterval).MustWait(fix.ctx)
		select {
		case res := <-results:
			t.Fatalf("gate exited before a snapshot existed: %+v", res)
		default:
		}

		// The agent's first push arrives mid-wait; the next tick sees it.
		hash := []byte{0xcc}
		seedAgentContext(fix.ctx, t, fix.db, fix.agentNoSnap, "/home/coder/AGENTS.md", hash,
			database.WorkspaceAgentContextBodyKindInstructionFile,
			json.RawMessage(`{"instruction_file":{"content":"pushed mid-wait"}}`))
		mClock.Advance(contextReportPollInterval).MustWait(fix.ctx)

		res := testutil.RequireReceive(fix.ctx, t, results)
		require.NoError(t, res.err)
		require.Equal(t, fix.agentNoSnap, res.agent.ID)

		post, err := fix.db.GetChatByID(fix.ctx, chat.ID)
		require.NoError(t, err)
		require.Equal(t, hash, post.ContextAggregateHash, "gate exit pins the mid-wait push")
	})

	// CeilingElapses: an agent that never reports degrades the turn once
	// the ceiling passes: the turn proceeds without error and the wait is
	// named in the recorded context error. The ceiling is deliberately not
	// a multiple of the poll interval so the timeout fires alone.
	t.Run("CeilingElapses", func(t *testing.T) {
		t.Parallel()
		fix := newRebindFixture(t)
		mClock := quartz.NewMock(t)
		trap := mClock.Trap().NewTicker(contextReportTimerTagGroup, contextReportTickerTag)
		defer trap.Close()
		ceiling := 2500 * time.Millisecond
		server := newGateServer(t, fix, mClock, ceiling)
		chat := gateChat(t, fix, fix.agentNoSnap, fix.buildID)
		events := subscribeChatWatchEvents(t, fix)

		wc := newGateTurnContext(t, server, chat)
		results := awaitGateAsync(fix.ctx, server, wc)
		trap.MustWait(fix.ctx).MustRelease(fix.ctx)

		mClock.Advance(contextReportPollInterval).MustWait(fix.ctx)
		mClock.Advance(contextReportPollInterval).MustWait(fix.ctx)
		mClock.Advance(ceiling - 2*contextReportPollInterval).MustWait(fix.ctx)

		res := testutil.RequireReceive(fix.ctx, t, results)
		require.NoError(t, res.err, "the gate must not fail the turn")
		require.Equal(t, uuid.Nil, res.agent.ID)

		requireChatDegraded(t, fix, chat.ID,
			"workspace agent did not report chat context within 2.5s")
		requireContextErrorEvent(t, fix, events, chat.ID,
			"workspace agent did not report chat context within 2.5s")
	})

	// InterruptDuringWait: canceling the turn context mid-wait propagates
	// the cancellation unchanged so the interrupt path sees a plain context
	// error, not a degrade that would pollute the chat's context state.
	t.Run("InterruptDuringWait", func(t *testing.T) {
		t.Parallel()
		fix := newRebindFixture(t)
		mClock := quartz.NewMock(t)
		trap := mClock.Trap().NewTicker(contextReportTimerTagGroup, contextReportTickerTag)
		defer trap.Close()
		server := newGateServer(t, fix, mClock, defaultContextReportTimeout)
		chat := gateChat(t, fix, fix.agentNoSnap, fix.buildID)

		turnCtx, cancel := context.WithCancel(fix.ctx)
		wc := newGateTurnContext(t, server, chat)
		results := awaitGateAsync(turnCtx, server, wc)
		trap.MustWait(fix.ctx).MustRelease(fix.ctx)

		cancel()
		res := testutil.RequireReceive(fix.ctx, t, results)
		require.ErrorIs(t, res.err, context.Canceled)

		post, err := fix.db.GetChatByID(fix.ctx, chat.ID)
		require.NoError(t, err)
		require.Empty(t, post.ContextError, "an interrupt is not a degrade")
	})

	// RebindOnNewStartBuild: a pinned chat bound to a previous build's agent
	// is rebound to the new start build's agent at turn prep. The rebind
	// re-pins, which clears the pinned context because the new agent has not
	// pushed, so the gate then waits for the new agent's report and pins the
	// chat to it.
	t.Run("RebindOnNewStartBuild", func(t *testing.T) {
		t.Parallel()
		fix := newRebindFixture(t)
		mClock := quartz.NewMock(t)
		trap := mClock.Trap().NewTicker(contextReportTimerTagGroup, contextReportTickerTag)
		defer trap.Close()
		server := newGateServer(t, fix, mClock, defaultContextReportTimeout)

		chat := gateChat(t, fix, fix.agentA, fix.buildID)
		_, hydrateErr := fix.db.HydrateAgentChatsContext(fix.ctx, database.HydrateAgentChatsContextParams{
			AgentID:       fix.agentA,
			AggregateHash: fix.hashA,
		})
		require.NoError(t, hydrateErr)
		newBuild, newAgent := seedStartBuildWithAgent(t, fix.db, fix)

		chat, err := fix.db.GetChatByID(fix.ctx, chat.ID)
		require.NoError(t, err)
		require.Equal(t, fix.hashA, chat.ContextAggregateHash, "chat starts pinned to the old agent")

		wc := newGateTurnContext(t, server, chat)
		results := awaitGateAsync(fix.ctx, server, wc)
		trap.MustWait(fix.ctx).MustRelease(fix.ctx)

		// The rebind committed before the gate started waiting: the chat now
		// points at the new build's agent and its stale pin is cleared.
		mid, err := fix.db.GetChatByID(fix.ctx, chat.ID)
		require.NoError(t, err)
		require.Equal(t, newAgent.ID, mid.AgentID.UUID, "prep rebinds to the new build's agent")
		require.Equal(t, newBuild.ID, mid.BuildID.UUID)
		require.Empty(t, mid.ContextAggregateHash, "re-pin clears the old agent's pinned context")

		// Subscribe after the rebind so the received event is the gate-exit
		// re-pin's context_ready, not noise from earlier writes.
		events := subscribeChatWatchEvents(t, fix)

		hash := []byte{0xdd}
		seedAgentContext(fix.ctx, t, fix.db, newAgent.ID, "/home/coder/AGENTS.md", hash,
			database.WorkspaceAgentContextBodyKindInstructionFile,
			json.RawMessage(`{"instruction_file":{"content":"new build context"}}`))
		mClock.Advance(contextReportPollInterval).MustWait(fix.ctx)

		res := testutil.RequireReceive(fix.ctx, t, results)
		require.NoError(t, res.err)
		require.Equal(t, newAgent.ID, res.agent.ID)

		post, err := fix.db.GetChatByID(fix.ctx, chat.ID)
		require.NoError(t, err)
		require.Equal(t, hash, post.ContextAggregateHash, "chat hydrates with the new agent's context")
		resources, err := fix.db.ListChatContextResourcesByChatID(fix.ctx, chat.ID)
		require.NoError(t, err)
		require.Len(t, resources, 1)
		require.Equal(t, "/home/coder/AGENTS.md", resources[0].Source)

		// The re-pin branch (empty non-NULL hash) publishes context_ready
		// once the pin commits.
		event := testutil.RequireReceive(fix.ctx, t, events)
		require.Equal(t, codersdk.ChatWatchEventKindContextReady, event.Kind,
			"gate-exit re-pinning publishes a context_ready event")
		require.Equal(t, chat.ID, event.Chat.ID)
		require.NotNil(t, event.Chat.Context)
		require.Equal(t, codersdk.ChatContextStateReady, event.Chat.Context.State)
	})
}

// TestPrepareGenerationContextGate proves the gate end to end through
// prepareGeneration: an unpinned workspace chat blocks until the agent's
// first push, then the prepared turn carries the pushed context (the
// instruction lands in the system prompt inputs and the pushed MCP server
// yields a workspace MCP tool).
func TestPrepareGenerationContextGate(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ctx := chatdTestContext(t)
	mClock := quartz.NewMock(t)
	trap := mClock.Trap().NewTicker(contextReportTimerTagGroup, contextReportTickerTag)
	defer trap.Close()

	user := dbgen.User(t, db, database.User{})
	apiKey, _ := dbgen.APIKey(t, db, database.APIKey{UserID: user.ID})
	org := dbgen.Organization(t, db, database.Organization{})
	dbgen.OrganizationMember(t, db, database.OrganizationMember{
		UserID:         user.ID,
		OrganizationID: org.ID,
	})
	provider := dbgen.AIProviderWithOptionalKey(t, db, database.AIProvider{
		Type: database.AIProviderTypeOpenai,
	}, "test-key")
	modelConfig := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{
		Model:        "gpt-4o-mini",
		AIProviderID: uuid.NullUUID{UUID: provider.ID, Valid: true},
	}, func(p *database.InsertChatModelConfigParams) {
		p.Enabled = true
	})

	// A workspace whose agent has not pushed any context yet.
	tv := dbgen.TemplateVersion(t, db, database.TemplateVersion{
		OrganizationID: org.ID,
		CreatedBy:      user.ID,
	})
	tmpl := dbgen.Template(t, db, database.Template{
		OrganizationID:  org.ID,
		ActiveVersionID: tv.ID,
		CreatedBy:       user.ID,
	})
	ws := dbgen.Workspace(t, db, database.WorkspaceTable{
		OwnerID:        user.ID,
		OrganizationID: org.ID,
		TemplateID:     tmpl.ID,
	})
	pj := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
		OrganizationID: org.ID,
		CompletedAt:    sql.NullTime{Valid: true, Time: dbtime.Now()},
	})
	dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
		WorkspaceID:       ws.ID,
		TemplateVersionID: tv.ID,
		JobID:             pj.ID,
		Transition:        database.WorkspaceTransitionStart,
	})
	res := dbgen.WorkspaceResource(t, db, database.WorkspaceResource{
		Transition: database.WorkspaceTransitionStart,
		JobID:      pj.ID,
	})
	agent := dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{ResourceID: res.ID})

	created, err := chatstate.CreateChat(ctx, db, ps, chatstate.CreateChatInput{
		OrganizationID:    org.ID,
		OwnerID:           user.ID,
		WorkspaceID:       uuid.NullUUID{UUID: ws.ID, Valid: true},
		LastModelConfigID: modelConfig.ID,
		Title:             "context gate end to end",
		ClientType:        database.ChatClientTypeApi,
		InitialMessages: []chatstate.Message{
			{
				Role:           database.ChatMessageRoleUser,
				Content:        mustMarshalText(t, "hello"),
				Visibility:     database.ChatMessageVisibilityBoth,
				ModelConfigID:  uuid.NullUUID{UUID: modelConfig.ID, Valid: true},
				CreatedBy:      uuid.NullUUID{UUID: user.ID, Valid: true},
				ContentVersion: chatprompt.CurrentContentVersion,
				APIKeyID:       sql.NullString{String: apiKey.ID, Valid: true},
			},
		},
	})
	require.NoError(t, err)

	server := newInternalTestServer(
		t,
		db,
		ps,
		chatprovider.ProviderAPIKeys{},
		withInternalTestServerClock(mClock),
		withInternalTestServerTransportFactory(&aibridgeTestFactory{}),
	)

	type prepareResult struct {
		prepared generationPrepared
		err      error
	}
	results := make(chan prepareResult, 1)
	go func() {
		prepared, err := server.prepareGeneration(ctx, generationPrepareInput{
			Chat:     created.Chat,
			Messages: created.InitialMessages,
		})
		results <- prepareResult{prepared: prepared, err: err}
	}()

	// The gate is waiting: the agent has no snapshot yet.
	trap.MustWait(ctx).MustRelease(ctx)

	// The agent's first push arrives while the turn waits: an instruction
	// file and an MCP server with one tool.
	now := dbtime.Now()
	hash := []byte{0xab, 0xcd}
	_, err = db.UpsertWorkspaceAgentContextSnapshot(ctx, database.UpsertWorkspaceAgentContextSnapshotParams{
		WorkspaceAgentID: agent.ID,
		Version:          1,
		AggregateHash:    hash,
		ReceivedAt:       now,
	})
	require.NoError(t, err)
	instructionBody, err := protojson.Marshal(&agentproto.InstructionFileBody{Content: []byte("gate instruction content")})
	require.NoError(t, err)
	_, err = db.UpsertWorkspaceAgentContextResource(ctx, database.UpsertWorkspaceAgentContextResourceParams{
		WorkspaceAgentID: agent.ID,
		Source:           "/home/coder/AGENTS.md",
		BodyKind:         database.WorkspaceAgentContextBodyKindInstructionFile,
		Body:             instructionBody,
		ContentHash:      hash,
		SizeBytes:        int64(len(instructionBody)),
		Status:           database.WorkspaceAgentContextResourceStatusOk,
		Now:              now,
	})
	require.NoError(t, err)
	schema, err := structpb.NewStruct(map[string]any{
		"type":       "object",
		"properties": map[string]any{"input": map[string]any{"type": "string"}},
	})
	require.NoError(t, err)
	mcpBody, err := protojson.Marshal(&agentproto.MCPServerBody{
		ServerName: "gatesrv",
		Tools: []*agentproto.MCPTool{{
			Name:        "echo",
			Description: "gate echo tool",
			InputSchema: schema,
		}},
	})
	require.NoError(t, err)
	_, err = db.UpsertWorkspaceAgentContextResource(ctx, database.UpsertWorkspaceAgentContextResourceParams{
		WorkspaceAgentID: agent.ID,
		Source:           "gatesrv",
		BodyKind:         database.WorkspaceAgentContextBodyKindMcpServer,
		Body:             mcpBody,
		ContentHash:      hash,
		SizeBytes:        int64(len(mcpBody)),
		Status:           database.WorkspaceAgentContextResourceStatusOk,
		Now:              now,
	})
	require.NoError(t, err)

	mClock.Advance(contextReportPollInterval).MustWait(ctx)

	result := testutil.RequireReceive(ctx, t, results)
	require.NoError(t, result.err)
	t.Cleanup(result.prepared.Cleanup)

	require.Equal(t, agent.ID, result.prepared.Chat.AgentID.UUID, "turn prep bound the chat to the agent")

	// The pushed MCP server surfaces as a workspace MCP tool on the turn.
	require.NotNil(t, findToolByName(result.prepared.Tools, "gatesrv__echo"),
		"pushed MCP server must yield a workspace MCP tool")

	// The pushed instruction file lands in the system prompt.
	promptText := systemPromptText(t, result.prepared.Prompt)
	require.Contains(t, promptText, "gate instruction content",
		"pushed instruction file must land in the system prompt")

	post, err := db.GetChatByID(ctx, created.Chat.ID)
	require.NoError(t, err)
	require.Equal(t, hash, post.ContextAggregateHash, "the turn pinned the pushed snapshot")
}
