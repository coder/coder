//go:build !windows
// +build !windows

// Windows tests fail because the \n\r vs \n. It's not worth trying
// to replace newlines for os tests. If people start using this tool on windows
// and are seeing problems, then we can add build tags and figure it out.
package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

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
			require.Equal(t, expectedString, output, "matched output")
		})
	}
}
