package integrationtest

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/aibridge/config"
	"github.com/coder/coder/v2/aibridge/extract"
	extractresponses "github.com/coder/coder/v2/aibridge/extract/responses"
	"github.com/coder/coder/v2/aibridge/fixtures"
	"github.com/coder/coder/v2/aibridge/interceptionerror"
	"github.com/coder/coder/v2/aibridge/internal/testutil"
	"github.com/coder/coder/v2/aibridge/provider"
	"github.com/coder/coder/v2/aibridge/recorder"
)

// TestResponsesExtractorRecordingParity replays every OpenAI Responses
// fixture through the Responses interceptor and through the read-only
// extractor, and requires both to produce the same prompt, token, tool, and
// model thought records and the same categorized terminal error.
//
// Fixtures with injected tools run without an MCP proxy, so the interceptor
// makes a single upstream call like a reverse proxy would; the extractor
// never injects tools or runs the inner agentic loop.
func TestResponsesExtractorRecordingParity(t *testing.T) {
	t.Parallel()

	// Intentional differences in the categorized terminal error. The
	// interceptor relays "error" events with top-level fields and
	// "response.failed" without returning an error, so the interception is
	// recorded as a success; the extractor reports them as provider errors,
	// which categorize as unknown without an HTTP status (like the nested
	// "error" events the interceptor already reports). A blocking body that
	// fails to decode is a gateway-side SDK error for the interceptor, but
	// only a parse note for the fail-open extractor.
	errorTypeDiffs := map[string]struct{ interceptor, extractor recorder.ErrorType }{
		"streaming/stream_error":         {interceptor: "", extractor: recorder.ErrorTypeUnknown},
		"streaming/stream_failure":       {interceptor: "", extractor: recorder.ErrorTypeUnknown},
		"blocking/wrong_response_format": {interceptor: recorder.ErrorTypeUnknown, extractor: ""},
	}

	paths, err := filepath.Glob("../../fixtures/openai/responses/*/*.txtar")
	require.NoError(t, err)
	require.NotEmpty(t, paths)

	for _, path := range paths {
		name := filepath.Base(filepath.Dir(path)) + "/" + strings.TrimSuffix(filepath.Base(path), ".txtar")
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithTimeout(t.Context(), testutil.WaitLong)
			t.Cleanup(cancel)

			data, err := os.ReadFile(path)
			require.NoError(t, err)
			fix := fixtures.Parse(t, data)

			// Interceptor path.
			upstream := testutil.NewMockUpstream(ctx, t, testutil.NewFixtureResponse(fix))
			bridgeServer := newBridgeTestServer(ctx, t, upstream.URL, withProvider(config.ProviderOpenAI))
			resp, err := bridgeServer.makeRequest(t, http.MethodPost, pathOpenAIResponses, fix.Request())
			require.NoError(t, err)
			_, _ = io.Copy(io.Discard, resp.Body)
			require.NoError(t, resp.Body.Close())

			intcs := bridgeServer.Recorder.RecordedInterceptions()
			require.Len(t, intcs, 1)
			interceptionID := intcs[0].ID
			ended := bridgeServer.Recorder.RecordedInterceptionEnd(interceptionID)
			require.NotNil(t, ended)

			// Extractor path.
			reqFacts, err := extractresponses.RequestExtractor{}.ExtractRequest(fix.Request())
			require.NoError(t, err)
			require.Equal(t, intcs[0].Model, reqFacts.Model)
			var raw []byte
			if reqFacts.Streaming {
				raw = fix.Streaming()
			} else {
				raw = fix.NonStreaming()
			}
			ext := extractresponses.NewResponseExtraction()
			var respFacts extract.ResponseFacts
			switch {
			case bytes.HasPrefix(raw, []byte("HTTP/")):
				// Error fixtures hold a raw HTTP response.
				httpResp, err := http.ReadResponse(bufio.NewReader(bytes.NewReader(raw)), nil)
				require.NoError(t, err)
				respFacts = extract.FromBody(ext, httpResp.StatusCode, httpResp.Header.Get("Content-Encoding"), httpResp.Body)
				require.NoError(t, httpResp.Body.Close())
			case reqFacts.Streaming:
				sse := extract.NewSSEStream(ext)
				_, _ = sse.Write(raw)
				respFacts = sse.Result()
			default:
				respFacts = extract.FromBody(ext, http.StatusOK, "", bytes.NewReader(raw))
			}
			extracted := &testutil.MockRecorder{}
			require.NoError(t, extract.NewRecords(interceptionID, reqFacts, respFacts, time.Time{}).Record(ctx, extracted))

			// Records match, ignoring timestamps. The bridge records
			// asynchronously, so order is not significant.
			require.ElementsMatch(t, withoutTime(extracted.RecordedPromptUsages()), withoutTime(bridgeServer.Recorder.RecordedPromptUsages()), "prompt usage")
			require.ElementsMatch(t, withoutTime(extracted.RecordedTokenUsages()), withoutTime(bridgeServer.Recorder.RecordedTokenUsages()), "token usage")
			require.ElementsMatch(t, withoutTime(extracted.RecordedToolUsages()), withoutTime(bridgeServer.Recorder.RecordedToolUsages()), "tool usage")
			require.ElementsMatch(t, withoutTime(extracted.RecordedModelThoughts()), withoutTime(bridgeServer.Recorder.RecordedModelThoughts()), "model thoughts")

			// Terminal errors categorize the same way.
			gotType, _ := interceptionerror.Categorize((*provider.OpenAI)(nil), respFacts.Err)
			if diff, ok := errorTypeDiffs[name]; ok {
				require.Equal(t, diff.interceptor, ended.ErrorType, "interceptor error type")
				require.Equal(t, diff.extractor, gotType, "extractor error type")
				return
			}
			require.Equal(t, ended.ErrorType, gotType, "error type")
		})
	}
}

// withoutTime clears CreatedAt so records from both paths compare equal.
func withoutTime[T any](records []*T) []T {
	out := make([]T, 0, len(records))
	for _, r := range records {
		v := *r
		switch rec := any(&v).(type) {
		case *recorder.PromptUsageRecord:
			rec.CreatedAt = time.Time{}
		case *recorder.TokenUsageRecord:
			rec.CreatedAt = time.Time{}
		case *recorder.ToolUsageRecord:
			rec.CreatedAt = time.Time{}
		case *recorder.ModelThoughtRecord:
			rec.CreatedAt = time.Time{}
		}
		out = append(out, v)
	}
	return out
}
