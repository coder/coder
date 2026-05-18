package main

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/enterprise/audit"
)

// TestResourceRenamesCompleteness verifies two invariants of the resourceRenames
// map:
//  1. Every renamed value resolves to a key in audit.AuditActionMap. A stale
//     rename (e.g. pointing to a key that was removed) would silently produce a
//     row with empty actions in the generated docs.
//  2. Every *Table-suffixed struct in audit.AuditableResources that has a
//     corresponding base name (minus "Table") in audit.AuditActionMap is covered
//     by a rename entry. Missing entries cause those resources to appear with the
//     raw struct name and empty actions.
func TestResourceRenamesCompleteness(t *testing.T) {
	t.Parallel()

	t.Run("all rename targets exist in AuditActionMap", func(t *testing.T) {
		t.Parallel()
		for from, to := range resourceRenames {
			_, ok := audit.AuditActionMap[to]
			assert.True(t, ok,
				"resourceRenames[%q] = %q but %q is missing from AuditActionMap",
				from, to, to,
			)
		}
	})

	t.Run("Table-suffix resources with AuditActionMap entries have renames", func(t *testing.T) {
		t.Parallel()
		for typeName := range audit.AuditableResources {
			// typeName is the fully qualified name, e.g.
			// "github.com/coder/coder/v2/coderd/database.WorkspaceTable".
			parts := strings.Split(typeName, ".")
			require.NotEmpty(t, parts)
			structName := parts[len(parts)-1]

			if !strings.HasSuffix(structName, "Table") {
				continue
			}
			baseName := strings.TrimSuffix(structName, "Table")
			_, inActionMap := audit.AuditActionMap[baseName]
			_, inRenames := resourceRenames[structName]
			if inActionMap {
				assert.True(t, inRenames,
					"%q has a *Table suffix and %q exists in AuditActionMap, "+
						"but no rename entry exists in resourceRenames; add %q: %q",
					structName, baseName, structName, baseName,
				)
			}
		}
	})
}

// TestParseResourceAllowlist verifies that parseResourceAllowlist correctly
// handles empty, single, multi-value, and whitespace-padded inputs.
func TestParseResourceAllowlist(t *testing.T) {
	t.Parallel()

	t.Run("empty string returns nil", func(t *testing.T) {
		t.Parallel()
		result := parseResourceAllowlist("")
		assert.Nil(t, result)
	})

	t.Run("single resource", func(t *testing.T) {
		t.Parallel()
		result := parseResourceAllowlist("AIProvider")
		require.NotNil(t, result)
		_, ok := result["AIProvider"]
		assert.True(t, ok)
		assert.Len(t, result, 1)
	})

	t.Run("multiple resources", func(t *testing.T) {
		t.Parallel()
		result := parseResourceAllowlist("AIProvider,Chat,Task")
		require.NotNil(t, result)
		assert.Len(t, result, 3)
		for _, name := range []string{"AIProvider", "Chat", "Task"} {
			_, ok := result[name]
			assert.True(t, ok, "expected %q in allowlist", name)
		}
	})

	t.Run("whitespace is trimmed", func(t *testing.T) {
		t.Parallel()
		result := parseResourceAllowlist("AIProvider, Chat , Task")
		require.NotNil(t, result)
		assert.Len(t, result, 3)
		_, ok := result["Chat"]
		assert.True(t, ok)
	})
}

// TestReadAuditableResources verifies that readAuditableResources correctly
// applies renames and filters by allowlist.
func TestReadAuditableResources(t *testing.T) {
	t.Parallel()

	t.Run("all resources included when allowlist is nil", func(t *testing.T) {
		t.Parallel()
		m := readAuditableResources(nil)
		assert.Greater(t, len(m), 0)
	})

	t.Run("allowlist filters to matching resources", func(t *testing.T) {
		t.Parallel()
		allowlist := map[string]struct{}{
			"AIProvider": {},
		}
		m := readAuditableResources(allowlist)
		require.Len(t, m, 1)
		_, ok := m["AIProvider"]
		assert.True(t, ok)
	})

	t.Run("renames are applied before allowlist check", func(t *testing.T) {
		t.Parallel()
		// TaskTable is renamed to Task; the allowlist must use the renamed value.
		allowlist := map[string]struct{}{
			"Task": {},
		}
		m := readAuditableResources(allowlist)
		require.Len(t, m, 1, "expected exactly one result for Task (renamed from TaskTable)")
		_, ok := m["Task"]
		assert.True(t, ok)
	})

	t.Run("empty allowlist returns no resources", func(t *testing.T) {
		t.Parallel()
		// Non-nil but empty allowlist means no resource matches.
		allowlist := map[string]struct{}{}
		m := readAuditableResources(allowlist)
		assert.Empty(t, m)
	})
}
