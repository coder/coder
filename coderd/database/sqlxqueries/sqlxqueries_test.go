package sqlxqueries_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/database/sqlxqueries"
)

func Test_loadQueries(t *testing.T) {
	t.Parallel()
	_, err := sqlxqueries.LoadQueries()
	require.NoError(t, err)
}
