package chatd

import (
	"context"
	"database/sql"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/codersdk"
)

func TestBuildWorkspaceMCPView(t *testing.T) {
	t.Parallel()

	agentID := uuid.New()
	okServer := mcpServerResource(t, "fs", &agentproto.MCPServerBody{
		ServerName: "fs",
		Tools:      []*agentproto.MCPTool{{Name: "read"}, {Name: "write"}},
	}, database.WorkspaceAgentContextResourceStatusOk)
	emptyServer := mcpServerResource(t, "empty", &agentproto.MCPServerBody{ServerName: "empty"}, database.WorkspaceAgentContextResourceStatusOk)
	failedServer := mcpServerResource(t, "broken", &agentproto.MCPServerBody{ServerName: "broken"}, database.WorkspaceAgentContextResourceStatusUnreadable)
	failedServer.Error = "connect \"broken\": exec: not found"
	warnedServer := mcpServerResource(t, "warned", &agentproto.MCPServerBody{
		ServerName: "warned",
		Tools:      []*agentproto.MCPTool{{Name: "ping"}},
	}, database.WorkspaceAgentContextResourceStatusOk)
	warnedServer.Error = "reconnect failed"
	badConfig := database.ChatContextResource{
		Source:   "/w/.mcp.json",
		BodyKind: database.WorkspaceAgentContextBodyKindMcpConfig,
		Status:   database.WorkspaceAgentContextResourceStatusInvalid,
		Error:    "server \"x\" has no command or url",
	}
	okConfig := database.ChatContextResource{
		Source:   "/w/ok/.mcp.json",
		BodyKind: database.WorkspaceAgentContextBodyKindMcpConfig,
		Status:   database.WorkspaceAgentContextResourceStatusOk,
	}
	complete := &database.WorkspaceAgentContextSnapshot{AgentRunID: "run-a", McpDiscoveryPhase: database.WorkspaceAgentMcpDiscoveryPhaseComplete}

	t.Run("NoAgent", func(t *testing.T) {
		t.Parallel()
		view := buildWorkspaceMCPView(database.WorkspaceAgent{}, nil, []database.ChatContextResource{okServer})
		require.Equal(t, codersdk.ChatContextMCPDiscoveryPhaseUnknown, view.phase)
		require.False(t, view.stale)
		require.Len(t, view.Tools(), 2, "nothing can be proven stale without an agent")
		require.False(t, view.Incomplete())
	})

	t.Run("PhaseProjection", func(t *testing.T) {
		t.Parallel()
		for _, tc := range []struct {
			name     string
			agent    database.WorkspaceAgent
			snapshot *database.WorkspaceAgentContextSnapshot
			want     codersdk.ChatContextMCPDiscoveryPhase
		}{
			{name: "LegacyAgentNoSnapshot", agent: database.WorkspaceAgent{ID: agentID}, want: codersdk.ChatContextMCPDiscoveryPhaseUnknown},
			{name: "CurrentAgentNoSnapshot", agent: database.WorkspaceAgent{ID: agentID, AgentRunID: "run-a"}, want: codersdk.ChatContextMCPDiscoveryPhasePending},
			{name: "Unspecified", agent: database.WorkspaceAgent{ID: agentID}, snapshot: &database.WorkspaceAgentContextSnapshot{McpDiscoveryPhase: database.WorkspaceAgentMcpDiscoveryPhaseUnspecified}, want: codersdk.ChatContextMCPDiscoveryPhaseUnknown},
			{name: "Pending", agent: database.WorkspaceAgent{ID: agentID, AgentRunID: "run-a"}, snapshot: &database.WorkspaceAgentContextSnapshot{AgentRunID: "run-a", McpDiscoveryPhase: database.WorkspaceAgentMcpDiscoveryPhasePending}, want: codersdk.ChatContextMCPDiscoveryPhasePending},
			{name: "Complete", agent: database.WorkspaceAgent{ID: agentID, AgentRunID: "run-a"}, snapshot: complete, want: codersdk.ChatContextMCPDiscoveryPhaseComplete},
		} {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				view := buildWorkspaceMCPView(tc.agent, tc.snapshot, nil)
				require.Equal(t, tc.want, view.phase)
				require.Equal(t, &codersdk.ChatContextMCPDiscovery{Phase: tc.want}, view.Discovery())
			})
		}
	})

	t.Run("FreshnessRule", func(t *testing.T) {
		t.Parallel()
		for _, tc := range []struct {
			name        string
			agentRun    string
			snapshotRun string
			stale       bool
		}{
			{name: "SameRun", agentRun: "run-a", snapshotRun: "run-a"},
			{name: "DifferentRun", agentRun: "run-b", snapshotRun: "run-a", stale: true},
			// One known id is enough: a legacy snapshot under a current
			// agent, or a rollback to a legacy agent over a newer snapshot,
			// both name a different process.
			{name: "LegacySnapshotUnderCurrentAgent", agentRun: "run-b", snapshotRun: "", stale: true},
			{name: "NewerSnapshotUnderLegacyAgent", agentRun: "", snapshotRun: "run-a", stale: true},
			{name: "BothEmpty"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				view := buildWorkspaceMCPView(
					database.WorkspaceAgent{ID: agentID, AgentRunID: tc.agentRun},
					&database.WorkspaceAgentContextSnapshot{AgentRunID: tc.snapshotRun, McpDiscoveryPhase: database.WorkspaceAgentMcpDiscoveryPhaseComplete},
					[]database.ChatContextResource{okServer},
				)
				require.Equal(t, tc.stale, view.stale)
				if tc.stale {
					require.Empty(t, view.Tools(), "a stale view withholds every tool")
					require.True(t, view.Incomplete())
					require.Contains(t, view.Summary(), "previous agent process")
				} else {
					require.Len(t, view.Tools(), 2, "an unprovable staleness withholds nothing")
				}
				// Diagnostics are reported either way.
				require.Len(t, view.servers, 1)
				require.Equal(t, 2, view.servers[0].toolCount)
			})
		}
	})

	t.Run("OutcomesAndSummary", func(t *testing.T) {
		t.Parallel()
		view := buildWorkspaceMCPView(
			database.WorkspaceAgent{ID: agentID, AgentRunID: "run-a"},
			complete,
			[]database.ChatContextResource{okServer, emptyServer, failedServer, warnedServer, badConfig, okConfig},
		)
		require.Equal(t, codersdk.ChatContextMCPDiscoveryPhaseComplete, view.phase)
		require.False(t, view.stale)
		require.True(t, view.Incomplete(), "a failed source makes the view worth reporting")

		names := make([]string, 0, len(view.Tools()))
		for _, tool := range view.Tools() {
			names = append(names, tool.Name)
		}
		require.Equal(t, []string{"fs__read", "fs__write", "warned__ping"}, names, "only OK servers contribute tools")
		require.Len(t, view.servers, 4)
		require.Len(t, view.configs, 1, "only the invalid config is an outcome")

		summary := view.Summary()
		require.Contains(t, summary, "Workspace MCP discovery is complete.")
		require.Contains(t, summary, "fs (2 tools)")
		require.Contains(t, summary, "empty (0 tools)")
		require.Contains(t, summary, "warned (1 tools, warning: reconnect failed)")
		require.Contains(t, summary, "broken (connect \"broken\": exec: not found)")
		require.Contains(t, summary, "/w/.mcp.json (server \"x\" has no command or url)")
		require.NotContains(t, summary, "healthy")
		require.NotContains(t, summary, "connected")
	})

	t.Run("CompleteWithoutIssuesIsNotIncomplete", func(t *testing.T) {
		t.Parallel()
		view := buildWorkspaceMCPView(database.WorkspaceAgent{ID: agentID, AgentRunID: "run-a"}, complete, []database.ChatContextResource{okServer, okConfig})
		require.False(t, view.Incomplete())
		require.Contains(t, view.Summary(), "fs (2 tools)")
	})

	t.Run("PendingIsIncomplete", func(t *testing.T) {
		t.Parallel()
		view := buildWorkspaceMCPView(database.WorkspaceAgent{ID: agentID, AgentRunID: "run-a"}, &database.WorkspaceAgentContextSnapshot{AgentRunID: "run-a", McpDiscoveryPhase: database.WorkspaceAgentMcpDiscoveryPhasePending}, []database.ChatContextResource{okServer})
		require.True(t, view.Incomplete())
		require.Len(t, view.Tools(), 2, "pending does not withhold discovered tools")
		require.Contains(t, view.Summary(), "still initializing")
	})
}

