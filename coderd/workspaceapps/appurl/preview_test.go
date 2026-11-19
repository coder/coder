package appurl_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/workspaceapps/appurl"
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
