package httpmw

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
)

const (
	testParam            = "workspaceagent"
	testWorkspaceAgentID = "8a70c576-12dc-42bc-b791-112a32b5bd43"
)

func TestParseUUID_Valid(t *testing.T) {
	t.Parallel()

	rw := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/{workspaceagent}", nil)

	ctx := chi.NewRouteContext()
	ctx.URLParams.Add(testParam, testWorkspaceAgentID)
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, ctx))

	parsed, ok := ParseUUIDParam(rw, r, "workspaceagent")
	assert.True(t, ok, "UUID should be parsed")
	assert.Equal(t, testWorkspaceAgentID, parsed.String())
}

func TestParseUUID_Invalid(t *testing.T) {
	t.Parallel()

	rw := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/{workspaceagent}", nil)

	ctx := chi.NewRouteContext()
	ctx.URLParams.Add(testParam, "wrong-id")
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, ctx))

	_, ok := ParseUUIDParam(rw, r, "workspaceagent")
	assert.False(t, ok, "UUID should not be parsed")
	assert.Equal(t, http.StatusBadRequest, rw.Code)

	var response codersdk.Response
	err := json.Unmarshal(rw.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Contains(t, response.Message, `Invalid UUID "wrong-id"`)
}

// TestNormalizeAudienceURI tests URI normalization for OAuth2 audience validation
func TestNormalizeAudienceURI(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "EmptyString",
			input:    "",
			expected: "",
		},
		{
			name:     "SimpleHTTPWithoutTrailingSlash",
			input:    "http://example.com",
			expected: "http://example.com/",
		},
		{
			name:     "SimpleHTTPWithTrailingSlash",
			input:    "http://example.com/",
			expected: "http://example.com/",
		},
		{
			name:     "HTTPSWithPath",
			input:    "https://api.example.com/v1/",
			expected: "https://api.example.com/v1",
		},
		{
			name:     "CaseNormalization",
			input:    "HTTPS://API.EXAMPLE.COM/V1/",
			expected: "https://api.example.com/V1",
		},
		{
			name:     "DefaultHTTPPort",
			input:    "http://example.com:80/api/",
			expected: "http://example.com/api",
		},
		{
			name:     "DefaultHTTPSPort",
			input:    "https://example.com:443/api/",
			expected: "https://example.com/api",
		},
		{
			name:     "NonDefaultPort",
			input:    "http://example.com:8080/api/",
			expected: "http://example.com:8080/api",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result := normalizeAudienceURI(tc.input)
			assert.Equal(t, tc.expected, result)
		})
	}
}

// TestNormalizeHost tests host normalization including IDN support
func TestNormalizeHost(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "EmptyString",
			input:    "",
			expected: "",
		},
		{
			name:     "SimpleHost",
			input:    "example.com",
			expected: "example.com",
		},
		{
			name:     "HostWithPort",
			input:    "example.com:8080",
			expected: "example.com:8080",
		},
		{
			name:     "CaseNormalization",
			input:    "EXAMPLE.COM",
			expected: "example.com",
		},
		{
			name:     "IPv4Address",
			input:    "192.168.1.1",
			expected: "192.168.1.1",
		},
		{
			name:     "IPv6Address",
			input:    "[::1]:8080",
			expected: "[::1]:8080",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result := normalizeHost(tc.input)
			assert.Equal(t, tc.expected, result)
		})
	}
}

// TestNormalizePathSegments tests path normalization including dot-segment removal
func TestNormalizePathSegments(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "EmptyString",
			input:    "",
			expected: "/",
		},
		{
			name:     "SimplePath",
			input:    "/api/v1",
			expected: "/api/v1",
		},
		{
			name:     "PathWithDotSegments",
			input:    "/api/../v1/./test",
			expected: "/v1/test",
		},
		{
			name:     "TrailingSlash",
			input:    "/api/v1/",
			expected: "/api/v1",
		},
		{
			name:     "MultipleSlashes",
			input:    "/api//v1///test",
			expected: "/api//v1///test",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result := normalizePathSegments(tc.input)
			assert.Equal(t, tc.expected, result)
		})
	}
}

// TestExtractExpectedAudience tests audience extraction from HTTP requests
func TestExtractExpectedAudience(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		scheme   string
		host     string
		path     string
		expected string
	}{
		{
			name:     "SimpleHTTP",
			scheme:   "http",
			host:     "example.com",
			path:     "/api/test",
			expected: "http://example.com/",
		},
		{
			name:     "HTTPS",
			scheme:   "https",
			host:     "api.example.com",
			path:     "/v1/users",
			expected: "https://api.example.com/",
		},
		{
			name:     "WithPort",
			scheme:   "http",
			host:     "localhost:8080",
			path:     "/api",
			expected: "http://localhost:8080/",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var req *http.Request
			if tc.scheme == "https" {
				req = httptest.NewRequest("GET", "https://"+tc.host+tc.path, nil)
			} else {
				req = httptest.NewRequest("GET", "http://"+tc.host+tc.path, nil)
			}
			req.Host = tc.host

			result := extractExpectedAudience(nil, req)
			assert.Equal(t, tc.expected, result)
		})
	}
}
