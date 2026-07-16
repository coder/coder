package chatd //nolint:testpackage // Tests the unexported execution sweep reconciler.

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk/agentconnmock"
	"github.com/coder/coder/v2/testutil"
)

// seedSweepWorkspaceAgent creates the workspace scaffolding needed
// for an execution row's workspace_agent_id foreign key.
func seedSweepWorkspaceAgent(t *testing.T, db database.Store, userID uuid.UUID, orgID uuid.UUID) database.WorkspaceAgent {
	t.Helper()
	tv := dbgen.TemplateVersion(t, db, database.TemplateVersion{OrganizationID: orgID, CreatedBy: userID})
	tpl := dbgen.Template(t, db, database.Template{CreatedBy: userID, OrganizationID: orgID, ActiveVersionID: tv.ID})
	ws := dbgen.Workspace(t, db, database.WorkspaceTable{TemplateID: tpl.ID, OwnerID: userID, OrganizationID: orgID})
	pj := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{InitiatorID: userID, OrganizationID: orgID})
	_ = dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{TemplateVersionID: tv.ID, WorkspaceID: ws.ID, JobID: pj.ID})
	res := dbgen.WorkspaceResource(t, db, database.WorkspaceResource{Transition: database.WorkspaceTransitionStart, JobID: pj.ID})
	return dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{ResourceID: res.ID})
}

// seedCancelRequestedExecution creates a chat with one execution
// row in cancel_requested carrying full process identity, with
// updated_at backdated by staleAgo.
func seedCancelRequestedExecution(ctx context.Context, t *testing.T, db database.Store, staleAgo time.Duration) database.ChatToolCallExecution {
	t.Helper()
	user := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})
	_ = dbgen.OrganizationMember(t, db, database.OrganizationMember{UserID: user.ID, OrganizationID: org.ID})
	_ = dbgen.ChatProvider(t, db, database.ChatProvider{Provider: "openai", DisplayName: "OpenAI"})
	mc := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{Model: "test-model", ContextLimit: 8192})
	chat := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    org.ID,
		OwnerID:           user.ID,
		LastModelConfigID: mc.ID,
		Title:             "sweep-test",
	})
	msg := dbgen.ChatMessage(t, db, database.ChatMessage{
		ChatID: chat.ID,
		Role:   database.ChatMessageRoleAssistant,
	})
	agent := seedSweepWorkspaceAgent(t, db, user.ID, org.ID)

	past := dbtime.Now().Add(-staleAgo)
	claimed, err := db.ClaimChatToolCallExecution(ctx, database.ClaimChatToolCallExecutionParams{
		ID:                 uuid.New(),
		ChatID:             chat.ID,
		AssistantMessageID: msg.ID,
		ToolCallID:         "sweep-call",
		InputSha256:        "hash",
		Command:            "sleep 600",
		TimeoutSecs:        600,
		Now:                past,
		StaleBefore:        time.Time{},
	})
	require.NoError(t, err)
	_, err = db.UpdateChatToolCallExecutionProcess(ctx, database.UpdateChatToolCallExecutionProcessParams{
		ChatID:             chat.ID,
		AssistantMessageID: msg.ID,
		ToolCallID:         "sweep-call",
		ClaimEpoch:         claimed.ClaimEpoch,
		ProcessID:          "proc-sweep",
		WorkspaceAgentID:   agent.ID,
		StartedAt:          past,
	})
	require.NoError(t, err)
	rows, err := db.MarkChatToolCallExecutionsInterrupted(ctx, database.MarkChatToolCallExecutionsInterruptedParams{
		ChatID:             chat.ID,
		AssistantMessageID: msg.ID,
		ToolCallIds:        []string{"sweep-call"},
		UpdatedAt:          past,
	})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, database.ChatToolCallExecutionStatusCancelRequested, rows[0].Status)
	return rows[0]
}

