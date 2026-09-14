package main

import (
	"strings"
	"testing"

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
