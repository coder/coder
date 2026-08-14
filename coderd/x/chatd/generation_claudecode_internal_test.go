package chatd

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	acp "github.com/coder/acp-go-sdk"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chaterror"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/claudecode"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func TestWaitForClaudeCodeAdapter(t *testing.T) {
	t.Parallel()

	adapterMissing := xerrors.New("exit status 1")

	t.Run("RetriesWhileScriptsRun", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		clock := quartz.NewMock(t)
		deadline := clock.Now("chatworker", "claudecode-readiness").Add(claudeCodeWorkspaceReadyTimeout)
		trap := clock.Trap().NewTimer("chatworker", "claudecode-preflight")
		defer trap.Close()

		probes := 0
		probe := func(context.Context) error {
			probes++
			if probes >= 3 {
				return nil
			}
			return adapterMissing
		}

		done := make(chan error, 1)
		go func() {
			done <- waitForClaudeCodeAdapter(ctx, clock, deadline, probe,
				func(context.Context) bool { return false })
		}()

		for range 2 {
			call := trap.MustWait(ctx)
			call.MustRelease(ctx)
			clock.Advance(claudeCodeWorkspacePollInterval).MustWait(ctx)
		}
		require.NoError(t, testutil.RequireReceive(ctx, t, done))
		require.Equal(t, 3, probes)
	})

	t.Run("SettledScriptsFailImmediately", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		clock := quartz.NewMock(t)
		deadline := clock.Now("chatworker", "claudecode-readiness").Add(claudeCodeWorkspaceReadyTimeout)

		probes := 0
		err := waitForClaudeCodeAdapter(ctx, clock, deadline,
			func(context.Context) error { probes++; return adapterMissing },
			func(context.Context) bool { return true })
		require.Error(t, err)
		require.Equal(t, 1, probes)
		classified := chaterror.Classify(err)
		require.Equal(t, codersdk.ChatErrorKindConfig, classified.Kind)
	})

	t.Run("DeadlineBoundsUnsettledScripts", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		clock := quartz.NewMock(t)
		deadline := clock.Now("chatworker", "claudecode-readiness").Add(claudeCodeWorkspaceReadyTimeout)
		trap := clock.Trap().NewTimer("chatworker", "claudecode-preflight")
		defer trap.Close()

		done := make(chan error, 1)
		go func() {
			done <- waitForClaudeCodeAdapter(ctx, clock, deadline,
				func(context.Context) error { return adapterMissing },
				func(context.Context) bool { return false })
		}()

		elapsed := time.Duration(0)
		for elapsed < claudeCodeWorkspaceReadyTimeout {
			call := trap.MustWait(ctx)
			call.MustRelease(ctx)
			clock.Advance(claudeCodeWorkspacePollInterval).MustWait(ctx)
			elapsed += claudeCodeWorkspacePollInterval
		}
		err := testutil.RequireReceive(ctx, t, done)
		require.Error(t, err)
		classified := chaterror.Classify(err)
		require.Equal(t, codersdk.ChatErrorKindConfig, classified.Kind)
	})

	t.Run("ScriptsSettlingAfterFailedProbeReprobes", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		clock := quartz.NewMock(t)
		deadline := clock.Now("chatworker", "claudecode-readiness").Add(claudeCodeWorkspaceReadyTimeout)
		trap := clock.Trap().NewTimer("chatworker", "claudecode-preflight")
		defer trap.Close()

		probes := 0
		probe := func(context.Context) error {
			probes++
			if probes >= 2 {
				return nil
			}
			return adapterMissing
		}
		settledCalls := 0
		settled := func(context.Context) bool {
			settledCalls++
			return settledCalls > 1
		}

		done := make(chan error, 1)
		go func() {
			done <- waitForClaudeCodeAdapter(ctx, clock, deadline, probe, settled)
		}()

		call := trap.MustWait(ctx)
		call.MustRelease(ctx)
		clock.Advance(claudeCodeWorkspacePollInterval).MustWait(ctx)
		require.NoError(t, testutil.RequireReceive(ctx, t, done))
		require.Equal(t, 2, probes)
	})
}

func claudeCodeTextMessage(t *testing.T, id int64, role database.ChatMessageRole, text string) database.ChatMessage {
	t.Helper()
	content, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{
		codersdk.ChatMessageText(text),
	})
	require.NoError(t, err)
	return database.ChatMessage{
		ID:             id,
		Role:           role,
		Content:        content,
		ContentVersion: chatprompt.CurrentContentVersion,
	}
}

