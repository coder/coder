package chatd

import (
	"context"
	"database/sql"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
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

	t.Run("AbortedPassLeavesUnclaimedRowsEligible", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		baseCtx := testutil.Context(t, testutil.WaitLong)
		ctx, cancel := context.WithCancel(baseCtx)
		defer cancel()

		// Two stale rows; the pass dies on its first dial. The
		// second row must keep its old lease so the next sweep
		// claims it, instead of being leased upfront and starved.
		_ = seedCancelRequestedExecution(ctx, t, db, 20*time.Minute, 20*time.Minute)
		_ = seedCancelRequestedExecution(ctx, t, db, 10*time.Minute, 10*time.Minute)

		r := executionReconciler{
			store:  db,
			logger: slogtest.Make(t, nil),
			agentConn: func(_ context.Context, _ uuid.UUID) (workspacesdk.AgentConn, func(), error) {
				cancel()
				return nil, nil, xerrors.New("pass aborted")
			},
		}
		now := dbtime.Now()
		r.sweepOnce(ctx, now)

		remaining, err := db.ClaimStaleChatToolCallExecutionCancels(baseCtx, database.ClaimStaleChatToolCallExecutionCancelsParams{
			UpdatedBefore: now.Add(-executionSweepRetryAge),
			LimitCount:    10,
			Now:           dbtime.Now(),
		})
		require.NoError(t, err)
		require.Len(t, remaining, 1)
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

	t.Run("ExpiredCancelDeletedAgentResolvesCanceled", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitLong)
		// The kill has been unconfirmable past the give-up bound,
		// but the dispatch target is deleted: the process died
		// with its pod, so the outcome is a confirmed canceled,
		// not unknown.
		record := seedCancelRequestedExecution(ctx, t, db, executionCancelGiveUpAfter+2*time.Hour, executionCancelGiveUpAfter+time.Hour)
		ws, err := db.GetWorkspaceByAgentID(ctx, record.WorkspaceAgentID.UUID)
		require.NoError(t, err)
		require.NoError(t, db.SoftDeleteWorkspaceAgentsByWorkspaceID(ctx, ws.ID))

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
		require.Equal(t, database.ChatToolCallExecutionStatusCanceled, row.Status)
	})

	t.Run("EditMapsCommittedStartingClaim", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitLong)
		// A claim whose handle write never landed but whose tool
		// result committed stays starting; an edit deleting its
		// carrier must still route it to the cancel path.
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
		committedAt := dbtime.Now().Add(-time.Hour)
		_, err := db.ClaimChatToolCallExecution(ctx, database.ClaimChatToolCallExecutionParams{
			ID:                 uuid.New(),
			ChatID:             chat.ID,
			AssistantMessageID: msg.ID,
			ToolCallID:         "sweep-call",
			InputSha256:        "hash",
			Command:            "sleep 600",
			TimeoutSecs:        600,
			Now:                committedAt,
			StaleBefore:        time.Time{},
		})
		require.NoError(t, err)
		err = db.MarkChatToolCallExecutionsResultCommitted(ctx, database.MarkChatToolCallExecutionsResultCommittedParams{
			ChatID:             chat.ID,
			AssistantMessageID: msg.ID,
			ToolCallIds:        []string{"sweep-call"},
			ResultCommittedAt:  committedAt,
		})
		require.NoError(t, err)

		editAt := dbtime.Now()
		mapped, err := db.MarkChatToolCallExecutionsCancelRequestedForHistoryDelete(ctx, database.MarkChatToolCallExecutionsCancelRequestedForHistoryDeleteParams{
			ChatID:        chat.ID,
			FromMessageID: msg.ID,
			UpdatedAt:     editAt,
		})
		require.NoError(t, err)
		require.Len(t, mapped, 1)
		require.Equal(t, database.ChatToolCallExecutionStatusCancelRequested, mapped[0].Status)
		// The give-up clock restarts at the edit, not the old
		// result commit.
		require.True(t, mapped[0].ResultCommittedAt.Valid)
		require.True(t, mapped[0].ResultCommittedAt.Time.After(committedAt))
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