func TestExecutionSweep(t *testing.T) {
	t.Parallel()

	t.Run("KillsStalledCancelRequested", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitLong)
		record := seedCancelRequestedExecution(ctx, t, db, 10*time.Minute)

		ctrl := gomock.NewController(t)
		conn := agentconnmock.NewMockAgentConn(ctrl)
		conn.EXPECT().SignalProcess(gomock.Any(), "proc-sweep", "kill").Return(nil)
		exitCode := -1
		conn.EXPECT().ProcessOutput(gomock.Any(), "proc-sweep", gomock.Any()).
			Return(workspacesdk.ProcessOutputResponse{Running: false, ExitCode: &exitCode}, nil)

		r := executionReconciler{
			store:  db,
			logger: slogtest.Make(t, nil),
			agentConn: func(_ context.Context, agentID uuid.UUID) (workspacesdk.AgentConn, func(), error) {
				require.Equal(t, record.WorkspaceAgentID.UUID, agentID)
				// A nil release must not panic.
				return conn, nil, nil
			},
		}
		r.sweepOnce(ctx, dbtime.Now())

		row, err := db.GetChatToolCallExecution(ctx, database.GetChatToolCallExecutionParams{
			ChatID:             record.ChatID,
			AssistantMessageID: record.AssistantMessageID,
			ToolCallID:         record.ToolCallID,
		})
		require.NoError(t, err)
		require.Equal(t, database.ChatToolCallExecutionStatusCanceled, row.Status)
		require.True(t, row.CancelSignalSentAt.Valid)
	})

	t.Run("UnreachableAgentLeavesRowForNextSweep", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitLong)
		record := seedCancelRequestedExecution(ctx, t, db, 10*time.Minute)

		dialErr := xerrors.New("agent unreachable")
		r := executionReconciler{
			store:  db,
			logger: slogtest.Make(t, nil),
			agentConn: func(_ context.Context, _ uuid.UUID) (workspacesdk.AgentConn, func(), error) {
				return nil, nil, dialErr
			},
		}
		firstSweep := dbtime.Now()
		r.sweepOnce(ctx, firstSweep)

		row, err := db.GetChatToolCallExecution(ctx, database.GetChatToolCallExecutionParams{
			ChatID:             record.ChatID,
			AssistantMessageID: record.AssistantMessageID,
			ToolCallID:         record.ToolCallID,
		})
		require.NoError(t, err)
		// The row survives with its identity, leased forward so
		// the next sweep (not an immediate re-claim) retries it.
		require.Equal(t, database.ChatToolCallExecutionStatusCancelRequested, row.Status)
		require.Equal(t, "proc-sweep", row.ProcessID.String)
		require.True(t, row.UpdatedAt.After(record.UpdatedAt))

		// Within the retry age nothing is claimable.
		claimed, err := db.ClaimStaleChatToolCallExecutionCancels(ctx, database.ClaimStaleChatToolCallExecutionCancelsParams{
			UpdatedBefore: firstSweep.Add(-executionSweepRetryAge),
			LimitCount:    10,
			Now:           dbtime.Now(),
		})
		require.NoError(t, err)
		require.Empty(t, claimed)

		// Once the lease ages out, a later sweep claims and kills.
		ctrl := gomock.NewController(t)
		conn := agentconnmock.NewMockAgentConn(ctrl)
		conn.EXPECT().SignalProcess(gomock.Any(), "proc-sweep", "kill").Return(nil)
		exitCode := -1
		conn.EXPECT().ProcessOutput(gomock.Any(), "proc-sweep", gomock.Any()).
			Return(workspacesdk.ProcessOutputResponse{Running: false, ExitCode: &exitCode}, nil)
		r.agentConn = func(_ context.Context, _ uuid.UUID) (workspacesdk.AgentConn, func(), error) {
			return conn, func() {}, nil
		}
		r.sweepOnce(ctx, row.UpdatedAt.Add(executionSweepRetryAge+time.Second))

		row, err = db.GetChatToolCallExecution(ctx, database.GetChatToolCallExecutionParams{
			ChatID:             record.ChatID,
			AssistantMessageID: record.AssistantMessageID,
			ToolCallID:         record.ToolCallID,
		})
		require.NoError(t, err)
		require.Equal(t, database.ChatToolCallExecutionStatusCanceled, row.Status)
	})

	t.Run("ClaimSkipsFreshAndResolvedRows", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitLong)
		record := seedCancelRequestedExecution(ctx, t, db, time.Minute)

		// Fresh row (updated during the immediate pass's window):
		// not claimable.
		claimed, err := db.ClaimStaleChatToolCallExecutionCancels(ctx, database.ClaimStaleChatToolCallExecutionCancelsParams{
			UpdatedBefore: dbtime.Now().Add(-executionSweepRetryAge),
			LimitCount:    10,
			Now:           dbtime.Now(),
		})
		require.NoError(t, err)
		require.Empty(t, claimed)

		// Resolved rows are never claimable regardless of age.
		_, err = db.UpdateChatToolCallExecutionCancelOutcome(ctx, database.UpdateChatToolCallExecutionCancelOutcomeParams{
			ChatID:             record.ChatID,
			AssistantMessageID: record.AssistantMessageID,
			ToolCallID:         record.ToolCallID,
			Status:             database.ChatToolCallExecutionStatusCanceled,
			CancelSignalSentAt: sql.NullTime{},
			UpdatedAt:          dbtime.Now().Add(-time.Hour),
		})
		require.NoError(t, err)
		claimed, err = db.ClaimStaleChatToolCallExecutionCancels(ctx, database.ClaimStaleChatToolCallExecutionCancelsParams{
			UpdatedBefore: dbtime.Now(),
			LimitCount:    10,
			Now:           dbtime.Now(),
		})
		require.NoError(t, err)
		require.Empty(t, claimed)
	})
}

