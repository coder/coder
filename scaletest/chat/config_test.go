package chat_test

import (
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/scaletest/chat"
	"github.com/coder/coder/v2/scaletest/chatcontrol"
	"github.com/coder/coder/v2/scaletest/workspacebuild"
)

func newConfig(t *testing.T) chat.Config {
	t.Helper()
	reg := prometheus.NewRegistry()
	return chat.Config{
		RunID:             "run-123",
		WorkspaceID:       uuid.MustParse("11111111-1111-1111-1111-111111111111"),
		Prompt:            "Reply with one short sentence.",
		Turns:             2,
		FollowUpPrompt:    "Continue.",
		ReadyWaitGroup:    &sync.WaitGroup{},
		StartChan:         make(chan struct{}),
		Metrics:           chat.NewMetrics(reg, chat.MetricLabelNames()...),
		MetricLabelValues: chat.MetricLabelValues("run-123"),
	}
}

func TestConfigPromptForTurn(t *testing.T) {
	t.Parallel()

	t.Run("NoToolCallsUsesPlainPrompt", func(t *testing.T) {
		t.Parallel()
		cfg := newConfig(t)

		prompt, err := cfg.PromptForTurn(0)
		require.NoError(t, err)
		require.Equal(t, cfg.Prompt, prompt)

		followUpPrompt, err := cfg.PromptForTurn(1)
		require.NoError(t, err)
		require.Equal(t, cfg.FollowUpPrompt, followUpPrompt)
	})

	t.Run("ZeroToolTurnSkipsControlPrefix", func(t *testing.T) {
		t.Parallel()
		cfg := newConfig(t)
		cfg.Turns = 5
		cfg.ToolCallsPerChat = 3
		cfg.ToolCallSeed = 1
		cfg.ToolCallTool = chatcontrol.DefaultToolName
		cfg.ToolCallCommand = chatcontrol.DefaultToolCommand

		toolCallsByTurn := chatcontrol.ToolCallsByTurn(cfg.Turns, cfg.ToolCallsPerChat, cfg.ToolCallSeed)
		turnIndex := -1
		for i, toolCalls := range toolCallsByTurn {
			if toolCalls == 0 {
				turnIndex = i
				break
			}
		}
		require.NotEqual(t, -1, turnIndex)

		turnPrompt, err := cfg.PromptForTurn(turnIndex)
		require.NoError(t, err)
		expectedPrompt := cfg.FollowUpPrompt
		if turnIndex == 0 {
			expectedPrompt = cfg.Prompt
		}
		require.Equal(t, expectedPrompt, turnPrompt)
	})

	t.Run("ToolCallTurnAddsControlPrefix", func(t *testing.T) {
		t.Parallel()
		cfg := newConfig(t)
		cfg.Turns = 5
		cfg.ToolCallsPerChat = 3
		cfg.ToolCallSeed = 1
		cfg.ToolCallTool = chatcontrol.DefaultToolName
		cfg.ToolCallCommand = chatcontrol.DefaultToolCommand

		toolCallsByTurn := chatcontrol.ToolCallsByTurn(cfg.Turns, cfg.ToolCallsPerChat, cfg.ToolCallSeed)
		turnIndex := -1
		var expectedToolCalls int
		for i, toolCalls := range toolCallsByTurn {
			if toolCalls > 0 {
				turnIndex = i
				expectedToolCalls = toolCalls
				break
			}
		}
		require.NotEqual(t, -1, turnIndex)

		turnPrompt, err := cfg.PromptForTurn(turnIndex)
		require.NoError(t, err)
		control, stripped, found, err := chatcontrol.ParsePrompt(turnPrompt)
		require.NoError(t, err)
		require.True(t, found)
		expectedPrompt := cfg.FollowUpPrompt
		if turnIndex == 0 {
			expectedPrompt = cfg.Prompt
		}
		require.Equal(t, expectedPrompt, stripped)
		require.Equal(t, expectedToolCalls, control.ToolCallsThisTurn)
		require.Equal(t, chatcontrol.DefaultToolName, control.Tool)
		require.Equal(t, chatcontrol.DefaultToolCommand, control.Command)
	})
}

func TestConfigValidate(t *testing.T) {
	t.Parallel()

	t.Run("ValidSharedWorkspace", func(t *testing.T) {
		t.Parallel()
		cfg := newConfig(t)
		require.NoError(t, cfg.Validate())
	})

	t.Run("ValidTemplateWorkspace", func(t *testing.T) {
		t.Parallel()
		cfg := newConfig(t)
		cfg.WorkspaceID = uuid.Nil
		cfg.Workspace = workspacebuild.Config{
			OrganizationID: uuid.MustParse("33333333-3333-3333-3333-333333333333"),
			UserID:         codersdk.Me,
			Request: codersdk.CreateWorkspaceRequest{
				TemplateID: uuid.MustParse("44444444-4444-4444-4444-444444444444"),
			},
		}
		require.NoError(t, cfg.Validate())
	})

	t.Run("WorkspaceSelectionIsMutuallyExclusive", func(t *testing.T) {
		t.Parallel()
		cfg := newConfig(t)
		cfg.Workspace = workspacebuild.Config{
			OrganizationID: uuid.MustParse("33333333-3333-3333-3333-333333333333"),
			UserID:         codersdk.Me,
			Request: codersdk.CreateWorkspaceRequest{
				TemplateID: uuid.MustParse("44444444-4444-4444-4444-444444444444"),
			},
		}
		require.ErrorContains(t, cfg.Validate(), "exactly one of workspace_id or workspace config")
	})

	t.Run("ToolCallsPerChatAccepted", func(t *testing.T) {
		t.Parallel()
		cfg := newConfig(t)
		cfg.ToolCallsPerChat = 3
		cfg.ToolCallSeed = 42
		cfg.ToolCallCommand = "echo scaletest"
		require.NoError(t, cfg.Validate())
	})

	t.Run("NegativeToolCallsRejected", func(t *testing.T) {
		t.Parallel()
		cfg := newConfig(t)
		cfg.ToolCallsPerChat = -1
		require.ErrorContains(t, cfg.Validate(), "tool_calls_per_chat")
	})

	t.Run("TooManyToolCallsRejected", func(t *testing.T) {
		t.Parallel()
		cfg := newConfig(t)
		cfg.ToolCallsPerChat = cfg.Turns*chat.MaxToolCallStepsPerTurn + 1
		require.ErrorContains(t, cfg.Validate(), "must be at most")
	})

	t.Run("DelayedFollowUpsRequireBarrier", func(t *testing.T) {
		t.Parallel()
		cfg := newConfig(t)
		cfg.FollowUpStartDelay = 10 * time.Second
		require.ErrorContains(t, cfg.Validate(), "follow_up_ready_wait_group")

		cfg.FollowUpReadyWaitGroup = &sync.WaitGroup{}
		require.ErrorContains(t, cfg.Validate(), "start_follow_up_chan")

		cfg.StartFollowUpChan = make(chan struct{})
		require.NoError(t, cfg.Validate())
	})

	t.Run("NegativeFollowUpDelayRejected", func(t *testing.T) {
		t.Parallel()
		cfg := newConfig(t)
		cfg.FollowUpStartDelay = -time.Second
		require.ErrorContains(t, cfg.Validate(), "must not be negative")
	})
}
