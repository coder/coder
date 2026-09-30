package routing_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/aibridge/routing"
)

func TestValidateForwardPath(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		path    string
		wantErr bool
	}{
		{name: "Plain", path: "/v1/models"},
		{name: "ModelID", path: "/v1/models/gpt-4.1"},
		{name: "ResourceID", path: "/v1/responses/resp_abc123"},
		{name: "TrailingSlash", path: "/v1/models/"},
		{name: "EscapedSeparator", path: "/v1/models/org%2Fmodel"},
		{name: "EscapedSpace", path: "/v1/models/a%20b"},
		{name: "DotsInName", path: "/v1/models/..model"},
		{name: "LiteralDotDot", path: "/v1/models/../files", wantErr: true},
		{name: "LiteralDot", path: "/v1/./models", wantErr: true},
		{name: "EncodedDotDot", path: "/v1/models/%2e%2e/files", wantErr: true},
		{name: "EncodedDotDotUpper", path: "/v1/models/%2E%2E/files", wantErr: true},
		{name: "MixedDotDot", path: "/v1/models/.%2e/files", wantErr: true},
		{name: "EncodedDot", path: "/v1/%2e/models", wantErr: true},
		{name: "DotDotEscapedSlash", path: "/v1/models/..%2F..%2Ffiles", wantErr: true},
		{name: "DotDotInsideSegment", path: "/v1/models/a%2F..%2Fb", wantErr: true},
		{name: "DotDotEscapedBackslash", path: "/v1/models/..%5Cfiles", wantErr: true},
		{name: "DoubleEncodedDotDot", path: "/v1/models/%252e%252e/files", wantErr: true},
		{name: "TripleEncodedDotDot", path: "/v1/models/%25252e%25252e/files", wantErr: true},
		{name: "EncodedNUL", path: "/v1/models/a%00b", wantErr: true},
		{name: "EncodedNewline", path: "/v1/models/a%0Ab", wantErr: true},
		{name: "InvalidEscape", path: "/v1/models/%zz", wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Build the URL the way the server does for an incoming
			// request line, preserving the client's escaping.
			u, err := url.ParseRequestURI(tc.path)
			if err != nil {
				// Paths the server itself refuses to parse never reach
				// the handler.
				require.True(t, tc.wantErr, "unexpected parse error: %v", err)
				return
			}
			err = routing.ValidateForwardPath(u)
			if tc.wantErr {
				require.ErrorIs(t, err, routing.ErrInvalidForwardPath)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestRejectInvalidForwardPath(t *testing.T) {
	t.Parallel()

	var called bool
	handler := routing.RejectInvalidForwardPath(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/openai/v1/models/%2e%2e/files", nil))
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "invalid request path")
	assert.False(t, called)

	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/openai/v1/models/org%2Fmodel", nil))
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.True(t, called)
}
