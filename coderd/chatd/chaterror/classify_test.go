package chaterror_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/chatd/chaterror"
)

func TestClassify(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want chaterror.ClassifiedError
	}{
		{
			name: "AmbiguousOverloadKeepsProviderUnknown",
			err:  xerrors.New("status 529 from upstream"),
			want: chaterror.ClassifiedError{
				Message:    "The AI provider is temporarily overloaded (HTTP 529). Please try again later.",
				Kind:       chaterror.KindOverloaded,
				Provider:   "",
				Retryable:  true,
				StatusCode: 529,
			},
		},
		{
			name: "ExplicitAnthropicOverload",
			err:  xerrors.New("anthropic overloaded_error"),
			want: chaterror.ClassifiedError{
				Message:    "Anthropic is temporarily overloaded (HTTP 529). Please try again later.",
				Kind:       chaterror.KindOverloaded,
				Provider:   "anthropic",
				Retryable:  true,
				StatusCode: 529,
			},
		},
		{
			name: "AuthBeatsConfig",
			err:  xerrors.New("authentication failed: invalid model"),
			want: chaterror.ClassifiedError{
				Message:    "Authentication with the AI provider failed. Check the API key, permissions, and billing settings.",
				Kind:       chaterror.KindAuth,
				Provider:   "",
				Retryable:  false,
				StatusCode: 0,
			},
		},
		{
			name: "PureConfig",
			err:  xerrors.New("invalid model"),
			want: chaterror.ClassifiedError{
				Message:    "The AI provider rejected the model configuration. Check the selected model and provider settings.",
				Kind:       chaterror.KindConfig,
				Provider:   "",
				Retryable:  false,
				StatusCode: 0,
			},
		},
		{
			name: "ForbiddenContextLengthClassifiesAsAuth",
			err:  xerrors.New("forbidden: context length exceeded"),
			want: chaterror.ClassifiedError{
				Message:    "Authentication with the AI provider failed. Check the API key, permissions, and billing settings.",
				Kind:       chaterror.KindAuth,
				Provider:   "",
				Retryable:  false,
				StatusCode: 403,
			},
		},
		{
			name: "DeadlineExceededStaysNonRetryableTimeout",
			err:  context.DeadlineExceeded,
			want: chaterror.ClassifiedError{
				Message:    "The request timed out before it completed. Please try again.",
				Kind:       chaterror.KindTimeout,
				Provider:   "",
				Retryable:  false,
				StatusCode: 0,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, chaterror.Classify(tt.err))
		})
	}
}

func TestClassify_StartupTimeoutWrappedClassificationWins(t *testing.T) {
	t.Parallel()

	wrapped := chaterror.WithClassification(
		xerrors.New("context canceled"),
		chaterror.ClassifiedError{
			Kind:      chaterror.KindStartupTimeout,
			Provider:  "openai",
			Retryable: true,
		},
	)

	require.Equal(t, chaterror.ClassifiedError{
		Message:    "OpenAI did not start responding in time. Please try again.",
		Kind:       chaterror.KindStartupTimeout,
		Provider:   "openai",
		Retryable:  true,
		StatusCode: 0,
	}, chaterror.Classify(wrapped))
}

func TestWithProviderUsesExplicitHint(t *testing.T) {
	t.Parallel()

	classified := chaterror.Classify(xerrors.New("openai received status 429 from upstream"))
	require.Equal(t, "openai", classified.Provider)

	enriched := chaterror.WithProvider(classified, "azure openai")
	require.Equal(t, chaterror.ClassifiedError{
		Message:    "Azure OpenAI is rate limiting requests (HTTP 429). Please try again later.",
		Kind:       chaterror.KindRateLimit,
		Provider:   "azure",
		Retryable:  true,
		StatusCode: 429,
	}, enriched)
}

func TestWithProviderAddsProviderWhenUnknown(t *testing.T) {
	t.Parallel()

	classified := chaterror.Classify(xerrors.New("received status 429 from upstream"))
	require.Empty(t, classified.Provider)

	enriched := classified.WithProvider("openai")
	require.Equal(t, chaterror.ClassifiedError{
		Message:    "OpenAI is rate limiting requests (HTTP 429). Please try again later.",
		Kind:       chaterror.KindRateLimit,
		Provider:   "openai",
		Retryable:  true,
		StatusCode: 429,
	}, enriched)
}
