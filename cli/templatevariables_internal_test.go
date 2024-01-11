package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseVariableValuesFromVarsFiles(t *testing.T) {
	// Given
	tempDir, err := os.MkdirTemp(os.TempDir(), "test-parse-variable-values-from-vars-files-*")
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = os.RemoveAll(tempDir)
	})

	const (
		hclFile1  = "file1.hcl"
		jsonFile1 = "file1.json"
		hclFile2  = "file2.hcl"
		jsonFile2 = "file2.json"
	)

	err = os.WriteFile(hclFile1, []byte("s"), 0o600)

	// When
	actual, err := parseVariableValuesFromVarsFiles([]string{
		filepath.Join(tempDir, hclFile1),
		filepath.Join(tempDir, jsonFile1),
		filepath.Join(tempDir, hclFile2),
		filepath.Join(tempDir, jsonFile2),
	})
	require.NoError(t, err)

	// Then

}
