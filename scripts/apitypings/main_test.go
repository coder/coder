package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGeneration(t *testing.T) {
	files, err := os.ReadDir("testdata")
	require.NoError(t, err, "read dir")

	for _, f := range files {
		if !f.IsDir() {
			// Only test directories
			continue
		}
		f := f
		t.Run(f.Name(), func(t *testing.T) {
			dir := filepath.Join(".", "testdata", f.Name())
			output, err := Generate("./" + dir)
			require.NoErrorf(t, err, "generate %q", dir)

			golden := filepath.Join(dir, f.Name()+".ts")
			expected, err := os.ReadFile(golden)
			require.NoErrorf(t, err, "read file %s", golden)
			expectedString := strings.TrimSpace(string(expected))
			output = strings.TrimSpace(output)
			require.Equal(t, expectedString, output, "matched output")
		})
	}
}