// TestReconcileCancelRequestedAlreadyExitedConfirmsCanceled asserts
// that the agent's 409 (process known but no longer running) counts
// as confirmed termination: the process died on its own, so the row
// must not stay cancel_requested until the give-up bound.
func TestReconcileCancelRequestedAlreadyExitedConfirmsCanceled(t *testing.T) {
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
		Title:             "exited-409-test",
	})
	msg := dbgen.ChatMessage(t, db, database.ChatMessage{ChatID: chat.ID, Role: database.ChatMessageRoleAssistant})
	agent := seedSweepWorkspaceAgent(t, db, user.ID, org.ID)

	claimed, err := db.ClaimChatToolCallExecution(ctx, database.ClaimChatToolCallExecutionParams{
		ID:                 uuid.New(),
		ChatID:             chat.ID,
		AssistantMessageID: msg.ID,
		ToolCallID:         "fg-call",
		InputSha256:        "hash",
		Command:            "echo done",
		TimeoutSecs:        600,
		Now:                dbtime.Now(),
		StaleBefore:        time.Time{},
	})
	require.NoError(t, err)
	_, err = db.UpdateChatToolCallExecutionProcess(ctx, database.UpdateChatToolCallExecutionProcessParams{
		ChatID:             chat.ID,
		AssistantMessageID: msg.ID,
		ToolCallID:         "fg-call",
		ClaimEpoch:         claimed.ClaimEpoch,
		ProcessID:          "proc-fg",
		WorkspaceAgentID:   agent.ID,
		StartedAt:          dbtime.Now(),
	})
	require.NoError(t, err)
	rows, err := db.MarkChatToolCallExecutionsInterrupted(ctx, database.MarkChatToolCallExecutionsInterruptedParams{
		ChatID:             chat.ID,
		AssistantMessageID: msg.ID,
		ToolCallIds:        []string{"fg-call"},
		UpdatedAt:          dbtime.Now(),
	})
	require.NoError(t, err)
	require.Len(t, rows, 1)

	record, err := db.GetChatToolCallExecution(ctx, database.GetChatToolCallExecutionParams{
		ChatID:             chat.ID,
		AssistantMessageID: msg.ID,
		ToolCallID:         "fg-call",
	})
	require.NoError(t, err)

	sigRes := &http.Response{
		StatusCode: http.StatusConflict,
		Request:    &http.Request{Method: http.MethodPost, URL: &url.URL{Path: "/api/v0/processes/proc-fg/signal"}},
		Body:       io.NopCloser(strings.NewReader(`{"message":"Process is not running."}`)),
	}
	ctrl := gomock.NewController(t)
	conn := agentconnmock.NewMockAgentConn(ctrl)
	conn.EXPECT().SignalProcess(gomock.Any(), "proc-fg", "kill").Return(codersdk.ReadBodyAsError(sigRes))
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
		ToolCallID:         "fg-call",
	})
	require.NoError(t, err)
	require.Equal(t, database.ChatToolCallExecutionStatusCanceled, row.Status)
	require.False(t, row.CancelSignalSentAt.Valid)
}

// lateHandleStore lands the execution's process handle on the first
// GetChatToolCallExecution poll, modeling a recordProcessStart write
// that arrives only after the reconciler snapshotted the row: only a
// wait loop that actually polls observes it.
type lateHandleStore struct {
	database.Store
	t          *testing.T
	claimEpoch int64
	agentID    uuid.UUID
	processID  string
	once       sync.Once
}

func (s *lateHandleStore) GetChatToolCallExecution(ctx context.Context, arg database.GetChatToolCallExecutionParams) (database.ChatToolCallExecution, error) {
	s.once.Do(func() {
		_, err := s.Store.UpdateChatToolCallExecutionProcess(ctx, database.UpdateChatToolCallExecutionProcessParams{
			ChatID:             arg.ChatID,
			AssistantMessageID: arg.AssistantMessageID,
			ToolCallID:         arg.ToolCallID,
			ClaimEpoch:         s.claimEpoch,
			ProcessID:          s.processID,
			WorkspaceAgentID:   s.agentID,
			StartedAt:          dbtime.Now(),
		})
		require.NoError(s.t, err)
	})
	return s.Store.GetChatToolCallExecution(ctx, arg)
}

