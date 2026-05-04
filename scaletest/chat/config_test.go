package chat_test

import (
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/scaletest/chat"
	"github.com/coder/coder/v2/scaletest/chatcontrol"
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

	tests := []struct {
		name    string
		mutate  func(*chat.Config)
		wantErr string
	}{
		{
			name: "ValidSharedWorkspace",
		},
		{
			name: "TurnsMustBePositive",
			mutate: func(cfg *chat.Config) {
				cfg.Turns = 0
			},
			wantErr: "validate turns: must be at least 1",
		},
		{
			name: "NegativeToolCallsRejected",
			mutate: func(cfg *chat.Config) {
				cfg.ToolCallsPerChat = -1
			},
			wantErr: "validate tool_calls_per_chat: must not be negative",
		},
		{
			name: "TooManyToolCallsRejected",
			mutate: func(cfg *chat.Config) {
				cfg.ToolCallsPerChat = cfg.Turns*chat.MaxToolCallStepsPerTurn + 1
			},
			wantErr: "validate tool_calls_per_chat: must be at most 2398 for 2 turns",
		},
		{
			name: "ToolCallsRequireCommand",
			mutate: func(cfg *chat.Config) {
				cfg.ToolCallsPerChat = 1
				cfg.ToolCallCommand = ""
			},
			wantErr: "validate tool_call_command: must not be empty when tool_calls_per_chat > 0",
		},
		{
			name: "DelayedFollowUpsRequireBarrier",
			mutate: func(cfg *chat.Config) {
				cfg.FollowUpStartDelay = 10 * time.Second
			},
			wantErr: "validate follow_up_ready_wait_group: must not be nil when follow-up delay is enabled",
		},
		{
			name: "DelayedFollowUpsRequireStartChan",
			mutate: func(cfg *chat.Config) {
				cfg.FollowUpStartDelay = 10 * time.Second
				cfg.FollowUpReadyWaitGroup = &sync.WaitGroup{}
			},
			wantErr: "validate start_follow_up_chan: must not be nil when follow-up delay is enabled",
		},
		{
			name: "MetricsRequired",
			mutate: func(cfg *chat.Config) {
				cfg.Metrics = nil
			},
			wantErr: "validate metrics: must not be nil",
		},
		{
			name: "MetricLabelValuesCardinalityMismatch",
			mutate: func(cfg *chat.Config) {
				cfg.MetricLabelValues = nil
			},
			wantErr: "validate metric_label_values: got 0 values, want 1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := newConfig(t)
			if tt.mutate != nil {
				tt.mutate(&cfg)
			}

			err := cfg.Validate()
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}

			require.EqualError(t, err, tt.wantErr)
		})
	}
}
