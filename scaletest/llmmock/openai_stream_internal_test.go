package llmmock

import (
	"context"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/testutil"
)

func TestStreamWordChunks(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		content string
		want    []string
	}{
		{
			name:    "words",
			content: "hello world test",
			want:    []string{"hello ", "world ", "test"},
		},
		{
			name:    "trailing space",
			content: "hello world ",
			want:    []string{"hello ", "world "},
		},
		{
			name:    "single word",
			content: "single",
			want:    []string{"single"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tc.want, streamWordChunks(tc.content))
		})
	}
}

func TestStreamContentChunksWordSplit(t *testing.T) {
	t.Parallel()

	srv := &Server{}
	ctx := testutil.Context(t, testutil.WaitShort)
	got := slices.Collect(srv.streamContentChunks(ctx, time.Nanosecond, "hello world test"))
	require.Equal(t, []string{"hello ", "world ", "test"}, got)
}

func TestStreamContentChunksFixedWindow(t *testing.T) {
	t.Parallel()

	srv := &Server{responsePayloadSize: 1}
	ctx := testutil.Context(t, testutil.WaitShort)
	content := strings.Repeat("x", 2050)

	got := slices.Collect(srv.streamContentChunks(ctx, time.Nanosecond, content))
	require.Len(t, got, 3)
	require.Equal(t, streamFixedWindowSize, len(got[0]))
	require.Equal(t, streamFixedWindowSize, len(got[1]))
	require.Equal(t, 2, len(got[2]))
	require.Equal(t, content, strings.Join(got, ""))
}

func TestStreamContentChunksShortCircuits(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name          string
		totalDuration time.Duration
		content       string
		want          []string
	}{
		{
			name:          "non-positive duration",
			totalDuration: 0,
			content:       "anything",
			want:          []string{"anything"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			srv := &Server{}
			ctx := testutil.Context(t, testutil.WaitShort)
			got := slices.Collect(srv.streamContentChunks(ctx, tc.totalDuration, tc.content))
			require.Equal(t, tc.want, got)
		})
	}
}

func TestStreamPacedChunksStopsOnCancel(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(testutil.Context(t, testutil.WaitShort))
	defer cancel()

	var got []string
	for chunk := range streamPacedChunks(ctx, time.Hour, 3, slices.Values([]string{"a", "b", "c"})) {
		got = append(got, chunk)
		cancel()
	}
	require.Equal(t, []string{"a"}, got)
}

func TestRandomStreamDuration(t *testing.T) {
	t.Parallel()

	require.Zero(t, (&Server{}).randomStreamDuration())
	require.Equal(t, 5*time.Second, (&Server{
		minStreamDuration: 5 * time.Second,
		maxStreamDuration: 5 * time.Second,
	}).randomStreamDuration())

	duration := (&Server{
		minStreamDuration: time.Second,
		maxStreamDuration: 2 * time.Second,
	}).randomStreamDuration()
	require.GreaterOrEqual(t, duration, time.Second)
	require.Less(t, duration, 2*time.Second)
}

func TestSendOpenAIStreamTextContent(t *testing.T) {
	t.Parallel()

	srv := &Server{}
	writer := httptest.NewRecorder()
	resp := openAIResponse{
		ID:      "chatcmpl-text",
		Object:  "chat.completion",
		Created: 7,
		Model:   "scaletest-model",
		Choices: []openAIResponseChoice{{
			Message:      openAIMessage{Role: "assistant", Content: "hello there"},
			FinishReason: openAIStopFinishReason,
		}},
	}

	ctx := testutil.Context(t, testutil.WaitShort)
	srv.sendOpenAIStream(ctx, writer, resp)
	events := sseDataEvents(t, writer.Body.String())
	require.Len(t, events, 3)

	first := decodeStreamChunk(t, events[0])
	require.Len(t, first.Choices, 1)
	require.Nil(t, first.Choices[0].FinishReason)
	require.Equal(t, "assistant", first.Choices[0].Delta.Role)
	require.NotNil(t, first.Choices[0].Delta.Content)
	require.Equal(t, "hello there", *first.Choices[0].Delta.Content)

	second := decodeStreamChunk(t, events[1])
	require.Len(t, second.Choices, 1)
	require.NotNil(t, second.Choices[0].FinishReason)
	require.Equal(t, openAIStopFinishReason, *second.Choices[0].FinishReason)
	require.Empty(t, second.Choices[0].Delta.Role)
	require.Nil(t, second.Choices[0].Delta.Content)

	require.Equal(t, "[DONE]", events[2])
}
