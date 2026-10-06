package workspaceapps

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogjson"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/tracing"
	"github.com/coder/coder/v2/coderd/workspaceapps/appurl"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
	"github.com/coder/websocket"
)

// Test_originLocalURL checks that originLocalURL produces a redirect target that
// stays on the current origin.
func Test_originLocalURL(t *testing.T) {
	t.Parallel()

	t.Run("RejectsOffOrigin", func(t *testing.T) {
		t.Parallel()

		// Each path models an already-percent-decoded r.URL.Path that tries to
		// smuggle a separate host into the redirect.
		cases := []struct {
			name string
			path string
		}{
			{name: "DoubleSlash", path: "//evil.com/phish"},
			{name: "TripleSlash", path: "///evil.com/phish"},
			{name: "SlashBackslash", path: "/\\evil.com/phish"},
			{name: "SlashBackslashSlash", path: "/\\/evil.com/phish"},
			{name: "DoubleBackslash", path: "\\\\evil.com/phish"},
			{name: "SlashTab", path: "/\t/evil.com/phish"},
			{name: "SlashTabBackslash", path: "/\t\\evil.com/phish"},
			{name: "SlashNewline", path: "/\n/evil.com/phish"},
			{name: "SlashCarriageReturn", path: "/\r/evil.com/phish"},
			{name: "SlashTabDoubleSlash", path: "/\t//evil.com/phish"},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				loc := originLocalURL(tc.path).String()

				// The Location must parse as a relative, same-origin reference.
				require.Falsef(t, strings.HasPrefix(loc, "//"),
					"path %q produced scheme-relative Location %q", tc.path, loc)
				parsed, err := url.Parse(loc)
				require.NoErrorf(t, err, "path %q produced unparseable Location %q", tc.path, loc)
				require.Emptyf(t, parsed.Scheme, "path %q produced Location %q with a scheme", tc.path, loc)
				require.Emptyf(t, parsed.Host, "path %q produced Location %q with a host", tc.path, loc)

				// It must also be free of raw bytes a browser would normalize back
				// into an authority before resolving (a backslash becomes "/", and
				// tab/newline/CR are stripped, either of which could re-form
				// "//host"). url.URL.String() guarantees this by percent-encoding
				// them; we assert it here rather than reproducing browser
				// normalization in the code.
				for _, raw := range []string{`\`, "\t", "\n", "\r"} {
					require.NotContainsf(t, loc, raw,
						"path %q produced Location %q containing a raw %q", tc.path, loc, raw)
				}
			})
		}
	})

	t.Run("EscapesControlCharacters", func(t *testing.T) {
		t.Parallel()

		// A redirect built from a path containing a raw control character is an
		// open redirect: http.Redirect emits it verbatim (url.Parse rejects the
		// control byte and skips cleaning) and browsers strip tab/newline/CR
		// before resolving, re-forming "//evil.com". originLocalURL percent-encodes
		// each one. Assert every class is escaped so a future change that breaks
		// encoding for only one class is caught.
		cases := []struct {
			name string
			in   string
			want string
		}{
			{name: "Tab", in: "/\t/evil.com", want: "/%09/evil.com"},
			{name: "Newline", in: "/\n/evil.com", want: "/%0A/evil.com"},
			{name: "CarriageReturn", in: "/\r/evil.com", want: "/%0D/evil.com"},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				require.Equalf(t, tc.want, originLocalURL(tc.in).String(),
					"originLocalURL(%q) must percent-encode the control character", tc.in)
			})
		}
	})

	t.Run("PreservesLegitPaths", func(t *testing.T) {
		t.Parallel()

		cases := []struct {
			name string
			in   string
			want string
		}{
			{name: "Empty", in: "", want: "/"},
			{name: "Root", in: "/", want: "/"},
			{name: "Simple", in: "/test", want: "/test"},
			{name: "Nested", in: "/app/sub/page", want: "/app/sub/page"},
			{name: "PathApp", in: "/@user/ws/apps/app", want: "/@user/ws/apps/app"},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				require.Equalf(t, tc.want, originLocalURL(tc.in).String(), "originLocalURL(%q)", tc.in)
			})
		}
	})
}

// fakeTokenProvider returns the same token for every request.
type fakeTokenProvider struct {
	token *SignedToken
}

func (p fakeTokenProvider) FromRequest(*http.Request) (*SignedToken, bool) {
	return p.token, true
}

func (fakeTokenProvider) Issue(context.Context, http.ResponseWriter, *http.Request, IssueTokenRequest) (*SignedToken, string, bool) {
	panic("unexpected Issue call")
}

// unreachableAgentProvider fails every AgentConn with AgentUnreachableError.
type unreachableAgentProvider struct {
	fields []slog.Field
}

func (unreachableAgentProvider) ReverseProxy(*url.URL, *url.URL, uuid.UUID, appurl.ApplicationURL, string) *httputil.ReverseProxy {
	panic("unexpected ReverseProxy call")
}

func (p unreachableAgentProvider) AgentConn(context.Context, uuid.UUID) (workspacesdk.AgentConn, func(), error) {
	return nil, nil, &AgentUnreachableError{Fields: p.fields}
}

func (unreachableAgentProvider) ServeHTTPDebug(http.ResponseWriter, *http.Request) {}

func (unreachableAgentProvider) Close() error { return nil }

func TestWorkspaceAgentPTY_AgentUnreachable(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	logs := testutil.NewWaitBuffer()
	logger := slog.Make(slogjson.Sink(logs)).Leveled(slog.LevelDebug)

	agentID := uuid.New()
	basePath := fmt.Sprintf("/api/v2/workspaceagents/%s/pty", agentID)
	accessURL, err := url.Parse("http://coder.test")
	require.NoError(t, err)

	s := NewServer(ServerOptions{
		Logger:       logger,
		DashboardURL: accessURL,
		AccessURL:    accessURL,
		SignedTokenProvider: fakeTokenProvider{token: &SignedToken{
			Request: Request{
				AccessMethod:  AccessMethodTerminal,
				BasePath:      basePath,
				AgentNameOrID: agentID.String(),
			},
			AgentID: agentID,
		}},
		AgentProvider: unreachableAgentProvider{fields: []slog.Field{
			slog.F("agent_id", agentID),
			slog.F("reason", "no_node"),
		}},
		WSWatcher: httpapi.NewWSWatcher(quartz.NewReal(), nil),
	})
	r := chi.NewRouter()
	r.Use(tracing.StatusWriterMiddleware, tracing.Middleware(nil, tracing.DefaultRoutePatterns, "coderd"))
	r.Get("/api/v2/workspaceagents/{workspaceagent}/pty", s.workspaceAgentPTY)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + basePath + "?reconnect=" + uuid.NewString()
	// The response body is nil after a successful upgrade.
	conn, _, err := websocket.Dial(ctx, wsURL, nil) //nolint:bodyclose
	require.NoError(t, err)
	defer conn.Close(websocket.StatusNormalClosure, "")

	// The handler closes the socket once the dial fails.
	_, _, err = conn.Read(ctx)
	var closeErr websocket.CloseError
	require.ErrorAs(t, err, &closeErr)
	require.Equal(t, websocket.StatusInternalError, closeErr.Code)
	require.Contains(t, closeErr.Reason, "agent is unreachable")

	var found bool
	for _, line := range strings.Split(strings.TrimSpace(logs.String()), "\n") {
		var entry struct {
			Level  string         `json:"level"`
			Msg    string         `json:"msg"`
			Fields map[string]any `json:"fields"`
		}
		require.NoError(t, json.Unmarshal([]byte(line), &entry), line)
		if entry.Msg != "agent is unreachable" {
			continue
		}
		found = true
		require.Equal(t, "WARN", entry.Level)
		require.Equal(t, agentID.String(), entry.Fields["agent_id"])
		require.Equal(t, "no_node", entry.Fields["reason"])
	}
	require.True(t, found, "no agent is unreachable log line")
}
