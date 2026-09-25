package database_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/testutil"
)

// Constraint names raised by the cap and deleted-user trigger functions
// with RAISE ... USING CONSTRAINT. They are not table constraints, so
// check_constraint.go does not declare them.
const (
	userSecretsTotalBytesLimit database.CheckConstraint = "user_secrets_per_user_total_bytes_limit"
	userSkillsPerUserLimit     database.CheckConstraint = "user_skills_per_user_limit"
	userSkillUserDeleted       database.CheckConstraint = "user_skill_user_deleted"
)

// capTable describes one capped table for the soft-delete ordering tests.
type capTable struct {
	name   string
	insert func(userID uuid.UUID) stmt
}

var capTables = []capTable{
	{
		name: "user_secrets",
		insert: func(userID uuid.UUID) stmt {
			id := uuid.New()
			return stmt{`
				INSERT INTO user_secrets (id, user_id, name, description, value, env_name, file_path)
				VALUES ($1, $2, $3, '', 'value', '', $4)
			`, []any{id, userID, "secret-" + id.String(), fmt.Sprintf("/tmp/secret-%s", id)}}
		},
	},
	{
		name: "user_skills",
		insert: func(userID uuid.UUID) stmt {
			id := uuid.New()
			return stmt{`
				INSERT INTO user_skills (id, user_id, name, description, content)
				VALUES ($1, $2, $3, '', 'content')
			`, []any{id, userID, "skill-" + id.String()}}
		},
	},
}

const softDeleteUser = `UPDATE users SET deleted = true WHERE id = $1`

// TestUserCapsWriteBeforeSoftDelete pins the order where a capped write
// locks the users row first: the soft-delete must wait for it, so its
// cleanup trigger sees the committed row and deletes it. No row may be left
// behind for the deleted user.
func TestUserCapsWriteBeforeSoftDelete(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.SkipNow()
	}

	db, _, sqlDB := dbtestutil.NewDBWithSQLDB(t)
	for _, table := range capTables {
		t.Run(table.name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitLong)
			user := dbgen.User(t, db, database.User{})

			err := runLockRace(ctx, t, sqlDB,
				[]stmt{table.insert(user.ID)},
				stmt{softDeleteUser, []any{user.ID}},
			)
			require.NoError(t, err, "the soft-delete must succeed after the write commits")

			var count int
			require.NoError(t, sqlDB.QueryRowContext(ctx,
				`SELECT count(*) FROM `+table.name+` WHERE user_id = $1`, user.ID,
			).Scan(&count))
			require.Zero(t, count, "the soft-delete cleanup must remove the row written before it")
		})
	}
}

// TestUserCapsSoftDeleteBeforeWrite pins the order where the soft-delete
// locks the users row first: the capped write passes the earlier
// deleted-user trigger (the delete is not committed yet), waits for the
// users-row lock, and must then fail the recheck instead of committing a
// row for the deleted user.
func TestUserCapsSoftDeleteBeforeWrite(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.SkipNow()
	}

	db, _, sqlDB := dbtestutil.NewDBWithSQLDB(t)
	for _, table := range capTables {
		t.Run(table.name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitLong)
			user := dbgen.User(t, db, database.User{})

			err := runLockRace(ctx, t, sqlDB,
				[]stmt{{softDeleteUser, []any{user.ID}}},
				table.insert(user.ID),
			)
			require.Error(t, err, "a write racing a committed soft-delete must fail")
			require.ErrorContains(t, err, "for deleted user")
			if table.name == "user_skills" {
				require.True(t, database.IsCheckViolation(err, userSkillUserDeleted),
					"expected the deleted-user constraint, got: %v", err)
			}

			var count int
			require.NoError(t, sqlDB.QueryRowContext(ctx,
				`SELECT count(*) FROM `+table.name+` WHERE user_id = $1`, user.ID,
			).Scan(&count))
			require.Zero(t, count, "no row may be committed for the deleted user")
		})
	}
}

// TestUserSecretsCapConcurrentUpdates verifies the byte caps hold under
// concurrent user_secrets updates: two transactions growing different rows
// of the same user must not both pass the pre-statement aggregate check.
func TestUserSecretsCapConcurrentUpdates(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.SkipNow()
	}

	db, _, sqlDB := dbtestutil.NewDBWithSQLDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)
	user := dbgen.User(t, db, database.User{})

	// file_path targets keep the values out of the (much smaller)
	// env-injected byte cap so the test exercises the total-bytes cap.
	secretA, secretB := uuid.New(), uuid.New()
	for i, id := range []uuid.UUID{secretA, secretB} {
		_, err := sqlDB.ExecContext(ctx, `
			INSERT INTO user_secrets (id, user_id, name, description, value, env_name, file_path)
			VALUES ($1, $2, 'cap-secret-'||$3::text, '', 'small', '', '/tmp/cap-secret-'||$3::text)
		`, id, user.ID, i)
		require.NoError(t, err)
	}

	// 150000 bytes each: either alone fits the 204800-byte cap, both
	// together exceed it. The second update must block on the users-row
	// lock held by the first transaction, then recount against its
	// committed state and fail the cap.
	bigValue := strings.Repeat("x", 150000)
	err := runLockRace(ctx, t, sqlDB,
		[]stmt{{`UPDATE user_secrets SET value = $1 WHERE id = $2`, []any{bigValue, secretA}}},
		stmt{`UPDATE user_secrets SET value = $1 WHERE id = $2`, []any{bigValue, secretB}},
	)
	require.Error(t, err, "the second update must not bypass the byte cap")
	require.True(t, database.IsCheckViolation(err, userSecretsTotalBytesLimit),
		"expected the total-bytes cap violation, got: %v", err)

	var totalBytes int64
	err = sqlDB.QueryRowContext(ctx,
		`SELECT coalesce(sum(octet_length(value)), 0) FROM user_secrets WHERE user_id = $1`, user.ID,
	).Scan(&totalBytes)
	require.NoError(t, err)
	require.LessOrEqual(t, totalBytes, int64(204800), "the committed total must respect the cap")
}

// TestUserSkillsCapConcurrentInserts verifies the count cap holds under
// concurrent inserts: with one slot left, two racing inserts serialize and
// exactly one lands.
func TestUserSkillsCapConcurrentInserts(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.SkipNow()
	}

	db, _, sqlDB := dbtestutil.NewDBWithSQLDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)
	user := dbgen.User(t, db, database.User{})

	_, err := sqlDB.ExecContext(ctx, `
		INSERT INTO user_skills (id, user_id, name, description, content)
		SELECT gen_random_uuid(), $1, 'seed-skill-' || g, '', 'content'
		FROM generate_series(1, 99) AS g
	`, user.ID)
	require.NoError(t, err)

	insert := func(name string) stmt {
		return stmt{`
			INSERT INTO user_skills (id, user_id, name, description, content)
			VALUES ($1, $2, $3, '', 'content')
		`, []any{uuid.New(), user.ID, name}}
	}
	err = runLockRace(ctx, t, sqlDB,
		[]stmt{insert("winner-skill")},
		insert("loser-skill"),
	)
	require.Error(t, err, "the racing insert must recount and fail the cap")
	require.True(t, database.IsCheckViolation(err, userSkillsPerUserLimit),
		"expected the skill cap violation, got: %v", err)

	var count int
	require.NoError(t, sqlDB.QueryRowContext(ctx,
		`SELECT count(*) FROM user_skills WHERE user_id = $1`, user.ID,
	).Scan(&count))
	require.Equal(t, 100, count, "the cap must hold at exactly 100")
}
