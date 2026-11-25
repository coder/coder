package terraform_test

import (
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/provisioner/terraform"
)

// scripts/Dockerfile.base and install.sh verify the bundled Terraform against
// terraform_SHA256SUMS, so it must be for the same version as TerraformVersion
// and cover the platforms the Coder images are built for.
func TestTerraformChecksumFileMatchesVersion(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("terraform_SHA256SUMS")
	require.NoError(t, err)

	want := terraform.TerraformVersion.String()
	lineRe := regexp.MustCompile(`^[0-9a-f]{64}  terraform_([0-9]+\.[0-9]+\.[0-9]+)_[a-z0-9]+_[a-z0-9]+\.zip$`)
	lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	for _, line := range lines {
		m := lineRe.FindStringSubmatch(line)
		require.NotNil(t, m, "terraform_SHA256SUMS line is not in SHA256SUMS format: %q", line)
		require.Equal(t, want, m[1], "terraform_SHA256SUMS is for a different Terraform version than TerraformVersion")
	}
	for _, arch := range []string{"amd64", "arm64", "arm"} {
		suffix := "  terraform_" + want + "_linux_" + arch + ".zip"
		require.True(t, slices.ContainsFunc(lines, func(line string) bool { return strings.HasSuffix(line, suffix) }),
			"terraform_SHA256SUMS has no entry for linux_%s, which scripts/Dockerfile.base builds for", arch)
	}
}
