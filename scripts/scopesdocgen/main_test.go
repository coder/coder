package main

import (
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
		{"leaves inner words alone", "read api key details", "Read api key details."},
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

	if want := []string{string(rbac.ScopeAll), string(rbac.ScopeApplicationConnect)}; !equal(builtin, want) {
		t.Errorf("builtin = %v, want %v", builtin, want)
	}
	if len(composite) == 0 || len(lowLevel) == 0 {
		t.Fatalf("composite (%d) and low-level (%d) scopes must both be populated", len(composite), len(lowLevel))
	}
	for _, name := range composite {
		if _, ok := rbac.CompositeSitePermissions(rbac.ScopeName(name)); !ok {
			t.Errorf("composite scope %q has no permissions", name)
		}
	}
	for _, name := range lowLevel {
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
		if !contains(rbac.ExternalScopeNames(), string(scope)) {
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

	_, err := actionDescription("missing_resource", "read")
	require.Error(t, err)

	_, err = actionDescription("workspace", "missing_action")
	require.Error(t, err)
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
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

func equal(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
