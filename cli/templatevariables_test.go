package cli_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/coder/coder/v2/cli"
	"github.com/stretchr/testify/require"
)

func TestDiscoverVarsFiles(t *testing.T) {
	t.Parallel()

	// Given
	tempDir, err := os.MkdirTemp(os.TempDir(), "test-discover-vars-files")
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = os.RemoveAll(tempDir)
	})

	testFiles := []string{
		"terraform.tfvars",              // ok
		"terraform.tfvars.json",         // ok
		"aaa.tf",                        // not Terraform vars
		"bbb.tf",                        // not Terraform vars
		"example.auto.tfvars",           // ok
		"example.auto.tfvars.bak",       // not Terraform vars
		"example.auto.tfvars.json",      // ok
		"example.auto.tfvars.json.bak",  // not Terraform vars
		"other_file.txt",                // not Terraform vars
		"random_file1.tfvars",           // should be .auto.tfvars, otherwise ignored
		"random_file2.tf",               // not Terraform vars
		"random_file2.tfvars.json",      // should be .auto.tfvars.json, otherwise ignored
		"random_file3.auto.tfvars",      // ok
		"random_file3.tf",               // not Terraform vars
		"random_file4.auto.tfvars.json", // ok
	}

	for _, file := range testFiles {
		filePath := filepath.Join(tempDir, file)
		err := os.WriteFile(filePath, []byte(""), 0o600)
		require.NoError(t, err)
	}

	// When
	found, err := cli.DiscoverVarsFiles(tempDir)
	require.NoError(t, err)

	// Then
	expected := []string{
		filepath.Join(tempDir, "terraform.tfvars"),
		filepath.Join(tempDir, "terraform.tfvars.json"),
		filepath.Join(tempDir, "example.auto.tfvars"),
		filepath.Join(tempDir, "example.auto.tfvars.json"),
		filepath.Join(tempDir, "random_file3.auto.tfvars"),
		filepath.Join(tempDir, "random_file4.auto.tfvars.json"),
	}
	require.EqualValues(t, expected, found)
}
