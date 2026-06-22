package chaterror_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/x/chatd/chaterror"
	"github.com/coder/coder/v2/codersdk"
)

// TestTerminalMessage covers the per-provider "temporarily
// unavailable" copy, the stream-silence timeout copy, and the generic
// fallback string for its intended (unclassified, non-retryable)
// path.
func TestTerminalMessage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		kind       codersdk.ChatErrorKind
		provider   string
		retryable  bool
		statusCode int
		want       string
	}{
		{
			name:      "Timeout_Retryable_Anthropic",
			kind:      codersdk.ChatErrorKindTimeout,
			provider:  "anthropic",
			retryable: true,
			want:      "Anthropic is temporarily unavailable.",
		},
		{
			name:      "Timeout_Retryable_OpenAI",
			kind:      codersdk.ChatErrorKindTimeout,
			provider:  "openai",
			retryable: true,
			want:      "OpenAI is temporarily unavailable.",
		},
		{
			name:      "Timeout_Retryable_UnknownProvider",
			kind:      codersdk.ChatErrorKindTimeout,
			provider:  "",
			retryable: true,
			want:      "The AI provider is temporarily unavailable.",
		},
		{
			name:      "Timeout_NotRetryable_NoStatus",
			kind:      codersdk.ChatErrorKindTimeout,
			provider:  "",
			retryable: false,
			want:      "The request timed out before it completed.",
		},
		{
			name:      "StreamSilenceTimeout_Anthropic",
			kind:      codersdk.ChatErrorKindStreamSilenceTimeout,
			provider:  "anthropic",
			retryable: true,
			want:      "Anthropic did not send response data in time.",
		},
		{
			name:      "StreamSilenceTimeout_OpenAI",
			kind:      codersdk.ChatErrorKindStreamSilenceTimeout,
			provider:  "openai",
			retryable: true,
			want:      "OpenAI did not send response data in time.",
		},
		{
			// Generic fallback reserved for genuinely
			// unclassified non-retryable failures.
			name:      "Generic_NotRetryable_NoStatus",
			kind:      codersdk.ChatErrorKindGeneric,
			provider:  "",
			retryable: false,
			want:      "The chat request failed unexpectedly.",
		},
		{
			name:      "UsageLimit_OpenAI",
			kind:      codersdk.ChatErrorKindUsageLimit,
			provider:  "openai",
			retryable: false,
			want:      "The usage quota for OpenAI has been exceeded. Check the billing and quota settings for the provider account.",
		},
		{
			name:      "UsageLimit_UnknownProvider",
			kind:      codersdk.ChatErrorKindUsageLimit,
			provider:  "",
			retryable: false,
			want:      "The usage quota for the AI provider has been exceeded. Check the billing and quota settings for the provider account.",
		},
		{
			name:      "MissingKey",
			kind:      codersdk.ChatErrorKindMissingKey,
			provider:  "",
			retryable: false,
			want:      "This conversation was started with an API key that is no longer available. Send your message again to continue.",
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
