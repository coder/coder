package chatd

import (
	"context"
	"database/sql"
	"encoding/json"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/testutil"
)

// TestPersistBuildAgentBindingRepinsContext covers the agent-rebind re-pin path
// in persistBuildAgentBinding, which the end-to-end context test does not
// reach: the shared repinChatContext helper is proven through the refresh
// endpoint, but the rebind-specific wiring (the AgentID change guard, the
// AsChatd escalation, the ReadModifyUpdate transaction, and the best-effort
// error swallow) is exercised only here.
func TestPersistBuildAgentBindingRepinsContext(t *testing.T) {
	t.Parallel()

	// RebindsToNewAgent: a chat pinned to agent A is rebound to agent B, so
	// its pinned hash and pinned resources must switch to B's snapshot.
	t.Run("RebindsToNewAgent", func(t *testing.T) {
		t.Parallel()
		fix := newRebindFixture(t)

		chat := dbgen.Chat(t, fix.db, database.Chat{
			OwnerID:           fix.user.ID,
			OrganizationID:    fix.org.ID,
			LastModelConfigID: fix.model.ID,
			WorkspaceID:       uuid.NullUUID{UUID: fix.ws.ID, Valid: true},
			AgentID:           uuid.NullUUID{UUID: fix.agentA, Valid: true},
			Status:            database.ChatStatusWaiting,
		})

		// Pin the chat to agent A through the production hydrate path so it
		// starts with A's hash and A's resources, exactly as an agent push
		// would leave it.
		_, err := fix.db.HydrateAgentChatsContext(fix.ctx, database.HydrateAgentChatsContextParams{
			AgentID:       fix.agentA,
			AggregateHash: fix.hashA,
		})
		require.NoError(t, err)
		preRes, err := fix.db.ListChatContextResourcesByChatID(fix.ctx, chat.ID)
		require.NoError(t, err)
		require.Len(t, preRes, 1)
		require.Equal(t, fix.srcA, preRes[0].Source)

		wc := newRebindTurnContext(t, fix.db, chat)
		updated, err := wc.persistBuildAgentBinding(fix.ctx, chat, fix.buildID, fix.agentB)
		require.NoError(t, err)
		require.True(t, updated.AgentID.Valid)
		require.Equal(t, fix.agentB, updated.AgentID.UUID, "the binding commits the new agent")

		// The re-pin runs in its own transaction after the binding row is
		// written, so re-read the chat to observe the new pinned state.
		post, err := fix.db.GetChatByID(fix.ctx, chat.ID)
		require.NoError(t, err)
		require.Equal(t, fix.hashB, post.ContextAggregateHash, "rebind re-pins the new agent's hash")

		postRes, err := fix.db.ListChatContextResourcesByChatID(fix.ctx, chat.ID)
		require.NoError(t, err)
		require.Len(t, postRes, 1, "rebind swaps the pinned resources to the new agent's set")
		require.Equal(t, fix.srcB, postRes[0].Source)
		require.Equal(t, fix.hashB, postRes[0].ContentHash)
		require.JSONEq(t, string(fix.bodyB), string(postRes[0].Body), "the new agent's resource body is copied verbatim")
	})

	// SkipsRepinWhenNoPriorAgent: binding a chat that had no agent must not
	// re-pin here (the create/push path owns first-time pinning), so the guard
	// leaves the chat's context untouched.
	t.Run("SkipsRepinWhenNoPriorAgent", func(t *testing.T) {
		t.Parallel()
		fix := newRebindFixture(t)

		chat := dbgen.Chat(t, fix.db, database.Chat{
			OwnerID:           fix.user.ID,
			OrganizationID:    fix.org.ID,
			LastModelConfigID: fix.model.ID,
			WorkspaceID:       uuid.NullUUID{UUID: fix.ws.ID, Valid: true},
			Status:            database.ChatStatusWaiting,
		})
		require.False(t, chat.AgentID.Valid, "chat starts with no bound agent")

		wc := newRebindTurnContext(t, fix.db, chat)
		updated, err := wc.persistBuildAgentBinding(fix.ctx, chat, fix.buildID, fix.agentB)
		require.NoError(t, err)
		require.Equal(t, fix.agentB, updated.AgentID.UUID, "the binding commits the new agent")

		post, err := fix.db.GetChatByID(fix.ctx, chat.ID)
		require.NoError(t, err)
		require.Nil(t, post.ContextAggregateHash, "first-time binding does not re-pin a hash")
		postRes, err := fix.db.ListChatContextResourcesByChatID(fix.ctx, chat.ID)
		require.NoError(t, err)
		require.Empty(t, postRes, "first-time binding copies no resources via the rebind path")
	})

	// ClearsContextWhenNewAgentHasNoSnapshot: rebinding to an agent that has
	// not pushed a snapshot yet clears the chat's pinned hash and resources
	// (repinChatContext's no-snapshot branch).
	t.Run("ClearsContextWhenNewAgentHasNoSnapshot", func(t *testing.T) {
		t.Parallel()
		fix := newRebindFixture(t)

		chat := dbgen.Chat(t, fix.db, database.Chat{
			OwnerID:           fix.user.ID,
			OrganizationID:    fix.org.ID,
			LastModelConfigID: fix.model.ID,
			WorkspaceID:       uuid.NullUUID{UUID: fix.ws.ID, Valid: true},
			AgentID:           uuid.NullUUID{UUID: fix.agentA, Valid: true},
			Status:            database.ChatStatusWaiting,
		})
		_, err := fix.db.HydrateAgentChatsContext(fix.ctx, database.HydrateAgentChatsContextParams{
			AgentID:       fix.agentA,
			AggregateHash: fix.hashA,
		})
		require.NoError(t, err)
		preRes, err := fix.db.ListChatContextResourcesByChatID(fix.ctx, chat.ID)
		require.NoError(t, err)
		require.Len(t, preRes, 1, "chat starts pinned to agent A")

		wc := newRebindTurnContext(t, fix.db, chat)
		updated, err := wc.persistBuildAgentBinding(fix.ctx, chat, fix.buildID, fix.agentNoSnap)
		require.NoError(t, err)
		require.Equal(t, fix.agentNoSnap, updated.AgentID.UUID)

		post, err := fix.db.GetChatByID(fix.ctx, chat.ID)
		require.NoError(t, err)
		require.Empty(t, post.ContextAggregateHash, "rebinding to an agent with no snapshot clears the pinned hash")
		postRes, err := fix.db.ListChatContextResourcesByChatID(fix.ctx, chat.ID)
		require.NoError(t, err)
		require.Empty(t, postRes, "rebinding to an agent with no snapshot clears the pinned resources")
	})

	// SwallowsRepinError: a re-pin failure is logged and swallowed so it never
	// fails the agent binding itself.
	t.Run("SwallowsRepinError", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		ctrl := gomock.NewController(t)
		dbm := dbmock.NewMockStore(ctrl)
		server := &Server{db: dbm, logger: slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})}

		chatID := uuid.New()
		priorAgent := uuid.New()
		newAgent := uuid.New()
		buildID := uuid.New()
		boundChat := database.Chat{
			ID:      chatID,
			BuildID: uuid.NullUUID{UUID: buildID, Valid: true},
			AgentID: uuid.NullUUID{UUID: newAgent, Valid: true},
		}

		dbm.EXPECT().UpdateChatBuildAgentBinding(gomock.Any(), database.UpdateChatBuildAgentBindingParams{
			ID:      chatID,
			BuildID: uuid.NullUUID{UUID: buildID, Valid: true},
			AgentID: uuid.NullUUID{UUID: newAgent, Valid: true},
		}).Return(boundChat, nil)
		// The re-pin runs inside ReadModifyUpdate; drive the closure and fail
		// its first read so repinChatContext returns an error.
		dbm.EXPECT().InTx(gomock.Any(), gomock.Any()).DoAndReturn(
			func(f func(database.Store) error, _ *database.TxOptions) error {
				return f(dbm)
			})
		dbm.EXPECT().GetLatestWorkspaceAgentContextSnapshot(gomock.Any(), newAgent).
			Return(database.WorkspaceAgentContextSnapshot{}, xerrors.New("boom"))

		cur := database.Chat{ID: chatID}
		wc := turnWorkspaceContext{
			server:           server,
			chatStateMu:      &sync.Mutex{},
			currentChat:      &cur,
			loadChatSnapshot: func(context.Context, uuid.UUID) (database.Chat, error) { return database.Chat{}, nil },
		}
		t.Cleanup(wc.close)

		prior := database.Chat{ID: chatID, AgentID: uuid.NullUUID{UUID: priorAgent, Valid: true}}
		updated, err := wc.persistBuildAgentBinding(ctx, prior, buildID, newAgent)
		require.NoError(t, err, "a re-pin failure must not fail the binding")
		require.Equal(t, newAgent, updated.AgentID.UUID)
		require.Equal(t, boundChat, wc.currentChatSnapshot(), "the binding still commits the new agent")
	})
}

