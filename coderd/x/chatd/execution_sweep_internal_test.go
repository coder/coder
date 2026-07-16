package chatd //nolint:testpackage // Tests the unexported execution sweep reconciler.

import (
	"context"
	"database/sql"
	"io"
	"net/http"
	"net/url"
	"strings"
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
	"github.com/coder/coder/v2/codersdk"
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
// row in cancel_requested carrying full process identity, claimed
// claimAgo ago and canceled cancelAgo ago. result_committed_at is
// stamped at the cancellation like both production mappings, so the
// suite runs the shape the interrupt commit and the transition
// mapping actually create.
func seedCancelRequestedExecution(ctx context.Context, t *testing.T, db database.Store, claimAgo, cancelAgo time.Duration) database.ChatToolCallExecution {
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

	claimPast := dbtime.Now().Add(-claimAgo)
	cancelPast := dbtime.Now().Add(-cancelAgo)
	claimed, err := db.ClaimChatToolCallExecution(ctx, database.ClaimChatToolCallExecutionParams{
		ID:                 uuid.New(),
		ChatID:             chat.ID,
		AssistantMessageID: msg.ID,
		ToolCallID:         "sweep-call",
		InputSha256:        "hash",
		Command:            "sleep 600",
		TimeoutSecs:        600,
		Now:                claimPast,
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
		StartedAt:          claimPast,
		UpdatedAt:          claimPast,
	})
	require.NoError(t, err)
	rows, err := db.MarkChatToolCallExecutionsInterrupted(ctx, database.MarkChatToolCallExecutionsInterruptedParams{
		ChatID:             chat.ID,
		AssistantMessageID: msg.ID,
		ToolCallIds:        []string{"sweep-call"},
		SpareBackground:    true,
		UpdatedAt:          cancelPast,
	})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, database.ChatToolCallExecutionStatusCancelRequested, rows[0].Status)
	err = db.MarkChatToolCallExecutionsResultCommitted(ctx, database.MarkChatToolCallExecutionsResultCommittedParams{
		ChatID:             chat.ID,
		AssistantMessageID: msg.ID,
		ToolCallIds:        []string{"sweep-call"},
		ResultCommittedAt:  cancelPast,
	})
	require.NoError(t, err)
	record, err := db.GetChatToolCallExecution(ctx, database.GetChatToolCallExecutionParams{
		ChatID:             chat.ID,
		AssistantMessageID: msg.ID,
		ToolCallID:         "sweep-call",
	})
	require.NoError(t, err)
	return record
}