// expectWorkspaceMCPViewTx drives the view's read-only transaction against
// the same mock so the agent and snapshot expectations below are reached.
func expectWorkspaceMCPViewTx(db *dbmock.MockStore) {
	db.EXPECT().InTx(gomock.Any(), gomock.Any()).DoAndReturn(
		func(f func(database.Store) error, _ *database.TxOptions) error { return f(db) })
}

func TestLoadWorkspaceMCPView(t *testing.T) {
	t.Parallel()

	// A restart committing between the two reads would otherwise pair one
	// process's agent row with another process's snapshot, so both reads
	// go through one repeatable-read, read-only transaction handle.
	t.Run("ReadsAgentAndSnapshotInOneTransaction", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		tx := dbmock.NewMockStore(ctrl)
		agentID := uuid.New()
		db.EXPECT().InTx(gomock.Any(), gomock.Any()).DoAndReturn(
			func(f func(database.Store) error, opts *database.TxOptions) error {
				require.Equal(t, sql.LevelRepeatableRead, opts.Isolation)
				require.True(t, opts.ReadOnly)
				return f(tx)
			})
		tx.EXPECT().GetWorkspaceAgentByID(gomock.Any(), agentID).
			Return(database.WorkspaceAgent{ID: agentID, AgentRunID: "run-a"}, nil)
		tx.EXPECT().GetLatestWorkspaceAgentContextSnapshot(gomock.Any(), agentID).
			Return(database.WorkspaceAgentContextSnapshot{AgentRunID: "run-a", McpDiscoveryPhase: database.WorkspaceAgentMcpDiscoveryPhaseComplete}, nil)
		server := newPinServer(t, db)

		view, err := server.workspaceMCPViewForPinned(context.Background(), database.Chat{
			ID:      uuid.New(),
			AgentID: uuid.NullUUID{UUID: agentID, Valid: true},
		}, nil)
		require.NoError(t, err)
		require.Equal(t, &codersdk.ChatContextMCPDiscovery{Phase: codersdk.ChatContextMCPDiscoveryPhaseComplete}, view.Discovery())
	})

	t.Run("NoSnapshotYet", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		chatID := uuid.New()
		agentID := uuid.New()
		expectWorkspaceMCPViewTx(db)
		db.EXPECT().GetWorkspaceAgentByID(gomock.Any(), agentID).
			Return(database.WorkspaceAgent{ID: agentID, AgentRunID: "run-a"}, nil)
		db.EXPECT().GetLatestWorkspaceAgentContextSnapshot(gomock.Any(), agentID).
			Return(database.WorkspaceAgentContextSnapshot{}, sql.ErrNoRows)
		db.EXPECT().ListChatContextResourcesByChatID(gomock.Any(), chatID).Return(nil, nil)
		server := newPinServer(t, db)

		view, err := server.loadWorkspaceMCPView(context.Background(), database.Chat{
			ID:      chatID,
			AgentID: uuid.NullUUID{UUID: agentID, Valid: true},
		})
		require.NoError(t, err)
		require.Equal(t, codersdk.ChatContextMCPDiscoveryPhasePending, view.phase)
		require.False(t, view.stale)
	})

	t.Run("ReusesPinnedRows", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		agentID := uuid.New()
		expectWorkspaceMCPViewTx(db)
		db.EXPECT().GetWorkspaceAgentByID(gomock.Any(), agentID).
			Return(database.WorkspaceAgent{ID: agentID, AgentRunID: "run-a"}, nil)
		db.EXPECT().GetLatestWorkspaceAgentContextSnapshot(gomock.Any(), agentID).
			Return(database.WorkspaceAgentContextSnapshot{AgentRunID: "run-a", McpDiscoveryPhase: database.WorkspaceAgentMcpDiscoveryPhaseComplete}, nil)
		// No ListChatContextResourcesByChatID expectation: the caller's rows are used.
		server := newPinServer(t, db)

		pinned := []database.ChatContextResource{mcpServerResource(t, "fs", &agentproto.MCPServerBody{
			ServerName: "fs", Tools: []*agentproto.MCPTool{{Name: "read"}},
		}, database.WorkspaceAgentContextResourceStatusOk)}
		view, err := server.workspaceMCPViewForPinned(context.Background(), database.Chat{
			ID:      uuid.New(),
			AgentID: uuid.NullUUID{UUID: agentID, Valid: true},
		}, pinned)
		require.NoError(t, err)
		require.Len(t, view.Tools(), 1)
	})

	t.Run("ReplacedAgentProjectsStaleRows", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		agentID := uuid.New()
		// A rebuild soft-deleted the bound agent, which GetWorkspaceAgentByID
		// filters out, and purged its snapshot.
		expectWorkspaceMCPViewTx(db)
		db.EXPECT().GetWorkspaceAgentByID(gomock.Any(), agentID).
			Return(database.WorkspaceAgent{}, sql.ErrNoRows)
		server := newPinServer(t, db)

		pinned := []database.ChatContextResource{mcpServerResource(t, "fs", &agentproto.MCPServerBody{
			ServerName: "fs", Tools: []*agentproto.MCPTool{{Name: "read"}},
		}, database.WorkspaceAgentContextResourceStatusOk)}
		view, err := server.workspaceMCPViewForPinned(context.Background(), database.Chat{
			ID:      uuid.New(),
			AgentID: uuid.NullUUID{UUID: agentID, Valid: true},
		}, pinned)
		require.NoError(t, err, "a replaced agent must not fail the projection")
		require.Equal(t, &codersdk.ChatContextMCPDiscovery{Phase: codersdk.ChatContextMCPDiscoveryPhaseUnknown, Stale: true}, view.Discovery())
		require.Empty(t, view.Tools(), "rows from a replaced agent are withheld")
		require.Len(t, view.servers, 1, "the pinned outcomes are still reported")
		require.Equal(t, 1, view.servers[0].toolCount)
	})
}

