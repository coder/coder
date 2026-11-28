package provider

import (
	"testing"

	"github.com/anthropics/anthropic-sdk-go/shared"
	"github.com/stretchr/testify/assert"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/aibridge/intercept"
	"github.com/coder/coder/v2/aibridge/intercept/messages"
	"github.com/coder/coder/v2/aibridge/recorder"
)

func ptr(t recorder.ErrorType) *recorder.ErrorType { return &t }

func TestAnthropicCategorizeError(t *testing.T) {
	t.Parallel()

	p := &Anthropic{}
	envelope := func(status int, typ string) *messages.ResponseError {
		return &messages.ResponseError{
			StatusCode:    status,
			ErrorResponse: &shared.ErrorResponse{Error: shared.ErrorObjectUnion{Type: typ}},
		}
	}
	cases := []struct {
		name string
		err  error
		want *recorder.ErrorType
	}{
		{"overloaded", &messages.ResponseError{StatusCode: statusOverloaded}, ptr(recorder.ErrorTypeOverloaded)},
		{"unauthorized", &messages.ResponseError{StatusCode: 401}, ptr(recorder.ErrorTypeUnauthorized)},
		{"bad request", &messages.ResponseError{StatusCode: 400}, ptr(recorder.ErrorTypeBadRequest)},
		{"not found is bad request", &messages.ResponseError{StatusCode: 404}, ptr(recorder.ErrorTypeBadRequest)},
		{"payload too large is bad request", &messages.ResponseError{StatusCode: 413}, ptr(recorder.ErrorTypeBadRequest)},
		{"timeout", &messages.ResponseError{StatusCode: 408}, ptr(recorder.ErrorTypeTimeout)},
		{"server error", &messages.ResponseError{StatusCode: 503}, ptr(recorder.ErrorTypeServerError)},
		{"status takes precedence", envelope(429, "overloaded_error"), ptr(recorder.ErrorTypeRateLimited)},
		{"invalid request event", envelope(0, "invalid_request_error"), ptr(recorder.ErrorTypeBadRequest)},
		{"not found event", envelope(0, "not_found_error"), ptr(recorder.ErrorTypeBadRequest)},
		{"request too large event", envelope(0, "request_too_large"), ptr(recorder.ErrorTypeBadRequest)},
		{"authentication event", envelope(0, "authentication_error"), ptr(recorder.ErrorTypeUnauthorized)},
		{"permission event", envelope(0, "permission_error"), ptr(recorder.ErrorTypeUnauthorized)},
		{"rate limit event", envelope(0, "rate_limit_error"), ptr(recorder.ErrorTypeRateLimited)},
		{"timeout event", envelope(0, "timeout_error"), ptr(recorder.ErrorTypeTimeout)},
		{"API error event", envelope(0, "api_error"), ptr(recorder.ErrorTypeServerError)},
		{"overloaded event", envelope(0, "overloaded_error"), ptr(recorder.ErrorTypeOverloaded)},
		{"unrecognized event error type", envelope(0, "future_error"), ptr(recorder.ErrorTypeUnknown)},
		{"not this provider", xerrors.New("mystery"), nil},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, p.CategorizeError(tc.err))
		})
	}
}

func TestCopilotCategorizeError(t *testing.T) {
	t.Parallel()

	// Copilot serves both OpenAI-compatible routes and an Anthropic-style
	// /v1/messages route, so it tries the OpenAI shapes first and falls back to
	// the Anthropic shapes.
	p := &Copilot{}
	cases := []struct {
		name string
		err  error
		want *recorder.ErrorType
	}{
		// OpenAI envelope is categorized via the OpenAI path.
		{"openai envelope unauthorized", &intercept.ResponseError{StatusCode: 401}, ptr(recorder.ErrorTypeUnauthorized)},
		// A 529 in the OpenAI envelope is a generic 5xx (the OpenAI path has no
		// "overloaded" notion), which proves the OpenAI path wins first.
		{"openai envelope 529 is server error", &intercept.ResponseError{StatusCode: statusOverloaded}, ptr(recorder.ErrorTypeServerError)},
		// Anthropic envelope falls through to the Anthropic path, where 529 is
		// "overloaded".
		{"anthropic envelope overloaded", &messages.ResponseError{StatusCode: statusOverloaded}, ptr(recorder.ErrorTypeOverloaded)},
		{"anthropic envelope bad request", &messages.ResponseError{StatusCode: 400}, ptr(recorder.ErrorTypeBadRequest)},
		{"neither provider", xerrors.New("mystery"), nil},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, p.CategorizeError(tc.err))
		})
	}
}

func TestOpenAICategorizeError(t *testing.T) {
	t.Parallel()

	p := &OpenAI{}
	cases := []struct {
		name string
		err  error
		want *recorder.ErrorType
	}{
		{"rate limited", &intercept.ResponseError{StatusCode: 429}, ptr(recorder.ErrorTypeRateLimited)},
		{"unauthorized", &intercept.ResponseError{StatusCode: 403}, ptr(recorder.ErrorTypeUnauthorized)},
		{"not found is bad request", &intercept.ResponseError{StatusCode: 404}, ptr(recorder.ErrorTypeBadRequest)},
		{"unprocessable entity is bad request", &intercept.ResponseError{StatusCode: 422}, ptr(recorder.ErrorTypeBadRequest)},
		{"timeout", &intercept.ResponseError{StatusCode: 408}, ptr(recorder.ErrorTypeTimeout)},
		{"server error", &intercept.ResponseError{StatusCode: 500}, ptr(recorder.ErrorTypeServerError)},
		// OpenAI returns 503 when its engine is overloaded.
		{"503 is overloaded", &intercept.ResponseError{StatusCode: 503}, ptr(recorder.ErrorTypeOverloaded)},
		// Anthropic's 529 is just another 5xx for OpenAI, not "overloaded".
		{"529 is a generic server error", &intercept.ResponseError{StatusCode: statusOverloaded}, ptr(recorder.ErrorTypeServerError)},
		{"not this provider", xerrors.New("mystery"), nil},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, p.CategorizeError(tc.err))
		})
	}
}