type rebindFixture struct {
	db          database.Store
	ctx         context.Context
	org         database.Organization
	user        database.User
	model       database.ChatModelConfig
	ws          database.WorkspaceTable
	buildID     uuid.UUID
	agentA      uuid.UUID
	agentB      uuid.UUID
	agentNoSnap uuid.UUID
	hashA       []byte
	hashB       []byte
	srcA        string
	srcB        string
	bodyB       json.RawMessage
}

// newRebindFixture seeds a workspace with agents A and B that each have a
// pushed context snapshot and one resource (so a chat can be pinned to A and
// rebound to B), plus a third agent that never pushes a snapshot (for the
// clear-only re-pin path). The agents share one build/resource because the
// rebind guard keys on the agent, not the build.
func newRebindFixture(t *testing.T) rebindFixture {
	t.Helper()
	db, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	user := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})
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
	build := dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
		WorkspaceID:       ws.ID,
		TemplateVersionID: tv.ID,
		JobID:             pj.ID,
		Transition:        database.WorkspaceTransitionStart,
	})
	res := dbgen.WorkspaceResource(t, db, database.WorkspaceResource{
		Transition: database.WorkspaceTransitionStart,
		JobID:      pj.ID,
	})
	agentA := dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{ResourceID: res.ID})
	agentB := dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{ResourceID: res.ID})
	// A third agent that never pushes a snapshot, for the clear-only re-pin path.
	agentNoSnap := dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{ResourceID: res.ID})
	model := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{})

	fix := rebindFixture{
		db:          db,
		ctx:         ctx,
		org:         org,
		user:        user,
		model:       model,
		ws:          ws,
		buildID:     build.ID,
		agentA:      agentA.ID,
		agentB:      agentB.ID,
		agentNoSnap: agentNoSnap.ID,
		hashA:       []byte{0xa1, 0xa2},
		hashB:       []byte{0xb1, 0xb2},
		srcA:        "/home/coder/workspace/AGENTS.md",
		srcB:        "/home/coder/workspace/.agents/skills/example/SKILL.md",
		bodyB:       json.RawMessage(`{"skill":{"name":"example"}}`),
	}

	seedAgentContext(ctx, t, db, fix.agentA, fix.srcA, fix.hashA,
		database.WorkspaceAgentContextBodyKindInstructionFile, json.RawMessage(`{"instruction_file":{"content":"agent-a"}}`))
	seedAgentContext(ctx, t, db, fix.agentB, fix.srcB, fix.hashB,
		database.WorkspaceAgentContextBodyKindSkill, fix.bodyB)
	return fix
}

