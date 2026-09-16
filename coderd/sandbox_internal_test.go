package coderd

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/provisioner/sandbox"
)

func TestSandboxTemplateManifest(t *testing.T) {
	t.Parallel()
	manifest := []byte("version: 1\nimage: coder/sandbox@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\n")
	for _, mimeType := range []string{"application/x-tar", "application/zip"} {
		t.Run(mimeType, func(t *testing.T) {
			t.Parallel()
			var buffer bytes.Buffer
			if mimeType == "application/x-tar" {
				writer := tar.NewWriter(&buffer)
				require.NoError(t, writer.WriteHeader(&tar.Header{Name: sandbox.ManifestFilename, Mode: 0o600, Size: int64(len(manifest))}))
				_, err := writer.Write(manifest)
				require.NoError(t, err)
				require.NoError(t, writer.Close())
			} else {
				writer := zip.NewWriter(&buffer)
				file, err := writer.Create(sandbox.ManifestFilename)
				require.NoError(t, err)
				_, err = file.Write(manifest)
				require.NoError(t, err)
				require.NoError(t, writer.Close())
			}
			parsed, err := sandboxTemplateManifest(database.File{Data: buffer.Bytes(), Mimetype: mimeType})
			require.NoError(t, err)
			require.Equal(t, 1, parsed.Version)
			require.Equal(t, int64(2048), parsed.MemoryMiB)
			require.Equal(t, sandbox.HostID, parsed.Tags()[sandbox.HostTag])
		})
	}
	_, err := sandboxTemplateManifest(database.File{Mimetype: "text/plain", Data: manifest})
	require.ErrorContains(t, err, "unsupported sandbox template file type")
}
