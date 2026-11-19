package previewhost_test

import (
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/workspaceapps/appurl"
	"github.com/coder/coder/v2/coderd/workspaceapps/previewhost"
)

func TestHostingURL(t *testing.T) {
	t.Parallel()

	accessURL, err := url.Parse("https://coder.example.com:8443")
	require.NoError(t, err)
	app := appurl.ApplicationURL{AppSlugOrPort: "preview--8080", AgentName: "main", Username: "alice"}
	for _, tc := range []struct {
		name          string
		wildcard      string
		workspaceName string
		urlPath       string
		want          string
		wantErr       string
	}{
		{name: "Root", wildcard: "*.apps.example.com", workspaceName: "dev", want: "https://preview--8080--main--dev--alice.apps.example.com:8443/"},
		{name: "PathAndQuery", wildcard: "*.apps.example.com", workspaceName: "dev", urlPath: "/docs/a%20b.html?page=2", want: "https://preview--8080--main--dev--alice.apps.example.com:8443/docs/a%20b.html?page=2"},
		{name: "Suffix", wildcard: "*--apps.example.com", workspaceName: "dev", want: "https://preview--8080--main--dev--alice--apps.example.com:8443/"},
		{name: "DNSLimit", wildcard: "*.apps.example.com", workspaceName: strings.Repeat("w", 35), want: "https://preview--8080--main--" + strings.Repeat("w", 35) + "--alice.apps.example.com:8443/"},
		{name: "OverDNSLimit", wildcard: "*.apps.example.com", workspaceName: strings.Repeat("w", 36), wantErr: "over the DNS limit of 63"},
		{name: "OverDNSLimitWithSuffix", wildcard: "*--apps.example.com", workspaceName: strings.Repeat("w", 30), wantErr: "over the DNS limit of 63"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

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

	// Hosts that look like previews but do not parse are still covered, so
	// their error pages carry the policy.
	for _, host := range []string{
		"preview--08080--main--dev--alice.apps.example.com",
		"preview--zz--main--dev--alice.apps.example.com",
		"x---preview--8080--main--dev--alice.apps.example.com",
	} {
		require.True(t, hosting.Covers(host), host)
		_, _, err := hosting.Parse(host)
		require.Error(t, err, host)
	}

	for _, host := range []string{
		"8080--main--dev--alice.apps.example.com",
		"preview--8080--main--dev--alice.other.example.com",
		"coder.example.com",
	} {
		require.False(t, hosting.Covers(host), host)
	}
}
