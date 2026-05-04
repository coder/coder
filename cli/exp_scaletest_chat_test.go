//go:build !slim

//nolint:testpackage // Tests cover an unexported validation helper.
package cli

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/scaletest/chat"
)

func TestValidateChatToolCallConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		llmMockURL       string
		turns            int64
		toolCallsPerChat int64
		wantErr          string
	}{
		{
			name:             "Disabled",
			turns:            10,
			toolCallsPerChat: 0,
		},
		{
			name:             "Enabled",
			llmMockURL:       "http://127.0.0.1:8080/v1",
			turns:            10,
			toolCallsPerChat: 5,
		},
		{
			name:             "RequiresMockURL",
			turns:            10,
			toolCallsPerChat: 1,
			wantErr:          "--tool-calls-per-chat requires --llm-mock-url",
		},
		{
			name:             "RejectsNegativeValues",
			turns:            10,
			toolCallsPerChat: -1,
			wantErr:          "--tool-calls-per-chat must not be negative",
		},
		{
			name:             "RejectsTooManyToolCalls",
			llmMockURL:       "http://127.0.0.1:8080/v1",
			turns:            1,
			toolCallsPerChat: int64(chat.MaxToolCallStepsPerTurn) + 1,
			wantErr:          "--tool-calls-per-chat must be at most",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := validateChatToolCallConfig(tt.llmMockURL, tt.turns, tt.toolCallsPerChat)
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tt.wantErr)
		})
	}
}
