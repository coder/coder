package cli

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/provisioner/sandbox"
	"github.com/coder/coder/v2/provisionersdk"
)

const archiveSandboxManifest = "version: 1\nimage: coder/sandbox@sha256:" +
	"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\n"

func TestTemplateArchiveProvisioner(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name        string
		provisioner codersdk.ProvisionerType
		files       map[string]string
		wantError   string
	}{
		{name: "SandboxYAMLOnly", provisioner: codersdk.ProvisionerTypeSandbox, files: map[string]string{"sandbox.yaml": archiveSandboxManifest}},
		{name: "TerraformRequiresSource", provisioner: codersdk.ProvisionerTypeTerraform, files: map[string]string{"sandbox.yaml": archiveSandboxManifest}, wantError: "has no [.tf .tf.json] files"},
		{name: "TerraformIgnoresNativeManifest", provisioner: codersdk.ProvisionerTypeTerraform, files: map[string]string{"main.tf": "", "sandbox.yaml": "invalid"}},
		{name: "SandboxRequiresManifest", provisioner: codersdk.ProvisionerTypeSandbox, files: map[string]string{"main.tf": ""}, wantError: "sandbox.yaml"},
		{name: "SandboxRejectsInvalidManifest", provisioner: codersdk.ProvisionerTypeSandbox, files: map[string]string{"sandbox.yaml": "version: 1\nimage: coder/sandbox:latest\n"}, wantError: "invalid sandbox template"},
		{name: "SandboxBoundedArchive", provisioner: codersdk.ProvisionerTypeSandbox, files: map[string]string{"sandbox.yaml": archiveSandboxManifest, "large": strings.Repeat("x", provisionersdk.TemplateArchiveLimit)}, wantError: "Archive too big"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			for name, contents := range tc.files {
				require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(contents), 0o600))
			}
			var archive bytes.Buffer
			err := archiveTemplateDirectory(&archive, slogtest.Make(t, nil), dir, tc.provisioner)
			if tc.wantError != "" {
				require.ErrorContains(t, err, tc.wantError)
				return
			}
			require.NoError(t, err)
			require.Positive(t, archive.Len())
			if tc.provisioner == codersdk.ProvisionerTypeSandbox {
				manifest, err := sandbox.ReadManifestArchive(archive.Bytes())
				require.NoError(t, err)
				require.Equal(t, int32(1), manifest.DailyCost)
			}
		})
	}
}

func TestTemplateArchiveRejectsManifestSymlink(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "manifest"), []byte(archiveSandboxManifest), 0o600))
	require.NoError(t, os.Symlink("manifest", filepath.Join(dir, "sandbox.yaml")))
	err := archiveTemplateDirectory(io.Discard, slogtest.Make(t, nil), dir, codersdk.ProvisionerTypeSandbox)
	require.ErrorContains(t, err, "sandbox manifest must be a regular file")
}
