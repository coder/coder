package sqlxqueries

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_loadQueries(t *testing.T) {
	t.Parallel()
	_, err := loadQueries()
	require.NoError(t, err)
}
