package chaterror_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/x/chatd/chaterror"
)

// TestTerminalMessage locks in R3 of CODAGT-212: the per-provider
// "temporarily unavailable" copy is produced for Kind=KindTimeout with
// Retryable=true, across multiple providers. Also guards the generic
// fallback string for the path it is actually supposed to apply to
// (unclassified, non-retryable failures) and the startup-timeout copy
// (important because CODAGT-212's retry attempts were correctly
// classified as KindStartupTimeout; only the terminal attempt was
// misclassified).
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
			// This is the exact string the customer saw in
			// CODAGT-212. Keep it locked in for the path it is
			// actually supposed to apply to: genuinely
			// unclassified, non-retryable failures. The fix is
			// that a transport error was reaching this branch
			// when it should have been classified as KindTimeout.
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
			// terminalMessage is unexported; exercise it via
			// WithClassification + Classify, which round-trips
			// through normalizeClassification and populates the
			// Message field by calling terminalMessage when no
			// explicit Message is supplied.
			wrapped := chaterror.WithClassification(
				errString(tt.name),
				classified,
			)
			require.Equal(t, tt.want, chaterror.Classify(wrapped).Message)
		})
	}
}

// errString returns an error whose Error() is the given string, so
// tests can verify that WithClassification's wrapped error preserves
// the cause without depending on the underlying error text.
func errString(s string) error { return stringError(s) }

type stringError string

func (e stringError) Error() string { return string(e) }
