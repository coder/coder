package workspacesdk

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	neturl "net/url"
	"strings"
	"testing"
	"testing/iotest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
)

func TestReadToolCallError(t *testing.T) {
	t.Parallel()

	const jsonType = "application/json"
	cases := []struct {
		name        string
		status      int
		contentType string
		body        string
		// wantCode is the expected ToolCallError code. Empty means the
		// result must equal codersdk.ReadBodyAsError for the same response.
		wantCode ToolCallErrorCode
	}{
		{name: "stale", status: http.StatusConflict, contentType: jsonType, body: `{"code":"stale_tool_call","message":"Stale.","detail":"latest is 43"}`, wantCode: ToolCallErrorStale},
		{name: "agent started after tool call", status: http.StatusConflict, contentType: jsonType, body: `{"code":"agent_started_after_tool_call","message":"Restarted."}`, wantCode: ToolCallErrorAgentStartedAfterToolCall},
		{name: "input mismatch", status: http.StatusConflict, contentType: jsonType, body: `{"code":"input_mismatch","message":"Differs."}`, wantCode: ToolCallErrorInputMismatch},
		{name: "canceled", status: http.StatusConflict, contentType: jsonType, body: `{"code":"tool_call_canceled","message":"Canceled."}`, wantCode: ToolCallErrorCanceled},
		{name: "conflict without code", status: http.StatusConflict, contentType: jsonType, body: `{"message":"Conflict."}`},
		{name: "conflict with unknown code", status: http.StatusConflict, contentType: jsonType, body: `{"code":"start_pending","message":"Pending."}`},
		{name: "conflict without message", status: http.StatusConflict, contentType: jsonType, body: `{}`},
		{name: "conflict empty body", status: http.StatusConflict, contentType: jsonType, body: ``},
		{name: "conflict invalid JSON", status: http.StatusConflict, contentType: jsonType, body: `{"code":`},
		{name: "conflict not JSON", status: http.StatusConflict, contentType: "text/plain", body: `conflict`},
		{name: "not found with known code", status: http.StatusNotFound, contentType: jsonType, body: `{"code":"stale_tool_call","message":"Not found."}`},
		{name: "not found plain", status: http.StatusNotFound, contentType: "text/plain", body: "404 page not found\n"},
		{name: "bad request", status: http.StatusBadRequest, contentType: jsonType, body: `{"message":"Invalid tool call headers."}`},
		{name: "internal server error", status: http.StatusInternalServerError, contentType: jsonType, body: `{"message":"Boom.","detail":"spawn failed"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			response := func() *http.Response {
				rec := httptest.NewRecorder()
				rec.Header().Set("Content-Type", tc.contentType)
				rec.WriteHeader(tc.status)
				_, _ = io.WriteString(rec, tc.body)
				res := rec.Result()
				res.Request = httptest.NewRequest(http.MethodPost, "http://agent/api/v0/processes/start", nil)
				return res
			}

			res := response()
			defer res.Body.Close()
			got := readToolCallError(res)
			require.Error(t, got)

			if tc.wantCode != "" {
				var tcErr *ToolCallError
				require.ErrorAs(t, got, &tcErr)
				assert.Equal(t, tc.wantCode, tcErr.Code)
				assert.NotEmpty(t, tcErr.Message)
				return
			}

			wantRes := response()
			defer wantRes.Body.Close()
			want := codersdk.ReadBodyAsError(wantRes)
			assert.Equal(t, fmt.Sprintf("%T", want), fmt.Sprintf("%T", got))
			assert.Equal(t, want.Error(), got.Error())
			var wantSDK, gotSDK *codersdk.Error
			wantIsSDK := errors.As(want, &wantSDK)
			if assert.Equal(t, wantIsSDK, errors.As(got, &gotSDK)) && wantIsSDK {
				assert.Equal(t, wantSDK.StatusCode(), gotSDK.StatusCode())
				assert.Equal(t, wantSDK.Response, gotSDK.Response)
			}
		})
	}

	t.Run("body read failure", func(t *testing.T) {
		t.Parallel()

		readErr := xerrors.New("connection reset")
		// The partial body has a known code, but a failed read must
		// still produce the codersdk.ReadBodyAsError error.
		response := func() *http.Response {
			return &http.Response{
				StatusCode: http.StatusConflict,
				Header:     http.Header{"Content-Type": {jsonType}},
				Body:       io.NopCloser(io.MultiReader(strings.NewReader(`{"code":"stale_tool_call"`), iotest.ErrReader(readErr))),
			}
		}

		res := response()
		defer res.Body.Close()
		got := readToolCallError(res)
		require.ErrorIs(t, got, readErr)
		var tcErr *ToolCallError
		require.False(t, errors.As(got, &tcErr))

		wantRes := response()
		defer wantRes.Body.Close()
		want := codersdk.ReadBodyAsError(wantRes)
		assert.Equal(t, want.Error(), got.Error())
	})
}

func TestAgentAPIPath(t *testing.T) {
	t.Parallel()

	t.Run("encodes reserved query characters", func(t *testing.T) {
		t.Parallel()

		path := "/tmp/a&b ?#%c.md"
		got := agentAPIPath("/api/v0/resolve-path", neturl.Values{
			"path": []string{path},
		})

		parsed, err := neturl.Parse(got)
		require.NoError(t, err)
		require.Equal(t, "/api/v0/resolve-path", parsed.Path)
		require.Equal(t, path, parsed.Query().Get("path"))
	})

	t.Run("preserves all query values", func(t *testing.T) {
		t.Parallel()

		got := agentAPIPath("/api/v0/read-file-lines", neturl.Values{
			"path":               []string{"/tmp/plan v1#.md"},
			"offset":             []string{"10"},
			"limit":              []string{"20"},
			"max_file_size":      []string{"30"},
			"max_line_bytes":     []string{"40"},
			"max_response_lines": []string{"50"},
			"max_response_bytes": []string{"60"},
		})

		parsed, err := neturl.Parse(got)
		require.NoError(t, err)
		require.Equal(t, "/api/v0/read-file-lines", parsed.Path)
		require.Equal(t, "/tmp/plan v1#.md", parsed.Query().Get("path"))
		require.Equal(t, "10", parsed.Query().Get("offset"))
		require.Equal(t, "20", parsed.Query().Get("limit"))
		require.Equal(t, "30", parsed.Query().Get("max_file_size"))
		require.Equal(t, "40", parsed.Query().Get("max_line_bytes"))
		require.Equal(t, "50", parsed.Query().Get("max_response_lines"))
		require.Equal(t, "60", parsed.Query().Get("max_response_bytes"))
	})

	t.Run("debug logs zero after", func(t *testing.T) {
		t.Parallel()

		got := debugLogsPath(time.Time{})
		require.Equal(t, "/debug/logs", got)
	})

	t.Run("debug logs after", func(t *testing.T) {
		t.Parallel()

		after := time.Date(2026, 5, 18, 12, 34, 56, 789, time.FixedZone("test", -7*60*60))
		got := debugLogsPath(after)
		parsed, err := neturl.Parse(got)
		require.NoError(t, err)
		require.Equal(t, "/debug/logs", parsed.Path)
		require.Equal(t, after.UTC().Format(time.RFC3339Nano), parsed.Query().Get("after"))
	})
}
