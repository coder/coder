package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
)

func TestParseVariableValuesFromVarsFiles(t *testing.T) {
	t.Parallel()

	// Given
	const (
		hclFilename1  = "file1.tfvars"
		hclFilename2  = "file2.tfvars"
		jsonFilename3 = "file3.tfvars.json"
		jsonFilename4 = "file4.tfvars.json"

		hclContent1 = `region = "us-east-1"
cores = 2`
		hclContent2 = `region = "us-west-2"
go_image = ["1.19","1.20","1.21"]`
		jsonContent3 = `{"cat": "foobar", "cores": 3}`
		jsonContent4 = `{"dog": 4}`
	)

	// Prepare the .tfvars files
	tempDir, err := os.MkdirTemp(os.TempDir(), "test-parse-variable-values-from-vars-files-*")
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = os.RemoveAll(tempDir)
	})

	err = os.WriteFile(filepath.Join(tempDir, hclFilename1), []byte(hclContent1), 0o600)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(tempDir, hclFilename2), []byte(hclContent2), 0o600)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(tempDir, jsonFilename3), []byte(jsonContent3), 0o600)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(tempDir, jsonFilename4), []byte(jsonContent4), 0o600)
	require.NoError(t, err)

	// When
	actual, err := parseVariableValuesFromVarsFiles([]string{
		filepath.Join(tempDir, hclFilename1),
		filepath.Join(tempDir, hclFilename2),
		filepath.Join(tempDir, jsonFilename3),
		filepath.Join(tempDir, jsonFilename4),
	})
	require.NoError(t, err)

	// Then
	expected := []codersdk.VariableValue{
		{Name: "cores", Value: "2"},
		{Name: "region", Value: "us-east-1"},
		{Name: "go_image", Value: "[\"1.19\",\"1.20\",\"1.21\"]"},
		{Name: "region", Value: "us-west-2"},
		{Name: "cat", Value: "foobar"},
		{Name: "cores", Value: "3"},
		{Name: "dog", Value: "4"}}
	require.Equal(t, expected, actual)
}
