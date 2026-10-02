package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/aibridge/fixtures"
)

func TestFixtures(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		path      string
		blocking  []byte
		streaming []byte
	}{
		{"/chat/completions", fixtures.OaiChatSimple, fixtures.OaiChatSimple},
		{"/responses", fixtures.OaiResponsesBlockingSimple, fixtures.OaiResponsesStreamingSimple},
		{"/messages", fixtures.AntSimple, fixtures.AntSimple},
	} {
		for _, prefix := range []string{"", "/v1"} {
			t.Run(prefix+tc.path, func(t *testing.T) {
				t.Parallel()
				for _, streaming := range []bool{false, true} {
					var output bytes.Buffer
					handler, err := newHandler(&output, []string{"x-custom-identity"})
					require.NoError(t, err)
					body, err := json.Marshal(map[string]any{"stream": streaming, "input": "private prompt"})
					require.NoError(t, err)
					req := httptest.NewRequest(http.MethodPost, prefix+tc.path+"?secret=hidden", bytes.NewReader(body))
					req.Header.Set("Authorization", "Bearer secret")
					req.Header.Set("X-Api-Key", "secret")
					req.Header.Set("Cookie", "secret")
					req.Header.Add("X-Smoke-ID", "first")
					req.Header.Add("X-Smoke-ID", "second")
					req.Header.Set("X-AI-Bridge-Actor-ID", "user-123")
					req.Header.Set("X-AI-Bridge-Actor-Metadata-Username", "alice")
					req.Header.Set("X-AI-Bridge-Actor-Metadata-Email", "alice@example.com")
					req.Header.Set("X-Custom-Identity", "alice")
					rw := httptest.NewRecorder()
					handler.ServeHTTP(rw, req)
					require.Equal(t, http.StatusOK, rw.Code)
					fixture, section, contentType := tc.blocking, "non-streaming", "application/json"
					if streaming {
						fixture, section, contentType = tc.streaming, "streaming", "text/event-stream"
					}
					want, err := fixtureSection(fixture, section)
					require.NoError(t, err)
					require.Equal(t, want, rw.Body.Bytes())
					require.Equal(t, contentType, rw.Header().Get("Content-Type"))
					var record requestRecord
					require.NoError(t, json.Unmarshal(output.Bytes(), &record))
					require.Equal(t, requestRecord{
						Path: prefix + tc.path, Stream: streaming,
						Headers: http.Header{
							"X-Smoke-Id":                          {"first", "second"},
							"X-Ai-Bridge-Actor-Id":                {"user-123"},
							"X-Ai-Bridge-Actor-Metadata-Username": {"alice"},
							"X-Ai-Bridge-Actor-Metadata-Email":    {"alice@example.com"},
							"X-Custom-Identity":                   {"alice"},
						},
					}, record)
				}
			})
		}
	}
}
