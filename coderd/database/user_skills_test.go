package database_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/x/skills"
	"github.com/coder/coder/v2/testutil"
)

func TestUserSkillSchemaConstants(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.SkipNow()
	}

	ctx := testutil.Context(t, testutil.WaitMedium)
	_, _, sqlDB := dbtestutil.NewDBWithSQLDB(t)
	var triggerDef string
	err := sqlDB.QueryRowContext(ctx,
		`SELECT pg_get_functiondef('enforce_user_skills_per_user_limit'::regproc)`,
	).Scan(&triggerDef)
	require.NoError(t, err)
	require.Contains(t, triggerDef, fmt.Sprintf(
		"skill_limit constant int := %d",
		skills.MaxPersonalSkillsPerUser,
	))

	constraints := map[database.CheckConstraint]string{
		database.CheckUserSkillsNameSize: fmt.Sprintf(
			"octet_length(name) <= %d",
			skills.MaxPersonalSkillNameBytes,
		),
		database.CheckUserSkillsNameFormat: "name ~ '^[a-z0-9]+(-[a-z0-9]+)*$'::text",
		database.CheckUserSkillsDescriptionSize: fmt.Sprintf(
			"octet_length(description) <= %d",
			skills.MaxPersonalSkillDescriptionBytes,
		),
		database.CheckUserSkillsContentSize: fmt.Sprintf(
			"octet_length(content) <= %d",
			skills.MaxPersonalSkillSizeBytes,
		),
	}
	for constraint, expected := range constraints {
		t.Run(string(constraint), func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitMedium)
			var constraintDef string
			err := sqlDB.QueryRowContext(ctx,
				`SELECT pg_get_constraintdef(oid) FROM pg_constraint WHERE conname = $1`,
				constraint,
			).Scan(&constraintDef)
			require.NoError(t, err)
			require.Contains(t, constraintDef, expected)
		})
	}
}
