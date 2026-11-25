package main

import (
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/scripts/docgenenv"
)

func TestSentence(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"capitalizes and terminates", "read user data", "Read user data."},
		{"keeps existing period", "delete a template.", "Delete a template."},
		{"restores leading acronym", "ssh into a given workspace", "SSH into a given workspace."},
		{"restores multi-case acronym", "api key details", "API key details."},
		{"capitalizes a multibyte rune", "éclair", "Éclair."},
		{"restores mid-sentence acronym", "read api key details", "Read API key details."},
		{"empty stays empty", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := sentence(tc.in); got != tc.want {
				t.Errorf("sentence(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestPartitionScopes(t *testing.T) {
	t.Parallel()

	builtin, composite, lowLevel := partitionScopes(rbac.ExternalScopeNames())

	if want := []string{string(rbac.ScopeAll), string(rbac.ScopeApplicationConnect)}; !slices.Equal(builtin, want) {
		t.Errorf("builtin = %v, want %v", builtin, want)
	}
	if len(composite) == 0 || len(lowLevel) == 0 {
		t.Fatalf("composite (%d) and low-level (%d) scopes must both be populated", len(composite), len(lowLevel))
	}
	for _, name := range lowLevel {
		if _, ok := rbac.CompositeSitePermissions(rbac.ScopeName(name)); ok {
			t.Errorf("low-level scope %q is a composite scope", name)
		}
		if _, _, ok := rbac.ParseResourceAction(name); !ok {
			t.Errorf("low-level scope %q is not a resource:action pair", name)
		}
	}
}

// TestRenderCoversEveryPublicScope guards the page's central promise: it lists
// every scope a token may request. A scope added to the catalog without a
// rendering path would otherwise be published as unrequestable.
func TestRenderCoversEveryPublicScope(t *testing.T) {
	t.Parallel()

	page, err := render(docgenenv.Route{Title: "API key scopes"})
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	for _, name := range rbac.ExternalScopeNames() {
		if !strings.Contains(page, "`"+name+"`") {
			t.Errorf("generated page is missing scope %q", name)
		}
	}

	for alias, canonical := range rbac.ScopeAliases() {
		if !strings.Contains(page, "| `"+string(alias)+"` | `"+string(canonical)+"` |") {
			t.Errorf("generated page is missing alias %q", alias)
		}
	}

	for _, scope := range exampleScopes {
		if !slices.Contains(rbac.ExternalScopeNames(), string(scope)) {
			t.Errorf("example scope %q is not public", scope)
		}
	}

	builtin := section(page, "## Built-in scopes", "## Composite scopes")
	for _, name := range []rbac.ScopeName{rbac.ScopeAll, rbac.ScopeApplicationConnect} {
		if !strings.Contains(builtin, "`"+string(name)+"`") {
			t.Errorf("built-in section is missing scope %q", name)
		}
	}
}

func TestActionDescription(t *testing.T) {
	t.Parallel()

	t.Run("wildcard", func(t *testing.T) {
		t.Parallel()

		got, err := actionDescription("workspace", "*")
		require.NoError(t, err)
		require.Equal(t, "Every action on `workspace`, including actions not listed on this page.", got)
		require.NotContains(t, got, "listed for")
	})

	t.Run("policy action", func(t *testing.T) {
		t.Parallel()

		got, err := actionDescription("workspace", "ssh")
		require.NoError(t, err)
		require.Equal(t, "SSH into a given workspace.", got)
	})

	t.Run("missing resource", func(t *testing.T) {
		t.Parallel()

		_, err := actionDescription("missing_resource", "read")
		require.Error(t, err)
	})

	t.Run("missing action", func(t *testing.T) {
		t.Parallel()

		_, err := actionDescription("workspace", "missing_action")
		require.Error(t, err)
	})
}

func TestRenderLowLevel(t *testing.T) {
	t.Parallel()

	var b strings.Builder
	err := renderLowLevel(&b, []string{"not-a-scope"})
	require.ErrorContains(t, err, "low-level scope \"not-a-scope\" is not a resource:action pair")

	b.Reset()
	err = renderLowLevel(&b, []string{"missing_resource:read"})
	require.ErrorContains(t, err, "describe low-level scope \"missing_resource:read\"")
}

func TestValidateTableCells(t *testing.T) {
	t.Parallel()

	require.NoError(t, validateTableCells("read API key details"))
	require.ErrorContains(t, validateTableCells("read | write"), "table cell contains a pipe")
}

func section(page, start, end string) string {
	startIndex := strings.Index(page, start)
	if startIndex == -1 {
		return ""
	}
	page = page[startIndex:]
	endIndex := strings.Index(page, end)
	if endIndex == -1 {
		return page
	}
	return page[:endIndex]
}
