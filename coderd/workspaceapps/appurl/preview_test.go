package appurl_test

import (
	"encoding/json"
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/workspaceapps/appurl"
	"github.com/coder/coder/v2/provisioner"
)

func TestNewPreviewID(t *testing.T) {
	t.Parallel()

	// Saved preview tabs and links hold this ID, so it must not change.
	require.Equal(t, appurl.PreviewID("092f225d"), appurl.NewPreviewID("/home/coder/page.html"))
}

func TestPreviewIDUnmarshalText(t *testing.T) {
	t.Parallel()

	var id appurl.PreviewID
	require.NoError(t, json.Unmarshal([]byte(`"0123abcd"`), &id))
	require.Equal(t, appurl.PreviewID("0123abcd"), id)

	for _, raw := range []string{`"0123ABCD"`, `"0123abc"`, `"0123abcd0"`, `"evil.example.com/"`} {
		require.Error(t, json.Unmarshal([]byte(raw), &id), raw)
	}
}

func TestPreviewSlugIsNeverAnAppSlug(t *testing.T) {
	t.Parallel()

	for _, id := range []appurl.PreviewID{"00000000", "ffffffff", appurl.NewPreviewID("/home/coder/site")} {
		require.False(t, provisioner.AppSlugRegex.MatchString(id.Slug()), id.Slug())
	}
	for _, slug := range []string{"preview--9", "preview--8080", "preview--65535"} {
		require.False(t, provisioner.AppSlugRegex.MatchString(slug), slug)
	}
}

func TestParsePreviewSlug(t *testing.T) {
	t.Parallel()

	for slug, want := range map[string]appurl.PreviewTarget{
		"preview--0123abcd": {ID: "0123abcd"},
		"preview--8080":     {Port: 8080},
		"preview--9":        {Port: 9},
		"preview--65535":    {Port: 65535},
	} {
		got, ok := appurl.ParsePreviewSlug(slug)
		require.True(t, ok, slug)
		require.Equal(t, want, got, slug)
	}

	// A port has one hostname, so leading zeros are refused.
	for _, slug := range []string{"preview--08080", "preview--0", "preview--65536", "preview--8080s", "preview--", "8080", "preview--0123ABCD"} {
		_, ok := appurl.ParsePreviewSlug(slug)
		require.False(t, ok, slug)
	}
}

func TestParsePreviewHostname(t *testing.T) {
	t.Parallel()

	dotPattern, err := appurl.CompileHostnamePattern("*.apps.example.com")
	require.NoError(t, err)
	suffixPattern, err := appurl.CompileHostnamePattern("*--apps.example.com")
	require.NoError(t, err)
	portPreview := appurl.ApplicationURL{AppSlugOrPort: "preview--8080", AgentName: "agent", WorkspaceName: "workspace", Username: "user"}
	for _, tc := range []struct {
		name       string
		pattern    *regexp.Regexp
		hostname   string
		wantApp    appurl.ApplicationURL
		wantTarget appurl.PreviewTarget
	}{
		{
			name:       "FilePreview",
			pattern:    dotPattern,
			hostname:   "preview--0123abcd--agent--workspace--user.apps.example.com",
			wantApp:    appurl.ApplicationURL{AppSlugOrPort: "preview--0123abcd", AgentName: "agent", WorkspaceName: "workspace", Username: "user"},
			wantTarget: appurl.PreviewTarget{ID: "0123abcd"},
		},
		{name: "PortPreview", pattern: dotPattern, hostname: "preview--8080--agent--workspace--user.apps.example.com", wantApp: portPreview, wantTarget: appurl.PreviewTarget{Port: 8080}},
		{name: "UpperCaseWithPort", pattern: dotPattern, hostname: "PREVIEW--8080--agent--workspace--user.apps.example.com:8443", wantApp: portPreview, wantTarget: appurl.PreviewTarget{Port: 8080}},
		{name: "SuffixPattern", pattern: suffixPattern, hostname: "preview--8080--agent--workspace--user--apps.example.com", wantApp: portPreview, wantTarget: appurl.PreviewTarget{Port: 8080}},
		{
			name:       "Prefixed",
			pattern:    dotPattern,
			hostname:   "x---preview--8080--agent--workspace--user.apps.example.com",
			wantApp:    appurl.ApplicationURL{Prefix: "x---", AppSlugOrPort: "preview--8080", AgentName: "agent", WorkspaceName: "workspace", Username: "user"},
			wantTarget: appurl.PreviewTarget{Port: 8080},
		},
		// An app whose slug is "preview" is not a preview, whatever its agent
		// is named.
		{name: "AppNamedPreview", pattern: dotPattern, hostname: "preview--agent--workspace--user.apps.example.com"},
		{name: "AppNamedPreviewWithoutAgent", pattern: dotPattern, hostname: "preview--workspace--user.apps.example.com"},
		{name: "AppNamedPreviewOnAgentNamedLikePort", pattern: dotPattern, hostname: "preview--8080--workspace--user.apps.example.com"},
		{name: "AppNamedPreviewUnderSuffixPattern", pattern: suffixPattern, hostname: "preview--8080--workspace--user--apps.example.com"},
		{name: "InvalidPreviewSlug", pattern: dotPattern, hostname: "preview--garbage--agent--workspace--user.apps.example.com"},
		{name: "PortWithLeadingZero", pattern: dotPattern, hostname: "preview--08080--agent--workspace--user.apps.example.com"},
		{name: "ExtraPart", pattern: dotPattern, hostname: "preview--8080--agent--workspace--user--extra.apps.example.com"},
		{name: "PortApp", pattern: dotPattern, hostname: "8080--agent--workspace--user.apps.example.com"},
		{name: "OtherHostname", pattern: dotPattern, hostname: "preview--8080--agent--workspace--user.other.example.com"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			app, target, ok := appurl.ParsePreviewHostname(tc.pattern, tc.hostname)
			require.Equal(t, tc.wantApp != appurl.ApplicationURL{}, ok)
			require.Equal(t, tc.wantApp, app)
			require.Equal(t, tc.wantTarget, target)
		})
	}
}
