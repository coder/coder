package chaterror

import (
	"time"

	"github.com/coder/coder/v2/codersdk"
)

func StreamErrorPayload(classified ClassifiedError) *codersdk.ChatStreamError {
	if classified.Message == "" {
		return nil
	}
	return &codersdk.ChatStreamError{
		Message:    classified.Message,
		Detail:     classified.Detail,
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
	if classified.Message == "" {
		return nil
	}
	return &codersdk.ChatStreamRetry{
		Attempt:    attempt,
		DelayMs:    delay.Milliseconds(),
		Error:      retryMessage(classified),
		Kind:       classified.Kind,
		Provider:   classified.Provider,
		StatusCode: classified.StatusCode,
		RetryingAt: time.Now().Add(delay),
	}
}
