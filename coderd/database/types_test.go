package database_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
)

func TestStringMapOfIntScan(t *testing.T) {
	t.Parallel()

	// A null column clears the map, so callers can tell no usage recorded from
	// an empty object.
	m := database.StringMapOfInt{"cursor": 1}
	require.NoError(t, m.Scan(nil))
	require.Nil(t, m)
	require.NoError(t, m.Scan([]byte(`{}`)))
	require.NotNil(t, m)
	require.Empty(t, m)
}
