package cli_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/codersdk"
)

func TestExpAPI(t *testing.T) {
	t.Parallel()

	t.Run("GetMe", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)

		inv, root := clitest.New(t, "exp", "api", "users/me")
		clitest.SetupConfig(t, client, root)
		out := new(bytes.Buffer)
		inv.Stdout = out
		require.NoError(t, inv.Run())

		var user codersdk.User
		require.NoError(t, json.Unmarshal(out.Bytes(), &user))
		require.Equal(t, coderdtest.FirstUserParams.Username, user.Username)
	})

	t.Run("NotFoundExitsNonZeroAndPrintsBody", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)

		inv, root := clitest.New(t, "exp", "api", "/api/v2/users/does-not-exist")
		clitest.SetupConfig(t, client, root)
		out := new(bytes.Buffer)
		inv.Stdout = out
		err := inv.Run()
		require.ErrorContains(t, err, "404")

		var resp codersdk.Response
		require.NoError(t, json.Unmarshal(out.Bytes(), &resp))
		require.NotEmpty(t, resp.Message)
	})

	t.Run("RejectsAbsoluteURL", func(t *testing.T) {
		t.Parallel()
		srv, rec := newRecordingServer(t)

		inv, root := clitest.New(t, "exp", "api", "https://example.com/api/v2/users/me")
		clitest.SetupConfig(t, newTokenClient(t, srv.URL), root)
		err := inv.Run()
		require.ErrorContains(t, err, "must not include a scheme or host")
		require.Empty(t, rec.last().method, "no request should be sent")
	})

	t.Run("FieldsBecomeQueryForGET", func(t *testing.T) {
		t.Parallel()
		srv, rec := newRecordingServer(t)

		inv, root := clitest.New(t, "exp", "api", "-X", "GET", "workspaces", "-f", "q=owner:me", "-f", "limit=5")
		clitest.SetupConfig(t, newTokenClient(t, srv.URL), root)
		require.NoError(t, inv.Run())

		require.Equal(t, http.MethodGet, rec.last().method)
		require.Equal(t, "/api/v2/workspaces", rec.last().path)
		require.Equal(t, "owner:me", rec.last().query.Get("q"))
		require.Equal(t, "5", rec.last().query.Get("limit"))
		require.Empty(t, rec.last().body)
	})

	t.Run("FieldsBecomeJSONBodyAndDefaultToPOST", func(t *testing.T) {
		t.Parallel()
		srv, rec := newRecordingServer(t)

		inv, root := clitest.New(t, "exp", "api", "users", "-f", "username=alice", "-f", "email=a=b@example.com")
		clitest.SetupConfig(t, newTokenClient(t, srv.URL), root)
		require.NoError(t, inv.Run())

		require.Equal(t, http.MethodPost, rec.last().method)
		require.Equal(t, "application/json", rec.last().header.Get("Content-Type"))
		var body map[string]string
		require.NoError(t, json.Unmarshal(rec.last().body, &body))
		require.Equal(t, map[string]string{"username": "alice", "email": "a=b@example.com"}, body)
	})

	t.Run("InputFromStdinWithMethod", func(t *testing.T) {
		t.Parallel()
		srv, rec := newRecordingServer(t)

		inv, root := clitest.New(t, "exp", "api", "-X", "patch", "workspaces/abc", "--input", "-")
		clitest.SetupConfig(t, newTokenClient(t, srv.URL), root)
		inv.Stdin = strings.NewReader(`{"name":"new"}`)
		require.NoError(t, inv.Run())

		require.Equal(t, http.MethodPatch, rec.last().method)
		require.JSONEq(t, `{"name":"new"}`, string(rec.last().body))
	})

	t.Run("InputFromFile", func(t *testing.T) {
		t.Parallel()
		srv, rec := newRecordingServer(t)
		path := filepath.Join(t.TempDir(), "body.json")
		require.NoError(t, os.WriteFile(path, []byte("{\"a\":1}\r\n"), 0o600))

		inv, root := clitest.New(t, "exp", "api", "things", "--input", path)
		clitest.SetupConfig(t, newTokenClient(t, srv.URL), root)
		require.NoError(t, inv.Run())

		require.Equal(t, http.MethodPost, rec.last().method)
		require.Equal(t, "{\"a\":1}\r\n", string(rec.last().body), "body must be sent unmodified")
	})

	t.Run("RequestHeadersTrimCRLFAndOverrideContentType", func(t *testing.T) {
		t.Parallel()
		srv, rec := newRecordingServer(t)

		inv, root := clitest.New(t, "exp", "api", "things", "--input", "-",
			"-H", "Content-Type: text/plain\r",
			"-H", "X-Custom:  value\r\n",
		)
		clitest.SetupConfig(t, newTokenClient(t, srv.URL), root)
		inv.Stdin = strings.NewReader("hello")
		require.NoError(t, inv.Run())

		require.Equal(t, "text/plain", rec.last().header.Get("Content-Type"))
		require.Equal(t, "value", rec.last().header.Get("X-Custom"))
		require.Equal(t, "test-token", rec.last().header.Get(codersdk.SessionTokenHeader))
	})

	t.Run("IncludePrintsStatusAndHeaders", func(t *testing.T) {
		t.Parallel()
		srv, _ := newRecordingServer(t)

		inv, root := clitest.New(t, "exp", "api", "things", "-i")
		clitest.SetupConfig(t, newTokenClient(t, srv.URL), root)
		out := new(bytes.Buffer)
		inv.Stdout = out
		require.NoError(t, inv.Run())

		require.True(t, strings.HasPrefix(out.String(), "HTTP/1.1 200 OK\n"), out.String())
		require.Contains(t, out.String(), "X-Test: recorded")
		require.True(t, strings.HasSuffix(out.String(), `{"ok":true}`), out.String())
	})

	t.Run("InvalidInputs", func(t *testing.T) {
		t.Parallel()
		for _, tc := range []struct {
			name string
			args []string
			want string
		}{
			{"FieldWithoutEquals", []string{"-f", "novalue"}, "expected key=value"},
			{"HeaderWithoutColon", []string{"-H", "novalue"}, "expected key:value"},
			{"InputAndField", []string{"--input", "-", "-f", "a=b"}, "cannot be used together"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				srv, rec := newRecordingServer(t)
				inv, root := clitest.New(t, append([]string{"exp", "api", "things"}, tc.args...)...)
				clitest.SetupConfig(t, newTokenClient(t, srv.URL), root)
				require.ErrorContains(t, inv.Run(), tc.want)
				require.Empty(t, rec.last().method, "no request should be sent")
			})
		}
	})
}

type recordedRequest struct {
	method string
	path   string
	query  url.Values
	header http.Header
	body   []byte
}

type requestRecorder struct {
	mu  sync.Mutex
	req recordedRequest
}

func (r *requestRecorder) last() recordedRequest {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.req
}

// newRecordingServer returns a server that records the last request it
// received and replies with a fixed JSON body.
func newRecordingServer(t *testing.T) (*httptest.Server, *requestRecorder) {
	t.Helper()
	rec := &requestRecorder{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		assert.NoError(t, err)
		rec.mu.Lock()
		defer rec.mu.Unlock()
		rec.req = recordedRequest{
			method: r.Method,
			path:   r.URL.Path,
			query:  r.URL.Query(),
			header: r.Header.Clone(),
			body:   body,
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Test", "recorded")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(srv.Close)
	return srv, rec
}

func newTokenClient(t *testing.T, rawURL string) *codersdk.Client {
	t.Helper()
	u, err := url.Parse(rawURL)
	require.NoError(t, err)
	return codersdk.New(u, codersdk.WithSessionToken("test-token"))
}