func TestExecutionSweep(t *testing.T) {
	t.Parallel()

	t.Run("KillsStalledCancelRequested", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitLong)
		record := seedCancelRequestedExecution(ctx, t, db, 10*time.Minute, 10*time.Minute)

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
		record := seedCancelRequestedExecution(ctx, t, db, 10*time.Minute, 10*time.Minute)

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

	t.Run("HandleLessRowResolvesUnknown", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitLong)

		// A crash between the interrupt commit and reconciliation
		// leaves a cancel_requested row with no process identity.
		// The grace window since its claim has long closed, so the
		// sweep resolves it unknown without dialing.
		user := dbgen.User(t, db, database.User{})
		org := dbgen.Organization(t, db, database.Organization{})
		_ = dbgen.OrganizationMember(t, db, database.OrganizationMember{UserID: user.ID, OrganizationID: org.ID})
		_ = dbgen.ChatProvider(t, db, database.ChatProvider{Provider: "openai", DisplayName: "OpenAI"})
		mc := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{Model: "test-model", ContextLimit: 8192})
		chat := dbgen.Chat(t, db, database.Chat{
			OrganizationID:    org.ID,
			OwnerID:           user.ID,
			LastModelConfigID: mc.ID,
			Title:             "sweep-handle-less",
		})
		msg := dbgen.ChatMessage(t, db, database.ChatMessage{
			ChatID: chat.ID,
			Role:   database.ChatMessageRoleAssistant,
		})
		past := dbtime.Now().Add(-10 * time.Minute)
		_, err := db.ClaimChatToolCallExecution(ctx, database.ClaimChatToolCallExecutionParams{
			ID:                 uuid.New(),
			ChatID:             chat.ID,
			AssistantMessageID: msg.ID,
			ToolCallID:         "handle-less-call",
			InputSha256:        "hash",
			Command:            "sleep 600",
			TimeoutSecs:        600,
			Now:                past,
			StaleBefore:        time.Time{},
		})
		require.NoError(t, err)
		rows, err := db.MarkChatToolCallExecutionsInterrupted(ctx, database.MarkChatToolCallExecutionsInterruptedParams{
			ChatID:             chat.ID,
			AssistantMessageID: msg.ID,
			ToolCallIds:        []string{"handle-less-call"},
			SpareBackground:    true,
			UpdatedAt:          past,
		})
		require.NoError(t, err)
		require.Len(t, rows, 1)
		require.False(t, rows[0].ProcessID.Valid)

		r := executionReconciler{
			store:  db,
			logger: slogtest.Make(t, nil),
			agentConn: func(_ context.Context, _ uuid.UUID) (workspacesdk.AgentConn, func(), error) {
				return nil, nil, xerrors.New("should not dial")
			},
		}
		r.sweepOnce(ctx, dbtime.Now())

		row, err := db.GetChatToolCallExecution(ctx, database.GetChatToolCallExecutionParams{
			ChatID:             chat.ID,
			AssistantMessageID: msg.ID,
			ToolCallID:         "handle-less-call",
		})
		require.NoError(t, err)
		require.Equal(t, database.ChatToolCallExecutionStatusUnknown, row.Status)
	})

	t.Run("LateHandleBlocksUnknownResolution", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitLong)
		// The row carries full process identity, but the
		// reconciler decided unknown from a stale snapshot taken
		// before the handle landed. The guarded write must lose
		// and the kill flow must run on the fresh identity.
		record := seedCancelRequestedExecution(ctx, t, db, time.Minute, time.Minute)

		ctrl := gomock.NewController(t)
		conn := agentconnmock.NewMockAgentConn(ctrl)
		conn.EXPECT().SignalProcess(gomock.Any(), "proc-sweep", "kill").Return(nil)
		exitCode := -1
		conn.EXPECT().ProcessOutput(gomock.Any(), "proc-sweep", gomock.Any()).
			Return(workspacesdk.ProcessOutputResponse{Running: false, ExitCode: &exitCode}, nil)

		r := executionReconciler{
			store:  db,
			logger: slogtest.Make(t, nil),
			agentConn: func(_ context.Context, _ uuid.UUID) (workspacesdk.AgentConn, func(), error) {
				return conn, nil, nil
			},
		}
		stale := record
		stale.ProcessID = sql.NullString{}
		stale.WorkspaceAgentID = uuid.NullUUID{}
		r.resolveCancelOutcomeWithoutProcess(ctx, stale, database.ChatToolCallExecutionStatusUnknown)

		row, err := db.GetChatToolCallExecution(ctx, database.GetChatToolCallExecutionParams{
			ChatID:             record.ChatID,
			AssistantMessageID: record.AssistantMessageID,
			ToolCallID:         record.ToolCallID,
		})
		require.NoError(t, err)
		require.Equal(t, database.ChatToolCallExecutionStatusCanceled, row.Status)
		require.Equal(t, "proc-sweep", row.ProcessID.String)
	})

	t.Run("ClaimSkipsFreshAndResolvedRows", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitLong)
		record := seedCancelRequestedExecution(ctx, t, db, time.Minute, time.Minute)

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

	t.Run("GiveUpAfterCancelAgeResolvesUnknown", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitLong)
		record := seedCancelRequestedExecution(ctx, t, db, executionCancelGiveUpAfter+time.Hour, executionCancelGiveUpAfter+time.Hour)

		// The kill has been unconfirmable for over a day; the row
		// must terminalize without another dial.
		r := executionReconciler{
			store:  db,
			logger: slogtest.Make(t, nil),
			agentConn: func(_ context.Context, _ uuid.UUID) (workspacesdk.AgentConn, func(), error) {
				t.Fatal("an expired cancel must not dial the agent")
				return nil, nil, nil
			},
		}
		r.sweepOnce(ctx, dbtime.Now())

		row, err := db.GetChatToolCallExecution(ctx, database.GetChatToolCallExecutionParams{
			ChatID:             record.ChatID,
			AssistantMessageID: record.AssistantMessageID,
			ToolCallID:         record.ToolCallID,
		})
		require.NoError(t, err)
		require.Equal(t, database.ChatToolCallExecutionStatusUnknown, row.Status)
	})

	t.Run("EditOfOldDetachedRowStillKills", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitLong)
		// A background row detached (with result_committed_at
		// stamped) over a give-up window ago, then edited away.
		record := seedCancelRequestedExecution(ctx, t, db, executionCancelGiveUpAfter+2*time.Hour, executionCancelGiveUpAfter+time.Hour)
		_, err := db.UpdateChatToolCallExecutionCancelOutcome(ctx, database.UpdateChatToolCallExecutionCancelOutcomeParams{
			ChatID:             record.ChatID,
			AssistantMessageID: record.AssistantMessageID,
			ToolCallID:         record.ToolCallID,
			Status:             database.ChatToolCallExecutionStatusDetached,
			CancelSignalSentAt: sql.NullTime{},
			UpdatedAt:          dbtime.Now().Add(-executionCancelGiveUpAfter - time.Hour),
		})
		require.NoError(t, err)
		mapped, err := db.MarkChatToolCallExecutionsCancelRequestedForHistoryDelete(ctx, database.MarkChatToolCallExecutionsCancelRequestedForHistoryDeleteParams{
			ChatID:        record.ChatID,
			FromMessageID: record.AssistantMessageID,
			UpdatedAt:     dbtime.Now(),
		})
		require.NoError(t, err)
		require.Len(t, mapped, 1)

		// The history-delete mapping restarts the give-up clock, so
		// the sweep must attempt the kill instead of resolving
		// unknown with zero attempts.
		ctrl := gomock.NewController(t)
		conn := agentconnmock.NewMockAgentConn(ctrl)
		conn.EXPECT().SignalProcess(gomock.Any(), "proc-sweep", "kill").Return(nil)
		exitCode := -1
		conn.EXPECT().ProcessOutput(gomock.Any(), "proc-sweep", gomock.Any()).
			Return(workspacesdk.ProcessOutputResponse{Running: false, ExitCode: &exitCode}, nil)
		r := executionReconciler{
			store:  db,
			logger: slogtest.Make(t, nil),
			agentConn: func(_ context.Context, _ uuid.UUID) (workspacesdk.AgentConn, func(), error) {
				return conn, func() {}, nil
			},
		}
		r.reconcile(ctx, mapped[0])

		row, err := db.GetChatToolCallExecution(ctx, database.GetChatToolCallExecutionParams{
			ChatID:             record.ChatID,
			AssistantMessageID: record.AssistantMessageID,
			ToolCallID:         record.ToolCallID,
		})
		require.NoError(t, err)
		require.Equal(t, database.ChatToolCallExecutionStatusCanceled, row.Status)
	})

	t.Run("OldClaimFreshCancelStillKills", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitLong)
		// The give-up bound anchors on the cancellation commit's
		// result_committed_at, so a call claimed over a day ago
		// that was interrupted just now still gets its kill.
		record := seedCancelRequestedExecution(ctx, t, db, executionCancelGiveUpAfter+time.Hour, 0)

		ctrl := gomock.NewController(t)
		conn := agentconnmock.NewMockAgentConn(ctrl)
		conn.EXPECT().SignalProcess(gomock.Any(), "proc-sweep", "kill").Return(nil)
		exitCode := -1
		conn.EXPECT().ProcessOutput(gomock.Any(), "proc-sweep", gomock.Any()).
			Return(workspacesdk.ProcessOutputResponse{Running: false, ExitCode: &exitCode}, nil)
		r := executionReconciler{
			store:  db,
			logger: slogtest.Make(t, nil),
			agentConn: func(_ context.Context, _ uuid.UUID) (workspacesdk.AgentConn, func(), error) {
				return conn, func() {}, nil
			},
		}
		r.reconcile(ctx, record)

		row, err := db.GetChatToolCallExecution(ctx, database.GetChatToolCallExecutionParams{
			ChatID:             record.ChatID,
			AssistantMessageID: record.AssistantMessageID,
			ToolCallID:         record.ToolCallID,
		})
		require.NoError(t, err)
		require.Equal(t, database.ChatToolCallExecutionStatusCanceled, row.Status)
	})

	t.Run("AgentRowGoneResolvesCanceled", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitLong)
		record := seedCancelRequestedExecution(ctx, t, db, 10*time.Minute, 10*time.Minute)

		ws, err := db.GetWorkspaceByAgentID(ctx, record.WorkspaceAgentID.UUID)
		require.NoError(t, err)
		require.NoError(t, db.SoftDeleteWorkspaceAgentsByWorkspaceID(ctx, ws.ID))

		// The agent row is gone, so its workspace pod (and the
		// process) died with it; dialing would fail every sweep
		// forever.
		r := executionReconciler{
			store:  db,
			logger: slogtest.Make(t, nil),
			agentConn: func(_ context.Context, _ uuid.UUID) (workspacesdk.AgentConn, func(), error) {
				t.Fatal("a deleted agent must not be dialed")
				return nil, nil, nil
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
	})

	t.Run("AgentFKNulledResolvesCanceled", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitLong)
		// Canceled past the give-up bound: the agent-gone check
		// must still win, so a provably dead process records
		// canceled rather than unknown.
		record := seedCancelRequestedExecution(ctx, t, db, executionCancelGiveUpAfter+time.Hour, executionCancelGiveUpAfter+time.Hour)

		// Simulate the workspace_agents FK having nulled the agent
		// on hard delete while the process handle stays recorded.
		// Neither the kill flow nor the missing-process guard can
		// resolve this shape.
		record.WorkspaceAgentID = uuid.NullUUID{}
		r := executionReconciler{
			store:  db,
			logger: slogtest.Make(t, nil),
			agentConn: func(_ context.Context, _ uuid.UUID) (workspacesdk.AgentConn, func(), error) {
				t.Fatal("a row without an agent must not be dialed")
				return nil, nil, nil
			},
		}
		r.reconcile(ctx, record)

		row, err := db.GetChatToolCallExecution(ctx, database.GetChatToolCallExecutionParams{
			ChatID:             record.ChatID,
			AssistantMessageID: record.AssistantMessageID,
			ToolCallID:         record.ToolCallID,
		})
		require.NoError(t, err)
		require.Equal(t, database.ChatToolCallExecutionStatusCanceled, row.Status)
	})
}

