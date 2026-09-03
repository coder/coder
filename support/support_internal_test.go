package support

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/testutil"
)

func TestUnsupportedWorkspaceFilesArchive(t *testing.T) {
	t.Parallel()

	patterns := []string{"~/a.log", "~/logs/**/*.log"}
	entries := testutil.ReadTar(t, unsupportedWorkspaceFilesArchive(patterns))

	var manifest workspacesdk.BundleFilesManifest
	require.NoError(t, json.Unmarshal(entries["manifest.json"], &manifest))
	require.Equal(t, patterns, manifest.Requested)
	require.Len(t, manifest.Errors, 1)
	require.Contains(t, manifest.Errors[0].Reason, "not supported")
	require.Empty(t, manifest.Files)
	require.Len(t, entries, 1)
}