// TestIdentityWaitAnchorsOnCancellationCommit asserts the
// late-handle wait runs from the cancellation commit when the claim
// is older: an interrupt near the end of the claim window must
// still wait out the uncanceled recordProcessStart write instead of
// terminalizing unknown and losing the handle to the status guard.
func TestIdentityWaitAnchorsOnCancellationCommit(t *testing.T) {
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
		Title:             "late-anchor-test",
	})
	msg := dbgen.ChatMessage(t, db, database.ChatMessage{ChatID: chat.ID, Role: database.ChatMessageRoleAssistant})
	agent := seedSweepWorkspaceAgent(t, db, user.ID, org.ID)

	// The claim is older than the record grace window; the interrupt
	// commit is fresh.
	claimed, err := db.ClaimChatToolCallExecution(ctx, database.ClaimChatToolCallExecutionParams{
		ID:                 uuid.New(),
		ChatID:             chat.ID,
		AssistantMessageID: msg.ID,
		ToolCallID:         "late-anchor-call",
		InputSha256:        "hash",
		Command:            "sleep 600",
		TimeoutSecs:        600,
		Now:                dbtime.Now().Add(-2 * interruptRecordGrace),
		StaleBefore:        time.Time{},
	})
	require.NoError(t, err)

	rows, err := db.MarkChatToolCallExecutionsInterrupted(ctx, database.MarkChatToolCallExecutionsInterruptedParams{
		ChatID:             chat.ID,
		AssistantMessageID: msg.ID,
		ToolCallIds:        []string{"late-anchor-call"},
		UpdatedAt:          dbtime.Now(),
	})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, database.ChatToolCallExecutionStatusCancelRequested, rows[0].Status)
	err = db.MarkChatToolCallExecutionsResultCommitted(ctx, database.MarkChatToolCallExecutionsResultCommittedParams{
		ChatID:             chat.ID,
		AssistantMessageID: msg.ID,
		ToolCallIds:        []string{"late-anchor-call"},
		ResultCommittedAt:  dbtime.Now(),
	})
	require.NoError(t, err)

	snapshot, err := db.GetChatToolCallExecution(ctx, database.GetChatToolCallExecutionParams{
		ChatID:             chat.ID,
		AssistantMessageID: msg.ID,
		ToolCallID:         "late-anchor-call",
	})
	require.NoError(t, err)
	require.False(t, snapshot.ProcessID.Valid)

	ctrl := gomock.NewController(t)
	conn := agentconnmock.NewMockAgentConn(ctrl)
	conn.EXPECT().SignalProcess(gomock.Any(), "proc-late", "kill").Return(nil)
	exitCode := -1
	conn.EXPECT().ProcessOutput(gomock.Any(), "proc-late", gomock.Any()).
		Return(workspacesdk.ProcessOutputResponse{Running: false, ExitCode: &exitCode}, nil)
	r := executionReconciler{
		store: &lateHandleStore{
			Store:      db,
			t:          t,
			claimEpoch: claimed.ClaimEpoch,
			agentID:    agent.ID,
			processID:  "proc-late",
		},
		logger: slogtest.Make(t, nil),
		agentConn: func(_ context.Context, agentID uuid.UUID) (workspacesdk.AgentConn, func(), error) {
			require.Equal(t, agent.ID, agentID)
			return conn, func() {}, nil
		},
	}
	r.resolveUnknownAfterIdentityWait(ctx, snapshot)

	row, err := db.GetChatToolCallExecution(ctx, database.GetChatToolCallExecutionParams{
		ChatID:             chat.ID,
		AssistantMessageID: msg.ID,
		ToolCallID:         "late-anchor-call",
	})
	require.NoError(t, err)
	require.Equal(t, database.ChatToolCallExecutionStatusCanceled, row.Status)
	require.True(t, row.CancelSignalSentAt.Valid)
}

