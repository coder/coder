package chaterror_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/chatd/chaterror"
	"github.com/coder/coder/v2/codersdk"
)

func TestStreamErrorPayloadNormalizesClassification(t *testing.T) {
	t.Parallel()

	payload := chaterror.StreamErrorPayload(chaterror.ClassifiedError{
		Kind:       chaterror.KindRateLimit,
		Provider:   " Azure OpenAI ",
		Retryable:  true,
		StatusCode: 429,
	})

	require.Equal(t, &codersdk.ChatStreamError{
		Message:    "Azure OpenAI is rate limiting requests (HTTP 429). Please try again later.",
		Kind:       chaterror.KindRateLimit,
		Provider:   "azure",
		Retryable:  true,
		StatusCode: 429,
	}, payload)
}

func TestStreamErrorPayloadNilForEmptyClassification(t *testing.T) {
	t.Parallel()

	require.Nil(t, chaterror.StreamErrorPayload(chaterror.ClassifiedError{}))
}

func TestStreamRetryPayloadNormalizesClassification(t *testing.T) {
	t.Parallel()

	delay := 3 * time.Second
	startedAt := time.Now()
	payload := chaterror.StreamRetryPayload(2, delay, chaterror.ClassifiedError{
		Message:    "  retry me  ",
		Provider:   " OpenAI ",
		Retryable:  true,
		StatusCode: 503,
	})

	require.NotNil(t, payload)
	require.Equal(t, 2, payload.Attempt)
	require.Equal(t, delay.Milliseconds(), payload.DelayMs)
	require.Equal(t, "retry me", payload.Error)
	require.Equal(t, chaterror.KindGeneric, payload.Kind)
	require.Equal(t, "openai", payload.Provider)
	require.True(t, payload.Retryable)
	require.Equal(t, 503, payload.StatusCode)
	require.WithinDuration(t, startedAt.Add(delay), payload.RetryingAt, time.Second)
}
