package wsproxysdk_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/workspaceapps"
	"github.com/coder/coder/v2/enterprise/wsproxy/wsproxysdk"
	"github.com/coder/coder/v2/testutil"
)

func Test_IssueSignedAppTokenHTML(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		var (
			expectedProxyToken = "hi:test"
			expectedAppReq     = workspaceapps.Request{
				AccessMethod:      workspaceapps.AccessMethodPath,
				BasePath:          "/@user/workspace/apps/slug",
				UsernameOrID:      "user",
				WorkspaceNameOrID: "workspace",
				AppSlugOrPort:     "slug",
			}
			expectedSessionToken   = "user-session-token"
			expectedSignedTokenStr = "signed-app-token"
		)
		var called int64
		srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			atomic.AddInt64(&called, 1)

			assert.Equal(t, r.Method, http.MethodPost)
			assert.Equal(t, r.URL.Path, "/api/v2/workspaceproxies/me/issue-signed-app-token")
			assert.Equal(t, r.Header.Get(httpmw.WorkspaceProxyAuthTokenHeader), expectedProxyToken)

			var req workspaceapps.IssueTokenRequest
			err := json.NewDecoder(r.Body).Decode(&req)
			assert.NoError(t, err)
			assert.Equal(t, req.AppRequest, expectedAppReq)
			assert.Equal(t, req.SessionToken, expectedSessionToken)

			rw.WriteHeader(http.StatusCreated)
			err = json.NewEncoder(rw).Encode(wsproxysdk.IssueSignedAppTokenResponse{
				SignedTokenStr: expectedSignedTokenStr,
			})
			assert.NoError(t, err)
		}))

		u, err := url.Parse(srv.URL)
		require.NoError(t, err)
		client := wsproxysdk.New(u)
		client.SetSessionToken(expectedProxyToken)

		ctx := testutil.Context(t, testutil.WaitLong)

		rw := newResponseRecorder()
		tokenRes, ok := client.IssueSignedAppTokenHTML(ctx, rw, workspaceapps.IssueTokenRequest{
			AppRequest:   expectedAppReq,
			SessionToken: expectedSessionToken,
		})
		if !assert.True(t, ok) {
			t.Log("issue request failed when it should've succeeded")
			t.Log("response dump:")
			res := rw.Result()
			defer res.Body.Close()
			dump, err := httputil.DumpResponse(res, true)
			if err != nil {
				t.Logf("failed to dump response: %v", err)
			} else {
				t.Log(string(dump))
			}
			t.FailNow()
		}
		require.Equal(t, expectedSignedTokenStr, tokenRes.SignedTokenStr)
		require.False(t, rw.WasWritten())

		require.EqualValues(t, called, 1)
	})

	t.Run("Error", func(t *testing.T) {
		t.Parallel()

		var (
			expectedProxyToken     = "hi:test"
			expectedResponseStatus = http.StatusBadRequest
			expectedResponseBody   = "bad request"
		)
		var called int64
		srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			atomic.AddInt64(&called, 1)

			assert.Equal(t, r.Method, http.MethodPost)
			assert.Equal(t, r.URL.Path, "/api/v2/workspaceproxies/me/issue-signed-app-token")
			assert.Equal(t, r.Header.Get(httpmw.WorkspaceProxyAuthTokenHeader), expectedProxyToken)

			rw.WriteHeader(expectedResponseStatus)
			_, _ = rw.Write([]byte(expectedResponseBody))
		}))

		u, err := url.Parse(srv.URL)
		require.NoError(t, err)
		client := wsproxysdk.New(u)
		_ = client.SetSessionToken(expectedProxyToken)

		ctx := testutil.Context(t, testutil.WaitLong)

		rw := newResponseRecorder()
		tokenRes, ok := client.IssueSignedAppTokenHTML(ctx, rw, workspaceapps.IssueTokenRequest{
			AppRequest:   workspaceapps.Request{},
			SessionToken: "user-session-token",
		})
		require.False(t, ok)
		require.Empty(t, tokenRes)
		require.True(t, rw.WasWritten())

		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, expectedResponseStatus, res.StatusCode)
		body, err := io.ReadAll(res.Body)
		require.NoError(t, err)
		require.Equal(t, expectedResponseBody, string(body))

		require.EqualValues(t, called, 1)
	})
}

type ResponseRecorder struct {
	rw         *httptest.ResponseRecorder
	wasWritten atomic.Bool
}

var _ http.ResponseWriter = &ResponseRecorder{}

func newResponseRecorder() *ResponseRecorder {
	return &ResponseRecorder{
		rw: httptest.NewRecorder(),
	}
}

func (r *ResponseRecorder) WasWritten() bool {
	return r.wasWritten.Load()
}

func (r *ResponseRecorder) Result() *http.Response {
	return r.rw.Result()
}

func (r *ResponseRecorder) Flush() {
	r.wasWritten.Store(true)
	r.rw.Flush()
}

func (r *ResponseRecorder) Header() http.Header {
	// Usually when retrieving the headers for the response, it means you're
	// trying to write a header.
	r.wasWritten.Store(true)
	return r.rw.Header()
}

func (r *ResponseRecorder) Write(b []byte) (int, error) {
	r.wasWritten.Store(true)
	return r.rw.Write(b)
}

func (r *ResponseRecorder) WriteHeader(statusCode int) {
	r.wasWritten.Store(true)
	r.rw.WriteHeader(statusCode)
}
