package migrations

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsNoTransaction(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		migration string
		want      bool
	}{
		{
			name:      "Empty",
			migration: "",
			want:      false,
		},
		{
			name:      "PlainSQL",
			migration: "CREATE INDEX idx ON t (id);",
			want:      false,
		},
		{
			name:      "MarkerFirst",
			migration: NoTransactionMarker + "\nCREATE INDEX CONCURRENTLY idx ON t (id);",
			want:      true,
		},
		{
			name:      "MarkerAfterOtherComments",
			migration: "-- Rationale for the index.\n\n  " + NoTransactionMarker + "  \nCREATE INDEX CONCURRENTLY idx ON t (id);",
			want:      true,
		},
		{
			name:      "MarkerAfterSQLIsIgnored",
			migration: "SELECT 1;\n" + NoTransactionMarker + "\nCREATE INDEX CONCURRENTLY idx ON t (id);",
			want:      false,
		},
		{
			name:      "MarkerMustBeWholeComment",
			migration: "-- coder:no-transaction because reasons\nCREATE INDEX CONCURRENTLY idx ON t (id);",
			want:      false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, isNoTransaction([]byte(tc.migration)))
		})
	}
}
