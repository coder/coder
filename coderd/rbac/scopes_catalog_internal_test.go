package rbac

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExternalScopeNames(t *testing.T) {
	t.Parallel()

	names := ExternalScopeNames()
	require.NotEmpty(t, names)

	// Ensure sorted ascending
	sorted := append([]string(nil), names...)
	sort.Strings(sorted)
	require.Equal(t, sorted, names)

	// Ensure each entry parses and expands to site-only
	for _, name := range names {
		// Skip `all` and `application_connect` since they do not
		// expand into a low level scope.
		// They are handled differently.
		if name == string(ScopeAll) || name == string(ScopeApplicationConnect) {
			continue
		}

		res, act, ok := parseLowLevelScope(ScopeName(name))
		require.Truef(t, ok, "catalog entry should parse: %s", name)

		s, err := ScopeName(name).Expand()
		require.NoErrorf(t, err, "catalog entry should expand: %s", name)
		require.Len(t, s.Site, 1)
		require.Equal(t, res, s.Site[0].ResourceType)
		require.Equal(t, act, s.Site[0].Action)
		require.Empty(t, s.Org)
		require.Empty(t, s.User)
	}
}

func TestIsExternalScope(t *testing.T) {
	t.Parallel()

	require.True(t, IsExternalScope("workspace:read"))
	require.True(t, IsExternalScope("template:use"))
	require.True(t, IsExternalScope("workspace:*"))
	require.False(t, IsExternalScope("debug_info:read")) // internal-only
	require.False(t, IsExternalScope("unknown:read"))
}
