package main

import (
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/wsrelated"
	"github.com/coder/coder/v2/scripts/docgenenv"
)

type rowsRoot struct {
	ID       string        `json:"id" wsrelated:""`
	Outdated bool          `json:"outdated" wsrelated:"template,latest_build"`
	Build    rowsBuild     `json:"build" wsrelated:"latest_build"`
	Skipped  string        `json:"-" wsrelated:""`
	Hidden   string        `json:"hidden"`
	Health   rowsUntagged  `json:"health" wsrelated:"latest_build.resources.agents"`
	Agents   []rowsAgent   `json:"agents" wsrelated:"latest_build.resources.agents"`
	Covered  *rowsUntagged `json:"covered,omitempty" wsrelated:"template"`
}

type rowsBuild struct {
	Number int    `json:"number" wsrelated:"latest_build"`
	Worker string `json:"worker"`
}

type rowsAgent struct {
	Name    string         `json:"name" wsrelated:"latest_build.resources.agents"`
	Scripts []rowsUntagged `json:"scripts" wsrelated:"latest_build.resources.agents.scripts"`
}

type rowsUntagged struct {
	Value string `json:"value"`
}

type rowsInvalidTag struct {
	Name string `json:"name" wsrelated:"latest_build.bogus"`
}

func TestFieldRows(t *testing.T) {
	t.Parallel()

	t.Run("Rows", func(t *testing.T) {
		t.Parallel()

		rows, err := fieldRows(reflect.TypeFor[rowsRoot]())
		require.NoError(t, err)

		// A struct with its own tags is described by its fields, a struct
		// without tags is one row, and an untagged field of a tagged struct
		// is not returned. Rows are grouped by the object that holds them.
		require.Equal(t, []row{
			{Field: "covered", Paths: "`template`"},
			{Field: "health", Paths: "`latest_build.resources.agents`"},
			{Field: "hidden", Paths: notReturned},
			{Field: "id", Paths: "Always returned"},
			{Field: "outdated", Paths: "`template` and `latest_build`"},
			{Field: "agents[].name", Paths: "`latest_build.resources.agents`"},
			{Field: "agents[].scripts", Paths: "`latest_build.resources.agents.scripts`"},
			{Field: "build.number", Paths: "`latest_build`"},
			{Field: "build.worker", Paths: notReturned},
		}, rows)
	})

	t.Run("InvalidTag", func(t *testing.T) {
		t.Parallel()

		_, err := fieldRows(reflect.TypeFor[rowsInvalidTag]())
		require.ErrorContains(t, err, "latest_build.bogus")
	})
}

func TestIncludeRelatedPaths(t *testing.T) {
	t.Parallel()

	// Each path appears once, after its ancestors, and a multi-path tag
	// contributes each of its paths separately.
	require.Equal(t, []string{
		"latest_build",
		"latest_build.resources.agents",
		"latest_build.resources.agents.scripts",
		"template",
	}, includeRelatedPaths(reflect.TypeFor[rowsRoot]()))
}

// TestRenderCoversEveryWorkspaceField guards the page's central promise: every
// field of the workspace response appears in the table, either as its own row
// or under the row of an object that holds it. A struct shape the row logic
// doesn't handle would otherwise drop fields from the page without failing
// generation.
func TestRenderCoversEveryWorkspaceField(t *testing.T) {
	t.Parallel()

	page, err := render(docgenenv.Route{Title: "Workspace related data"})
	require.NoError(t, err)

	fields := wsrelated.Fields(reflect.TypeFor[codersdk.Workspace]())
	require.NotEmpty(t, fields)
	for _, f := range fields {
		isLeaf := true
		for _, g := range fields {
			if strings.HasPrefix(g.JSONPath, f.JSONPath+".") || strings.HasPrefix(g.JSONPath, f.JSONPath+"[].") {
				isLeaf = false
				break
			}
		}
		if !isLeaf {
			continue
		}
		require.Truef(t, hasRowForPathOrAncestor(page, f.JSONPath), "page has no row for %s or an object that holds it", f.JSONPath)
	}
}

// hasRowForPathOrAncestor reports whether page has a table row whose first
// cell is path or a JSON path that holds it.
func hasRowForPathOrAncestor(page, path string) bool {
	for {
		if strings.Contains(page, "\n| `"+path+"` |") {
			return true
		}
		i := strings.LastIndex(path, ".")
		if i < 0 {
			return false
		}
		path = strings.TrimSuffix(path[:i], "[]")
	}
}