// TestReconcileCancelRequestedLateBackgroundKills asserts that a
// background execution whose handle landed only after the interrupt
// commit is killed: its committed result carries no handle, so the
// process would otherwise keep running unaddressable.
func TestReconcileCancelRequestedLateBackgroundKills(t *testing.T) {
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
		SpareBackground:    true,
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
		UpdatedAt:          dbtime.Now(),
	})
	require.NoError(t, err)

	record, err := db.GetChatToolCallExecution(ctx, database.GetChatToolCallExecutionParams{
		ChatID:             chat.ID,
		AssistantMessageID: msg.ID,
		ToolCallID:         "bg-call",
	})
	require.NoError(t, err)

	// The committed synthetic result carries no handle, so the
	// late-identified background process must be killed, not
	// spared as detached.
	ctrl := gomock.NewController(t)
	conn := agentconnmock.NewMockAgentConn(ctrl)
	conn.EXPECT().SignalProcess(gomock.Any(), "proc-bg", "kill").Return(nil)
	exitCode := -1
	conn.EXPECT().ProcessOutput(gomock.Any(), "proc-bg", gomock.Any()).
		Return(workspacesdk.ProcessOutputResponse{Running: false, ExitCode: &exitCode}, nil)
	r := executionReconciler{
		store:  db,
		logger: slogtest.Make(t, nil),
		agentConn: func(_ context.Context, agentID uuid.UUID) (workspacesdk.AgentConn, func(), error) {
			require.Equal(t, agent.ID, agentID)
			return conn, func() {}, nil
		},
	}
	r.reconcileCancelRequested(ctx, record)

	row, err := db.GetChatToolCallExecution(ctx, database.GetChatToolCallExecutionParams{
		ChatID:             chat.ID,
		AssistantMessageID: msg.ID,
		ToolCallID:         "bg-call",
	})
	require.NoError(t, err)
	require.Equal(t, database.ChatToolCallExecutionStatusCanceled, row.Status)
	require.True(t, row.CancelSignalSentAt.Valid)
}

