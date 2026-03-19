package chaterror

import (
	"strings"
	"time"

	"github.com/coder/coder/v2/codersdk"
)

func StreamErrorPayload(classified ClassifiedError) *codersdk.ChatStreamError {
	classified.Message = strings.TrimSpace(classified.Message)
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
	classified.Message = strings.TrimSpace(classified.Message)
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
