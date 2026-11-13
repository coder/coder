package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/scripts/docgenenv"
)

func TestRepeatedStates(t *testing.T) {
	t.Parallel()

	const index = "versions: [main]\nsections:\n  - home.yml\n"
	for _, tc := range []struct {
		name  string
		files map[string]string
		want  []string // Error substrings, one per expected error.
	}{
		{
			name: "NoStates",
			files: map[string]string{
				"home.yml": "title: Home\npath: ./index.md\nchildren:\n  - title: A\n    path: ./a.md\n",
			},
		},
		{
			name: "DifferentStateOnDescendant",
			files: map[string]string{
				"home.yml": "title: Home\npath: ./index.md\nstate: [premium]\nchildren:\n  - title: A\n    path: ./a.md\n    state: [early access]\n",
			},
		},
		{
			name: "SameStateOnSiblings",
			files: map[string]string{
				"home.yml": "title: Home\npath: ./index.md\nchildren:\n  - title: A\n    path: ./a.md\n    state: [beta]\n  - title: B\n    path: ./b.md\n    state: [beta]\n",
			},
		},
		{
			name: "ChildRepeatsParent",
			files: map[string]string{
				"home.yml": "title: Home\npath: ./index.md\nstate: [premium]\nchildren:\n  - title: A\n    path: ./a.md\n    state: [premium]\n",
			},
			want: []string{`home.yml: Home > A: state "premium" is already set on ancestor "Home" in home.yml`},
		},
		{
			name: "OnlyRepeatedValueReported",
			files: map[string]string{
				"home.yml": "title: Home\npath: ./index.md\nstate: [premium]\nchildren:\n  - title: A\n    path: ./a.md\n    state: [premium, beta]\n",
			},
			want: []string{`home.yml: Home > A: state "premium"`},
		},
		{
			name: "StateListedTwiceOnOneRoute",
			files: map[string]string{
				"home.yml": "title: Home\npath: ./index.md\nstate: [beta, beta]\n",
			},
			want: []string{`home.yml: Home: state "beta" is listed more than once`},
		},
		{
			name: "IncludedFileRepeatsAncestor",
			files: map[string]string{
				"home.yml":       "title: Home\npath: ./index.md\nstate: [premium]\nchildren:\n  - include: home/sub.yml\n",
				"home/sub.yml":   "title: Sub\npath: ./sub/index.md\nchildren:\n  - include: home/sub/x.yml\n",
				"home/sub/x.yml": "title: X\npath: ./sub/x.md\nstate: [premium]\n",
			},
			want: []string{`home/sub/x.yml: Home > Sub > X: state "premium" is already set on ancestor "Home" in home.yml`},
		},
		{
			name: "EachRepeatReportedAgainstFirstAncestor",
			files: map[string]string{
				"home.yml":     "title: Home\npath: ./index.md\nstate: [beta]\nchildren:\n  - include: home/sub.yml\n",
				"home/sub.yml": "title: Sub\npath: ./sub/index.md\nstate: [beta]\nchildren:\n  - title: X\n    path: ./sub/x.md\n    state: [beta]\n",
			},
			want: []string{
				`home/sub.yml: Home > Sub: state "beta" is already set on ancestor "Home" in home.yml`,
				`home/sub.yml: Home > Sub > X: state "beta" is already set on ancestor "Home" in home.yml`,
			},
		},
		{
			name: "OverlayRepeatsAncestor",
			files: map[string]string{
				"home.yml": "title: Home\npath: ./index.md\nstate: [premium]\nchildren_from: generated/home.yml\nchildren:\n  - title: A\n    state: [premium]\n",
			},
			want: []string{`home.yml: Home > A: state "premium" is already set on ancestor "Home" in home.yml`},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			files := map[string]string{docgenenv.ManifestIndexFile: index}
			for name, body := range tc.files {
				files[name] = body
			}
			for name, body := range files {
				p := filepath.Join(dir, filepath.FromSlash(name))
				require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
				require.NoError(t, os.WriteFile(p, []byte(body), 0o600))
			}

			errs := repeatedStates(dir)
			require.Len(t, errs, len(tc.want), "errors: %v", errs)
			for i, want := range tc.want {
				require.ErrorContains(t, errs[i], want)
			}
		})
	}
}

func TestCheckReportsRepeatedStates(t *testing.T) {
	t.Parallel()

	docsDir := t.TempDir()
	sources := filepath.Join(docsDir, docgenenv.ManifestSourcesDir)
	require.NoError(t, os.MkdirAll(sources, 0o755))
	for _, page := range []string{"index.md", "a.md"} {
		require.NoError(t, os.WriteFile(filepath.Join(docsDir, page), []byte("# page\n"), 0o600))
	}
	require.NoError(t, os.WriteFile(filepath.Join(sources, docgenenv.ManifestIndexFile), []byte("versions: [main]\nsections:\n  - home.yml\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(sources, "home.yml"), []byte("title: Home\npath: ./index.md\nstate: [premium]\nchildren:\n  - title: A\n    path: ./a.md\n    state: [premium]\n"), 0o600))

	err := run([]string{"check", "-docs", docsDir})
	require.ErrorContains(t, err, `home.yml: Home > A: state "premium" is already set on ancestor "Home" in home.yml`)
}
