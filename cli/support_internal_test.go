package cli

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
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
		err = writeAgentLogFilesArchiveWithLimits(agentArchive.Bytes(), bundleZip, 4, 1024)
		require.NoError(t, err)
	})

	t.Run("RejectsOversizedLogEntries", func(t *testing.T) {
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
		err = writeAgentLogFilesArchive(archiveBytes, bundleZip)
		require.ErrorContains(t, err, "agent log files archive exceeds")
	})
}
