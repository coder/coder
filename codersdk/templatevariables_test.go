package codersdk_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
)

func TestDiscoverVarsFiles(t *testing.T) {
	t.Parallel()

	// Given
	tempDir, err := os.MkdirTemp(os.TempDir(), "test-discover-vars-files-*")
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
	found, err := codersdk.DiscoverVarsFiles(tempDir)
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
		jsonContent4 = `{"dog": 4, "go_image": "[\"1.19\",\"1.20\"]"}`
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
	actual, err := codersdk.ParseUserVariableValues([]string{
		filepath.Join(tempDir, hclFilename1),
		filepath.Join(tempDir, hclFilename2),
		filepath.Join(tempDir, jsonFilename3),
		filepath.Join(tempDir, jsonFilename4),
	}, "", nil)
	require.NoError(t, err)

	// Then
	expected := []codersdk.VariableValue{
		{Name: "cat", Value: "foobar"},
		{Name: "cores", Value: "3"},
		{Name: "dog", Value: "4"},
		{Name: "go_image", Value: "[\"1.19\",\"1.20\"]"},
		{Name: "region", Value: "us-west-2"},
	}
	require.Equal(t, expected, actual)
}

func TestParseVariableValuesFromVarsFiles_InvalidJSON(t *testing.T) {
	t.Parallel()

	// Given
	const (
		jsonFilename = "file.tfvars.json"
		jsonContent  = `{"cat": "foobar", cores: 3}` // invalid content: no quotes around "cores"
	)

	// Prepare the .tfvars files
	tempDir, err := os.MkdirTemp(os.TempDir(), "test-parse-variable-values-from-vars-files-invalid-json-*")
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = os.RemoveAll(tempDir)
	})

	err = os.WriteFile(filepath.Join(tempDir, jsonFilename), []byte(jsonContent), 0o600)
	require.NoError(t, err)

	// When
	actual, err := codersdk.ParseUserVariableValues([]string{
		filepath.Join(tempDir, jsonFilename),
	}, "", nil)

	// Then
	require.Nil(t, actual)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unable to parse JSON content")
}

func TestParseVariableValuesFromVarsFiles_InvalidHCL(t *testing.T) {
	t.Parallel()

	// Given
	const (
		hclFilename = "file.tfvars"
		hclContent  = `region = "us-east-1"
cores: 2`
	)

	// Prepare the .tfvars files
	tempDir, err := os.MkdirTemp(os.TempDir(), "test-parse-variable-values-from-vars-files-invalid-hcl-*")
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = os.RemoveAll(tempDir)
	})

	err = os.WriteFile(filepath.Join(tempDir, hclFilename), []byte(hclContent), 0o600)
	require.NoError(t, err)

	// When
	actual, err := codersdk.ParseUserVariableValues([]string{
		filepath.Join(tempDir, hclFilename),
	}, "", nil)

	// Then
	require.Nil(t, actual)
	require.Error(t, err)
	require.Contains(t, err.Error(), `use the equals sign "=" to introduce the argument value`)
}

func TestParseVariableValuesFromVarsFiles_MapOfStrings(t *testing.T) {
	t.Parallel()

	// Given
	const (
		hclFilename = "file.tfvars"
		hclContent  = `region = "us-east-1"
default_tags = {
  owner_name  = "John Doe"
  owner_email = "john@example.com"
  owner_slack = "@johndoe"
}`
	)

	// Prepare the .tfvars files
	tempDir, err := os.MkdirTemp(os.TempDir(), "test-parse-variable-values-from-vars-files-map-of-strings-*")
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = os.RemoveAll(tempDir)
	})

	err = os.WriteFile(filepath.Join(tempDir, hclFilename), []byte(hclContent), 0o600)
	require.NoError(t, err)

	// When
	actual, err := codersdk.ParseUserVariableValues([]string{
		filepath.Join(tempDir, hclFilename),
	}, "", nil)

	// Then
	require.NoError(t, err)
	require.Len(t, actual, 2)

	// Results are sorted by name
	require.Equal(t, "default_tags", actual[0].Name)
	require.JSONEq(t, `{"owner_email":"john@example.com","owner_name":"John Doe","owner_slack":"@johndoe"}`, actual[0].Value)
	require.Equal(t, "region", actual[1].Name)
	require.Equal(t, "us-east-1", actual[1].Value)
}

func TestParseVariableValuesFromVarsFiles_MapWithNonStringValues(t *testing.T) {
	t.Parallel()

	// Given - a map with non-string values should error
	const (
		hclFilename = "file.tfvars"
		hclContent  = `config = {
  name  = "test"
  count = 5
}`
	)

	// Prepare the .tfvars files
	tempDir, err := os.MkdirTemp(os.TempDir(), "test-parse-variable-values-from-vars-files-map-non-string-*")
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = os.RemoveAll(tempDir)
	})

	err = os.WriteFile(filepath.Join(tempDir, hclFilename), []byte(hclContent), 0o600)
	require.NoError(t, err)

	// When
	actual, err := codersdk.ParseUserVariableValues([]string{
		filepath.Join(tempDir, hclFilename),
	}, "", nil)

	// Then
	require.Nil(t, actual)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unsupported map value type")
}
