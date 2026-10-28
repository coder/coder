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
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/rbac"
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

	t.Run("PathResolution", func(t *testing.T) {
		t.Parallel()
		for _, tc := range []struct{ arg, want string }{
			{"organizations", "/api/v2/organizations"},
			{"/organizations", "/api/v2/organizations"},
			{"/api/v2/organizations", "/api/v2/organizations"},
			{"/api/experimental/chats", "/api/experimental/chats"},
		} {
			t.Run(tc.arg, func(t *testing.T) {
				t.Parallel()
				srv, rec := newRecordingServer(t)
				inv, root := clitest.New(t, "exp", "api", tc.arg)
				clitest.SetupConfig(t, newTokenClient(t, srv.URL), root)
				require.NoError(t, inv.Run())
				require.Equal(t, tc.want, rec.last().path)
			})
		}
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

	t.Run("TypedFieldsBecomeNestedJSON", func(t *testing.T) {
		t.Parallel()
		srv, rec := newRecordingServer(t)
		path := filepath.Join(t.TempDir(), "script.sh")
		require.NoError(t, os.WriteFile(path, []byte("echo hi\r\n"), 0o600))

		inv, root := clitest.New(t, "exp", "api", "things",
			"-F", "count=5", "-F", "enabled=true", "-F", "off=false", "-F", "nothing=null",
			"-F", "tags[]=a", "-F", "tags[]=2", "-F", "meta[owner][name]=bob",
			"-F", "script=@"+path, "-f", "raw=5", "-f", "at=@literal",
		)
		clitest.SetupConfig(t, newTokenClient(t, srv.URL), root)
		require.NoError(t, inv.Run())

		require.Equal(t, http.MethodPost, rec.last().method)
		require.JSONEq(t, `{
			"count": 5, "enabled": true, "off": false, "nothing": null,
			"tags": ["a", 2], "meta": {"owner": {"name": "bob"}},
			"script": "echo hi\r\n", "raw": "5", "at": "@literal"
		}`, string(rec.last().body))
	})

	t.Run("TypedFieldFromStdin", func(t *testing.T) {
		t.Parallel()
		srv, rec := newRecordingServer(t)

		inv, root := clitest.New(t, "exp", "api", "things", "-F", "content=@-")
		clitest.SetupConfig(t, newTokenClient(t, srv.URL), root)
		inv.Stdin = strings.NewReader("line1\nline2\n")
		require.NoError(t, inv.Run())

		require.JSONEq(t, `{"content":"line1\nline2\n"}`, string(rec.last().body))
	})

	t.Run("InputWithFieldsSendsQuery", func(t *testing.T) {
		t.Parallel()
		srv, rec := newRecordingServer(t)

		inv, root := clitest.New(t, "exp", "api", "things", "--input", "-", "-f", "a=b", "-F", "n=1")
		clitest.SetupConfig(t, newTokenClient(t, srv.URL), root)
		inv.Stdin = strings.NewReader(`{"x":1}`)
		require.NoError(t, inv.Run())

		require.Equal(t, http.MethodPost, rec.last().method)
		require.Equal(t, "b", rec.last().query.Get("a"))
		require.Equal(t, "1", rec.last().query.Get("n"))
		require.JSONEq(t, `{"x":1}`, string(rec.last().body))
	})

	t.Run("JQ", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)

		inv, root := clitest.New(t, "exp", "api", "users/me", "--jq", ".username, {id: .id} | select(. != null)")
		clitest.SetupConfig(t, client, root)
		out := new(bytes.Buffer)
		inv.Stdout = out
		require.NoError(t, inv.Run())

		lines := strings.Split(strings.TrimSpace(out.String()), "\n")
		require.Len(t, lines, 2)
		require.Equal(t, coderdtest.FirstUserParams.Username, lines[0], "strings print without quotes")
		require.True(t, strings.HasPrefix(lines[1], `{"id":"`), lines[1])
	})

	t.Run("PaginateRealEndpoint", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		owner := coderdtest.CreateFirstUser(t, client)
		userAdmin, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.RoleUserAdmin())
		_, _ = coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)

		inv, root := clitest.New(t, "exp", "api", "users", "--paginate", "-f", "limit=1", "--jq", ".users[].username")
		clitest.SetupConfig(t, userAdmin, root)
		out := new(bytes.Buffer)
		inv.Stdout = out
		require.NoError(t, inv.Run())

		lines := strings.Split(strings.TrimSpace(out.String()), "\n")
		require.Len(t, lines, 3, out.String())
		require.Contains(t, lines, coderdtest.FirstUserParams.Username)
	})

	t.Run("Paginate", func(t *testing.T) {
		t.Parallel()
		items := []int{0, 1, 2, 3, 4, 5, 6}
		for _, tc := range []struct {
			name       string
			shape      func(page []int) any
			args       []string
			wantPages  int
			wantOutput string
		}{
			{
				name:       "Array",
				shape:      func(page []int) any { return page },
				args:       []string{"-f", "limit=3"},
				wantPages:  3,
				wantOutput: "[0,1,2]\n[3,4,5]\n[6]\n",
			},
			{
				name:       "ArrayEndsOnEmptyPage",
				shape:      func(page []int) any { return page },
				args:       []string{"-f", "limit=7"},
				wantPages:  2,
				wantOutput: "[0,1,2,3,4,5,6]\n",
			},
			{
				name:       "ObjectWithCountStopsAtTotal",
				shape:      func(page []int) any { return map[string]any{"things": page, "count": len(items)} },
				args:       []string{"-f", "limit=7", "--jq", ".things | length"},
				wantPages:  1,
				wantOutput: "7\n",
			},
			{
				name:       "StartsAtOffset",
				shape:      func(page []int) any { return page },
				args:       []string{"-f", "offset=5"},
				wantPages:  1,
				wantOutput: "[5,6]\n",
			},
		} {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				var (
					mu    sync.Mutex
					pages int
				)
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					mu.Lock()
					pages++
					mu.Unlock()
					assert.Equal(t, http.MethodGet, r.Method)
					offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
					limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
					end := min(offset+limit, len(items))
					page := []int{}
					if offset < len(items) {
						page = items[offset:end]
					}
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(tc.shape(page))
				}))
				t.Cleanup(srv.Close)

				inv, root := clitest.New(t, append([]string{"exp", "api", "things", "--paginate"}, tc.args...)...)
				clitest.SetupConfig(t, newTokenClient(t, srv.URL), root)
				out := new(bytes.Buffer)
				inv.Stdout = out
				require.NoError(t, inv.Run())

				require.Equal(t, tc.wantOutput, out.String())
				mu.Lock()
				defer mu.Unlock()
				require.Equal(t, tc.wantPages, pages)
			})
		}
	})

	t.Run("PaginateStopsWhenOffsetIsIgnored", func(t *testing.T) {
		t.Parallel()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[1,2]`))
		}))
		t.Cleanup(srv.Close)

		inv, root := clitest.New(t, "exp", "api", "things", "--paginate", "-f", "limit=2")
		clitest.SetupConfig(t, newTokenClient(t, srv.URL), root)
		require.ErrorContains(t, inv.Run(), "might not support pagination")
	})

	t.Run("PaginateNonListWarns", func(t *testing.T) {
		t.Parallel()
		srv, _ := newRecordingServer(t)

		inv, root := clitest.New(t, "exp", "api", "things", "--paginate")
		clitest.SetupConfig(t, newTokenClient(t, srv.URL), root)
		out, stderr := new(bytes.Buffer), new(bytes.Buffer)
		inv.Stdout, inv.Stderr = out, stderr
		require.NoError(t, inv.Run())

		require.Equal(t, `{"ok":true}`, out.String())
		require.Contains(t, stderr.String(), "only one page")
	})

	t.Run("FriendlyErrors", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)

		inv, root := clitest.New(t, "exp", "api", "no-such-route")
		clitest.SetupConfig(t, client, root)
		err := inv.Run()
		require.ErrorContains(t, err, "Route not found")
		require.ErrorContains(t, err, "coder.com/docs/reference/api")

		unauthenticated := codersdk.New(client.URL, codersdk.WithSessionToken("bad-token"))
		inv, root = clitest.New(t, "exp", "api", "users/me")
		clitest.SetupConfig(t, unauthenticated, root)
		err = inv.Run()
		require.ErrorContains(t, err, "401")
		require.ErrorContains(t, err, "coder login")
	})

	t.Run("VerboseRedactsToken", func(t *testing.T) {
		t.Parallel()
		srv, _ := newRecordingServer(t)

		inv, root := clitest.New(t, "exp", "api", "things", "--verbose", "-H", "X-Custom: visible")
		clitest.SetupConfig(t, newTokenClient(t, srv.URL), root)
		out, stderr := new(bytes.Buffer), new(bytes.Buffer)
		inv.Stdout, inv.Stderr = out, stderr
		require.NoError(t, inv.Run())

		require.Equal(t, `{"ok":true}`, out.String(), "verbose output must not go to stdout")
		require.Contains(t, stderr.String(), "> GET "+srv.URL+"/api/v2/things")
		require.Contains(t, stderr.String(), "> X-Custom: visible")
		require.Contains(t, stderr.String(), "> Coder-Session-Token: REDACTED")
		require.Contains(t, stderr.String(), "< HTTP/1.1 200 OK")
		require.NotContains(t, stderr.String(), "test-token")
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
			{"TypedFieldWithoutEquals", []string{"-F", "novalue"}, "expected key=value"},
			{"BadFieldKey", []string{"-F", "a[b=1"}, "missing ]"},
			{"FieldKeyConflict", []string{"-F", "a=1", "-F", "a[b]=2"}, "non-object value"},
			{"MissingFieldFile", []string{"-F", "a=@does-not-exist.txt"}, "read value"},
			{"TwoStdinReaders", []string{"--input", "-", "-F", "a=@-"}, "cannot both read from stdin"},
			{"PaginateWithPOST", []string{"-X", "POST", "--paginate"}, "requires the GET method"},
			{"PaginateBadLimit", []string{"--paginate", "-f", "limit=abc"}, "positive limit"},
			{"BadJQ", []string{"--jq", ".["}, "parse --jq expression"},
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
