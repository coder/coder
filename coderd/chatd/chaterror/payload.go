package chaterror

import (
	"time"

	"github.com/coder/coder/v2/codersdk"
)

func StreamErrorPayload(classified ClassifiedError) *codersdk.ChatStreamError {
	classified = normalizeClassification(classified)
	if classified.Message == "" {
		return nil
	}
	return &codersdk.ChatStreamError{
		Message:    classified.Message,
		Kind:       classified.Kind,
		Provider:   classified.Provider,
		Retryable:  classified.Retryable,
		StatusCode: classified.StatusCode,
	}
}

func StreamRetryPayload(
	attempt int,
	delay time.Duration,
	classified ClassifiedError,
) *codersdk.ChatStreamRetry {
	classified = normalizeClassification(classified)
	if classified.Message == "" {
		return nil
	}
	return &codersdk.ChatStreamRetry{
		Attempt:    attempt,
		DelayMs:    delay.Milliseconds(),
		Error:      classified.Message,
		Kind:       classified.Kind,
		Provider:   classified.Provider,
		Retryable:  classified.Retryable,
		StatusCode: classified.StatusCode,
		RetryingAt: time.Now().Add(delay),
	}
}
