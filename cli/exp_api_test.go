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

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
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
		require.NoError(t, inv.WithContext(testutil.Context(t, testutil.WaitLong)).Run())

		var user codersdk.User
		require.NoError(t, json.Unmarshal(out.Bytes(), &user))
		require.Equal(t, coderdtest.FirstUserParams.Username, user.Username)
	})

	t.Run("QueryItemsFilterRealEndpoint", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		owner := coderdtest.CreateFirstUser(t, client)
		userAdmin, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.RoleUserAdmin())
		_, other := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)

		inv, root := clitest.New(t, "exp", "api", "get", "users?limit=10", "q==username:"+other.Username)
		clitest.SetupConfig(t, userAdmin, root)
		out := new(bytes.Buffer)
		inv.Stdout = out
		require.NoError(t, inv.WithContext(testutil.Context(t, testutil.WaitLong)).Run())

		var resp codersdk.GetUsersResponse
		require.NoError(t, json.Unmarshal(out.Bytes(), &resp))
		require.Len(t, resp.Users, 1)
		require.Equal(t, other.Username, resp.Users[0].Username)
	})

	t.Run("PostDataItemsCreateUser", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		owner := coderdtest.CreateFirstUser(t, client)
		userAdmin, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.RoleUserAdmin())

		inv, root := clitest.New(t, "exp", "api", "users",
			"username=api-alice",
			"email=api-alice@coder.com",
			"password=SomeSecurePassword!",
			"user_status=active",
			`organization_ids:=["`+owner.OrganizationID.String()+`"]`,
		)
		clitest.SetupConfig(t, userAdmin, root)
		out := new(bytes.Buffer)
		inv.Stdout = out
		ctx := testutil.Context(t, testutil.WaitLong)
		require.NoError(t, inv.WithContext(ctx).Run())

		var created codersdk.User
		require.NoError(t, json.Unmarshal(out.Bytes(), &created))
		require.Equal(t, "api-alice", created.Username)
		require.Equal(t, codersdk.UserStatusActive, created.Status)
		require.Equal(t, []uuid.UUID{owner.OrganizationID}, created.OrganizationIDs)

		user, err := client.User(ctx, "api-alice")
		require.NoError(t, err)
		require.Equal(t, created.ID, user.ID)
	})

	t.Run("NotFoundRouteHint", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)

		inv, root := clitest.New(t, "exp", "api", "no-such-route")
		clitest.SetupConfig(t, client, root)
		out := new(bytes.Buffer)
		inv.Stdout = out
		err := inv.WithContext(testutil.Context(t, testutil.WaitLong)).Run()
		require.ErrorContains(t, err, "Route not found")
		require.ErrorContains(t, err, "coder.com/docs/reference/api")

		var resp codersdk.Response
		require.NoError(t, json.Unmarshal(out.Bytes(), &resp), "error body must be printed to stdout")
		require.Equal(t, "Route not found.", resp.Message)
	})

	t.Run("NonSuccessExitsNonZeroAndPrintsBody", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)

		inv, root := clitest.New(t, "exp", "api", "users/does-not-exist")
		clitest.SetupConfig(t, client, root)
		out := new(bytes.Buffer)
		inv.Stdout = out
		err := inv.WithContext(testutil.Context(t, testutil.WaitLong)).Run()
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusNotFound, sdkErr.StatusCode())
		require.NotContains(t, err.Error(), "coder.com/docs/reference/api", "the hint is only for unknown routes")

		var resp codersdk.Response
		require.NoError(t, json.Unmarshal(out.Bytes(), &resp))
		require.NotEmpty(t, resp.Message)
	})

	t.Run("Requests", func(t *testing.T) {
		t.Parallel()
		inputFile := filepath.Join(t.TempDir(), "body.json")
		require.NoError(t, os.WriteFile(inputFile, []byte("{\"a\":1}\r\n"), 0o600))

		for _, tc := range []struct {
			name      string
			args      []string
			stdin     string
			want      recordedRequest
			wantQuery string
			wantJSON  string
		}{
			{
				name: "RelativePath",
				args: []string{"organizations"},
				want: recordedRequest{method: http.MethodGet, path: "/api/v2/organizations"},
			},
			{
				name: "LeadingSlash",
				args: []string{"/organizations"},
				want: recordedRequest{method: http.MethodGet, path: "/api/v2/organizations"},
			},
			{
				name: "APIPathAsGiven",
				args: []string{"/api/experimental/chats"},
				want: recordedRequest{method: http.MethodGet, path: "/api/experimental/chats"},
			},
			{
				name: "MethodIsUppercased",
				args: []string{"delete", "workspaces/abc"},
				want: recordedRequest{method: http.MethodDelete, path: "/api/v2/workspaces/abc"},
			},
			{
				name: "HeadMethod",
				args: []string{"Head", "users"},
				want: recordedRequest{method: http.MethodHead, path: "/api/v2/users"},
			},
			{
				name: "PathNamedLikeNonMethod",
				args: []string{"users", "limit==1"},
				want: recordedRequest{method: http.MethodGet, path: "/api/v2/users"},
				// A query item never changes the default method.
				wantQuery: "limit=1",
			},
			{
				name:      "QueryItemsAppendToPathQuery",
				args:      []string{"workspaces?q=owner:me&limit=5", "offset==10", "q==status:running"},
				want:      recordedRequest{method: http.MethodGet, path: "/api/v2/workspaces"},
				wantQuery: "q=owner:me&limit=5&offset=10&q=status%3Arunning",
			},
			{
				name: "DataItemsDefaultToPOST",
				args: []string{"users", "username=alice", "count:=5"},
				want: recordedRequest{
					method: http.MethodPost, path: "/api/v2/users",
					header: http.Header{"Content-Type": {"application/json"}},
				},
				wantJSON: `{"count":5,"username":"alice"}`,
			},
			{
				name:     "DataItemsWithExplicitMethod",
				args:     []string{"PUT", "things/1", "enabled:=false"},
				want:     recordedRequest{method: http.MethodPut, path: "/api/v2/things/1"},
				wantJSON: `{"enabled":false}`,
			},
			{
				name: "InputFromFileUnmodified",
				args: []string{"things", "--input", inputFile},
				want: recordedRequest{
					method: http.MethodPost, path: "/api/v2/things",
					body: []byte("{\"a\":1}\r\n"),
				},
			},
			{
				name:  "InputFromStdinWithQueryAndHeader",
				args:  []string{"PATCH", "workspaces/abc", "--input", "-", "dry_run==true", "Content-Type:text/plain"},
				stdin: "hello",
				want: recordedRequest{
					method: http.MethodPatch, path: "/api/v2/workspaces/abc",
					header: http.Header{"Content-Type": {"text/plain"}},
					body:   []byte("hello"),
				},
				wantQuery: "dry_run=true",
			},
			{
				name: "HeaderItemsReachServer",
				args: []string{"things", "X-Custom: value", "X-Multi:a", "X-Multi:b"},
				want: recordedRequest{
					method: http.MethodGet, path: "/api/v2/things",
					header: http.Header{"X-Custom": {"value"}, "X-Multi": {"a", "b"}},
				},
			},
		} {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				srv, rec := newRecordingServer(t)
				inv, root := clitest.New(t, append([]string{"exp", "api"}, tc.args...)...)
				clitest.SetupConfig(t, newTokenClient(t, srv.URL), root)
				inv.Stdin = strings.NewReader(tc.stdin)
				out := new(bytes.Buffer)
				inv.Stdout = out
				require.NoError(t, inv.WithContext(testutil.Context(t, testutil.WaitShort)).Run())

				got := rec.last()
				require.Equal(t, tc.want.method, got.method)
				require.Equal(t, tc.want.path, got.path)
				require.Equal(t, tc.wantQuery, got.rawQuery)
				require.Equal(t, "test-token", got.header.Get(codersdk.SessionTokenHeader))
				for k, vs := range tc.want.header {
					require.Equal(t, vs, got.header.Values(k), k)
				}
				if tc.wantJSON != "" {
					require.JSONEq(t, tc.wantJSON, string(got.body))
				} else {
					require.Equal(t, string(tc.want.body), string(got.body), "body must be sent unmodified")
				}
				if tc.want.method != http.MethodHead {
					require.Equal(t, `{"ok":true}`, out.String(), "response body must be printed unchanged")
				}
			})
		}
	})

	t.Run("GlobalHeaderFlag", func(t *testing.T) {
		t.Parallel()
		srv, rec := newRecordingServer(t)
		inv, root := clitest.New(t, "--header", "X-Global=yes", "exp", "api", "things", "X-Item:also")
		clitest.SetupConfig(t, newTokenClient(t, srv.URL), root)
		require.NoError(t, inv.WithContext(testutil.Context(t, testutil.WaitShort)).Run())
		require.Equal(t, "yes", rec.last().header.Get("X-Global"))
		require.Equal(t, "also", rec.last().header.Get("X-Item"))
	})

	t.Run("Errors", func(t *testing.T) {
		t.Parallel()
		for _, tc := range []struct {
			name string
			args []string
			want string
		}{
			{"AbsoluteURL", []string{"https://example.com/api/v2/users/me"}, "must not include a scheme or host"},
			{"SchemeRelativeURL", []string{"//example.com/api/v2/users/me"}, "must not include a scheme or host"},
			{"AbsoluteURLAfterMethod", []string{"GET", "http://example.com/users"}, "must not include a scheme or host"},
			{"MethodWithoutPath", []string{"POST"}, "missing path"},
			{"InputWithDataItems", []string{"things", "--input", "-", "a=b"}, "--input cannot be combined with data items"},
			{"StdinTwice", []string{"things", "--input", "-", "a=@-"}, "cannot both read stdin"},
			{"InvalidJSON", []string{"things", "a:=nope"}, "not valid JSON"},
			{"DuplicateKey", []string{"things", "a=1", "a=2"}, "duplicate data field"},
			{"InvalidItem", []string{"things", "novalue"}, "invalid request item"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				srv, rec := newRecordingServer(t)
				inv, root := clitest.New(t, append([]string{"exp", "api"}, tc.args...)...)
				clitest.SetupConfig(t, newTokenClient(t, srv.URL), root)
				inv.Stdin = strings.NewReader("")
				require.ErrorContains(t, inv.WithContext(testutil.Context(t, testutil.WaitShort)).Run(), tc.want)
				require.Empty(t, rec.last().method, "no request should be sent")
			})
		}
	})

	t.Run("WarnsOnInsecureURL", func(t *testing.T) {
		t.Parallel()
		u, err := url.Parse("http://coder.invalid")
		require.NoError(t, err)

		inv, root := clitest.New(t, "exp", "api", "users/me")
		clitest.SetupConfig(t, codersdk.New(u, codersdk.WithSessionToken("test-token")), root)
		stderr := new(bytes.Buffer)
		inv.Stderr = stderr
		// The host does not resolve, so the request fails. The warning is
		// printed before the request is sent.
		require.Error(t, inv.WithContext(testutil.Context(t, testutil.WaitShort)).Run())
		require.Contains(t, stderr.String(), "unencrypted HTTP to coder.invalid")
	})
}

type recordedRequest struct {
	method   string
	path     string
	rawQuery string
	header   http.Header
	body     []byte
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
			method:   r.Method,
			path:     r.URL.Path,
			rawQuery: r.URL.RawQuery,
			header:   r.Header.Clone(),
			body:     body,
		}
		w.Header().Set("Content-Type", "application/json")
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
