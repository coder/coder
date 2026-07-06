package cli

import (
	"archive/zip"
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/testutil"
)

func TestSafeAgentLogFilesArchiveName(t *testing.T) {
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

			got, ok := safeAgentLogFilesArchiveName(tt.name)
			require.Equal(t, tt.ok, ok)
			if tt.ok {
				require.Equal(t, tt.name, got)
			}
		})
	}
}

func TestWriteAgentLogFilesArchive(t *testing.T) {
	t.Parallel()

	t.Run("UnpacksManifestAndFiles", func(t *testing.T) {
		t.Parallel()

		agentArchive := makeAgentLogFilesArchive(t,
			"files/server.log", "server log",
			"manifest.json", `{"files":[{"archive_path":"files/server.log"}]}`,
			"../escape.log", "should be dropped and recorded",
		)

		var bundle bytes.Buffer
		bundleZip := zip.NewWriter(&bundle)
		require.NoError(t, writeAgentLogFilesArchive(agentArchive, bundleZip, supportBundleAgentLogFilesMaxBytes))
		require.NoError(t, bundleZip.Close())

		entries := testutil.ReadZip(t, bundle.Bytes())
		require.Equal(t, "server log", string(entries["agent/log_files/files/server.log"]))
		require.Contains(t, entries, "agent/log_files/manifest.json")
		require.Contains(t, string(entries["agent/log_files/collection_errors.txt"]), "../escape.log")
		require.Len(t, entries, 3)
	})

	t.Run("SkipsEntriesBeyondBudget", func(t *testing.T) {
		t.Parallel()

		agentArchive := makeAgentLogFilesArchive(t,
			"files/big.log", "this entry is too big",
			"files/small.log", "ok",
		)

		var bundle bytes.Buffer
		bundleZip := zip.NewWriter(&bundle)
		// A 4 byte budget fits small.log but not big.log.
		require.NoError(t, writeAgentLogFilesArchive(agentArchive, bundleZip, 4))
		require.NoError(t, bundleZip.Close())

		entries := testutil.ReadZip(t, bundle.Bytes())
		require.Equal(t, "ok", string(entries["agent/log_files/files/small.log"]))
		require.NotContains(t, entries, "agent/log_files/files/big.log")
		errs := string(entries["agent/log_files/collection_errors.txt"])
		require.Contains(t, errs, "files/big.log")
		require.Contains(t, errs, "budget")
	})

	t.Run("MalformedArchiveDoesNotFail", func(t *testing.T) {
		t.Parallel()

		var bundle bytes.Buffer
		bundleZip := zip.NewWriter(&bundle)
		require.NoError(t, writeAgentLogFilesArchive([]byte("not a zip"), bundleZip, supportBundleAgentLogFilesMaxBytes))
		require.NoError(t, bundleZip.Close())

		entries := testutil.ReadZip(t, bundle.Bytes())
		require.Contains(t, string(entries["agent/log_files/collection_errors.txt"]), "open agent log files archive")
	})
}

// makeAgentLogFilesArchive zips alternating name/content pairs in order.
func makeAgentLogFilesArchive(t *testing.T, pairs ...string) []byte {
	t.Helper()

	require.Zero(t, len(pairs)%2)
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for i := 0; i < len(pairs); i += 2 {
		entry, err := zw.Create(pairs[i])
		require.NoError(t, err)
		_, err = entry.Write([]byte(pairs[i+1]))
		require.NoError(t, err)
	}
	require.NoError(t, zw.Close())
	return buf.Bytes()
}
