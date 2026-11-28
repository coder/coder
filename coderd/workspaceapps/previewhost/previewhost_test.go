package previewhost_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/workspaceapps/appurl"
	"github.com/coder/coder/v2/coderd/workspaceapps/previewhost"
)

func TestHostingURL(t *testing.T) {
	t.Parallel()

	app := appurl.ApplicationURL{AppSlugOrPort: "preview--8080", AgentName: "main", Username: "alice"}
	for _, tc := range []struct {
		name          string
		accessURL     string
		wildcard      string
		workspaceName string
		urlPath       string
		want          string
		wantErr       string
	}{
		{name: "Root", wildcard: "*.apps.example.com", workspaceName: "dev", want: "https://preview--8080--main--dev--alice.apps.example.com:8443/"},
		{name: "PathAndQuery", wildcard: "*.apps.example.com", workspaceName: "dev", urlPath: "/docs/a%20b.html?page=2", want: "https://preview--8080--main--dev--alice.apps.example.com:8443/docs/a%20b.html?page=2"},
		{name: "Suffix", wildcard: "*--apps.example.com", workspaceName: "dev", want: "https://preview--8080--main--dev--alice--apps.example.com:8443/"},
		// Browsers send no port in the Host header for the scheme's default.
		{name: "DefaultPort", accessURL: "https://coder.example.com:443", wildcard: "*.apps.example.com", workspaceName: "dev", want: "https://preview--8080--main--dev--alice.apps.example.com/"},
		{name: "DNSLimit", wildcard: "*.apps.example.com", workspaceName: strings.Repeat("w", 35), want: "https://preview--8080--main--" + strings.Repeat("w", 35) + "--alice.apps.example.com:8443/"},
		{name: "OverDNSLimit", wildcard: "*.apps.example.com", workspaceName: strings.Repeat("w", 36), wantErr: "over the DNS limit of 63"},
		{name: "OverDNSLimitWithSuffix", wildcard: "*--apps.example.com", workspaceName: strings.Repeat("w", 30), wantErr: "over the DNS limit of 63"},
		{name: "OtherHost", wildcard: "*.apps.example.com", workspaceName: "dev", urlPath: "//evil.example.com/", wantErr: "must start with a single /"},
		{name: "Fragment", wildcard: "*.apps.example.com", workspaceName: "dev", urlPath: "/docs#top", wantErr: "must start with a single /"},
		{name: "BadEscape", wildcard: "*.apps.example.com", workspaceName: "dev", urlPath: "/a%2", wantErr: "valid percent-encoding"},
		{name: "SignInParam", wildcard: "*.apps.example.com", workspaceName: "dev", urlPath: "/?page=2&" + previewhost.SignInParam + "=x", wantErr: previewhost.SignInParam},
		{name: "AppAPIKeyParam", wildcard: "*.apps.example.com", workspaceName: "dev", urlPath: "/?" + appurl.SubdomainProxyAPIKeyParam + "=x", wantErr: appurl.SubdomainProxyAPIKeyParam},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rawAccessURL := tc.accessURL
			if rawAccessURL == "" {
				rawAccessURL = "https://coder.example.com:8443"
			}
			accessURL, err := url.Parse(rawAccessURL)
			require.NoError(t, err)
			hosting, err := previewhost.Strict(tc.wildcard, accessURL, accessURL)
			require.NoError(t, err)
			app := app
			app.WorkspaceName = tc.workspaceName
			got, err := hosting.URL(app, tc.urlPath)
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, got.String())
		})
	}
}

