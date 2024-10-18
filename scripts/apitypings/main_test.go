//go:build !windows
// +build !windows

// Windows tests fail because the \n\r vs \n. It's not worth trying
// to replace newlines for os tests. If people start using this tool on windows
// and are seeing problems, then we can add build tags and figure it out.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// updateGoldenFiles is a flag that can be set to update golden files.
var updateGoldenFiles = flag.Bool("update", false, "Update golden files")

func TestGeneration(t *testing.T) {
	t.Parallel()
	files, err := os.ReadDir("testdata")
	require.NoError(t, err, "read dir")

	for _, f := range files {
		if !f.IsDir() {
			// Only test directories
			continue
		}
		f := f
		t.Run(f.Name(), func(t *testing.T) {
			t.Parallel()
			dir := filepath.Join(".", "testdata", f.Name())
			output, err := Generate("./" + dir)
			require.NoErrorf(t, err, "generate %q", dir)

			golden := filepath.Join(dir, f.Name()+".ts")
			expected, err := os.ReadFile(golden)
			require.NoErrorf(t, err, "read file %s", golden)
			expectedString := strings.TrimSpace(string(expected))
			output = strings.TrimSpace(output)
			if *updateGoldenFiles {
				// nolint:gosec
				err := os.WriteFile(golden, []byte(output+"\n"), 0o644)
				require.NoError(t, err, "write golden file")
			} else {
				require.Equal(t, expectedString, output, "matched output")
			}
		})
	}
}

func TestGenerateSliceOfObjectsTypeScript(t *testing.T) {

	t.Run("Valid slice input", func(t *testing.T) {
		t.Parallel()
		type testObject struct {
			Name  string `json:"name"`
			Value int    `json:"value"`
		}

		testData := []testObject{
			{Name: "Item1", Value: 1},
			{Name: "Item2", Value: 2},
			{Name: "Item3", Value: 3},
		}

		expected := fmt.Sprintf(`export const testObjects = [
	{"name":"Item1","value":1},
	{"name":"Item2","value":2},
	{"name":"Item3","value":3},%s]%s%s`, "\n", "\n", "\n")

		result, err := generateSliceOfObjectsTypeScript("testObjects", testData)
		require.NoError(t, err, "generate slice of objects")
		require.Equal(t, expected, result, "generated TypeScript matches expected output")
	})

	t.Run("Invalid non-slice input", func(t *testing.T) {
		t.Parallel()
		_, err := generateSliceOfObjectsTypeScript("invalidData", "not a slice")
		require.Error(t, err, "should return error for non-slice input")
		require.Contains(t, err.Error(), "data must be a slice", "error message should indicate invalid input type")
	})
}
