package workspaceartifacts_test

import (
	"archive/tar"
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/workspaceartifacts"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

type archiveFile struct {
	name string
	data []byte
}

func makeBundle(t *testing.T, entries []archiveFile, manifest *workspacesdk.BundleFilesManifest) []byte {
	t.Helper()
	var buf bytes.Buffer
	writer := tar.NewWriter(&buf)
	if manifest != nil {
		data, err := json.Marshal(manifest)
		require.NoError(t, err)
		entries = append(entries, archiveFile{"manifest.json", data})
	}
	for _, file := range entries {
		require.NoError(t, writer.WriteHeader(&tar.Header{Name: file.name, Mode: 0o600, Size: int64(len(file.data))}))
		_, err := writer.Write(file.data)
		require.NoError(t, err)
	}
	require.NoError(t, writer.Close())
	return buf.Bytes()
}

func TestValidateBundleRejectsIncompleteResults(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		mutate func(*workspacesdk.BundleFilesManifest, *[]archiveFile)
	}{
		{"manifest_truncated", func(m *workspacesdk.BundleFilesManifest, _ *[]archiveFile) { m.Truncated = true }},
		{"missing_file", func(_ *workspacesdk.BundleFilesManifest, f *[]archiveFile) { *f = nil }},
		{"traversal", func(m *workspacesdk.BundleFilesManifest, f *[]archiveFile) {
			(*f)[0].name = "files/../result.bin"
			m.Files[0].ArchivePath = (*f)[0].name
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			manifest := workspacesdk.BundleFilesManifest{
				Requested: []string{"/result.bin"},
				Files: []workspacesdk.BundleFilesManifestEntry{{
					Requested: "/result.bin", Path: "/result.bin", ArchivePath: "files/result.bin", Size: 3, BytesWritten: 3,
				}},
			}
			entries := []archiveFile{{"files/result.bin", []byte{0, 1, 255}}}
			tc.mutate(&manifest, &entries)
			_, err := workspaceartifacts.ValidateBundle(makeBundle(t, entries, &manifest), []string{"/result.bin"})
			require.Error(t, err)
		})
	}
}

func TestValidateBundleRequiresCompleteArchive(t *testing.T) {
	t.Parallel()
	manifest := workspacesdk.BundleFilesManifest{Requested: []string{}}
	bundle := makeBundle(t, nil, &manifest)
	_, err := workspaceartifacts.ValidateBundle(bundle, nil)
	require.NoError(t, err)
	for _, bad := range [][]byte{
		bundle[:len(bundle)-512],
		makeBundle(t, nil, nil),
		append(append([]byte{}, bundle...), make([]byte, 511)...),
		append(append(append([]byte{}, bundle...), 1), make([]byte, 1535)...),
	} {
		_, err := workspaceartifacts.ValidateBundle(bad, nil)
		require.Error(t, err)
	}
}
