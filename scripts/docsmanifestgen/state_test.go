package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func writeDocs(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte(body), 0o600))
	}
	return dir
}

func TestRepeatedStates(t *testing.T) {
	t.Parallel()

	const root = "versions: [main]\nroutes:\n  - folder: home\n"
	for _, tc := range []struct {
		name  string
		files map[string]string
		want  []string // Error substrings, one per expected error.
	}{
		{
			name: "NoStates",
			files: map[string]string{
				"home/sidebar.yml": "title: Home\npath: ./index.md\nchildren:\n  - title: A\n    path: ./a.md\n",
			},
		},
		{
			name: "DifferentStateOnDescendant",
			files: map[string]string{
				"home/sidebar.yml": "title: Home\npath: ./index.md\nstate: [premium]\nchildren:\n  - title: A\n    path: ./a.md\n    state: [early access]\n",
			},
		},
		{
			name: "SameStateOnSiblings",
			files: map[string]string{
				"home/sidebar.yml": "title: Home\npath: ./index.md\nchildren:\n  - title: A\n    path: ./a.md\n    state: [beta]\n  - title: B\n    path: ./b.md\n    state: [beta]\n",
			},
		},
		{
			name: "ChildRepeatsParent",
			files: map[string]string{
				"home/sidebar.yml": "title: Home\npath: ./index.md\nstate: [premium]\nchildren:\n  - title: A\n    path: ./a.md\n    state: [premium]\n",
			},
			want: []string{`Home > A: state "premium" is already set on ancestor "Home"`},
		},
		{
			name: "OnlyRepeatedValueReported",
			files: map[string]string{
				"home/sidebar.yml": "title: Home\npath: ./index.md\nstate: [premium]\nchildren:\n  - title: A\n    path: ./a.md\n    state: [premium, beta]\n",
			},
			want: []string{`Home > A: state "premium"`},
		},
		{
			name: "StateListedTwiceOnOneRoute",
			files: map[string]string{
				"home/sidebar.yml": "title: Home\npath: ./index.md\nstate: [beta, beta]\n",
			},
			want: []string{`Home: state "beta" is listed more than once`},
		},
		{
			name: "FolderFileRepeatsAncestor",
			files: map[string]string{
				"home/sidebar.yml":       "title: Home\npath: ./index.md\nstate: [premium]\nchildren:\n  - folder: sub\n",
				"home/sub/sidebar.yml":   "title: Sub\npath: ./index.md\nchildren:\n  - folder: x\n",
				"home/sub/x/sidebar.yml": "title: X\npath: ./index.md\nstate: [premium]\n",
			},
			want: []string{`Home > Sub > X: state "premium" is already set on ancestor "Home"`},
		},
		{
			name: "EachRepeatReportedAgainstFirstAncestor",
			files: map[string]string{
				"home/sidebar.yml":     "title: Home\npath: ./index.md\nstate: [beta]\nchildren:\n  - folder: sub\n",
				"home/sub/sidebar.yml": "title: Sub\npath: ./index.md\nstate: [beta]\nchildren:\n  - title: X\n    path: ./x.md\n    state: [beta]\n",
			},
			want: []string{
				`Home > Sub: state "beta" is already set on ancestor "Home"`,
				`Home > Sub > X: state "beta" is already set on ancestor "Home"`,
			},
		},
		{
			name: "OverlayRepeatsAncestor",
			files: map[string]string{
				"home/sidebar.yml": "title: Home\npath: ./index.md\nstate: [premium]\nchildren:\n  - title: Gen\n    path: ./gen/index.md\n    folder: gen\n    children:\n      - title: A\n        state: [premium]\n",
			},
			want: []string{`Home > Gen > A: state "premium" is already set on ancestor "Home"`},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			files := map[string]string{"sidebar.yml": root}
			for name, body := range tc.files {
				files[name] = body
			}
			dir := writeDocs(t, files)

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

	dir := writeDocs(t, map[string]string{
		"sidebar.yml":      "versions: [main]\nroutes:\n  - folder: home\n",
		"home/index.md":    "# page\n",
		"home/a.md":        "# page\n",
		"home/sidebar.yml": "title: Home\npath: ./index.md\nstate: [premium]\nchildren:\n  - title: A\n    path: ./a.md\n    state: [premium]\n",
	})

	err := run([]string{"check", "-docs", dir})
	require.ErrorContains(t, err, `Home > A: state "premium" is already set on ancestor "Home"`)
}

func TestCheckReportsUnlistedPages(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name  string
		files map[string]string
		want  string // Empty when the check passes.
	}{
		{
			name: "AllListed",
			files: map[string]string{
				"home/sidebar.yml": "title: Home\npath: ./index.md\nchildren:\n  - title: A\n    path: ./a.md\n",
			},
		},
		{
			name: "PageMissingFromSidebar",
			files: map[string]string{
				"home/sidebar.yml": "title: Home\npath: ./index.md\n",
			},
			want: "home/a.md: not listed in home/sidebar.yml",
		},
		{
			name: "FolderWithoutSidebar",
			files: map[string]string{
				"home/sidebar.yml": "title: Home\npath: ./index.md\nchildren:\n  - title: A\n    path: ./a.md\n",
				"home/extra/b.md":  "# page\n",
			},
			want: "home/extra/b.md: not listed in home/extra/sidebar.yml",
		},
		{
			name: "ParentPageBesideItsFolder",
			files: map[string]string{
				"home/sidebar.yml":     "title: Home\npath: ./index.md\nchildren:\n  - title: A\n    path: ./a.md\n  - folder: sub\n",
				"home/sub.md":          "# page\n",
				"home/sub/sidebar.yml": "title: Sub\npath: ../sub.md\n",
			},
		},
		{
			name: "ListedByAnotherFolder",
			files: map[string]string{
				"home/sidebar.yml": "title: Home\npath: ./index.md\nchildren:\n  - title: A\n    path: ./a.md\n  - title: Far\n    path: ../far/x.md\n",
				"far/x.md":         "# page\n",
			},
			want: "far/x.md: listed in home/sidebar.yml, not in the sidebar of its own folder",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			files := map[string]string{
				"sidebar.yml":   "versions: [main]\nroutes:\n  - folder: home\n",
				"home/index.md": "# page\n",
				"home/a.md":     "# page\n",
			}
			for name, body := range tc.files {
				files[name] = body
			}
			dir := writeDocs(t, files)

			err := run([]string{"check", "-docs", dir})
			if tc.want == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tc.want)
		})
	}
}