// TestIdentityWaitResolvesCanceledWhenAgentRowGone asserts a late
// handle whose workspace agent row was deleted (the FK nulls
// workspace_agent_id) still resolves canceled: the row has a
// process ID, so the missing-process guard can never fire, and only
// reconcile's agent-gone branch can terminalize it.
func TestIdentityWaitResolvesCanceledWhenAgentRowGone(t *testing.T) {
	t.Parallel()
	db, _, sqlDB := dbtestutil.NewDBWithSQLDB(t)
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
		Title:             "agent-gone-test",
	})
	msg := dbgen.ChatMessage(t, db, database.ChatMessage{ChatID: chat.ID, Role: database.ChatMessageRoleAssistant})
	agent := seedSweepWorkspaceAgent(t, db, user.ID, org.ID)

	// Both anchors are older than the grace window, so the wait
	// returns immediately with the pre-handle snapshot.
	claimed, err := db.ClaimChatToolCallExecution(ctx, database.ClaimChatToolCallExecutionParams{
		ID:                 uuid.New(),
		ChatID:             chat.ID,
		AssistantMessageID: msg.ID,
		ToolCallID:         "agent-gone-call",
		InputSha256:        "hash",
		Command:            "sleep 600",
		TimeoutSecs:        600,
		Now:                dbtime.Now().Add(-2 * interruptRecordGrace),
		StaleBefore:        time.Time{},
	})
	require.NoError(t, err)
	rows, err := db.MarkChatToolCallExecutionsInterrupted(ctx, database.MarkChatToolCallExecutionsInterruptedParams{
		ChatID:             chat.ID,
		AssistantMessageID: msg.ID,
		ToolCallIds:        []string{"agent-gone-call"},
		UpdatedAt:          dbtime.Now(),
	})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	err = db.MarkChatToolCallExecutionsResultCommitted(ctx, database.MarkChatToolCallExecutionsResultCommittedParams{
		ChatID:             chat.ID,
		AssistantMessageID: msg.ID,
		ToolCallIds:        []string{"agent-gone-call"},
		ResultCommittedAt:  dbtime.Now().Add(-2 * interruptRecordGrace),
	})
	require.NoError(t, err)

	snapshot, err := db.GetChatToolCallExecution(ctx, database.GetChatToolCallExecutionParams{
		ChatID:             chat.ID,
		AssistantMessageID: msg.ID,
		ToolCallID:         "agent-gone-call",
	})
	require.NoError(t, err)
	require.False(t, snapshot.ProcessID.Valid)

	// The handle lands, then the agent row is deleted out from
	// under it.
	_, err = db.UpdateChatToolCallExecutionProcess(ctx, database.UpdateChatToolCallExecutionProcessParams{
		ChatID:             chat.ID,
		AssistantMessageID: msg.ID,
		ToolCallID:         "agent-gone-call",
		ClaimEpoch:         claimed.ClaimEpoch,
		ProcessID:          "proc-gone",
		WorkspaceAgentID:   agent.ID,
		StartedAt:          dbtime.Now(),
	})
	require.NoError(t, err)
	_, err = sqlDB.ExecContext(ctx, "DELETE FROM workspace_agents WHERE id = $1", agent.ID)
	require.NoError(t, err)

	ctrl := gomock.NewController(t)
	conn := agentconnmock.NewMockAgentConn(ctrl)
	r := executionReconciler{
		store:  db,
		logger: slogtest.Make(t, nil),
		agentConn: func(_ context.Context, _ uuid.UUID) (workspacesdk.AgentConn, func(), error) {
			t.Fatal("no dial expected: the agent row is gone")
			return conn, func() {}, nil
		},
	}
	r.resolveUnknownAfterIdentityWait(ctx, snapshot)

	row, err := db.GetChatToolCallExecution(ctx, database.GetChatToolCallExecutionParams{
		ChatID:             chat.ID,
		AssistantMessageID: msg.ID,
		ToolCallID:         "agent-gone-call",
	})
	require.NoError(t, err)
	require.Equal(t, database.ChatToolCallExecutionStatusCanceled, row.Status)
}
