package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/scripts/docgenenv"
)

func yamlKeys(t reflect.Type) []string {
	var keys []string
	for i := range t.NumField() {
		keys = append(keys, strings.Split(t.Field(i).Tag.Get("yaml"), ",")[0])
	}
	return keys
}

func TestKeyOrderMatchesStructs(t *testing.T) {
	t.Parallel()

	require.Equal(t, yamlKeys(reflect.TypeOf(docgenenv.Route{})), routeKeyOrder)
	require.Equal(t, yamlKeys(reflect.TypeOf(docgenenv.ManifestIndex{})), indexKeyOrder)
}

func TestFormatFile(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		rel  string
		in   string
		want string
	}{
		{
			name: "ReordersKeysAndMovesCommentsWithThem",
			rel:  "section.yml",
			in: `children:
    - path: ./b.md
      title: B
path: ./a.md
# Above title.
title: A
`,
			want: `# Above title.
title: A
path: ./a.md
children:
  - title: B
    path: ./b.md
`,
		},
		{
			name: "KeepsCommentsInPlace",
			rel:  "section.yml",
			in: `# Header.

title: A
path: ./a.md
children:
  # Above B.
  - title: B
    path: ./b.md
  - title: C
    # Above path.
    path: ./c.md # trailing
`,
			want: `# Header.

title: A
path: ./a.md
children:
  # Above B.
  - title: B
    path: ./b.md
  - title: C
    # Above path.
    path: ./c.md # trailing
`,
		},
		{
			name: "NormalizesQuotingAndLists",
			rel:  "section.yml",
			in: `title: "Plain"
description: 'Needs: quotes'
path: ./a.md
state:
  - beta
  - early access
children:
  - title: "123"
    path: ./b.md
  - title: "true"
    path: ./c.md
  - title: "a #b"
    path: ./d.md
`,
			want: `title: Plain
description: "Needs: quotes"
path: ./a.md
state: [beta, early access]
children:
  - title: "123"
    path: ./b.md
  - title: "true"
    path: ./c.md
  - title: "a #b"
    path: ./d.md
`,
		},
		{
			name: "Folder",
			rel:  "section.yml",
			in:   "title: A\npath: ./a.md\nchildren:\n  - {folder: \"b\"}\n",
			want: "title: A\npath: ./a.md\nchildren:\n  - folder: b\n",
		},
		{
			name: "Root",
			rel:  docgenenv.SidebarFile,
			in:   "# Header.\n\nroutes: [{path: ./a.md, title: A}, {folder: b}]\nversions:\n  - main\n",
			want: "# Header.\n\nversions: [main]\nroutes:\n  - title: A\n    path: ./a.md\n  - folder: b\n",
		},
		{
			name: "Fragment",
			rel:  "ref/api/sidebar.yml",
			in:   "- path: ./a.md\n  title: A\n",
			want: "- title: A\n  path: ./a.md\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := formatFile(tc.rel, []byte(tc.in))
			require.NoError(t, err)
			require.Equal(t, tc.want, string(got))

			// Formatting is idempotent.
			again, err := formatFile(tc.rel, got)
			require.NoError(t, err)
			require.Equal(t, string(got), string(again))
		})
	}
}

func TestFormatFileErrors(t *testing.T) {
	t.Parallel()

	_, err := formatFile("section.yml", []byte("title: A\ntitel: B\n"))
	require.ErrorContains(t, err, `line 2: unknown key "titel"`)

	_, err = formatFile("section.yml", []byte("title: A\ntitle: B\n"))
	require.Error(t, err)

	_, err = formatFile("section.yml", []byte("title: [A]\n"))
	require.ErrorContains(t, err, "expected a string")
}

func TestCheckReportsUnformattedFiles(t *testing.T) {
	t.Parallel()

	docsDir := t.TempDir()
	write := func(name, content string) {
		p := filepath.Join(docsDir, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte(content), 0o600))
	}
	write("sidebar.yml", "versions: [main]\nroutes:\n  - folder: home\n")
	write("home/index.md", "# page\n")
	write("home/sidebar.yml", "path: ./index.md\ntitle: Home\n")

	err := run([]string{"check", "-docs", docsDir})
	require.ErrorContains(t, err, "home/sidebar.yml: not formatted")

	require.NoError(t, run([]string{"fmt", "-docs", docsDir}))
	require.NoError(t, run([]string{"check", "-docs", docsDir}))

	out := filepath.Join(t.TempDir(), "manifest.json")
	require.NoError(t, run([]string{"build", "-docs", docsDir, "-out", out}))
	b, err := os.ReadFile(out)
	require.NoError(t, err)
	require.JSONEq(t, `{"versions": ["main"], "routes": [{"title": "Home", "path": "./home/index.md"}]}`, string(b))
}
