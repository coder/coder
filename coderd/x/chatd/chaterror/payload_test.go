package chaterror_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/x/chatd/chaterror"
	"github.com/coder/coder/v2/codersdk"
)

func TestStreamErrorPayloadUsesNormalizedClassification(t *testing.T) {
	t.Parallel()

	classified := chaterror.Classify(
		xerrors.New("azure openai received status 429 from upstream"),
	)
	payload := chaterror.StreamErrorPayload(classified)

	require.Equal(t, &codersdk.ChatStreamError{
		Message:    "Azure OpenAI is rate limiting requests (HTTP 429).",
		Kind:       chaterror.KindRateLimit,
		Provider:   "azure",
		Retryable:  true,
		StatusCode: 429,
	}, payload)
}

func TestStreamErrorPayloadIncludesProviderDetail(t *testing.T) {
	t.Parallel()

	payload := chaterror.StreamErrorPayload(chaterror.Classify(testProviderError(
		"",
		400,
		nil,
		testProviderResponseDump(`{"error":{"message":"Image exceeds 5 MB maximum."}}`),
	)))

	require.Equal(t, "Image exceeds 5 MB maximum.", payload.Detail)
}

func TestStreamErrorPayloadNilForEmptyClassification(t *testing.T) {
	t.Parallel()

	require.Nil(t, chaterror.StreamErrorPayload(chaterror.ClassifiedError{}))
}

func TestStreamRetryPayloadUsesNormalizedClassification(t *testing.T) {
	t.Parallel()

	delay := 3 * time.Second
	startedAt := time.Now()
	payload := chaterror.StreamRetryPayload(2, delay, chaterror.ClassifiedError{
		Message:    "OpenAI returned an unexpected error (HTTP 503).",
		Kind:       chaterror.KindGeneric,
		Provider:   "openai",
		Retryable:  true,
		StatusCode: 503,
	})

	require.NotNil(t, payload)
	require.Equal(t, 2, payload.Attempt)
	require.Equal(t, delay.Milliseconds(), payload.DelayMs)
	// Retry messages omit the HTTP status code; the status code is
	// surfaced separately in the payload's StatusCode field.
	require.Equal(t, "OpenAI returned an unexpected error.", payload.Error)
	require.Equal(t, chaterror.KindGeneric, payload.Kind)
	require.Equal(t, "openai", payload.Provider)
	require.Equal(t, 503, payload.StatusCode)
	require.WithinDuration(t, startedAt.Add(delay), payload.RetryingAt, time.Second)
}
