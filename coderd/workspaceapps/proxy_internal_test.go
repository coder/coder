package workspaceapps

import (
	"context"
	"net"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/workspaceapps/appurl"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/testutil"
)

type appTransportProvider struct {
	AgentProvider
	transport *workspacesdk.AgentAppTransport
}

func (p appTransportProvider) AppTransport(uuid.UUID) *workspacesdk.AgentAppTransport {
	return p.transport
}

// TestServer_reverseProxyTargetsAgentAddress checks that the proxy sends every
// app URL form to the address its transport accepts.
func TestServer_reverseProxyTargetsAgentAddress(t *testing.T) {
	t.Parallel()

	agentID := uuid.New()
	transport := workspacesdk.NewAgentAppTransport(agentID, func(context.Context, uint16) (net.Conn, error) {
		return nil, xerrors.New("unused")
	}, testutil.Logger(t))
	s := &Server{ServerOptions: ServerOptions{
		AgentProvider: appTransportProvider{transport: transport},
		DashboardURL:  &url.URL{Scheme: "https", Host: "coder.example.com"},
	}}
	addr := transport.Addr().String()

	for _, tc := range []struct {
		appURL string
		want   string
	}{
		{appURL: "http://localhost:8080", want: "http://[" + addr + "]:8080"},
		{appURL: "http://127.0.0.1:3000/path", want: "http://[" + addr + "]:3000"},
		{appURL: "http://[::1]:3000", want: "http://[" + addr + "]:3000"},
		{appURL: "https://example.com:8443", want: "https://[" + addr + "]:8443"},
		// No port: the transport dials the scheme's default port.
		{appURL: "http://localhost", want: "http://[" + addr + "]:"},
	} {
		t.Run(tc.appURL, func(t *testing.T) {
			t.Parallel()

			u, err := url.Parse(tc.appURL)
			require.NoError(t, err)
			rp := s.reverseProxy(u, agentID, appurl.ApplicationURL{})
			require.Same(t, transport, rp.Transport)

			req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://app.example.com/", nil)
			require.NoError(t, err)
			rp.Director(req)
			require.Equal(t, tc.want, req.URL.Scheme+"://"+req.URL.Host)
		})
	}
}

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