// A name that leaves its own "--" segment of the hostname label would make
// the preview URL name another app or another host.
func TestHostingURLRefusesAppsThatDoNotParseBack(t *testing.T) {
	t.Parallel()

	// With no port in the access URL, a ":" in a name could pass for one.
	accessURL, err := url.Parse("https://coder.example.com")
	require.NoError(t, err)
	hosting, err := previewhost.Strict("*.apps.example.com", accessURL, accessURL)
	require.NoError(t, err)
	valid := appurl.ApplicationURL{AppSlugOrPort: "preview--0123abcd", AgentName: "main", WorkspaceName: "dev", Username: "alice"}
	_, err = hosting.URL(valid, "/")
	require.NoError(t, err)
	for _, tc := range []struct {
		name   string
		mutate func(*appurl.ApplicationURL)
	}{
		{name: "AgentNameEndsInOtherHost", mutate: func(a *appurl.ApplicationURL) { a.AgentName = "main--dev--bob.apps.example.com:1" }},
		{name: "DotInAgentName", mutate: func(a *appurl.ApplicationURL) { a.AgentName = "main.apps" }},
		{name: "ColonInWorkspaceName", mutate: func(a *appurl.ApplicationURL) { a.WorkspaceName = "dev:1" }},
		{name: "DotInUsername", mutate: func(a *appurl.ApplicationURL) { a.Username = "alice.example" }},
		{name: "SeparatorInAgentName", mutate: func(a *appurl.ApplicationURL) { a.AgentName = "main--dev" }},
		{name: "PrefixSeparatorInAgentName", mutate: func(a *appurl.ApplicationURL) { a.AgentName = "x---main" }},
		{name: "NoAgent", mutate: func(a *appurl.ApplicationURL) { a.AgentName = "" }},
		{name: "Prefix", mutate: func(a *appurl.ApplicationURL) { a.Prefix = "x---" }},
		// A port app's hostname parses back unchanged but is not a preview.
		{name: "PortSlug", mutate: func(a *appurl.ApplicationURL) { a.AppSlugOrPort = "8080" }},
		{name: "PortWithLeadingZero", mutate: func(a *appurl.ApplicationURL) { a.AppSlugOrPort = "preview--08080" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			app := valid
			tc.mutate(&app)
			_, err := hosting.URL(app, "/")
			require.ErrorContains(t, err, "is not a valid preview hostname")
		})
	}
}

func TestHostingCoversAndParse(t *testing.T) {
	t.Parallel()

	accessURL, err := url.Parse("https://coder.example.com")
	require.NoError(t, err)
	hosting, err := previewhost.Strict("*.apps.example.com", accessURL, accessURL)
	require.NoError(t, err)

	app, target, err := hosting.Parse("preview--8080--main--dev--alice.apps.example.com:443")
	require.NoError(t, err)
	require.Equal(t, appurl.ApplicationURL{AppSlugOrPort: "preview--8080", AgentName: "main", WorkspaceName: "dev", Username: "alice"}, app)
	require.Equal(t, appurl.PreviewTarget{Port: 8080}, target)
	require.True(t, hosting.Covers("preview--8080--main--dev--alice.apps.example.com"))

	// A prefixed preview host is covered, so its error page carries the
	// policy, but it does not name a preview.
	prefixed := "x---preview--8080--main--dev--alice.apps.example.com"
	require.True(t, hosting.Covers(prefixed))
	_, _, err = hosting.Parse(prefixed)
	require.Error(t, err)

	// Neither an app whose slug is "preview" nor another hostname is covered.
	for _, host := range []string{
		"preview--main--dev--alice.apps.example.com",
		"preview--8080--main--dev--alice.other.example.com",
	} {
		require.False(t, hosting.Covers(host), host)
		_, _, err := hosting.Parse(host)
		require.Error(t, err, host)
	}
}

// Writing a body or flushing without WriteHeader sends the header, so the
// policy must be applied first.
func TestWrapWithoutWriteHeader(t *testing.T) {
	t.Parallel()

	accessURL, err := url.Parse("https://coder.example.com")
	require.NoError(t, err)
	hosting, err := previewhost.Strict("*.apps.example.com", accessURL, accessURL)
	require.NoError(t, err)
	policy := http.Header{}
	hosting.ApplyPolicy(policy)

	for name, send := range map[string]func(http.ResponseWriter){
		"Write": func(rw http.ResponseWriter) { _, _ = rw.Write([]byte("x")) },
		"Flush": func(rw http.ResponseWriter) { rw.(http.Flusher).Flush() },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			rec := httptest.NewRecorder()
			rw := hosting.Wrap(rec)
			rw.Header().Set("Link", "<https://other.example.com/x.js>; rel=preload; as=script")
			rw.Header().Set("Clear-Site-Data", `"cookies"`)
			rw.Header().Add("Set-Cookie", "host=1; Path=/")
			rw.Header().Add("Set-Cookie", "wide=1; Path=/; Domain=example.com")
			send(rw)

			res := rec.Result()
			require.Equal(t, policy.Values("Content-Security-Policy"), res.Header.Values("Content-Security-Policy"))
			require.Len(t, res.Header.Values("Content-Security-Policy"), 1)
			require.Equal(t, []string{"host=1; Path=/"}, res.Header.Values("Set-Cookie"))
			require.Empty(t, res.Header.Values("Link"))
			require.Empty(t, res.Header.Values("Clear-Site-Data"))
		})
	}
}
