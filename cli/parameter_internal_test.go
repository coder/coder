package cli

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCreateParameterMapFromFile(t *testing.T) {
	t.Parallel()
	t.Run("CreateParameterMapFromFile", func(t *testing.T) {
		t.Parallel()
		parameterFile, _ := os.CreateTemp(t.TempDir(), "testParameterFile*.yaml")
		_, _ = parameterFile.WriteString("region: \"bananas\"\ndisk: \"20\"\n")

		parameterMapFromFile, err := createParameterMapFromFile(parameterFile.Name())

		expectedMap := map[string]string{
			"region": "bananas",
			"disk":   "20",
		}

		assert.Equal(t, expectedMap, parameterMapFromFile)
		assert.Nil(t, err)
	})
	t.Run("WithEmptyFilename", func(t *testing.T) {
		t.Parallel()

		parameterMapFromFile, err := createParameterMapFromFile("")

		assert.Nil(t, parameterMapFromFile)
		assert.EqualError(t, err, "Parameter file name is not specified")
	})
	t.Run("WithInvalidFilename", func(t *testing.T) {
		t.Parallel()

		parameterMapFromFile, err := createParameterMapFromFile("invalidFile.yaml")

		assert.Nil(t, parameterMapFromFile)
		assert.EqualError(t, err, "open invalidFile.yaml: no such file or directory")
	})
	t.Run("WithInvalidYAML", func(t *testing.T) {
		t.Parallel()
		parameterFile, _ := os.CreateTemp(t.TempDir(), "testParameterFile*.yaml")
		_, _ = parameterFile.WriteString("region = \"bananas\"\ndisk = \"20\"\n")

		parameterMapFromFile, err := createParameterMapFromFile(parameterFile.Name())

		assert.Nil(t, parameterMapFromFile)
		assert.EqualError(t, err, "yaml: unmarshal errors:\n  line 1: cannot unmarshal !!str `region ...` into map[string]string")
	})
}
