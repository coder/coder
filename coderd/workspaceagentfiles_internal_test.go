package coderd

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
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

	writeFileRoute := workspaceAgentFilesRoute{
		path:        "/api/v0/write-file",
		queryParams: []string{"path", "if_match"},
	}
	readFileRoute := workspaceAgentFilesRoute{
		path:             "/api/v0/read-file",
		queryParams:      []string{"path"},
		maxResponseBytes: workspaceAgentFilesMaxFileBytes,
	}

	type agentRequest struct {
		method   string
		path     string
		rawQuery string
		header   http.Header
		// body receives what the agent read once the body ends or fails.
		body <-chan []byte
	}

	// newFakeAgent starts an agent HTTP API stand-in that records each
	// request on entry and answers with respond.
	newFakeAgent := func(t *testing.T, respond func(http.ResponseWriter)) (*url.URL, *http.Client, <-chan agentRequest) {
		t.Helper()
		requests := make(chan agentRequest, 1)
		srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			body := make(chan []byte, 1)
			requests <- agentRequest{
				method:   r.Method,
				path:     r.URL.Path,
				rawQuery: r.URL.RawQuery,
				header:   r.Header.Clone(),
				body:     body,
			}
			data, _ := io.ReadAll(r.Body)
			body <- data
			respond(rw)
		}))
		t.Cleanup(srv.Close)
		agentURL, err := url.Parse(srv.URL)
		require.NoError(t, err)
		return agentURL, srv.Client(), requests
	}

	forward := func(t *testing.T, agentURL *url.URL, client *http.Client, route workspaceAgentFilesRoute, req *http.Request) *httptest.ResponseRecorder {
		t.Helper()
		api := &API{Options: &Options{Logger: testutil.Logger(t)}}
		rec := httptest.NewRecorder()
		api.forwardWorkspaceAgentFilesRequest(rec, req, client, agentURL, route)
		return rec
	}

	t.Run("Filters", func(t *testing.T) {
		t.Parallel()

		agentURL, client, requests := newFakeAgent(t, func(rw http.ResponseWriter) {
			rw.Header().Set("Content-Type", "text/html; charset=utf-8")
			rw.Header().Set("ETag", `"agent-etag"`)
			rw.Header().Set("Set-Cookie", codersdk.SessionTokenCookie+"=agent")
			rw.Header().Set("Cache-Control", "public, max-age=3600")
			rw.Header().Set("X-Agent-Header", "agent")
			rw.WriteHeader(http.StatusPreconditionFailed)
			_, _ = rw.Write([]byte("agent response"))
		})
		query := strings.Join([]string{
			codersdk.SessionTokenCookie + "=" + secret,
			"path=%2Ftmp%2Fa+b",
			"access_token=" + secret,
			"coder%5Fsession%5Ftoken=" + secret,
			"if_match=%22old%22",
			"path=%2Ftmp%2Fc",
		}, "&")
		req := httptest.NewRequest(http.MethodPost, "/files/write-file?"+query, strings.NewReader("new contents"))
		req.Header.Set("Content-Type", "application/octet-stream")
		req.Header.Set("Cookie", codersdk.SessionTokenCookie+"="+secret)
		req.Header.Set(codersdk.SessionTokenHeader, secret)
		req.Header.Set("Authorization", "Bearer "+secret)
		req.Header.Set("X-Forwarded-For", "203.0.113.1")
		req.Header.Set(workspacesdk.CoderChatIDHeader, uuid.NewString())
		req.Header.Set("Accept", "*/*")

		rec := forward(t, agentURL, client, writeFileRoute, req)

		ctx := testutil.Context(t, testutil.WaitShort)
		got := testutil.RequireReceive(ctx, t, requests)
		require.Equal(t, http.MethodPost, got.method)
		require.Equal(t, "/api/v0/write-file", got.path)
		require.Equal(t, "path=%2Ftmp%2Fa+b&if_match=%22old%22&path=%2Ftmp%2Fc", got.rawQuery)
		require.Equal(t, "new contents", string(testutil.RequireReceive(ctx, t, got.body)))
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
		require.Empty(t, rec.Header().Values("Cache-Control"))
		require.Empty(t, rec.Header().Values("X-Agent-Header"))
		require.Equal(t, "attachment", rec.Header().Get("Content-Disposition"))
		require.Equal(t, "sandbox; default-src 'none'", rec.Header().Get("Content-Security-Policy"))
	})

	t.Run("RequestTooLarge", func(t *testing.T) {
		t.Parallel()

		t.Run("ContentLength", func(t *testing.T) {
			t.Parallel()

			agentURL, client, requests := newFakeAgent(t, func(rw http.ResponseWriter) {
				rw.WriteHeader(http.StatusOK)
			})
			req := httptest.NewRequest(http.MethodPost, "/files/write-file?path=%2Ftmp%2Fa", bytes.NewReader(make([]byte, workspaceAgentFilesMaxFileBytes+1)))

			rec := forward(t, agentURL, client, writeFileRoute, req)

			require.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
			require.Empty(t, requests, "no request reaches the agent")
		})

		t.Run("Chunked", func(t *testing.T) {
			t.Parallel()

			agentURL, client, requests := newFakeAgent(t, func(rw http.ResponseWriter) {
				rw.WriteHeader(http.StatusOK)
			})
			req := httptest.NewRequest(http.MethodPost, "/files/write-file?path=%2Ftmp%2Fa", bytes.NewReader(make([]byte, workspaceAgentFilesMaxFileBytes+1)))
			req.ContentLength = -1

			rec := forward(t, agentURL, client, writeFileRoute, req)

			require.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
			ctx := testutil.Context(t, testutil.WaitShort)
			got := testutil.RequireReceive(ctx, t, requests)
			require.LessOrEqual(t, len(testutil.RequireReceive(ctx, t, got.body)), workspaceAgentFilesMaxFileBytes)
		})
	})

	t.Run("ResponseSize", func(t *testing.T) {
		t.Parallel()

		for _, tc := range []struct {
			name       string
			size       int
			chunked    bool
			wantStatus int
		}{
			{name: "ContentLengthTooLarge", size: workspaceAgentFilesMaxFileBytes + 1, wantStatus: http.StatusRequestEntityTooLarge},
			{name: "ChunkedTooLarge", size: workspaceAgentFilesMaxFileBytes + 1, chunked: true, wantStatus: http.StatusRequestEntityTooLarge},
			{name: "ChunkedWithinLimit", size: 4096, chunked: true, wantStatus: http.StatusOK},
		} {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				data := bytes.Repeat([]byte("a"), tc.size)
				agentURL, client, _ := newFakeAgent(t, func(rw http.ResponseWriter) {
					rw.Header().Set("Content-Type", "text/plain")
					if !tc.chunked {
						rw.Header().Set("Content-Length", strconv.Itoa(len(data)))
					}
					rw.WriteHeader(http.StatusOK)
					if tc.chunked {
						_ = http.NewResponseController(rw).Flush()
					}
					_, _ = rw.Write(data)
				})
				req := httptest.NewRequest(http.MethodGet, "/files/read-file?path=%2Ftmp%2Fa", nil)

				rec := forward(t, agentURL, client, readFileRoute, req)

				require.Equal(t, tc.wantStatus, rec.Code)
				if tc.wantStatus == http.StatusOK {
					require.Equal(t, strconv.Itoa(tc.size), rec.Header().Get("Content-Length"))
					require.Equal(t, data, rec.Body.Bytes())
				} else {
					require.NotContains(t, rec.Body.String(), "aaaa")
				}
			})
		}
	})
}

func TestFilterWorkspaceAgentFilesQuery(t *testing.T) {
	t.Parallel()

	allowed := []string{"root", "q", "limit"}
	for _, tc := range []struct {
		name  string
		query string
		want  string
	}{
		{name: "Empty", query: "", want: ""},
		{name: "KeepsOrderAndRepeats", query: "root=%2Fb&q=x+y&root=~&limit=5", want: "root=%2Fb&q=x+y&root=~&limit=5"},
		{name: "DropsCredentials", query: "coder_session_token=s&root=a&access_token=s", want: "root=a"},
		{name: "DropsEncodedCredentialKey", query: "coder%5Fsession%5Ftoken=s&q=a", want: "q=a"},
		{name: "DropsUnknown", query: "path=a&Root=b&q=c", want: "q=c"},
		{name: "DropsSemicolon", query: "q=a;coder_session_token=s&root=b", want: "root=b"},
		{name: "DropsInvalidEscape", query: "q=%zz&root=b", want: "root=b"},
		{name: "KeyWithoutValue", query: "q&root=b", want: "q=&root=b"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, filterWorkspaceAgentFilesQuery(tc.query, allowed))
		})
	}
}
