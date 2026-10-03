package dbtestutil

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// Recent pg_dump versions (13.22+ / 14.19+ / 15.14+ / 16.10+ / 17.6+) emit
// psql meta-commands at the head and tail of the dump that aren't valid SQL.
// normalizeDump is expected to strip them so downstream consumers (sqlc,
// schema-equality checks in scripts/migrate-test) don't have to.
//
// See https://github.com/coder/internal/issues/965.
func TestNormalizeDumpStripsRestrict(t *testing.T) {
	t.Parallel()

	// Raw string literals (backticks) make backslashes literal, so the
	// meta-command here matches what pg_dump actually emits.
	input := []byte(`-- header
\restrict XYZ

CREATE TABLE foo;

\unrestrict XYZ
`)

	out := string(normalizeDump(input))
	require.NotContains(t, out, `\restrict`, `normalizeDump must strip \restrict psql meta-command`)
	require.NotContains(t, out, `\unrestrict`, `normalizeDump must strip \unrestrict psql meta-command`)
	require.Contains(t, out, "CREATE TABLE foo;", "normalizeDump must preserve real SQL between the meta-commands")
}