// TestReconcileCancelRequestedBackgroundDetaches asserts that a
// background execution whose handle landed only after the interrupt
// commit is resolved to detached without signaling: the interrupt
// spares background processes.
func TestReconcileCancelRequestedBackgroundDetaches(t *testing.T) {
	t.Parallel()
	db, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	user := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})
	_ = dbgen.OrganizationMember(t, db, database.OrganizationMember{UserID: user.ID, OrganizationID: org.ID})
	_ = dbgen.ChatProvider(t, db, database.ChatProvider{Provider: "openai", DisplayName: "OpenAI"})
	mc := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{Model: "test-model", ContextLimit: 8192})
	chat := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    org.ID,
		OwnerID:           user.ID,
		LastModelConfigID: mc.ID,
		Title:             "bg-detach-test",
	})
	msg := dbgen.ChatMessage(t, db, database.ChatMessage{ChatID: chat.ID, Role: database.ChatMessageRoleAssistant})
	agent := seedSweepWorkspaceAgent(t, db, user.ID, org.ID)

	claimed, err := db.ClaimChatToolCallExecution(ctx, database.ClaimChatToolCallExecutionParams{
		ID:                 uuid.New(),
		ChatID:             chat.ID,
		AssistantMessageID: msg.ID,
		ToolCallID:         "bg-call",
		InputSha256:        "hash",
		Command:            "sleep 600",
		Background:         true,
		TimeoutSecs:        600,
		Now:                dbtime.Now(),
		StaleBefore:        time.Time{},
	})
	require.NoError(t, err)

	// The interrupt lands while the background start is in flight:
	// no handle yet, so the row maps to cancel_requested, not
	// detached.
	rows, err := db.MarkChatToolCallExecutionsInterrupted(ctx, database.MarkChatToolCallExecutionsInterruptedParams{
		ChatID:             chat.ID,
		AssistantMessageID: msg.ID,
		ToolCallIds:        []string{"bg-call"},
		UpdatedAt:          dbtime.Now(),
	})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, database.ChatToolCallExecutionStatusCancelRequested, rows[0].Status)

	// The late handle write lands on the cancel_requested row.
	_, err = db.UpdateChatToolCallExecutionProcess(ctx, database.UpdateChatToolCallExecutionProcessParams{
		ChatID:             chat.ID,
		AssistantMessageID: msg.ID,
		ToolCallID:         "bg-call",
		ClaimEpoch:         claimed.ClaimEpoch,
		ProcessID:          "proc-bg",
		WorkspaceAgentID:   agent.ID,
		StartedAt:          dbtime.Now(),
	})
	require.NoError(t, err)

	record, err := db.GetChatToolCallExecution(ctx, database.GetChatToolCallExecutionParams{
		ChatID:             chat.ID,
		AssistantMessageID: msg.ID,
		ToolCallID:         "bg-call",
	})
	require.NoError(t, err)

	// No SignalProcess expectation: background processes are
	// spared; the reconciler must resolve detached without dialing.
	r := executionReconciler{
		store:  db,
		logger: slogtest.Make(t, nil),
		agentConn: func(_ context.Context, _ uuid.UUID) (workspacesdk.AgentConn, func(), error) {
			t.Fatal("background rows must not dial the agent")
			return nil, nil, xerrors.New("unreachable")
		},
	}
	r.reconcileCancelRequested(ctx, record)

	row, err := db.GetChatToolCallExecution(ctx, database.GetChatToolCallExecutionParams{
		ChatID:             chat.ID,
		AssistantMessageID: msg.ID,
		ToolCallID:         "bg-call",
	})
	require.NoError(t, err)
	require.Equal(t, database.ChatToolCallExecutionStatusDetached, row.Status)
	require.Equal(t, "proc-bg", row.ProcessID.String)
}