// seedTokenOnlyCancelRequested creates an execution row whose claim
// recorded the dispatch target but whose process handle never
// landed, then interrupts it: a cancel_requested row resolvable
// only through the agent's token index.
func seedTokenOnlyCancelRequested(ctx context.Context, t *testing.T, db database.Store, staleAgo time.Duration, background bool) database.ChatToolCallExecution {
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
		Title:             "sweep-token-test",
	})
	msg := dbgen.ChatMessage(t, db, database.ChatMessage{
		ChatID: chat.ID,
		Role:   database.ChatMessageRoleAssistant,
	})
	agent := seedSweepWorkspaceAgent(t, db, user.ID, org.ID)

	past := dbtime.Now().Add(-staleAgo)
	_, err := db.ClaimChatToolCallExecution(ctx, database.ClaimChatToolCallExecutionParams{
		ID:                 uuid.New(),
		ChatID:             chat.ID,
		AssistantMessageID: msg.ID,
		ToolCallID:         "token-call",
		InputSha256:        "hash",
		Command:            "sleep 600",
		Background:         background,
		TimeoutSecs:        600,
		WorkspaceAgentID:   uuid.NullUUID{UUID: agent.ID, Valid: true},
		Now:                past,
		StaleBefore:        time.Time{},
	})
	require.NoError(t, err)
	rows, err := db.MarkChatToolCallExecutionsInterrupted(ctx, database.MarkChatToolCallExecutionsInterruptedParams{
		ChatID:             chat.ID,
		AssistantMessageID: msg.ID,
		ToolCallIds:        []string{"token-call"},
		UpdatedAt:          past,
	})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, database.ChatToolCallExecutionStatusCancelRequested, rows[0].Status)
	require.False(t, rows[0].ProcessID.Valid)
	return rows[0]
}

