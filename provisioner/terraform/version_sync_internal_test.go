package terraform

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMaxTerraformVersionTracksBundledMinor(t *testing.T) {
	t.Parallel()

	require.Equal(t, mustMaxPatchVersion(TerraformVersion).String(), maxTerraformVersion.String())
}

func TestBundledTerraformVersionPinsStayInSync(t *testing.T) {
	t.Parallel()

	_, filename, _, ok := runtime.Caller(0)
	require.True(t, ok)

	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
	bundledVersion := TerraformVersion.String()

	tests := []struct {
		name    string
		path    string
		substrs []string
	}{
		{
			name: "setup action",
			path: ".github/actions/setup-tf/action.yaml",
			substrs: []string{
				"terraform_version: " + bundledVersion,
			},
		},
		{
			name: "install script",
			path: "install.sh",
			substrs: []string{
				`TERRAFORM_VERSION="` + bundledVersion + `"`,
			},
		},
		{
			name: "base dockerfile",
			path: "scripts/Dockerfile.base",
			substrs: []string{
				"https://releases.hashicorp.com/terraform/" + bundledVersion + "/terraform_" + bundledVersion + "_linux_${ARCH}.zip",
			},
		},
		{
			name: "terraform testdata version",
			path: "provisioner/terraform/testdata/version.txt",
			substrs: []string{
				bundledVersion,
			},
		},
		{
			name: "dogfood ubuntu 22.04 dockerfile",
			path: "dogfood/coder/ubuntu-22.04/Dockerfile",
			substrs: []string{
				"https://releases.hashicorp.com/terraform/" + bundledVersion + "/terraform_" + bundledVersion + "_linux_amd64.zip",
			},
		},
		{
			name: "dogfood ubuntu 26.04 dockerfile",
			path: "dogfood/coder/ubuntu-26.04/Dockerfile",
			substrs: []string{
				"https://releases.hashicorp.com/terraform/" + bundledVersion + "/terraform_" + bundledVersion + "_linux_amd64.zip",
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			content, err := os.ReadFile(filepath.Join(repoRoot, tt.path))
			require.NoError(t, err)

			for _, substr := range tt.substrs {
				require.Truef(t, strings.Contains(string(content), substr), "%s missing %q", tt.path, substr)
			}
		})
	}
}
