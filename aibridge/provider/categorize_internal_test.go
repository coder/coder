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
		{"server error", &intercept.ResponseError{StatusCode: 500}, ptr(recorder.ErrorTypeServerError)},
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
