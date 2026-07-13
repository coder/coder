package aibridge

import (
	"context"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/aibridge/circuitbreaker"
	"github.com/coder/coder/v2/aibridge/keypool"
	"github.com/coder/coder/v2/aibridge/recorder"
)

// stubCategorizer is a test errorCategorizer standing in for a provider.
type stubCategorizer struct {
	result *recorder.ErrorType
}

func (s stubCategorizer) CategorizeError(error) *recorder.ErrorType {
	return s.result
}

func ptr(t recorder.ErrorType) *recorder.ErrorType { return &t }

func TestCategorizeInterceptionError(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		cat      stubCategorizer
		err      error
		wantType recorder.ErrorType
		wantMsg  string
	}{
		{
			name:     "nil success",
			err:      nil,
			wantType: "",
			wantMsg:  "",
		},
		{
			name:     "circuit open maps to server error",
			err:      circuitbreaker.ErrCircuitOpen,
			wantType: recorder.ErrorTypeServerError,
			wantMsg:  circuitbreaker.ErrCircuitOpen.Error(),
		},
		{
			name:     "context deadline is timeout",
			err:      context.DeadlineExceeded,
			wantType: recorder.ErrorTypeTimeout,
			wantMsg:  context.DeadlineExceeded.Error(),
		},
		{
			name:     "keypool permanent is unauthorized",
			err:      &keypool.Error{Kind: keypool.ErrorKindPermanent},
			wantType: recorder.ErrorTypeUnauthorized,
			wantMsg:  (&keypool.Error{Kind: keypool.ErrorKindPermanent}).Error(),
		},
		{
			name:     "keypool rate limited is rate limited",
			err:      &keypool.Error{Kind: keypool.ErrorKindRateLimited},
			wantType: recorder.ErrorTypeRateLimited,
			wantMsg:  (&keypool.Error{Kind: keypool.ErrorKindRateLimited}).Error(),
		},
		{
			name:     "keypool unrecognized kind is unknown",
			err:      &keypool.Error{Kind: keypool.ErrorKind(-1)},
			wantType: recorder.ErrorTypeUnknown,
			wantMsg:  (&keypool.Error{Kind: keypool.ErrorKind(-1)}).Error(),
		},
		{
			name:     "context canceled is unknown",
			err:      context.Canceled,
			wantType: recorder.ErrorTypeUnknown,
			wantMsg:  context.Canceled.Error(),
		},
		{
			name:     "wrapped keypool error is unwrapped",
			err:      xerrors.Errorf("key pool exhausted: %w", &keypool.Error{Kind: keypool.ErrorKindPermanent}),
			wantType: recorder.ErrorTypeUnauthorized,
			wantMsg:  "key pool exhausted: all configured keys failed authentication",
		},
		{
			name:     "delegated to provider",
			cat:      stubCategorizer{result: ptr(recorder.ErrorTypeOverloaded)},
			err:      xerrors.New("provider error"),
			wantType: recorder.ErrorTypeOverloaded,
			wantMsg:  "provider error",
		},
		{
			name:     "provider does not recognize the error",
			err:      xerrors.New("mystery"),
			wantType: recorder.ErrorTypeUnknown,
			wantMsg:  "mystery",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			gotType, gotMsg := categorizeInterceptionError(tc.cat, tc.err)
			assert.Equal(t, tc.wantType, gotType)
			assert.Equal(t, tc.wantMsg, gotMsg)
		})
	}
}

func TestCategorizeInterceptionErrorTruncatesMessage(t *testing.T) {
	t.Parallel()

	// ASCII: truncated exactly at the byte cap.
	ascii := strings.Repeat("a", maxRecordedErrorMessageBytes*2)
	_, gotMsg := categorizeInterceptionError(stubCategorizer{}, xerrors.New(ascii))
	assert.Len(t, gotMsg, maxRecordedErrorMessageBytes)

	// Multi-byte: the '€' rune (3 bytes) split at the cap is dropped, leaving
	// valid UTF-8 just below the cap rather than an invalid trailing fragment.
	multibyte := strings.Repeat("€", maxRecordedErrorMessageBytes)
	_, gotMsg = categorizeInterceptionError(stubCategorizer{}, xerrors.New(multibyte))
	assert.True(t, utf8.ValidString(gotMsg), "truncated message must stay valid UTF-8")
	assert.Less(t, len(gotMsg), maxRecordedErrorMessageBytes)
	assert.Positive(t, len(gotMsg))
}