func seedAgentContext(ctx context.Context, t *testing.T, db database.Store, agentID uuid.UUID, source string, hash []byte, kind database.WorkspaceAgentContextBodyKind, body json.RawMessage) {
	t.Helper()
	now := dbtime.Now()
	_, err := db.UpsertWorkspaceAgentContextSnapshot(ctx, database.UpsertWorkspaceAgentContextSnapshotParams{
		WorkspaceAgentID: agentID,
		Version:          1,
		AggregateHash:    hash,
		ReceivedAt:       now,
	})
	require.NoError(t, err)
	_, err = db.UpsertWorkspaceAgentContextResource(ctx, database.UpsertWorkspaceAgentContextResourceParams{
		WorkspaceAgentID: agentID,
		Source:           source,
		BodyKind:         kind,
		Body:             body,
		ContentHash:      hash,
		SizeBytes:        int64(len(body)),
		Status:           database.WorkspaceAgentContextResourceStatusOk,
		Now:              now,
	})
	require.NoError(t, err)
}

func newRebindTurnContext(t *testing.T, db database.Store, chat database.Chat) *turnWorkspaceContext {
	t.Helper()
	server := &Server{db: db, logger: slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})}
	cur := chat
	wc := &turnWorkspaceContext{
		server:           server,
		chatStateMu:      &sync.Mutex{},
		currentChat:      &cur,
		loadChatSnapshot: db.GetChatByID,
	}
	t.Cleanup(wc.close)
	return wc
}
