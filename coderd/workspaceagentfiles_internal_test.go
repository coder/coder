package coderd

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/testutil"
)

func TestForwardWorkspaceAgentFilesRequest(t *testing.T) {
	t.Parallel()

	const secret = "requester-secret-token"

	type agentRequest struct {
		method   string
		path     string
		rawQuery string
		header   http.Header
		body     string
	}

	// newFakeAgent starts an agent HTTP API stand-in that records each
	// request and replies with an ETag, a cookie and an extra header.
	newFakeAgent := func(t *testing.T, status int) (*httptest.Server, <-chan agentRequest) {
		t.Helper()
		requests := make(chan agentRequest, 1)
		srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			body, err := io.ReadAll(r.Body)
			if err != nil {
				return
			}
			requests <- agentRequest{
				method:   r.Method,
				path:     r.URL.Path,
				rawQuery: r.URL.RawQuery,
				header:   r.Header.Clone(),
				body:     string(body),
			}
			rw.Header().Set("Content-Type", "text/html; charset=utf-8")
			rw.Header().Set("ETag", `"agent-etag"`)
			rw.Header().Set("Set-Cookie", codersdk.SessionTokenCookie+"=agent")
			rw.Header().Set("X-Agent-Header", "agent")
			rw.WriteHeader(status)
			_, _ = rw.Write([]byte("agent response"))
		}))
		t.Cleanup(srv.Close)
		return srv, requests
	}

	forward := func(t *testing.T, srv *httptest.Server, req *http.Request) *httptest.ResponseRecorder {
		t.Helper()
		target, err := url.Parse(srv.URL + "/api/v0/write-file")
		require.NoError(t, err)
		api := &API{Options: &Options{Logger: testutil.Logger(t)}}
		rec := httptest.NewRecorder()
		api.forwardWorkspaceAgentFilesRequest(rec, req, srv.Client(), target)
		return rec
	}

	t.Run("FiltersHeaders", func(t *testing.T) {
		t.Parallel()

		srv, requests := newFakeAgent(t, http.StatusPreconditionFailed)
		req := httptest.NewRequest(http.MethodPost, "/files/write-file?path=%2Ftmp%2Fa+b&if_match=%22old%22", strings.NewReader("new contents"))
		req.Header.Set("Content-Type", "application/octet-stream")
		req.Header.Set("Cookie", codersdk.SessionTokenCookie+"="+secret)
		req.Header.Set(codersdk.SessionTokenHeader, secret)
		req.Header.Set("Authorization", "Bearer "+secret)
		req.Header.Set("X-Forwarded-For", "203.0.113.1")
		req.Header.Set(workspacesdk.CoderChatIDHeader, uuid.NewString())
		req.Header.Set("Accept", "*/*")

		rec := forward(t, srv, req)

		got := testutil.RequireReceive(testutil.Context(t, testutil.WaitShort), t, requests)
		require.Equal(t, http.MethodPost, got.method)
		require.Equal(t, "/api/v0/write-file", got.path)
		require.Equal(t, "path=%2Ftmp%2Fa+b&if_match=%22old%22", got.rawQuery)
		require.Equal(t, "new contents", got.body)
		require.Equal(t, "application/octet-stream", got.header.Get("Content-Type"))
		require.Equal(t, "12", got.header.Get("Content-Length"))
		// User-Agent and Accept-Encoding are set by the Go HTTP client.
		allowed := map[string]bool{"Content-Type": true, "Content-Length": true, "User-Agent": true, "Accept-Encoding": true}
		for name, values := range got.header {
			require.True(t, allowed[name], "unexpected header %q forwarded to agent", name)
			for _, v := range values {
				require.NotContains(t, v, secret)
			}
		}

		require.Equal(t, http.StatusPreconditionFailed, rec.Code)
		require.Equal(t, "agent response", rec.Body.String())
		require.Equal(t, "text/html; charset=utf-8", rec.Header().Get("Content-Type"))
		require.Equal(t, `"agent-etag"`, rec.Header().Get("ETag"))
		require.Equal(t, "14", rec.Header().Get("Content-Length"))
		require.Empty(t, rec.Header().Values("Set-Cookie"))
		require.Empty(t, rec.Header().Values("X-Agent-Header"))
		require.Equal(t, "attachment", rec.Header().Get("Content-Disposition"))
		require.Equal(t, "sandbox; default-src 'none'", rec.Header().Get("Content-Security-Policy"))
	})

	t.Run("BodyTooLarge", func(t *testing.T) {
		t.Parallel()

		for _, tc := range []struct {
			name          string
			contentLength int64
		}{
			{name: "ContentLength", contentLength: workspaceAgentFilesMaxRequestBytes + 1},
			{name: "Chunked", contentLength: -1},
		} {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				srv, requests := newFakeAgent(t, http.StatusOK)
				req := httptest.NewRequest(http.MethodPost, "/files/write-file?path=%2Ftmp%2Fa", bytes.NewReader(make([]byte, workspaceAgentFilesMaxRequestBytes+1)))
				req.ContentLength = tc.contentLength

				rec := forward(t, srv, req)

				require.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
				require.Empty(t, requests)
			})
		}
	})
}
