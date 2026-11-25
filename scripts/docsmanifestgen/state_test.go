package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/scripts/docgenenv"
)

// statePage returns a Markdown page whose front matter sets states.
func statePage(title string, states ...string) string {
	fm := "---\ntitle: " + title + "\n"
	if len(states) > 0 {
		fm += "state: [" + strings.Join(states, ", ") + "]\n"
	}
	return fm + "---\n\n# " + title + "\n"
}

// writeStateSite writes files (docs-relative name to content) under a new docs
// directory and returns it.
func writeStateSite(t *testing.T, files map[string]string) string {
	t.Helper()
	docsDir := t.TempDir()
	for name, body := range files {
		p := filepath.Join(docsDir, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte(body), 0o600))
	}
	return docsDir
}

func TestRepeatedStates(t *testing.T) {
	t.Parallel()

	const (
		index   = "versions: [main]\nsections:\n  - home.yml\n"
		home    = "title: Home\npath: ./index.md\nchildren:\n  - title: A\n    path: ./a.md\n"
		homeSub = "title: Home\npath: ./index.md\nchildren:\n  - include: home/sub.yml\n"
	)
	sub := "title: Sub\npath: ./sub/index.md\nchildren:\n  - include: home/sub/x.yml\n"
	x := "title: X\npath: ./sub/x.md\n"
	for _, tc := range []struct {
		name  string
		files map[string]string // Docs-relative name to content; sources live under manifest/.
		want  []string          // Error substrings, one per expected error.
	}{
		{
			name: "NoStates",
			files: map[string]string{
				"manifest/home.yml": home,
				"index.md":          statePage("Home"),
				"a.md":              statePage("A"),
			},
		},
		{
			name: "DifferentStateOnDescendant",
			files: map[string]string{
				"manifest/home.yml": home,
				"index.md":          statePage("Home", "premium"),
				"a.md":              statePage("A", "early access"),
			},
		},
		{
			name: "SameStateOnSiblings",
			files: map[string]string{
				"manifest/home.yml": "title: Home\npath: ./index.md\nchildren:\n  - title: A\n    path: ./a.md\n  - title: B\n    path: ./b.md\n",
				"index.md":          statePage("Home"),
				"a.md":              statePage("A", "beta"),
				"b.md":              statePage("B", "beta"),
			},
		},
		{
			name: "ChildRepeatsParent",
			files: map[string]string{
				"manifest/home.yml": home,
				"index.md":          statePage("Home", "premium"),
				"a.md":              statePage("A", "premium"),
			},
			want: []string{`./a.md: Home > A: state "premium" is already set on ancestor "Home" in ./index.md`},
		},
		{
			name: "OnlyRepeatedValueReported",
			files: map[string]string{
				"manifest/home.yml": home,
				"index.md":          statePage("Home", "premium"),
				"a.md":              statePage("A", "premium", "beta"),
			},
			want: []string{`./a.md: Home > A: state "premium"`},
		},
		{
			name: "StateListedTwiceOnOneRoute",
			files: map[string]string{
				"manifest/home.yml": "title: Home\npath: ./index.md\n",
				"index.md":          statePage("Home", "beta", "beta"),
			},
			want: []string{`./index.md: Home: state "beta" is listed more than once`},
		},
		{
			name: "IncludedFileRepeatsAncestor",
			files: map[string]string{
				"manifest/home.yml":       homeSub,
				"manifest/home/sub.yml":   sub,
				"manifest/home/sub/x.yml": x,
				"index.md":                statePage("Home", "premium"),
				"sub/index.md":            statePage("Sub"),
				"sub/x.md":                statePage("X", "premium"),
			},
			want: []string{`./sub/x.md: Home > Sub > X: state "premium" is already set on ancestor "Home" in ./index.md`},
		},
		{
			name: "EachRepeatReportedAgainstFirstAncestor",
			files: map[string]string{
				"manifest/home.yml":       homeSub,
				"manifest/home/sub.yml":   sub,
				"manifest/home/sub/x.yml": x,
				"index.md":                statePage("Home", "beta"),
				"sub/index.md":            statePage("Sub", "beta"),
				"sub/x.md":                statePage("X", "beta"),
			},
			want: []string{
				`./sub/index.md: Home > Sub: state "beta" is already set on ancestor "Home" in ./index.md`,
				`./sub/x.md: Home > Sub > X: state "beta" is already set on ancestor "Home" in ./index.md`,
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			files := map[string]string{"manifest/index.yml": index}
			for name, body := range tc.files {
				files[name] = body
			}
			docsDir := writeStateSite(t, files)
			m, err := docgenenv.BuildManifest(filepath.Join(docsDir, docgenenv.ManifestSourcesDir), docsDir)
			require.NoError(t, err)

			errs := repeatedStates(m)
			require.Len(t, errs, len(tc.want), "errors: %v", errs)
			for i, want := range tc.want {
				require.ErrorContains(t, errs[i], want)
			}
		})
	}
}

func TestCheckReportsRepeatedStates(t *testing.T) {
	t.Parallel()

	docsDir := writeStateSite(t, map[string]string{
		"manifest/index.yml": "versions: [main]\nsections:\n  - home.yml\n",
		"manifest/home.yml":  "title: Home\npath: ./index.md\nchildren:\n  - title: A\n    path: ./a.md\n",
		"index.md":           statePage("Home", "premium"),
		"a.md":               statePage("A", "premium"),
	})

	err := run([]string{"check", "-docs", docsDir})
	require.ErrorContains(t, err, `./a.md: Home > A: state "premium" is already set on ancestor "Home" in ./index.md`)
}
