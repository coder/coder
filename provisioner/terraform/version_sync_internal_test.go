package terraform

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/hashicorp/go-version"
	"github.com/stretchr/testify/require"
)

func TestTerraformVersionLine(t *testing.T) {
	t.Parallel()

	require.Equal(
		t,
		"1.14",
		terraformVersionLine(version.Must(version.NewVersion("1.14.5"))),
	)
}

func TestIsNewerTerraformVersionLine(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		installed string
		bundled   string
		want      bool
	}{
		{
			name:      "same patch release",
			installed: "1.14.5",
			bundled:   "1.14.5",
			want:      false,
		},
		{
			name:      "same major minor with higher patch",
			installed: "1.14.10",
			bundled:   "1.14.5",
			want:      false,
		},
		{
			name:      "newer minor release",
			installed: "1.15.0",
			bundled:   "1.14.5",
			want:      true,
		},
		{
			name:      "older minor release",
			installed: "1.13.9",
			bundled:   "1.14.5",
			want:      false,
		},
		{
			name:      "newer major release",
			installed: "2.0.0",
			bundled:   "1.14.5",
			want:      true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			installed := version.Must(version.NewVersion(tt.installed))
			bundled := version.Must(version.NewVersion(tt.bundled))

			require.Equal(t, tt.want, isNewerTerraformVersionLine(installed, bundled))
		})
	}
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

func TestIronBankVersionFileStaysInSync(t *testing.T) {
	t.Parallel()

	_, filename, _, ok := runtime.Caller(0)
	require.True(t, ok)

	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
	content, err := os.ReadFile(filepath.Join(repoRoot, "provisioner/terraform/ironbank_versions.json"))
	require.NoError(t, err)

	var versions struct {
		BundledTerraformVersion string `json:"bundled_terraform_version"`
		SupportedTerraformLine  string `json:"supported_terraform_line"`
	}
	require.NoError(t, json.Unmarshal(content, &versions))

	require.Equal(t, TerraformVersion.String(), versions.BundledTerraformVersion)
	require.Equal(t, terraformVersionLine(TerraformVersion), versions.SupportedTerraformLine)
}
