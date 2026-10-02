package docgenenv_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/scripts/docgenenv"
)

// writeTree writes files (sources-relative name to content) under a new docs
// directory's manifest sources and returns the docs and sources directories.
// Every route path used by the tests exists as a page under the docs dir.
func writeTree(t *testing.T, files map[string]string) (docsDir, sourcesDir string) {
	t.Helper()
	docsDir = t.TempDir()
	sourcesDir = filepath.Join(docsDir, docgenenv.ManifestSourcesDir)
	for _, page := range []string{"index.md", "ref/index.md", "ref/api/index.md", "ref/api/general.md", "ref/api/users.md"} {
		p := filepath.Join(docsDir, page)
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte("# page\n"), 0o600))
	}
	for name, content := range files {
		p := filepath.Join(sourcesDir, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte(content), 0o600))
	}
	return docsDir, sourcesDir
}

const (
	testIndex = "versions: [main]\nsections:\n  - home.yml\n  - ref.yml\n"
	testHome  = "title: Home\npath: ./index.md\n"
	testRef   = `title: Reference
path: ./ref/index.md
children:
  - include: ref/api.yml
`
	testRefAPI = `title: REST API
path: ./ref/api/index.md
children_from: generated/api.json
children:
  - title: Users
    description: Manage users.
    state: [beta]
`
	testAPI = `[{"title": "General", "path": "./ref/api/general.md"}, {"title": "Users", "path": "./ref/api/users.md"}]`
)

func validTree() map[string]string {
	return map[string]string{
		"index.yml":          testIndex,
		"home.yml":           testHome,
		"ref.yml":            testRef,
		"ref/api.yml":        testRefAPI,
		"generated/api.json": testAPI,
	}
}

func TestBuildManifest(t *testing.T) {
	t.Parallel()

	docsDir, sourcesDir := writeTree(t, validTree())
	m, err := docgenenv.BuildManifest(sourcesDir, docsDir)
	require.NoError(t, err)

	require.Equal(t, []string{"main"}, m.Versions)
	require.Len(t, m.Routes, 2)
	api := m.FindRoute("Reference", "REST API")
	require.NotNil(t, api)
	require.Empty(t, api.ChildrenFrom)
	// Generated routes keep their order; the overlay fills in metadata.
	require.Equal(t, []docgenenv.Route{
		{Title: "General", Path: "./ref/api/general.md"},
		{Title: "Users", Description: "Manage users.", Path: "./ref/api/users.md", State: []string{"beta"}},
	}, api.Children)

	// Keys follow the struct order and children_from never reaches the JSON.
	b, err := docgenenv.MarshalJSON(api)
	require.NoError(t, err)
	require.NotContains(t, string(b), "children_from")
	require.Regexp(t, `(?s)"title": "Users",\s*"description".*"path".*"state"`, string(b))
}

func TestBuildManifestYAMLFragment(t *testing.T) {
	t.Parallel()

	files := validTree()
	delete(files, "generated/api.json")
	files["ref/api.yml"] = `title: REST API
path: ./ref/api/index.md
children_from: generated/api.yml
`
	files["generated/api.yml"] = "- title: General\n  path: ./ref/api/general.md\n"
	docsDir, sourcesDir := writeTree(t, files)
	m, err := docgenenv.BuildManifest(sourcesDir, docsDir)
	require.NoError(t, err)
	require.Equal(t, []docgenenv.Route{{Title: "General", Path: "./ref/api/general.md"}}, m.FindRoute("Reference", "REST API").Children)
}

func TestLoadManifestSourcesLeavesOverlaysUnresolved(t *testing.T) {
	t.Parallel()

	_, sourcesDir := writeTree(t, validTree())
	m, err := docgenenv.LoadManifestSources(sourcesDir)
	require.NoError(t, err)
	api := m.FindRoute("Reference", "REST API")
	require.NotNil(t, api)
	require.Equal(t, "generated/api.json", api.ChildrenFrom)
	require.Equal(t, []docgenenv.Route{{Title: "Users", Description: "Manage users.", State: []string{"beta"}}}, api.Children)
}

