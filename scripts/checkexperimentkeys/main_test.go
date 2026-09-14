package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCheckSource(t *testing.T) {
	t.Parallel()

	known := map[string]bool{
		"known":       true,
		"another-key": true,
	}

	tests := []struct {
		name string
		src  string
		want []finding
	}{
		{
			name: "known key passes",
			src:  "coder server --experiments=known\n",
		},
		{
			name: "unknown key fails",
			src:  "CODER_EXPERIMENTS=unknown\n",
			want: []finding{{path: "docs/example.md", line: 1, key: "unknown"}},
		},
		{
			name: "placeholder forms are ignored",
			src: "coder server --experiments=feature1,feature2\n" +
				"CODER_EXPERIMENTS=<experiment-key>\n" +
				"coder server --experiments=*\n",
		},
		{
			name: "all keys in a list are checked",
			src:  "coder server --experiments=known,unknown,another-key,other-unknown\n",
			want: []finding{{path: "docs/example.md", line: 1, key: "unknown"}, {path: "docs/example.md", line: 1, key: "other-unknown"}},
		},
		{
			name: "bare identifiers are ignored",
			src:  "The unknown experiment is not an explicit enablement example.\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, test.want, checkSource("docs/example.md", []byte(test.src), known))
		})
	}
}

func TestRunReportsLocationAndRemediation(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	path := filepath.Join(directory, "example.md")
	require.NoError(t, os.WriteFile(path, []byte("\nCODER_EXPERIMENTS=unknown\n"), 0o600))

	var stderr bytes.Buffer
	require.Equal(t, 1, run([]string{path}, &stderr))
	require.Contains(t, stderr.String(), path+":2: unknown experiment key \"unknown\"")
	require.Contains(t, stderr.String(), "add it to codersdk.ExperimentsKnown or remove or update")
}
