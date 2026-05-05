package apidump //nolint:testpackage // tests unexported internals

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/quartz"
)

// findDumpFile finds a dump file matching the pattern in the given directory.
func findDumpFile(t *testing.T, dir, suffix string) string {
	t.Helper()
	pattern := filepath.Join(dir, "*"+suffix)
	matches, err := filepath.Glob(pattern)
	require.NoError(t, err)
	require.Len(t, matches, 1, "expected exactly one %s file in %s", suffix, dir)
	return matches[0]
}

func TestBridgedMiddleware_RedactsSensitiveRequestHeaders(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: false}).Leveled(slog.LevelDebug)
	clk := quartz.NewMock(t)
	interceptionID := uuid.New()

	middleware := NewBridgeMiddleware(tmpDir, "openai", "gpt-4", interceptionID, logger, clk)
	require.NotNil(t, middleware)

	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "https://api.openai.com/v1/chat/completions", bytes.NewReader([]byte(`{"test": true}`)))
	require.NoError(t, err)

	// Add sensitive headers that should be redacted
	req.Header.Set("Authorization", "Bearer sk-secret-key-12345")
	req.Header.Set("X-Api-Key", "secret-api-key-value")
	req.Header.Set("Cookie", "session=abc123")

	// Add non-sensitive headers that should be kept as-is
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "test-client")

	// Call middleware with a mock next function
	resp, err := middleware(req, func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Proto:      "HTTP/1.1",
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(bytes.NewReader([]byte(`{"ok": true}`))),
		}, nil
	})
	require.NoError(t, err)
	defer resp.Body.Close()

	// Read the request dump file
	modelDir := filepath.Join(tmpDir, "openai", "gpt-4")
	reqDumpPath := findDumpFile(t, modelDir, SuffixRequest)
	reqContent, err := os.ReadFile(reqDumpPath)
	require.NoError(t, err)

	content := string(reqContent)

	// Verify sensitive headers ARE present but redacted
	require.Contains(t, content, "Authorization: Bear...2345")
	require.Contains(t, content, "X-Api-Key: secr...alue")
	require.Contains(t, content, "Cookie: se...23") // "session=abc123" is 14 chars, so first 2 + last 2

	// Verify the full secret values are NOT present
	require.NotContains(t, content, "sk-secret-key-12345")
	require.NotContains(t, content, "secret-api-key-value")

	// Verify non-sensitive headers ARE present in full
	require.Contains(t, content, "Content-Type: application/json")
	require.Contains(t, content, "User-Agent: test-client")
}

func TestBridgedMiddleware_RedactsSensitiveResponseHeaders(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: false}).Leveled(slog.LevelDebug)
	clk := quartz.NewMock(t)
	interceptionID := uuid.New()

	middleware := NewBridgeMiddleware(tmpDir, "openai", "gpt-4", interceptionID, logger, clk)
	require.NotNil(t, middleware)

	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "https://api.openai.com/v1/chat/completions", bytes.NewReader([]byte(`{}`)))
	require.NoError(t, err)

	// Call middleware with a response containing sensitive headers
	resp, err := middleware(req, func(r *http.Request) (*http.Response, error) {
		resp := &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Proto:      "HTTP/1.1",
			Header:     make(http.Header),
			Body:       io.NopCloser(bytes.NewReader([]byte(`{"ok": true}`))),
		}
		// Add sensitive response headers
		resp.Header.Set("Set-Cookie", "session=secret123; HttpOnly; Secure")
		resp.Header.Set("WWW-Authenticate", "Bearer realm=\"api\"")
		// Add non-sensitive headers
		resp.Header.Set("Content-Type", "application/json")
		resp.Header.Set("X-Request-Id", "req-123")
		return resp, nil
	})
	require.NoError(t, err)

	// Must read and close response body to trigger the streaming dump
	_, err = io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())

	// Read the response dump file
	modelDir := filepath.Join(tmpDir, "openai", "gpt-4")
	respDumpPath := findDumpFile(t, modelDir, SuffixResponse)
	respContent, err := os.ReadFile(respDumpPath)
	require.NoError(t, err)

	content := string(respContent)

	// Verify sensitive headers are present but redacted
	require.Contains(t, content, "Set-Cookie: sess...cure")
	// Note: Go canonicalizes WWW-Authenticate to Www-Authenticate
	// "Bearer realm=\"api\"" = 18 chars, first 2 = "Be", last 2 = "i\""
	require.Contains(t, content, "Www-Authenticate: Be...i\"")

	// Verify full secret values are NOT present
	require.NotContains(t, content, "secret123")
	require.NotContains(t, content, "realm=\"api\"")

	// Verify non-sensitive headers ARE present in full
	require.Contains(t, content, "Content-Type: application/json")
	require.Contains(t, content, "X-Request-Id: req-123")
}

