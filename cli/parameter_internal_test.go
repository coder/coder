package cli

import (
	"os"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCreateParameterMapFromFile(t *testing.T) {
	t.Parallel()
	t.Run("CreateParameterMapFromFile", func(t *testing.T) {
		t.Parallel()
		tempDir := t.TempDir()
		parameterFile, _ := os.CreateTemp(tempDir, "testParameterFile*.yaml")
		_, _ = parameterFile.WriteString("region: \"bananas\"\ndisk: \"20\"\n")

		parameterMapFromFile, err := parseParameterMapFile(parameterFile.Name())

		expectedMap := map[string]string{
			"region": "bananas",
			"disk":   "20",
		}

		assert.Equal(t, expectedMap, parameterMapFromFile)
		assert.Nil(t, err)

		removeTmpDirUntilSuccess(t, tempDir)
	})
	t.Run("WithInvalidFilename", func(t *testing.T) {
		t.Parallel()

		parameterMapFromFile, err := parseParameterMapFile("invalidFile.yaml")

		assert.Nil(t, parameterMapFromFile)

		// On Unix based systems, it is: `open invalidFile.yaml: no such file or directory`
		// On Windows, it is `open invalidFile.yaml: The system cannot find the file specified.`
		if runtime.GOOS == "windows" {
			assert.EqualError(t, err, "open invalidFile.yaml: The system cannot find the file specified.")
		} else {
			assert.EqualError(t, err, "open invalidFile.yaml: no such file or directory")
		}
	})
	t.Run("WithInvalidYAML", func(t *testing.T) {
		t.Parallel()
		tempDir := t.TempDir()
		parameterFile, _ := os.CreateTemp(tempDir, "testParameterFile*.yaml")
		_, _ = parameterFile.WriteString("region = \"bananas\"\ndisk = \"20\"\n")

		parameterMapFromFile, err := parseParameterMapFile(parameterFile.Name())

		assert.Nil(t, parameterMapFromFile)
		assert.EqualError(t, err, "yaml: unmarshal errors:\n  line 1: cannot unmarshal !!str `region ...` into map[string]interface {}")

		removeTmpDirUntilSuccess(t, tempDir)
	})
}

// Need this for Windows because of a known issue with Go:
// https://github.com/golang/go/issues/52986
func removeTmpDirUntilSuccess(t *testing.T, tempDir string) {
	t.Helper()
	t.Cleanup(func() {
		err := os.RemoveAll(tempDir)
		for err != nil {
			err = os.RemoveAll(tempDir)
		}
	})
}