// TestExecutionSweepTokenOnlyRows asserts that cancel_requested rows
// without recorded process identity are claimed by the sweep and
// resolved through the token probe, so a failed post-interrupt dial
// is retried instead of stranding the row forever.
func TestExecutionSweepTokenOnlyRows(t *testing.T) {
	t.Parallel()

	t.Run("DialFailureRetriedThenKilled", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitLong)
		record := seedTokenOnlyCancelRequested(ctx, t, db, 10*time.Minute, false)

		dialErr := xerrors.New("agent unreachable")
		r := executionReconciler{
			store:  db,
			logger: slogtest.Make(t, nil),
			agentConn: func(_ context.Context, _ uuid.UUID) (workspacesdk.AgentConn, func(), error) {
				return nil, nil, dialErr
			},
		}
		r.sweepOnce(ctx, dbtime.Now())

		row, err := db.GetChatToolCallExecution(ctx, database.GetChatToolCallExecutionParams{
			ChatID:             record.ChatID,
			AssistantMessageID: record.AssistantMessageID,
			ToolCallID:         record.ToolCallID,
		})
		require.NoError(t, err)
		require.Equal(t, database.ChatToolCallExecutionStatusCancelRequested, row.Status)
		require.True(t, row.UpdatedAt.After(record.UpdatedAt))

		// A later sweep probes the token, finds the process, and
		// kills it.
		ctrl := gomock.NewController(t)
		conn := agentconnmock.NewMockAgentConn(ctrl)
		conn.EXPECT().ProcessByToken(gomock.Any(), record.ID.String()).
			Return(workspacesdk.ProcessByTokenResponse{Found: true, ProcessID: "proc-token"}, nil)
		conn.EXPECT().SignalProcess(gomock.Any(), "proc-token", "kill").Return(nil)
		exitCode := -1
		conn.EXPECT().ProcessOutput(gomock.Any(), "proc-token", gomock.Any()).
			Return(workspacesdk.ProcessOutputResponse{Running: false, ExitCode: &exitCode}, nil)
		r.agentConn = func(_ context.Context, agentID uuid.UUID) (workspacesdk.AgentConn, func(), error) {
			require.Equal(t, record.WorkspaceAgentID.UUID, agentID)
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
		// The probed handle was adopted onto the row.
		require.Equal(t, "proc-token", row.ProcessID.String)
	})

	t.Run("FoundProcessAdoptedBeforeKill", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitLong)
		record := seedTokenOnlyCancelRequested(ctx, t, db, 10*time.Minute, false)

		// The kill fails transiently after the probe found the
		// process; the row must keep the adopted handle so the
		// sweep can retry through the durable identity.
		ctrl := gomock.NewController(t)
		conn := agentconnmock.NewMockAgentConn(ctrl)
		conn.EXPECT().ProcessByToken(gomock.Any(), record.ID.String()).
			Return(workspacesdk.ProcessByTokenResponse{Found: true, ProcessID: "proc-adopt"}, nil)
		conn.EXPECT().SignalProcess(gomock.Any(), "proc-adopt", "kill").
			Return(xerrors.New("transient signal failure"))
		r := executionReconciler{
			store:  db,
			logger: slogtest.Make(t, nil),
			agentConn: func(_ context.Context, _ uuid.UUID) (workspacesdk.AgentConn, func(), error) {
				return conn, func() {}, nil
			},
		}
		r.sweepOnce(ctx, dbtime.Now())

		row, err := db.GetChatToolCallExecution(ctx, database.GetChatToolCallExecutionParams{
			ChatID:             record.ChatID,
			AssistantMessageID: record.AssistantMessageID,
			ToolCallID:         record.ToolCallID,
		})
		require.NoError(t, err)
		require.Equal(t, database.ChatToolCallExecutionStatusCancelRequested, row.Status)
		require.Equal(t, "proc-adopt", row.ProcessID.String)
		require.True(t, row.WorkspaceAgentID.Valid)
		// started_at anchors the deadline at the claim time, but
		// the adoption must not rewind updated_at with it: that
		// would void the sweep lease and re-claim the row on every
		// sweep while the agent keeps failing.
		require.True(t, row.StartedAt.Time.Equal(record.ClaimedAt.Time))
		require.True(t, row.UpdatedAt.After(record.ClaimedAt.Time))
	})

	t.Run("AbsentTokenWithinDispatchGraceReprobed", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitLong)
		// A fresh claim: the interrupt landed before the
		// already-sent StartProcess reached the agent, so the
		// first probe legitimately sees no reservation. Absence
		// must not be trusted yet; a re-probe finds the process
		// that arrived moments later and kills it.
		record := seedTokenOnlyCancelRequested(ctx, t, db, 0, false)

		ctrl := gomock.NewController(t)
		conn := agentconnmock.NewMockAgentConn(ctrl)
		exitCode := -1
		gomock.InOrder(
			conn.EXPECT().ProcessByToken(gomock.Any(), record.ID.String()).
				Return(workspacesdk.ProcessByTokenResponse{Found: false, TokenIndexAgeMS: time.Hour.Milliseconds()}, nil),
			conn.EXPECT().ProcessByToken(gomock.Any(), record.ID.String()).
				Return(workspacesdk.ProcessByTokenResponse{Found: true, ProcessID: "proc-race"}, nil),
			conn.EXPECT().SignalProcess(gomock.Any(), "proc-race", "kill").Return(nil),
			conn.EXPECT().ProcessOutput(gomock.Any(), "proc-race", gomock.Any()).
				Return(workspacesdk.ProcessOutputResponse{Running: false, ExitCode: &exitCode}, nil),
		)
		r := executionReconciler{
			store:  db,
			logger: slogtest.Make(t, nil),
			agentConn: func(_ context.Context, _ uuid.UUID) (workspacesdk.AgentConn, func(), error) {
				return conn, func() {}, nil
			},
		}
		r.reconcileCancelRequestedByToken(ctx, record)

		row, err := db.GetChatToolCallExecution(ctx, database.GetChatToolCallExecutionParams{
			ChatID:             record.ChatID,
			AssistantMessageID: record.AssistantMessageID,
			ToolCallID:         record.ToolCallID,
		})
		require.NoError(t, err)
		require.Equal(t, database.ChatToolCallExecutionStatusCanceled, row.Status)
		require.Equal(t, "proc-race", row.ProcessID.String)
	})

	t.Run("NoProbeLateHandleKilledNotUnknown", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitLong)
		record := seedTokenOnlyCancelRequested(ctx, t, db, 10*time.Minute, false)

		// The handle lands after the no-probe fallback's last
		// identity poll: the reconciler still holds the stale
		// handle-less row. The guarded unknown write must lose to
		// the recorded identity and run the kill flow instead of
		// terminalizing a row whose process was never killed.
		_, err := db.UpdateChatToolCallExecutionProcess(ctx, database.UpdateChatToolCallExecutionProcessParams{
			ChatID:             record.ChatID,
			AssistantMessageID: record.AssistantMessageID,
			ToolCallID:         record.ToolCallID,
			ClaimEpoch:         record.ClaimEpoch,
			ProcessID:          "proc-late",
			WorkspaceAgentID:   record.WorkspaceAgentID.UUID,
			StartedAt:          dbtime.Now(),
			UpdatedAt:          dbtime.Now(),
		})
		require.NoError(t, err)

		ctrl := gomock.NewController(t)
		conn := agentconnmock.NewMockAgentConn(ctrl)
		exitCode := -1
		conn.EXPECT().SignalProcess(gomock.Any(), "proc-late", "kill").Return(nil)
		conn.EXPECT().ProcessOutput(gomock.Any(), "proc-late", gomock.Any()).
			Return(workspacesdk.ProcessOutputResponse{Running: false, ExitCode: &exitCode}, nil)
		r := executionReconciler{
			store:  db,
			logger: slogtest.Make(t, nil),
			agentConn: func(_ context.Context, _ uuid.UUID) (workspacesdk.AgentConn, func(), error) {
				return conn, func() {}, nil
			},
		}
		r.resolveInterruptedWithoutProbe(ctx, conn, record)

		row, err := db.GetChatToolCallExecution(ctx, database.GetChatToolCallExecutionParams{
			ChatID:             record.ChatID,
			AssistantMessageID: record.AssistantMessageID,
			ToolCallID:         record.ToolCallID,
		})
		require.NoError(t, err)
		require.Equal(t, database.ChatToolCallExecutionStatusCanceled, row.Status)
		require.Equal(t, "proc-late", row.ProcessID.String)
	})

	t.Run("ExitedProcessFoundByTokenCanceled", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitLong)
		record := seedTokenOnlyCancelRequested(ctx, t, db, 10*time.Minute, false)

		// The token index keeps exited processes until reaping,
		// and signaling one answers 409. That is a definitive
		// exit, so the row must resolve canceled instead of
		// staying cancel_requested for every sweep to re-signal.
		signalConflict := codersdk.ReadBodyAsError(&http.Response{
			StatusCode: http.StatusConflict,
			Request: &http.Request{
				Method: http.MethodPost,
				URL:    &url.URL{Path: "/api/v0/processes/proc-exited/signal"},
			},
			Body: io.NopCloser(strings.NewReader(`{"message":"Process is not running."}`)),
		})
		ctrl := gomock.NewController(t)
		conn := agentconnmock.NewMockAgentConn(ctrl)
		conn.EXPECT().ProcessByToken(gomock.Any(), record.ID.String()).
			Return(workspacesdk.ProcessByTokenResponse{Found: true, ProcessID: "proc-exited"}, nil)
		conn.EXPECT().SignalProcess(gomock.Any(), "proc-exited", "kill").
			Return(signalConflict)
		r := executionReconciler{
			store:  db,
			logger: slogtest.Make(t, nil),
			agentConn: func(_ context.Context, _ uuid.UUID) (workspacesdk.AgentConn, func(), error) {
				return conn, func() {}, nil
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
		require.Equal(t, "proc-exited", row.ProcessID.String)
	})

	t.Run("BackgroundFoundByTokenKilled", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitLong)
		record := seedTokenOnlyCancelRequested(ctx, t, db, 10*time.Minute, true)

		// The committed synthetic result carries no handle, so a
		// background process found by token is adopted and killed,
		// not spared as detached.
		ctrl := gomock.NewController(t)
		conn := agentconnmock.NewMockAgentConn(ctrl)
		conn.EXPECT().ProcessByToken(gomock.Any(), record.ID.String()).
			Return(workspacesdk.ProcessByTokenResponse{Found: true, ProcessID: "proc-bg"}, nil)
		conn.EXPECT().SignalProcess(gomock.Any(), "proc-bg", "kill").Return(nil)
		exitCode := -1
		conn.EXPECT().ProcessOutput(gomock.Any(), "proc-bg", gomock.Any()).
			Return(workspacesdk.ProcessOutputResponse{Running: false, ExitCode: &exitCode}, nil)
		r := executionReconciler{
			store:  db,
			logger: slogtest.Make(t, nil),
			agentConn: func(_ context.Context, _ uuid.UUID) (workspacesdk.AgentConn, func(), error) {
				return conn, func() {}, nil
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
		require.Equal(t, "proc-bg", row.ProcessID.String)
	})
}
