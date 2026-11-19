package appurl_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/workspaceapps/appurl"
	"github.com/coder/coder/v2/provisioner"
)

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
		require.Equal(t, slug, got.Slug())
	}

	// A port has one hostname, so leading zeros are refused.
	for _, slug := range []string{"preview--08080", "preview--0", "preview--65536", "preview--8080s", "preview--", "8080", "preview--0123ABCD"} {
		_, ok := appurl.ParsePreviewSlug(slug)
		require.False(t, ok, slug)
	}
}

func TestIsPreviewHostname(t *testing.T) {
	t.Parallel()

	for _, hostname := range []string{
		"preview--0123abcd--agent--workspace--user.apps.example.com",
		"preview--8080--agent--workspace--user.apps.example.com",
		"PREVIEW--8080--agent--workspace--user.apps.example.com",
		"preview--8080--agent--workspace--user--sydney.example.com",
		"x---preview--8080--agent--workspace--user.apps.example.com",
	} {
		require.True(t, appurl.IsPreviewHostname(hostname), hostname)
	}
	for _, hostname := range []string{
		"8080--agent--workspace--user.apps.example.com",
		"preview--agent--workspace--user.apps.example.com",
		"preview--8080--workspace--user.apps.example.com",
		"apps.example.com",
	} {
		require.False(t, appurl.IsPreviewHostname(hostname), hostname)
	}
}
