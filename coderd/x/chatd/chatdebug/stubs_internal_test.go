package chatdebug

import (
	"context"
	"net/http"
	"testing"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestBeginStep_SkipsNilRunID(t *testing.T) {
	t.Parallel()

	ctx := ContextWithRun(context.Background(), &RunContext{ChatID: uuid.New()})
	handle, enriched := beginStep(ctx, &Service{}, RecorderOptions{ChatID: uuid.New()}, OperationGenerate, nil)
	require.Nil(t, handle)
	require.Equal(t, ctx, enriched)
}

func TestNewStepHandle_SkipsNilRunID(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	handle, enriched := newStepHandle(ctx, &RunContext{ChatID: uuid.New()}, RecorderOptions{ChatID: uuid.New()}, OperationGenerate)
	require.Nil(t, handle)
	require.Equal(t, ctx, enriched)
}

func TestTruncateLabel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		input  string
		maxLen int
		want   string
	}{
		{name: "Empty", input: "", maxLen: 10, want: ""},
		{name: "WhitespaceOnly", input: "  \t\n  ", maxLen: 10, want: ""},
		{name: "ShortText", input: "hello world", maxLen: 20, want: "hello world"},
		{name: "ExactLength", input: "abcde", maxLen: 5, want: "abcde"},
		{name: "LongTextTruncated", input: "abcdefghij", maxLen: 5, want: "abcd…"},
		{name: "NegativeMaxLen", input: "hello", maxLen: -1, want: ""},
		{name: "ZeroMaxLen", input: "hello", maxLen: 0, want: ""},
		{name: "SingleRuneLimit", input: "hello", maxLen: 1, want: "…"},
		{name: "MultipleWhitespaceRuns", input: "  hello   world  \t again  ", maxLen: 100, want: "hello world again"},
		{name: "UnicodeRunes", input: "こんにちは世界", maxLen: 3, want: "こん…"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := TruncateLabel(tc.input, tc.maxLen)
			require.Equal(t, tc.want, got)
			require.LessOrEqual(t, utf8.RuneCountInString(got), max(tc.maxLen, 0))
		})
	}
}

// RedactedValue replaces sensitive values in debug payloads.
const RedactedValue = "[REDACTED]"

// RecordingTransport is the branch-02 placeholder HTTP recording transport.
type RecordingTransport struct {
	Base http.RoundTripper
}

var _ http.RoundTripper = (*RecordingTransport)(nil)

func (t *RecordingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req == nil {
		panic("chatdebug: nil request")
	}

	base := t.Base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(req)
}
