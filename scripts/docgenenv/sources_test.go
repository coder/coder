package docgenenv_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/scripts/docgenenv"
)

// writeTree writes files (docs-relative name to content) under a new docs
// directory and returns it. Every route path used by the tests exists as a
// page under the docs directory.
func writeTree(t *testing.T, files map[string]string) string {
	t.Helper()
	docsDir := t.TempDir()
	for _, page := range []string{"index.md", "ref/index.md", "ref/api/index.md", "ref/api/general.md", "ref/api/users.md"} {
		p := filepath.Join(docsDir, filepath.FromSlash(page))
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte("# page\n"), 0o600))
	}
	for name, content := range files {
		p := filepath.Join(docsDir, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte(content), 0o600))
	}
	return docsDir
}

const (
	testRoot = "versions: [main]\nroutes:\n  - title: Home\n    path: ./index.md\n  - folder: ref\n"
	testRef  = `title: Reference
path: ./index.md
children:
  - title: REST API
    path: ./api/index.md
    folder: api
    children:
      - title: Users
        description: Manage users.
        state: [beta]
`
	testAPI = "- title: General\n  path: ./general.md\n- title: Users\n  path: ./users.md\n"
)

func validTree() map[string]string {
	return map[string]string{
		"sidebar.yml":         testRoot,
		"ref/sidebar.yml":     testRef,
		"ref/api/sidebar.yml": testAPI,
	}
}

func TestBuildManifest(t *testing.T) {
	t.Parallel()

	docsDir := writeTree(t, validTree())
	m, err := docgenenv.BuildManifest(docsDir)
	require.NoError(t, err)

	require.Equal(t, []string{"main"}, m.Versions)
	require.Len(t, m.Routes, 2)
	require.Equal(t, "./ref/index.md", m.FindRoute("Reference").Path)
	api := m.FindRoute("Reference", "REST API")
	require.NotNil(t, api)
	require.Empty(t, api.Folder)
	require.Equal(t, "./ref/api/index.md", api.Path)
	// Generated routes keep their order and take paths relative to the docs
	// directory; the overlay fills in metadata.
	require.Equal(t, []docgenenv.Route{
		{Title: "General", Path: "./ref/api/general.md"},
		{Title: "Users", Description: "Manage users.", Path: "./ref/api/users.md", State: []string{"beta"}},
	}, api.Children)

	// Keys follow the struct order and folder never reaches the JSON.
	b, err := docgenenv.MarshalJSON(api)
	require.NoError(t, err)
	require.NotContains(t, string(b), "folder")
	require.Regexp(t, `(?s)"title": "Users",\s*"description".*"path".*"state"`, string(b))
}

func TestBuildManifestFolderEntry(t *testing.T) {
	t.Parallel()

	files := validTree()
	files["sidebar.yml"] = "versions: [main]\nroutes:\n  - folder: ref\n"
	files["ref/sidebar.yml"] = `title: Reference
path: ./index.md
children:
  - folder: api
  - title: Bare
    path: general.md
`
	files["ref/api/sidebar.yml"] = "title: REST API\npath: ./index.md\nchildren:\n  - title: Users\n    path: users.md\n"
	docsDir := writeTree(t, files)
	require.NoError(t, os.WriteFile(filepath.Join(docsDir, "ref", "general.md"), []byte("# page\n"), 0o600))
	m, err := docgenenv.BuildManifest(docsDir)
	require.NoError(t, err)
	// A folder entry carries its paths with it. Each path keeps its leading
	// "./" or the lack of one.
	require.Equal(t, []docgenenv.Route{{
		Title: "Reference",
		Path:  "./ref/index.md",
		Children: []docgenenv.Route{
			{Title: "REST API", Path: "./ref/api/index.md", Children: []docgenenv.Route{{Title: "Users", Path: "ref/api/users.md"}}},
			{Title: "Bare", Path: "ref/general.md"},
		},
	}}, m.Routes)
}

func TestLoadSources(t *testing.T) {
	t.Parallel()

	docsDir := writeTree(t, validTree())
	s, err := docgenenv.LoadSources(docsDir)
	require.NoError(t, err)
	api := s.Manifest.FindRoute("Reference", "REST API")
	require.NotNil(t, api)
	// The generated sidebar stays unresolved, so the generators read the
	// curated overlays without depending on the file they rewrite.
	require.Equal(t, "ref/api", api.Folder)
	require.Equal(t, []docgenenv.Route{{Title: "Users", Description: "Manage users.", State: []string{"beta"}}}, api.Children)
	require.Equal(t, []string{"ref/api"}, s.GeneratedDirs)
	require.Equal(t, map[string]string{"index.md": ".", "ref/index.md": "ref", "ref/api/index.md": "ref"}, s.ListedBy)
}

func TestBuildManifestJSONSidebar(t *testing.T) {
	t.Parallel()

	files := validTree()
	files["ref/api/sidebar.yml"] = `[{"title": "General", "path": "./general.md"}, {"title": "Users", "path": "./users.md"}]`
	docsDir := writeTree(t, files)
	m, err := docgenenv.BuildManifest(docsDir)
	require.NoError(t, err)
	require.Len(t, m.FindRoute("Reference", "REST API").Children, 2)
}