func TestBuildManifestErrors(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		edit    func(files map[string]string)
		wantErr string
		// wantErrIs, when set, must be in the error chain. Use it for OS
		// errors, whose text differs between platforms.
		wantErrIs error
	}{
		{
			name:    "UnknownKey",
			edit:    func(f map[string]string) { f["home.yml"] = "title: Home\ntitel: typo\npath: ./index.md\n" },
			wantErr: "field titel not found",
		},
		{
			name:    "UnknownFragmentKey",
			edit:    func(f map[string]string) { f["generated/api.json"] = `[{"title": "General", "paht": "x"}]` },
			wantErr: `unknown field "paht"`,
		},
		{
			name:    "MissingPage",
			edit:    func(f map[string]string) { f["home.yml"] = "title: Home\npath: ./nope.md\n" },
			wantErr: `Home: path "./nope.md" does not exist`,
		},
		{
			name:    "MissingPath",
			edit:    func(f map[string]string) { f["home.yml"] = "title: Home\n" },
			wantErr: "home.yml: Home: route is missing a path",
		},
		{
			name:    "UnreferencedFile",
			edit:    func(f map[string]string) { f["orphan.yml"] = testHome },
			wantErr: "orphan.yml: not listed",
		},
		{
			name:    "DuplicateSection",
			edit:    func(f map[string]string) { f["index.yml"] = testIndex + "  - home.yml\n" },
			wantErr: `section "home.yml" is listed more than once`,
		},
		{
			name:    "SectionOutsideSources",
			edit:    func(f map[string]string) { f["index.yml"] = "versions: [main]\nsections:\n  - ../home.yml\n" },
			wantErr: "must be a relative path inside the sources directory",
		},
		{
			name: "UnmatchedOverlay",
			edit: func(f map[string]string) {
				f["generated/api.json"] = `[{"title": "General", "path": "./ref/api/general.md"}]`
			},
			wantErr: `overlay "Users" matches no route in generated/api.json`,
		},
		{
			name: "ConflictingOverlay",
			edit: func(f map[string]string) {
				f["generated/api.json"] = `[{"title": "Users", "description": "Generated.", "path": "./ref/api/users.md"}]`
			},
			wantErr: `overlay description "Manage users." conflicts with generated "Generated."`,
		},
		{
			name: "OverlayWithPath",
			edit: func(f map[string]string) {
				f["ref/api.yml"] = `title: REST API
path: ./ref/api/index.md
children_from: generated/api.json
children:
  - title: Users
    path: ./ref/api/users.md
`
			},
			wantErr: "overlays under children_from may set only",
		},
		{
			name: "GrandchildrenInOneFile",
			edit: func(f map[string]string) {
				delete(f, "ref/api.yml")
				f["ref.yml"] = `title: Reference
path: ./ref/index.md
children:
  - title: REST API
    path: ./ref/api/index.md
    children:
      - title: General
        path: ./ref/api/general.md
`
			},
			wantErr: "ref.yml: Reference > REST API: a source file holds one level of children; move this route into its own file and include it",
		},
		{
			name: "ChildWithChildrenFromInOneFile",
			edit: func(f map[string]string) {
				delete(f, "ref/api.yml")
				f["ref.yml"] = `title: Reference
path: ./ref/index.md
children:
  - title: REST API
    path: ./ref/api/index.md
    children_from: generated/api.json
`
			},
			wantErr: "move this route into its own file and include it",
		},
		{
			name:    "IncludeWithOtherKeys",
			edit:    func(f map[string]string) { f["ref.yml"] = testRef + "    title: REST API\n" },
			wantErr: `include "ref/api.yml": an include entry may set only include`,
		},
		{
			name:    "IncludedTwice",
			edit:    func(f map[string]string) { f["ref.yml"] = testRef + "  - include: ref/api.yml\n" },
			wantErr: `include "ref/api.yml": file is already listed in index.yml or included elsewhere`,
		},
		{
			name:    "IncludeSection",
			edit:    func(f map[string]string) { f["ref.yml"] = testRef + "  - include: home.yml\n" },
			wantErr: `include "home.yml": file is already listed`,
		},
		{
			name:    "IncludeOutsideSources",
			edit:    func(f map[string]string) { f["ref.yml"] = testRef + "  - include: ../api.yml\n" },
			wantErr: "must be a relative path inside the sources directory",
		},
		{
			name:    "IncludeNotYAML",
			edit:    func(f map[string]string) { f["ref.yml"] = testRef + "  - include: generated/api.json\n" },
			wantErr: `include "generated/api.json": must be a .yml file`,
		},
		{
			name:      "IncludeMissingFile",
			edit:      func(f map[string]string) { f["ref.yml"] = testRef + "  - include: ref/nope.yml\n" },
			wantErr:   "nope.yml",
			wantErrIs: fs.ErrNotExist,
		},
		{
			name:    "IncludeOnFileRoute",
			edit:    func(f map[string]string) { f["home.yml"] = testHome + "include: ref/api.yml\n" },
			wantErr: "home.yml: Home: include is allowed only on a child entry",
		},
		{
			name:    "MissingPathInIncludedFile",
			edit:    func(f map[string]string) { f["ref/api.yml"] = "title: REST API\n" },
			wantErr: "ref/api.yml: Reference > REST API: route is missing a path",
		},
		{
			name:    "UnreferencedNestedFile",
			edit:    func(f map[string]string) { f["ref/orphan.yml"] = testHome },
			wantErr: "ref/orphan.yml: not listed",
		},
		{
			name: "IncludeInFragment",
			edit: func(f map[string]string) {
				f["generated/api.json"] = `[{"title": "General", "path": "./ref/api/general.md", "include": "x.yml"}]`
			},
			wantErr: `unknown field "include"`,
		},
		{
			name: "IncludeInYAMLFragment",
			edit: func(f map[string]string) {
				delete(f, "generated/api.json")
				f["ref/api.yml"] = "title: REST API\npath: ./ref/api/index.md\nchildren_from: generated/api.yml\n"
				f["generated/api.yml"] = "- title: General\n  path: ./ref/api/general.md\n  include: x.yml\n"
			},
			wantErr: "generated/api.yml: General: fragments can't set children_from or include",
		},
		{
			name: "TrailingCommentOnDescription",
			edit: func(f map[string]string) {
				f["home.yml"] = "title: Home\ndescription: Use #tags here\npath: ./index.md\n"
			},
			wantErr: "home.yml: line 2: description has a trailing comment",
		},
		{
			name:    "MultipleDocuments",
			edit:    func(f map[string]string) { f["home.yml"] = testHome + "---\n" + testHome },
			wantErr: "single YAML document",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			files := validTree()
			tc.edit(files)
			docsDir, sourcesDir := writeTree(t, files)
			_, err := docgenenv.BuildManifest(sourcesDir, docsDir)
			require.ErrorContains(t, err, tc.wantErr)
			if tc.wantErrIs != nil {
				require.ErrorIs(t, err, tc.wantErrIs)
			}
		})
	}
}

func TestWriteRouteFragmentRoundTrips(t *testing.T) {
	t.Parallel()

	routes := []docgenenv.Route{
		{Title: "Groups & Roles", Path: "./a.md", Children: []docgenenv.Route{{Title: "b", Path: "./b.md", State: []string{"premium"}}}},
	}
	dir := t.TempDir()
	for _, name := range []string{"frag.json", "frag.yml"} {
		p := filepath.Join(dir, name)
		require.NoError(t, docgenenv.WriteRouteFragment(p, routes))
		var got []docgenenv.Route
		require.NoError(t, docgenenv.DecodeSourceFile(p, &got), name)
		require.Equal(t, routes, got, name)
	}
	b, err := os.ReadFile(filepath.Join(dir, "frag.json"))
	require.NoError(t, err)
	require.Contains(t, string(b), `Groups & Roles`)
}
