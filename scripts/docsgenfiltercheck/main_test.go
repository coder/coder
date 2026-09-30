package main

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGlobRegexp(t *testing.T) {
	t.Parallel()

	tests := []struct {
		glob    string
		match   []string
		noMatch []string
	}{
		{
			glob:    "docs/**",
			match:   []string{"docs", "docs/a.md", "docs/a/b/c.md"},
			noMatch: []string{"docsx/a.md", "site/docs/a.md"},
		},
		{
			glob:    "*.md",
			match:   []string{"README.md"},
			noMatch: []string{"docs/README.md"},
		},
		{
			glob:    "docs/reference/cli/**",
			match:   []string{"docs/reference/cli/users/create.md"},
			noMatch: []string{"docs/reference/api/users.md"},
		},
		{
			glob:    "docs/admin/*.md",
			match:   []string{"docs/admin/users.md"},
			noMatch: []string{"docs/admin/setup/index.md"},
		},
		{
			glob:    "docs/manifest.json",
			match:   []string{"docs/manifest.json"},
			noMatch: []string{"docs/manifestXjson", "docs/manifest.json.bak"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.glob, func(t *testing.T) {
			t.Parallel()
			re, err := globRegexp(tt.glob)
			require.NoError(t, err)
			for _, p := range tt.match {
				require.True(t, re.MatchString(p), "%q should match %q", tt.glob, p)
			}
			for _, p := range tt.noMatch {
				require.False(t, re.MatchString(p), "%q should not match %q", tt.glob, p)
			}
		})
	}

	// Forms picomatch reads differently from a naive translation, or that
	// the filters don't use, must be rejected.
	for _, glob := range []string{"", "**", "/**", "!docs/**", "docs/**/index.md", "docs/reference/**.md", `docs/\*.md`, "docs/?.md", "docs/{a,b}.md", "docs/[ab].md"} {
		_, err := globRegexp(glob)
		require.Error(t, err, "%q should be rejected", glob)
	}
}

func TestCheck(t *testing.T) {
	t.Parallel()

	filters := map[string][]string{
		"docs":     {"docs/**", "*.md"},
		"docs-gen": {"docs/manifest.json", "docs/reference/cli/**"},
	}
	tracked := []string{"docs/manifest.json", "docs/reference/cli/index.md", "docs/admin/users.md"}

	t.Run("Covered", func(t *testing.T) {
		t.Parallel()
		problems, err := check(filters, []string{"docs/manifest.json", "docs/reference/cli/index.md", "coderd/apidoc/swagger.json"}, tracked)
		require.NoError(t, err)
		require.Empty(t, problems)
	})

	t.Run("UncoveredGeneratedFile", func(t *testing.T) {
		t.Parallel()
		problems, err := check(filters, []string{"docs/admin/users.md", "GENERATED.md"}, tracked)
		require.NoError(t, err)
		require.Len(t, problems, 2)
		require.Contains(t, problems[0], "docs/admin/users.md")
		require.Contains(t, problems[1], "GENERATED.md")
	})

	t.Run("StalePattern", func(t *testing.T) {
		t.Parallel()
		stale := map[string][]string{
			"docs":     filters["docs"],
			"docs-gen": append([]string{"docs/gone/**"}, filters["docs-gen"]...),
		}
		problems, err := check(stale, []string{"docs/manifest.json"}, tracked)
		require.NoError(t, err)
		require.Len(t, problems, 1)
		require.Contains(t, problems[0], `"docs/gone/**"`)
	})

	t.Run("MissingFilter", func(t *testing.T) {
		t.Parallel()
		_, err := check(map[string][]string{"docs": {"docs/**"}}, []string{"docs/manifest.json"}, tracked)
		require.ErrorContains(t, err, `"docs-gen"`)
	})

	t.Run("StatusQualifiedRule", func(t *testing.T) {
		t.Parallel()
		_, err := check(map[string][]string{"docs": {"docs/**"}, "docs-gen": {"map[added|modified:docs/new/**]"}}, []string{"docs/manifest.json"}, tracked)
		require.ErrorContains(t, err, `unsupported pattern "map[added|modified:docs/new/**]"`)
	})
}

func TestParseFilters(t *testing.T) {
	t.Parallel()

	workflow := `
jobs:
  changes:
    steps:
      - name: Checkout
        with:
          fetch-depth: 1
          persist-credentials: false
      - id: filter
        with:
          filters: |
            docs:
              - "docs/**"
            docs-gen:
              - "docs/manifest.json"
              - added|modified: "docs/new/**"
`
	filters, err := parseFilters([]byte(workflow))
	require.NoError(t, err)
	require.Equal(t, []string{"docs/**"}, filters["docs"])
	require.Equal(t, []string{"docs/manifest.json", "map[added|modified:docs/new/**]"}, filters["docs-gen"])

	_, err = parseFilters([]byte("jobs:\n  changes:\n    steps: []\n"))
	require.ErrorContains(t, err, `no step with id "filter"`)
}

// TestRepositoryWorkflow parses the real workflow so a restructured changes
// job fails here rather than only in make lint.
func TestRepositoryWorkflow(t *testing.T) {
	t.Parallel()

	workflow, err := os.ReadFile("../../.github/workflows/ci.yaml")
	require.NoError(t, err)
	filters, err := parseFilters(workflow)
	require.NoError(t, err)
	for _, name := range []string{"docs", "docs-gen"} {
		require.NotEmpty(t, filters[name], "filter %q", name)
		for _, p := range filters[name] {
			_, err := globRegexp(p)
			require.NoError(t, err, "filter %q pattern %q", name, p)
		}
	}
}
