package database

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsAuthorizedQuery(t *testing.T) {
	t.Parallel()

	query := `SELECT true;`
	_, err := insertAuthorizedFilter(query, "")
	require.ErrorContains(t, err, "does not contain authorized replace string", "ensure replace string")
}
