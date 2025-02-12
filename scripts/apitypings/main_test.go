//go:build !windows
// +build !windows

// Windows tests fail because the \n\r vs \n. It's not worth trying
// to replace newlines for os tests. If people start using this tool on windows
// and are seeing problems, then we can add build tags and figure it out.
package main

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/guts"
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

			gen, err := guts.NewGolangParser()
			if err != nil {
				require.NoError(t, err)
			}
			err = gen.IncludeGenerate("./" + dir)
			require.NoError(t, err)

			err = TypeMappings(gen)
			require.NoError(t, err)

			ts, err := gen.ToTypescript()
			require.NoError(t, err)

			TsMutations(ts)

			output, err := ts.Serialize()
			require.NoError(t, err)

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
