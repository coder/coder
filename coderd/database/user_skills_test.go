package database_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/x/skills"
	"github.com/coder/coder/v2/testutil"
)

func TestPersonalSkillLimitMatchesTrigger(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.SkipNow()
	}

	ctx := testutil.Context(t, testutil.WaitMedium)
	_, _, sqlDB := dbtestutil.NewDBWithSQLDB(t)
	var def string
	err := sqlDB.QueryRowContext(ctx,
		`SELECT pg_get_functiondef('enforce_user_skills_per_user_limit'::regproc)`,
	).Scan(&def)
	require.NoError(t, err)
	require.Contains(t, def, fmt.Sprintf(
		"skill_limit constant int := %d",
		skills.MaxPersonalSkillsPerUser,
	))
}
