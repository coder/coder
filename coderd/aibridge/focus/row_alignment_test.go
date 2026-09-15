package focus_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/aibridge/focus"
)

// TestRowHeaderParquetTagAlignment pins the contract that csv_test.go's
// length-only check does not: every focus.Row field, in declaration order,
// must have a parquet tag whose name exactly matches the Header entry at the
// same position. csv_test.go only asserts len(cells) == len(Header), which
// stays green if a field is added to Row and the wrong (or a
// differently-cased) name is added to Header, or if the two lists drift out
// of order relative to each other. A silently misspelled or misordered tag
// breaks every downstream FOCUS consumer with no local symptom, since CSV
// and Parquet output would both still "look like" a valid FOCUS document.
func TestRowHeaderParquetTagAlignment(t *testing.T) {
	t.Parallel()

	rt := reflect.TypeOf(focus.Row{})
	require.Equal(t, rt.NumField(), len(focus.Header),
		"focus.Row has a different number of fields than focus.Header has entries")

	for i := range rt.NumField() {
		field := rt.Field(i)
		tag, _, _ := strings.Cut(field.Tag.Get("parquet"), ",")
		require.NotEmpty(t, tag, "field %s has no parquet tag", field.Name)
		require.Equal(t, focus.Header[i], tag,
			"focus.Row field %d (%s) has parquet tag %q but Header[%d] is %q; csv.go's cell order, row.go's field order, and the parquet tags must all agree",
			i, field.Name, tag, i, focus.Header[i])
	}
}
