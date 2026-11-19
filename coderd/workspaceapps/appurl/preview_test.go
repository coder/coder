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
