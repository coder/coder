package database

import (
	"testing"

	"github.com/coder/coder/coderd/rbac"
	"github.com/stretchr/testify/require"
)

func TestInsertAuthorized(t *testing.T) {
	query := `SELECT true;`
	_, err := insertAuthorizedFilter(query, nil, rbac.NoACLConfig())
	require.ErrorContains(t, err, "does not contain authorized replace string", "ensure replace string")
}
