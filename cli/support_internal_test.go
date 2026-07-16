package cli

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/testutil"
)

func TestSafeWorkspaceFilesArchiveEntryName(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name string
		ok   bool
	}{
		{name: "manifest.json", ok: true},
		{name: "files/server.log", ok: true},
		{name: "./files/server.log", ok: false},
		{name: "../manifest.json", ok: false},
		{name: "/manifest.json", ok: false},
		{name: "files/nested/../server.log", ok: false},
		{name: "files/../../manifest.json", ok: false},
		{name: "files\\nested\\server.log", ok: false},
		{name: `files/nested\..\server.log`, ok: false},
		{name: "other/server.log", ok: false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, ok := safeWorkspaceFilesArchiveEntryName(tt.name)
			require.Equal(t, tt.ok, ok)
			if tt.ok {
				require.Equal(t, tt.name, got)
			}
		})
	}
}

func TestWriteWorkspaceFilesArchive(t *testing.T) {
	t.Parallel()

	t.Run("UnpacksManifestAndFiles", func(t *testing.T) {
		t.Parallel()

		agentArchive := makeWorkspaceFilesArchive(t,
			"files/server.log", "server log",
			"manifest.json", `{"files":[{"archive_path":"files/server.log"}]}`,
			"../escape.log", "should be dropped and recorded",
		)

		var bundle bytes.Buffer
		bundleZip := zip.NewWriter(&bundle)
		require.NoError(t, writeWorkspaceFilesArchive(agentArchive, bundleZip, supportBundleWorkspaceFilesMaxBytes))
		require.NoError(t, bundleZip.Close())

		entries := testutil.ReadZip(t, bundle.Bytes())
		require.Equal(t, "server log", string(entries["agent/workspace_files/files/server.log"]))
		require.Contains(t, entries, "agent/workspace_files/manifest.json")
		require.Contains(t, string(entries["agent/workspace_files/collection_errors.txt"]), "../escape.log")
		require.Len(t, entries, 3)
	})

	t.Run("AbortsOnEntryBeyondBudget", func(t *testing.T) {
		t.Parallel()

		agentArchive := makeWorkspaceFilesArchive(t,
			"files/ok.log", "ok",
			"files/big.log", "this entry is too big",
			"files/after.log", "never reached",
		)

		var bundle bytes.Buffer
		bundleZip := zip.NewWriter(&bundle)
		// A 4 byte budget fits ok.log; big.log exceeds it and aborts the
		// rest.
		require.NoError(t, writeWorkspaceFilesArchive(agentArchive, bundleZip, 4))
		require.NoError(t, bundleZip.Close())

		entries := testutil.ReadZip(t, bundle.Bytes())
		require.Equal(t, "ok", string(entries["agent/workspace_files/files/ok.log"]))
		require.NotContains(t, entries, "agent/workspace_files/files/big.log")
		require.NotContains(t, entries, "agent/workspace_files/files/after.log")
		errs := string(entries["agent/workspace_files/collection_errors.txt"])
		require.Contains(t, errs, "files/big.log")
		require.Contains(t, errs, "budget")
	})

	t.Run("MalformedArchiveDoesNotFail", func(t *testing.T) {
		t.Parallel()

		var bundle bytes.Buffer
		bundleZip := zip.NewWriter(&bundle)
		require.NoError(t, writeWorkspaceFilesArchive([]byte("not a tar"), bundleZip, supportBundleWorkspaceFilesMaxBytes))
		require.NoError(t, bundleZip.Close())

		entries := testutil.ReadZip(t, bundle.Bytes())
		require.Contains(t, string(entries["agent/workspace_files/collection_errors.txt"]), "read workspace files archive")
	})
}

// makeWorkspaceFilesArchive tars alternating name/content pairs in order.
func makeWorkspaceFilesArchive(t *testing.T, pairs ...string) []byte {
	t.Helper()

	require.Zero(t, len(pairs)%2)
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for i := 0; i < len(pairs); i += 2 {
		require.NoError(t, tw.WriteHeader(&tar.Header{
			Name: pairs[i],
			Mode: 0o644,
			Size: int64(len(pairs[i+1])),
		}))
		_, err := tw.Write([]byte(pairs[i+1]))
		require.NoError(t, err)
	}
	require.NoError(t, tw.Close())
	return buf.Bytes()
}