func TestBuildManifestErrors(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		edit    func(files map[string]string)
		wantErr string
		// Set for OS errors, whose text differs between platforms.
		wantErrIs error
	}{
		{
			name: "UnknownKey",
			edit: func(f map[string]string) {
				f["sidebar.yml"] = "versions: [main]\nroutes:\n  - title: Home\n    titel: typo\n    path: ./index.md\n"
			},
			wantErr: "field titel not found",
		},
		{
			name:    "UnknownGeneratedKey",
			edit:    func(f map[string]string) { f["ref/api/sidebar.yml"] = `[{"title": "General", "paht": "x"}]` },
			wantErr: "field paht not found",
		},
		{
			name: "MissingPage",
			edit: func(f map[string]string) {
				f["sidebar.yml"] = "versions: [main]\nroutes:\n  - title: Home\n    path: ./nope.md\n"
			},
			wantErr: `Home: path "./nope.md" does not exist`,
		},
		{
			name:    "MissingPath",
			edit:    func(f map[string]string) { f["sidebar.yml"] = "versions: [main]\nroutes:\n  - title: Home\n" },
			wantErr: "sidebar.yml: Home: route is missing a path",
		},
		{
			name:    "NoVersions",
			edit:    func(f map[string]string) { f["sidebar.yml"] = "routes:\n  - folder: ref\n" },
			wantErr: "versions must list at least one version",
		},
		{
			name:    "NoRoutes",
			edit:    func(f map[string]string) { f["sidebar.yml"] = "versions: [main]\n" },
			wantErr: "routes must list at least one route",
		},
		{
			name:    "UnreferencedSidebar",
			edit:    func(f map[string]string) { f["orphan/sidebar.yml"] = "title: Orphan\npath: ./index.md\n" },
			wantErr: "orphan/sidebar.yml: no folder entry in the parent sidebar names this folder",
		},
		{
			name: "UnmatchedOverlay",
			edit: func(f map[string]string) {
				f["ref/api/sidebar.yml"] = "- title: General\n  path: ./general.md\n"
			},
			wantErr: `overlay "Users" matches no route in ref/api/sidebar.yml`,
		},
		{
			name: "ConflictingOverlay",
			edit: func(f map[string]string) {
				f["ref/api/sidebar.yml"] = "- title: Users\n  description: Generated.\n  path: ./users.md\n"
			},
			wantErr: `overlay description "Manage users." conflicts with generated "Generated."`,
		},
		{
			name: "OverlayWithPath",
			edit: func(f map[string]string) {
				f["ref/sidebar.yml"] = strings.Replace(testRef, "      - title: Users\n", "      - title: Users\n        path: ./users.md\n", 1)
			},
			wantErr: "overlays under a generated folder may set only",
		},
		{
			name: "GrandchildrenInOneFile",
			edit: func(f map[string]string) {
				f["ref/sidebar.yml"] = `title: Reference
path: ./index.md
children:
  - title: REST API
    path: ./api/index.md
    children:
      - title: General
        path: ./api/general.md
`
			},
			wantErr: "ref/sidebar.yml: Reference > REST API: a sidebar file holds one level of children; move this route into its own folder",
		},
		{
			name: "GrandchildrenOfSecondChild",
			edit: func(f map[string]string) {
				f["ref/sidebar.yml"] = testRef + "  - title: Extra\n    path: ./index.md\n    children:\n      - title: Deep\n        path: ./index.md\n"
			},
			wantErr: "Reference > Extra: a sidebar file holds one level of children",
		},
		{
			name:    "FolderListedTwice",
			edit:    func(f map[string]string) { f["sidebar.yml"] = testRoot + "  - folder: ref\n" },
			wantErr: `folder "ref": ref/sidebar.yml is already listed elsewhere`,
		},
		{
			name:    "FolderOutsideDocs",
			edit:    func(f map[string]string) { f["sidebar.yml"] = testRoot + "  - folder: ../ref\n" },
			wantErr: "folder must be a relative path to a folder inside the docs directory",
		},
		{
			name:    "FolderIsOwnFolder",
			edit:    func(f map[string]string) { f["sidebar.yml"] = testRoot + "  - folder: .\n" },
			wantErr: "folder must be a relative path to a folder inside the docs directory",
		},
		{
			name:    "FolderAbsolute",
			edit:    func(f map[string]string) { f["sidebar.yml"] = testRoot + "  - folder: /ref\n" },
			wantErr: "folder must be a relative path to a folder inside the docs directory",
		},
		{
			name: "PathOutsideDocs",
			edit: func(f map[string]string) {
				f["ref/sidebar.yml"] = testRef + "  - title: Escape\n    path: ../../outside.md\n"
			},
			wantErr: `Reference > Escape: path "../../outside.md": must be relative and stay inside the docs directory`,
		},
		{
			name:      "FolderMissingSidebar",
			edit:      func(f map[string]string) { f["sidebar.yml"] = testRoot + "  - folder: nope\n" },
			wantErr:   "sidebar.yml",
			wantErrIs: fs.ErrNotExist,
		},
		{
			name:    "FolderInFolderFile",
			edit:    func(f map[string]string) { f["ref/sidebar.yml"] = "folder: api\n" },
			wantErr: "ref/sidebar.yml: folder is allowed only on a child entry",
		},
		{
			name:    "MissingPathInFolderFile",
			edit:    func(f map[string]string) { f["ref/sidebar.yml"] = "title: Reference\n" },
			wantErr: "ref/sidebar.yml: Reference: route is missing a path",
		},
		{
			name: "FolderInGeneratedSidebar",
			edit: func(f map[string]string) {
				f["ref/api/sidebar.yml"] = "- title: General\n  path: ./general.md\n  folder: x\n"
			},
			wantErr: "ref/api/sidebar.yml: General: a generated sidebar can't set folder",
		},
		{
			name:    "GeneratedPathOutsideFolder",
			edit:    func(f map[string]string) { f["ref/api/sidebar.yml"] = "- title: General\n  path: ../../../x.md\n" },
			wantErr: `ref/api/sidebar.yml: General: path "../../../x.md": must be relative`,
		},
		{
			name: "TrailingCommentOnDescription",
			edit: func(f map[string]string) {
				f["sidebar.yml"] = "versions: [main]\nroutes:\n  - title: Home\n    description: Use #tags here\n    path: ./index.md\n"
			},
			wantErr: "sidebar.yml: line 4: description has a trailing comment",
		},
		{
			name: "DescriptionWithoutPeriod",
			edit: func(f map[string]string) {
				f["sidebar.yml"] = "versions: [main]\nroutes:\n  - title: Home\n    description: The home page\n    path: ./index.md\n"
			},
			wantErr: `sidebar.yml: Home: description must end with a period: "The home page"`,
		},
		{
			name: "DescriptionWithQuestionMark",
			edit: func(f map[string]string) {
				f["sidebar.yml"] = "versions: [main]\nroutes:\n  - title: Home\n    description: Where do I start?\n    path: ./index.md\n"
			},
			wantErr: `sidebar.yml: Home: description must end with a period: "Where do I start?"`,
		},
		{
			name: "ChildDescriptionWithoutPeriod",
			edit: func(f map[string]string) {
				f["ref/sidebar.yml"] = testRef + "  - title: Home again\n    description: Same page\n    path: ./index.md\n"
			},
			wantErr: `ref/sidebar.yml: Reference > Home again: description must end with a period: "Same page"`,
		},
		{
			name: "OverlayDescriptionWithoutPeriod",
			edit: func(f map[string]string) {
				f["ref/sidebar.yml"] = strings.Replace(testRef, "Manage users.", "Manage users", 1)
			},
			wantErr: `ref/sidebar.yml: Reference > REST API > Users: description must end with a period: "Manage users"`,
		},
		{
			name: "GeneratedDescriptionWithoutPeriod",
			edit: func(f map[string]string) {
				f["ref/api/sidebar.yml"] = "- title: General\n  description: General endpoints\n  path: ./general.md\n- title: Users\n  path: ./users.md\n"
			},
			wantErr: `ref/api/sidebar.yml: General: description must end with a period: "General endpoints"`,
		},
		{
			name:    "MultipleDocuments",
			edit:    func(f map[string]string) { f["ref/sidebar.yml"] = testRef + "---\n" + testRef },
			wantErr: "single YAML document",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			files := validTree()
			tc.edit(files)
			docsDir := writeTree(t, files)
			_, err := docgenenv.BuildManifest(docsDir)
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

// TestBuildManifestFrontMatterDescription checks the description in a page's
// own front matter, which the docs site prefers over the manifest's.
func TestBuildManifestFrontMatterDescription(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		page    string
		wantErr string
	}{
		{name: "Period", page: "---\ntitle: Home\ndescription: The home page.\n---\n\n# page\n"},
		{name: "NoDescription", page: "---\ntitle: Home\n---\n\n# page\n"},
		{name: "NoFrontMatter", page: "# page\n\n---\n\ndescription: not front matter\n"},
		{
			name:    "WithoutPeriod",
			page:    "---\ntitle: Home\ndescription: The home page\n---\n\n# page\n",
			wantErr: `index.md: Home: front matter description must end with a period: "The home page"`,
		},
		{
			name:    "Quoted",
			page:    "---\ntitle: Home\ndescription: \"Show role(s)\"\n---\n\n# page\n",
			wantErr: `index.md: Home: front matter description must end with a period: "Show role(s)"`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			docsDir := writeTree(t, validTree())
			require.NoError(t, os.WriteFile(filepath.Join(docsDir, "index.md"), []byte(tc.page), 0o600))
			_, err := docgenenv.BuildManifest(docsDir)
			if tc.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tc.wantErr)
		})
	}
}