// TestResolveWorkspaceMCPTools_UsesReboundAgent covers the first turn after
// a rebuild: getWorkspaceAgent has already rebound the turn's chat snapshot
// to the replacement agent, while the chat row loaded at turn start still
// names the soft-deleted one. The view must follow the rebound agent, or
// every replacement tool is withheld for that turn.
func TestResolveWorkspaceMCPTools_UsesReboundAgent(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	chatID := uuid.New()
	oldAgentID := uuid.New()
	newAgentID := uuid.New()
	db.EXPECT().ListChatContextResourcesByChatID(gomock.Any(), chatID).
		Return([]database.ChatContextResource{mcpServerResource(t, "fs", &agentproto.MCPServerBody{
			ServerName: "fs", Tools: []*agentproto.MCPTool{{Name: "read"}},
		}, database.WorkspaceAgentContextResourceStatusOk)}, nil)
	expectWorkspaceMCPViewTx(db)
	db.EXPECT().GetWorkspaceAgentByID(gomock.Any(), newAgentID).
		Return(database.WorkspaceAgent{ID: newAgentID, AgentRunID: "run-b"}, nil)
	db.EXPECT().GetLatestWorkspaceAgentContextSnapshot(gomock.Any(), newAgentID).
		Return(database.WorkspaceAgentContextSnapshot{AgentRunID: "run-b", McpDiscoveryPhase: database.WorkspaceAgentMcpDiscoveryPhaseComplete}, nil)
	server := newPinServer(t, db)

	loaded := database.Chat{ID: chatID, AgentID: uuid.NullUUID{UUID: oldAgentID, Valid: true}}
	rebound := loaded
	rebound.AgentID = uuid.NullUUID{UUID: newAgentID, Valid: true}
	workspaceCtx := &turnWorkspaceContext{
		server:      server,
		chatStateMu: &sync.Mutex{},
		currentChat: &rebound,
	}

	tools := server.resolveWorkspaceMCPTools(context.Background(), server.logger, loaded, workspaceCtx)
	require.Len(t, tools, 1, "replacement tools are served on the rebinding turn")
}
