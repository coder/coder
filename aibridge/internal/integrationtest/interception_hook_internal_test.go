package integrationtest

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/sjson"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/aibridge/fixtures"
	"github.com/coder/coder/v2/aibridge/internal/testutil"
	"github.com/coder/coder/v2/aibridge/recorder"
)

type failingInterceptionRecorder struct{ *testutil.MockRecorder }

func (failingInterceptionRecorder) RecordInterception(context.Context, *recorder.InterceptionRecord) error {
	return xerrors.New("insertion failed")
}

func TestInterceptionRecordedHookSkipsFailedRecord(t *testing.T) {
	t.Parallel()

	fix := fixtures.Parse(t, fixtures.OaiResponsesBlockingSimple)
	upstream := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("upstream contacted after failed recording")
	}))
	t.Cleanup(upstream.Close)
	notified := make(chan uuid.UUID, 1)
	server := newBridgeTestServer(t.Context(), t, upstream.URL, func(cfg *bridgeConfig) {
		cfg.interceptionHook = func(id uuid.UUID) { notified <- id }
		cfg.recorderOverride = failingInterceptionRecorder{MockRecorder: &testutil.MockRecorder{}}
	})
	resp, err := server.makeRequest(t, http.MethodPost, pathOpenAIResponses, fix.Request())
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	require.Empty(t, notified)
}

func TestInterceptionRecordedHook(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		fixture []byte
		path    string
		stream  bool
	}{
		{"anthropic_stream", fixtures.AntSingleBuiltinTool, pathAnthropicMessages, true},
		{"anthropic_blocking", fixtures.AntSingleBuiltinTool, pathAnthropicMessages, false},
		{"chat_stream", fixtures.OaiChatSingleBuiltinTool, pathOpenAIChatCompletions, true},
		{"chat_blocking", fixtures.OaiChatSingleBuiltinTool, pathOpenAIChatCompletions, false},
		{"responses_stream", fixtures.OaiResponsesStreamingSimple, pathOpenAIResponses, true},
		{"responses_blocking", fixtures.OaiResponsesBlockingSimple, pathOpenAIResponses, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := t.Context()
			fix := fixtures.Parse(t, tc.fixture)
			upstream := testutil.NewMockUpstream(ctx, t, testutil.NewFixtureResponse(fix))
			notified := make(chan uuid.UUID, 2)
			server := newBridgeTestServer(ctx, t, upstream.URL, func(cfg *bridgeConfig) {
				cfg.interceptionHook = func(id uuid.UUID) { notified <- id }
			})
			body, err := sjson.SetBytes(fix.Request(), "stream", tc.stream)
			require.NoError(t, err)
			resp, err := server.makeRequest(t, http.MethodPost, tc.path, body)
			require.NoError(t, err)
			defer resp.Body.Close()
			require.Equal(t, http.StatusOK, resp.StatusCode)
			_, err = io.Copy(io.Discard, resp.Body)
			require.NoError(t, err)
			require.Len(t, notified, 1)
			recorded := server.Recorder.RecordedInterceptions()
			require.Len(t, recorded, 1)
			require.Equal(t, recorded[0].ID, (<-notified).String())
		})
	}
}
