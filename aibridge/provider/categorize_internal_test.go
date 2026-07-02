package provider

import (
	"testing"

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
