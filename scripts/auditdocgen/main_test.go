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

// TestUpdateAuditDoc verifies that updateAuditDoc correctly replaces the
// content between generatorPrefix and generatorSuffix with a formatted
// markdown table. These tests cover the formatting logic where a previous
// regression (DEREM-6) introduced an incorrect table separator.
func TestUpdateAuditDoc(t *testing.T) {
	t.Parallel()

	const (
		testPrefix = "<!-- test-gen-prefix -->"
		testSuffix = "<!-- test-gen-suffix -->"
	)
	prefix := []byte(testPrefix)
	suffix := []byte(testSuffix)

	// makeDoc builds a minimal document with preamble, markers, optional body
	// between the markers, and a postamble.
	makeDoc := func(body string) []byte {
		return []byte("preamble\n" + testPrefix + "\n" + body + testSuffix + "\npostamble\n")
	}

	t.Run("missing prefix returns error", func(t *testing.T) {
		t.Parallel()
		_, err := updateAuditDoc([]byte("no markers here"), AuditableResourcesMap{}, prefix, suffix)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "prefix")
	})

	t.Run("missing suffix returns error", func(t *testing.T) {
		t.Parallel()
		doc := []byte("preamble\n" + testPrefix + "\nno suffix")
		_, err := updateAuditDoc(doc, AuditableResourcesMap{}, prefix, suffix)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "suffix")
	})

	t.Run("preamble and postamble are preserved", func(t *testing.T) {
		t.Parallel()
		out, err := updateAuditDoc(makeDoc(""), AuditableResourcesMap{}, prefix, suffix)
		require.NoError(t, err)
		outStr := string(out)
		assert.True(t, strings.HasPrefix(outStr, "preamble\n"), "preamble should be at start")
		assert.True(t, strings.HasSuffix(outStr, "postamble\n"), "postamble should be at end")
		assert.Contains(t, outStr, testPrefix)
		assert.Contains(t, outStr, testSuffix)
	})

	t.Run("old table body is replaced not kept", func(t *testing.T) {
		t.Parallel()
		doc := makeDoc("| old-stale-content |\n| that | should | be | gone |\n")
		out, err := updateAuditDoc(doc, AuditableResourcesMap{}, prefix, suffix)
		require.NoError(t, err)
		assert.NotContains(t, string(out), "old-stale-content")
	})

	t.Run("table header is present", func(t *testing.T) {
		t.Parallel()
		out, err := updateAuditDoc(makeDoc(""), AuditableResourcesMap{}, prefix, suffix)
		require.NoError(t, err)
		assert.Contains(t, string(out), "|<b>Resource<b>||")
	})

	t.Run("table separator matches expected format (DEREM-6 regression)", func(t *testing.T) {
		t.Parallel()
		out, err := updateAuditDoc(makeDoc(""), AuditableResourcesMap{}, prefix, suffix)
		require.NoError(t, err)
		// The separator must be exactly "|--|-----------------|" - any change to
		// column widths breaks the downstream markdown formatter.
		assert.Contains(t, string(out), "|--|-----------------|")
	})

	t.Run("resources appear in alphabetical order", func(t *testing.T) {
		t.Parallel()
		m := AuditableResourcesMap{
			"Zebra":  {"id": true},
			"Alpha":  {"id": false},
			"Middle": {"id": true},
		}
		out, err := updateAuditDoc(makeDoc(""), m, prefix, suffix)
		require.NoError(t, err)
		outStr := string(out)
		alphaPos := strings.Index(outStr, "|Alpha")
		middlePos := strings.Index(outStr, "|Middle")
		zebraPos := strings.Index(outStr, "|Zebra")
		require.Greater(t, alphaPos, -1, "Alpha row missing")
		require.Greater(t, middlePos, -1, "Middle row missing")
		require.Greater(t, zebraPos, -1, "Zebra row missing")
		assert.Less(t, alphaPos, middlePos, "Alpha should precede Middle")
		assert.Less(t, middlePos, zebraPos, "Middle should precede Zebra")
	})

	t.Run("fields appear in alphabetical order within a resource", func(t *testing.T) {
		t.Parallel()
		m := AuditableResourcesMap{
			"Res": {
				"z_field": false,
				"a_field": true,
				"m_field": true,
			},
		}
		out, err := updateAuditDoc(makeDoc(""), m, prefix, suffix)
		require.NoError(t, err)
		outStr := string(out)
		aPos := strings.Index(outStr, "<td>a_field</td>")
		mPos := strings.Index(outStr, "<td>m_field</td>")
		zPos := strings.Index(outStr, "<td>z_field</td>")
		require.Greater(t, aPos, -1, "a_field missing")
		require.Greater(t, mPos, -1, "m_field missing")
		require.Greater(t, zPos, -1, "z_field missing")
		assert.Less(t, aPos, mPos, "a_field should precede m_field")
		assert.Less(t, mPos, zPos, "m_field should precede z_field")
	})

	t.Run("field tracked values rendered as true and false", func(t *testing.T) {
		t.Parallel()
		m := AuditableResourcesMap{
			"Res": {
				"tracked":   true,
				"untracked": false,
			},
		}
		out, err := updateAuditDoc(makeDoc(""), m, prefix, suffix)
		require.NoError(t, err)
		outStr := string(out)
		assert.Contains(t, outStr, "<td>tracked</td><td>true</td>")
		assert.Contains(t, outStr, "<td>untracked</td><td>false</td>")
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