func TestBridgedMiddleware_WritesErrorFile_WhenNextFails(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: false}).Leveled(slog.LevelDebug)
	clk := quartz.NewMock(t)
	interceptionID := uuid.New()

	middleware := NewBridgeMiddleware(tmpDir, "openai", "gpt-4", interceptionID, logger, clk)
	require.NotNil(t, middleware)

	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "https://api.openai.com/v1/chat/completions", bytes.NewReader([]byte(`{}`)))
	require.NoError(t, err)

	upstreamErr := io.ErrUnexpectedEOF
	resp, err := middleware(req, func(_ *http.Request) (*http.Response, error) { //nolint:bodyclose // resp is nil on error
		return nil, upstreamErr
	})
	require.ErrorIs(t, err, upstreamErr)
	require.Nil(t, resp)

	modelDir := filepath.Join(tmpDir, "openai", "gpt-4")
	errDumpPath := findDumpFile(t, modelDir, SuffixError)
	content, readErr := os.ReadFile(errDumpPath)
	require.NoError(t, readErr)
	require.Contains(t, string(content), upstreamErr.Error())
}

func TestBridgedMiddleware_EmptyBaseDir_ReturnsNil(t *testing.T) {
	t.Parallel()

	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: false}).Leveled(slog.LevelDebug)
	middleware := NewBridgeMiddleware("", "openai", "gpt-4", uuid.New(), logger, quartz.NewMock(t))
	require.Nil(t, middleware)
}

func TestBridgedMiddleware_PreservesRequestBody(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: false}).Leveled(slog.LevelDebug)
	clk := quartz.NewMock(t)
	interceptionID := uuid.New()

	middleware := NewBridgeMiddleware(tmpDir, "openai", "gpt-4", interceptionID, logger, clk)
	require.NotNil(t, middleware)

	originalBody := `{"messages": [{"role": "user", "content": "hello"}]}`
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "https://api.openai.com/v1/chat/completions", bytes.NewReader([]byte(originalBody)))
	require.NoError(t, err)

	var capturedBody []byte
	resp2, err := middleware(req, func(r *http.Request) (*http.Response, error) {
		// Read the body in the next handler to verify it's still available
		capturedBody, _ = io.ReadAll(r.Body)
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Proto:      "HTTP/1.1",
			Header:     http.Header{},
			Body:       io.NopCloser(bytes.NewReader([]byte(`{}`))),
		}, nil
	})
	require.NoError(t, err)
	defer resp2.Body.Close()

	// Verify the body was preserved for the next handler
	require.Equal(t, originalBody, string(capturedBody))
}

func TestBridgedMiddleware_ModelWithSlash(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: false}).Leveled(slog.LevelDebug)
	clk := quartz.NewMock(t)
	interceptionID := uuid.New()

	// Model with slash should have it replaced with dash
	middleware := NewBridgeMiddleware(tmpDir, "google", "gemini/1.5-pro", interceptionID, logger, clk)
	require.NotNil(t, middleware)

	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "https://api.google.com/v1/chat", bytes.NewReader([]byte(`{}`)))
	require.NoError(t, err)

	resp3, err := middleware(req, func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Proto:      "HTTP/1.1",
			Header:     http.Header{},
			Body:       io.NopCloser(bytes.NewReader([]byte(`{}`))),
		}, nil
	})
	require.NoError(t, err)
	defer resp3.Body.Close()

	// Verify files are created with sanitized model name
	modelDir := filepath.Join(tmpDir, "google", "gemini-1.5-pro")
	reqDumpPath := findDumpFile(t, modelDir, SuffixRequest)
	_, err = os.Stat(reqDumpPath)
	require.NoError(t, err)
}

func TestPrettyPrintJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    []byte
		expected string
	}{
		{
			name:     "empty",
			input:    []byte{},
			expected: "",
		},
		{
			name:     "valid JSON",
			input:    []byte(`{"key":"value"}`),
			expected: "{\n  \"key\": \"value\"\n}\n",
		},
		{
			name:     "invalid JSON returns as-is",
			input:    []byte("not json"),
			expected: "not json",
		},
		// see: https://github.com/tidwall/pretty/blob/9090695766b652478676cc3e55bc3187056b1ff0/pretty.go#L117
		// for input starting with "t" it would change it to "true", eg. "t_rest_of_the_string_is_discarded" -> "true"
		// similar for inputs startrting with "f" and "n"
		{
			name:     "invalid JSON edge case t",
			input:    []byte("test"),
			expected: "test",
		},
		{
			name:     "invalid JSON edge case f",
			input:    []byte("f"),
			expected: "f",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result := prettyPrintJSON(tc.input)
			require.Equal(t, tc.expected, string(result))
		})
	}
}

func TestBridgedMiddleware_AllSensitiveRequestHeaders(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: false}).Leveled(slog.LevelDebug)
	clk := quartz.NewMock(t)
	interceptionID := uuid.New()

	middleware := NewBridgeMiddleware(tmpDir, "openai", "gpt-4", interceptionID, logger, clk)
	require.NotNil(t, middleware)

	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "https://api.openai.com/v1/chat/completions", bytes.NewReader([]byte(`{}`)))
	require.NoError(t, err)

	// Set all sensitive headers
	req.Header.Set("Authorization", "Bearer sk-secret-key")
	req.Header.Set("X-Api-Key", "secret-api-key")
	req.Header.Set("Api-Key", "another-secret")
	req.Header.Set("X-Auth-Token", "auth-token-val")
	req.Header.Set("Cookie", "session=abc123def")
	req.Header.Set("Proxy-Authorization", "Basic proxy-creds")
	req.Header.Set("X-Amz-Security-Token", "aws-security-token")

	resp4, err := middleware(req, func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Proto:      "HTTP/1.1",
			Header:     http.Header{},
			Body:       io.NopCloser(bytes.NewReader([]byte(`{}`))),
		}, nil
	})
	require.NoError(t, err)
	defer resp4.Body.Close()

	modelDir := filepath.Join(tmpDir, "openai", "gpt-4")
	reqDumpPath := findDumpFile(t, modelDir, SuffixRequest)
	reqContent, err := os.ReadFile(reqDumpPath)
	require.NoError(t, err)

	content := string(reqContent)

	// Verify none of the full secret values are present
	require.NotContains(t, content, "sk-secret-key")
	require.NotContains(t, content, "secret-api-key")
	require.NotContains(t, content, "another-secret")
	require.NotContains(t, content, "auth-token-val")
	require.NotContains(t, content, "abc123def")
	require.NotContains(t, content, "proxy-creds")
	require.NotContains(t, content, "aws-security-token")
	require.NotContains(t, content, "google-api-key")

	// But headers themselves are present (redacted)
	require.Contains(t, content, "Authorization:")
	require.Contains(t, content, "X-Api-Key:")
	require.Contains(t, content, "Api-Key:")
	require.Contains(t, content, "X-Auth-Token:")
	require.Contains(t, content, "Cookie:")
	require.Contains(t, content, "Proxy-Authorization:")
	require.Contains(t, content, "X-Amz-Security-Token:")
}

