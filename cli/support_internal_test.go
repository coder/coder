package cli

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSafeAgentLogFilesArchiveName(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name string
		want string
		ok   bool
	}{
		{name: "manifest.json", want: "manifest.json", ok: true},
		{name: "files/server.log", want: "files/server.log", ok: true},
		{name: "./files/server.log", want: "files/server.log", ok: true},
		{name: "../manifest.json", ok: false},
		{name: "/manifest.json", ok: false},
		{name: "files/nested/../server.log", ok: false},
		{name: "files/../../manifest.json", ok: false},
		{name: "files\\nested\\server.log", ok: false},
		{name: "other/server.log", ok: false},
	} {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, ok := safeAgentLogFilesArchiveName(tt.name)
			require.Equal(t, tt.ok, ok)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestWriteAgentLogFilesArchiveLimits(t *testing.T) {
	t.Parallel()

	t.Run("AllowsManifestBeyondLogLimit", func(t *testing.T) {
		t.Parallel()

		var agentArchive bytes.Buffer
		agentZip := zip.NewWriter(&agentArchive)
		entry, err := agentZip.Create("files/server.log")
		require.NoError(t, err)
		_, err = entry.Write([]byte("1234"))
		require.NoError(t, err)
		entry, err = agentZip.Create("manifest.json")
		require.NoError(t, err)
		_, err = entry.Write([]byte(`{"files":[{"archive_path":"files/server.log"}]}`))
		require.NoError(t, err)
		require.NoError(t, agentZip.Close())

		var bundle bytes.Buffer
		bundleZip := zip.NewWriter(&bundle)
		require.NoError(t, writeAgentLogFilesArchiveWithLimits(agentArchive.Bytes(), bundleZip, 4, 1024))
		require.NoError(t, bundleZip.Close())

		entries := readBundleEntries(t, bundle.Bytes())
		require.Equal(t, "1234", string(entries["agent/log_files/files/server.log"]))
		require.Contains(t, entries, "agent/log_files/manifest.json")
		require.NotContains(t, entries, "agent/log_files/collection_errors.txt")
	})

	t.Run("SkipsOversizedLogEntries", func(t *testing.T) {
		t.Parallel()

		var agentArchive bytes.Buffer
		agentZip := zip.NewWriter(&agentArchive)
		entry, err := agentZip.Create("files/server.log")
		require.NoError(t, err)
		_, err = entry.Write([]byte("server log"))
		require.NoError(t, err)
		require.NoError(t, agentZip.Close())

		archiveBytes := agentArchive.Bytes()
		centralDir := bytes.Index(archiveBytes, []byte{'P', 'K', 0x01, 0x02})
		require.NotEqual(t, -1, centralDir)
		binary.LittleEndian.PutUint32(archiveBytes[centralDir+24:centralDir+28], uint32(supportBundleAgentLogFilesMaxTotal+1))

		var bundle bytes.Buffer
		bundleZip := zip.NewWriter(&bundle)
		require.NoError(t, writeAgentLogFilesArchive(archiveBytes, bundleZip))
		require.NoError(t, bundleZip.Close())

		// The oversized entry is dropped, but the bundle still succeeds and
		// records why.
		entries := readBundleEntries(t, bundle.Bytes())
		require.NotContains(t, entries, "agent/log_files/files/server.log")
		require.Contains(t, string(entries["agent/log_files/collection_errors.txt"]), "files/server.log")
		require.Contains(t, string(entries["agent/log_files/collection_errors.txt"]), "exceeds")
	})

	t.Run("KeepsValidEntriesWhenOneIsOversized", func(t *testing.T) {
		t.Parallel()

		var agentArchive bytes.Buffer
		agentZip := zip.NewWriter(&agentArchive)
		for name, content := range map[string]string{
			"files/small.log": "ok",
			"files/big.log":   "this entry is too big",
		} {
			entry, err := agentZip.Create(name)
			require.NoError(t, err)
			_, err = entry.Write([]byte(content))
			require.NoError(t, err)
		}
		require.NoError(t, agentZip.Close())

		var bundle bytes.Buffer
		bundleZip := zip.NewWriter(&bundle)
		// A 4 byte log budget fits small.log but not big.log, regardless of
		// the order they're encountered in.
		require.NoError(t, writeAgentLogFilesArchiveWithLimits(agentArchive.Bytes(), bundleZip, 4, 1024))
		require.NoError(t, bundleZip.Close())

		entries := readBundleEntries(t, bundle.Bytes())
		require.Equal(t, "ok", string(entries["agent/log_files/files/small.log"]))
		require.NotContains(t, entries, "agent/log_files/files/big.log")
		require.Contains(t, string(entries["agent/log_files/collection_errors.txt"]), "files/big.log")
	})

	t.Run("MalformedArchiveDoesNotFail", func(t *testing.T) {
		t.Parallel()

		var bundle bytes.Buffer
		bundleZip := zip.NewWriter(&bundle)
		require.NoError(t, writeAgentLogFilesArchive([]byte("not a zip"), bundleZip))
		require.NoError(t, bundleZip.Close())

		entries := readBundleEntries(t, bundle.Bytes())
		require.Contains(t, string(entries["agent/log_files/collection_errors.txt"]), "open agent log files archive")
	})
}

func readBundleEntries(t *testing.T, data []byte) map[string][]byte {
	t.Helper()

	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	require.NoError(t, err)
	entries := map[string][]byte{}
	for _, file := range zr.File {
		rc, err := file.Open()
		require.NoError(t, err)
		content, err := io.ReadAll(rc)
		_ = rc.Close()
		require.NoError(t, err)
		entries[file.Name] = content
	}
	return entries
}
