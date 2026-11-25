package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"
)

func TestCheckSource(t *testing.T) {
	t.Parallel()

	known := map[string]bool{
		"known":         true,
		"another-key":   true,
		"notifications": true,
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
			name: "double-quoted command value is checked",
			src:  "To enable the preview, run `coder server --experiments=\"stale-key\"`.\n",
			want: []finding{{path: "docs/example.md", line: 1, key: "stale-key"}},
		},
		{
			name: "single-quoted assignment value is checked",
			src:  "Set `CODER_EXPERIMENTS='another-stale-key'` in the service environment.\n",
			want: []finding{{path: "docs/example.md", line: 1, key: "another-stale-key"}},
		},
		{
			name: "exported double-quoted assignment value is checked",
			src:  "For systemd, use `export CODER_EXPERIMENTS=\"third-stale-key\"`.\n",
			want: []finding{{path: "docs/example.md", line: 1, key: "third-stale-key"}},
		},
		{
			name: "mismatched quotes are not recognized as values",
			src:  "Do not copy this malformed command: CODER_EXPERIMENTS=\"stale-key'.\n",
		},
		{
			name: "shell continuation joins a split key from its first physical line",
			src: "Configure the container environment:\n\n```sh\nCODER_EXPERIMENTS=notifi\\\n" +
				"cations\n```\n",
		},
		{
			name: "shell continuation checks every list key from its first physical line",
			src: "Configure the container environment:\n\n```sh\nCODER_EXPERIMENTS=notifications,\\\n" +
				"stale-key\n```\n",
			want: []finding{{path: "docs/example.md", line: 4, key: "stale-key"}},
		},
		{
			name: "placeholders are constrained to documented examples and angle brackets",
			src: "Use a placeholder while drafting:\n\n```sh\nCODER_EXPERIMENTS=feature1,feature2,<key>,*\n" +
				"CODER_EXPERIMENTS=feature3\n```\n",
			want: []finding{{path: "docs/example.md", line: 5, key: "feature3"}},
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
			name: "trailing comma after key list is ignored",
			src:  "Set CODER_EXPERIMENTS=known, then restart.\n",
		},
		{
			name: "sentence-final period after key list is ignored",
			src:  "Set CODER_EXPERIMENTS=known. Then restart.\n",
		},
		{
			name: "closing parenthesis after key list is ignored",
			src:  "Set CODER_EXPERIMENTS=known) before restarting.\n",
		},
		{
			name: "closing bracket after key list is ignored",
			src:  "Set CODER_EXPERIMENTS=known] before restarting.\n",
		},
		{
			name: "quote after key list is ignored",
			src:  "Set CODER_EXPERIMENTS=known\" before restarting.\n",
		},
		{
			name: "semicolon after key list is ignored",
			src:  "Set CODER_EXPERIMENTS=known; then restart.\n",
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

func TestCollectMarkdown(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	nestedDirectory := filepath.Join(directory, "nested")
	require.NoError(t, os.Mkdir(nestedDirectory, 0o700))

	topLevelMarkdown := filepath.Join(directory, "top-level.md")
	nestedMarkdown := filepath.Join(nestedDirectory, "nested.md")
	for _, path := range []string{topLevelMarkdown, nestedMarkdown, filepath.Join(directory, "ignored.txt")} {
		require.NoError(t, os.WriteFile(path, nil, 0o600))
	}

	files, err := collectMarkdown([]string{directory, nestedMarkdown, filepath.Join(directory, "ignored.txt")})
	require.NoError(t, err)
	require.Equal(t, []string{nestedMarkdown, topLevelMarkdown}, files)

	t.Run("does not traverse directory symlinks", func(t *testing.T) {
		t.Parallel()

		outsideDirectory := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(outsideDirectory, "outside.md"), nil, 0o600))

		link := filepath.Join(directory, "linked-directory")
		require.NoError(t, os.Symlink(outsideDirectory, link))

		files, err := collectMarkdown([]string{directory})
		require.NoError(t, err)
		require.Equal(t, []string{nestedMarkdown, topLevelMarkdown}, files)
	})
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

func TestRunReportsIOErrors(t *testing.T) {
	t.Parallel()

	t.Run("collect markdown", func(t *testing.T) {
		t.Parallel()

		var stderr bytes.Buffer
		require.Equal(t, 2, run([]string{filepath.Join(t.TempDir(), "missing")}, &stderr))
		require.Contains(t, stderr.String(), "checkexperimentkeys:")
	})

	t.Run("read markdown", func(t *testing.T) {
		t.Parallel()

		path := filepath.Join(t.TempDir(), "example.md")
		require.NoError(t, os.WriteFile(path, nil, 0o600))

		var stderr bytes.Buffer
		require.Equal(t, 2, runWithReadFile([]string{path}, &stderr, func(string) ([]byte, error) {
			return nil, xerrors.New("read markdown")
		}))
		require.Contains(t, stderr.String(), "checkexperimentkeys: read markdown")
	})
}