func TestPassthroughMiddleware(t *testing.T) {
	t.Parallel()

	t.Run("empty_base_dir_returns_original_transport", func(t *testing.T) {
		t.Parallel()
		logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: false}).Leveled(slog.LevelDebug)
		inner := http.DefaultTransport
		rt := NewPassthroughMiddleware(inner, "", "openai", logger, quartz.NewMock(t))
		require.Equal(t, inner, rt)
	})

	t.Run("returns_error_from_inner_round_trip", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: false}).Leveled(slog.LevelDebug)
		clk := quartz.NewMock(t)

		innerErr := io.ErrUnexpectedEOF
		inner := &mockRoundTripper{
			roundTrip: func(_ *http.Request) (*http.Response, error) {
				return nil, innerErr
			},
		}

		rt := NewPassthroughMiddleware(inner, tmpDir, "openai", logger, clk)

		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "https://api.openai.com/v1/models", nil)
		require.NoError(t, err)

		resp, err := rt.RoundTrip(req) //nolint:bodyclose // resp is nil on error
		require.ErrorIs(t, err, innerErr)
		require.Nil(t, resp)

		passthroughDir := filepath.Join(tmpDir, "openai", "passthrough")
		errDumpPath := findDumpFile(t, passthroughDir, SuffixError)
		content, readErr := os.ReadFile(errDumpPath)
		require.NoError(t, readErr)
		require.Contains(t, string(content), innerErr.Error())
	})

	t.Run("dumps_request_and_response", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: false}).Leveled(slog.LevelDebug)
		clk := quartz.NewMock(t)

		req1Body := `first request`
		req2Body := `{"request": 2}`
		req2BodyPretty := "{\n  \"request\": 2\n}\n"

		callCount := 0
		inner := &mockRoundTripper{
			roundTrip: func(req *http.Request) (*http.Response, error) {
				// Verify body is still readable after dump
				body, err := io.ReadAll(req.Body)
				require.NoError(t, err)
				callCount++
				if callCount == 1 {
					require.Equal(t, req1Body, string(body))
				} else {
					require.Equal(t, req2Body, string(body))
				}

				return &http.Response{
					StatusCode: http.StatusOK,
					Status:     "200 OK",
					Proto:      "HTTP/1.1",
					Header:     http.Header{"Content-Type": []string{"application/json"}},
					Body:       io.NopCloser(bytes.NewReader([]byte(fmt.Sprintf(`{"call": %d}"`, callCount)))),
				}, nil
			},
		}

		rt := NewPassthroughMiddleware(inner, tmpDir, "openai", logger, clk)

		req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "/v1/models", bytes.NewReader([]byte(req1Body)))
		require.NoError(t, err)
		req.Header.Set("Authorization", "Bearer sk-secret-key-12345")
		resp, err := rt.RoundTrip(req)
		require.NoError(t, err)
		_, err = io.ReadAll(resp.Body)
		require.NoError(t, err)
		require.NoError(t, resp.Body.Close())

		// Second request should create new req/resp files
		req2, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "/v1/conversations", bytes.NewReader([]byte(req2Body)))
		require.NoError(t, err)
		resp2, err := rt.RoundTrip(req2)
		require.NoError(t, err)
		_, err = io.ReadAll(resp2.Body)
		require.NoError(t, err)
		require.NoError(t, resp2.Body.Close())

		// Validate request files contents
		passthroughDir := filepath.Join(tmpDir, "openai", "passthrough")
		req1Dump := readDumpFileContent(t, filepath.Join(passthroughDir, "*-v1-models-*"+SuffixRequest))
		req2Dump := readDumpFileContent(t, filepath.Join(passthroughDir, "*-v1-conversations-*"+SuffixRequest))

		require.Contains(t, req1Dump, req1Body+"\n")
		require.Contains(t, req2Dump, req2BodyPretty)
		// Sensitive header should be redacted
		require.NotContains(t, req1Dump, "sk-secret-key-12345")
		require.NotContains(t, req2Dump, "sk-secret-key-12345")
		require.Contains(t, req1Dump, "Authorization:")
		require.NotContains(t, req2Dump, "Authorization:")

		// Validate response files contents
		resp1Dump := readDumpFileContent(t, filepath.Join(passthroughDir, "*-v1-models-*"+SuffixResponse))
		resp2Dump := readDumpFileContent(t, filepath.Join(passthroughDir, "*-v1-conversations-*"+SuffixResponse))

		require.Contains(t, resp1Dump, "200 OK")
		require.Contains(t, resp1Dump, `{"call": 1}"`)
		require.Contains(t, resp2Dump, "200 OK")
		require.Contains(t, resp2Dump, `{"call": 2}"`)
	})
}

type mockRoundTripper struct {
	roundTrip func(*http.Request) (*http.Response, error)
}

func (m *mockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return m.roundTrip(req)
}

// readDumpFileContent reads the content of the dump file matching the pattern.
// Expects exactly one file to match the pattern.
func readDumpFileContent(t *testing.T, pattern string) string {
	t.Helper()
	matches, err := filepath.Glob(pattern)
	require.NoError(t, err)
	require.Len(t, matches, 1, "expected exactly one match got: %v %s", len(matches), strings.Join(matches, ", "), pattern)
	reqContent, readErr := os.ReadFile(matches[0])
	require.NoError(t, readErr)
	return string(reqContent)
}