func TestClaudeCodeTurnFromHistory(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	logger := slog.Logger{}

	t.Run("SingleUserMessage", func(t *testing.T) {
		t.Parallel()
		turn, err := claudeCodeTurnFromHistory(ctx, logger, []database.ChatMessage{
			claudeCodeTextMessage(t, 1, database.ChatMessageRoleUser, "hello"),
		})
		require.NoError(t, err)
		require.True(t, turn.generate)
		require.Equal(t, "hello", turn.prompt)
		require.Empty(t, turn.reseed)
	})

	t.Run("TrailingUserRunJoined", func(t *testing.T) {
		t.Parallel()
		turn, err := claudeCodeTurnFromHistory(ctx, logger, []database.ChatMessage{
			claudeCodeTextMessage(t, 1, database.ChatMessageRoleUser, "first"),
			claudeCodeTextMessage(t, 2, database.ChatMessageRoleAssistant, "reply"),
			claudeCodeTextMessage(t, 3, database.ChatMessageRoleUser, "second"),
			claudeCodeTextMessage(t, 4, database.ChatMessageRoleUser, "third"),
		})
		require.NoError(t, err)
		require.True(t, turn.generate)
		require.Equal(t, "second\n\nthird", turn.prompt)
		require.Len(t, turn.reseed, 2)
		require.Equal(t, "User", turn.reseed[0].Role)
		require.Equal(t, "first", turn.reseed[0].Text)
		require.Equal(t, "Assistant", turn.reseed[1].Role)
		require.Equal(t, "reply", turn.reseed[1].Text)
	})

	t.Run("HistoryEndsWithAssistant", func(t *testing.T) {
		t.Parallel()
		turn, err := claudeCodeTurnFromHistory(ctx, logger, []database.ChatMessage{
			claudeCodeTextMessage(t, 1, database.ChatMessageRoleUser, "hello"),
			claudeCodeTextMessage(t, 2, database.ChatMessageRoleAssistant, "done"),
		})
		require.NoError(t, err)
		require.False(t, turn.generate)
	})

	t.Run("EmptyHistory", func(t *testing.T) {
		t.Parallel()
		turn, err := claudeCodeTurnFromHistory(ctx, logger, nil)
		require.NoError(t, err)
		require.False(t, turn.generate)
	})

	t.Run("NonTextUserMessageErrors", func(t *testing.T) {
		t.Parallel()
		content, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{
			{Type: codersdk.ChatMessagePartTypeFile, FileName: "img.png", MediaType: "image/png"},
		})
		require.NoError(t, err)
		_, err = claudeCodeTurnFromHistory(ctx, logger, []database.ChatMessage{
			{
				ID:             1,
				Role:           database.ChatMessageRoleUser,
				Content:        content,
				ContentVersion: chatprompt.CurrentContentVersion,
			},
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "no text content")
	})
}

func TestClaudeCodeTurnUsage(t *testing.T) {
	t.Parallel()

	thought := func(v int) *int { return &v }

	t.Run("FreshSessionUsesRawCounts", func(t *testing.T) {
		t.Parallel()
		usage, totals := claudeCodeTurnUsage(claudecode.TurnOutcome{
			SessionID: "s1",
			Resumed:   false,
			Usage:     &acp.Usage{InputTokens: 100, OutputTokens: 40, TotalTokens: 140},
		}, claudecode.RuntimeState{})
		require.EqualValues(t, 100, usage.InputTokens)
		require.EqualValues(t, 40, usage.OutputTokens)
		require.EqualValues(t, 140, usage.TotalTokens)
		require.NotNil(t, totals)
		require.EqualValues(t, 140, totals.TotalTokens)
	})

	t.Run("ResumedSessionSubtractsPriorTotals", func(t *testing.T) {
		t.Parallel()
		usage, totals := claudeCodeTurnUsage(claudecode.TurnOutcome{
			SessionID: "s1",
			Resumed:   true,
			Usage:     &acp.Usage{InputTokens: 250, OutputTokens: 90, TotalTokens: 340, ThoughtTokens: thought(30)},
		}, claudecode.RuntimeState{
			SessionID: "s1",
			Usage: &claudecode.UsageTotals{
				InputTokens: 100, OutputTokens: 40, TotalTokens: 140, ReasoningTokens: 10,
			},
		})
		require.EqualValues(t, 150, usage.InputTokens)
		require.EqualValues(t, 50, usage.OutputTokens)
		require.EqualValues(t, 200, usage.TotalTokens)
		require.EqualValues(t, 20, usage.ReasoningTokens)
		require.EqualValues(t, 340, totals.TotalTokens)
	})

	t.Run("DifferentSessionSkipsSubtraction", func(t *testing.T) {
		t.Parallel()
		usage, _ := claudeCodeTurnUsage(claudecode.TurnOutcome{
			SessionID: "s2",
			Resumed:   false,
			Usage:     &acp.Usage{TotalTokens: 50},
		}, claudecode.RuntimeState{
			SessionID: "s1",
			Usage:     &claudecode.UsageTotals{TotalTokens: 140},
		})
		require.EqualValues(t, 50, usage.TotalTokens)
	})

	t.Run("CounterRestartFallsBackToRawCounts", func(t *testing.T) {
		t.Parallel()
		usage, totals := claudeCodeTurnUsage(claudecode.TurnOutcome{
			SessionID: "s1",
			Resumed:   true,
			Usage:     &acp.Usage{InputTokens: 20, OutputTokens: 5, TotalTokens: 25},
		}, claudecode.RuntimeState{
			SessionID: "s1",
			Usage:     &claudecode.UsageTotals{InputTokens: 100, OutputTokens: 40, TotalTokens: 140},
		})
		require.EqualValues(t, 25, usage.TotalTokens)
		require.EqualValues(t, 25, totals.TotalTokens)
	})

	t.Run("NoUsageCarriesPriorTotalsForward", func(t *testing.T) {
		t.Parallel()
		prior := &claudecode.UsageTotals{TotalTokens: 140}
		usage, totals := claudeCodeTurnUsage(claudecode.TurnOutcome{
			SessionID: "s1",
			Resumed:   true,
		}, claudecode.RuntimeState{SessionID: "s1", Usage: prior})
		require.Zero(t, usage.TotalTokens)
		require.Equal(t, prior, totals)

		_, totals = claudeCodeTurnUsage(claudecode.TurnOutcome{
			SessionID: "s2",
			Resumed:   false,
		}, claudecode.RuntimeState{SessionID: "s1", Usage: prior})
		require.Nil(t, totals)
	})
}
