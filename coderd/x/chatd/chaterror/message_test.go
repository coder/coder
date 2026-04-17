package chaterror_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/x/chatd/chaterror"
)

// TestTerminalMessage covers the per-provider "temporarily
// unavailable" copy, the startup-timeout copy, and the generic
// fallback string for its intended (unclassified, non-retryable)
// path.
func TestTerminalMessage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		kind       string
		provider   string
		retryable  bool
		statusCode int
		want       string
	}{
		{
			name:      "Timeout_Retryable_Anthropic",
			kind:      chaterror.KindTimeout,
			provider:  "anthropic",
			retryable: true,
			want:      "Anthropic is temporarily unavailable.",
		},
		{
			name:      "Timeout_Retryable_OpenAI",
			kind:      chaterror.KindTimeout,
			provider:  "openai",
			retryable: true,
			want:      "OpenAI is temporarily unavailable.",
		},
		{
			name:      "Timeout_Retryable_UnknownProvider",
			kind:      chaterror.KindTimeout,
			provider:  "",
			retryable: true,
			want:      "The AI provider is temporarily unavailable.",
		},
		{
			name:      "Timeout_NotRetryable_NoStatus",
			kind:      chaterror.KindTimeout,
			provider:  "",
			retryable: false,
			want:      "The request timed out before it completed.",
		},
		{
			name:      "StartupTimeout_Anthropic",
			kind:      chaterror.KindStartupTimeout,
			provider:  "anthropic",
			retryable: true,
			want:      "Anthropic did not start responding in time.",
		},
		{
			name:      "StartupTimeout_OpenAI",
			kind:      chaterror.KindStartupTimeout,
			provider:  "openai",
			retryable: true,
			want:      "OpenAI did not start responding in time.",
		},
		{
			// Generic fallback reserved for genuinely
			// unclassified non-retryable failures.
			name:      "Generic_NotRetryable_NoStatus",
			kind:      chaterror.KindGeneric,
			provider:  "",
			retryable: false,
			want:      "The chat request failed unexpectedly.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			classified := chaterror.ClassifiedError{
				Kind:       tt.kind,
				Provider:   tt.provider,
				Retryable:  tt.retryable,
				StatusCode: tt.statusCode,
			}
			// terminalMessage is unexported; round-trip through
			// WithClassification + Classify to exercise it.
			wrapped := chaterror.WithClassification(
				xerrors.New(tt.name),
				classified,
			)
			require.Equal(t, tt.want, chaterror.Classify(wrapped).Message)
		})
	}
}
