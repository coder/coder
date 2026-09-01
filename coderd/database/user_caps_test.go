package database_test

import (
	"database/sql"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/testutil"
)

// TestUserCapAdvisoryLocks pins the advisory-lock key derivations of the
// per-user cap triggers against the live function definitions, so renaming
// a key prefix in a future migration fails here instead of leaving the
// registry comment in coderd/database/lock.go silently stale. It also pins
// that neither function locks the users row: the FOR UPDATE it replaced
// conflicted with foreign-key validation and could deadlock with multi-row
// writers.
func TestUserCapAdvisoryLocks(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.SkipNow()
	}

	_, _, sqlDB := dbtestutil.NewDBWithSQLDB(t)
	ctx := testutil.Context(t, testutil.WaitMedium)

	for function, keyPrefix := range map[string]string{
		"enforce_user_secrets_per_user_limits": "user_secrets_cap:",
		"enforce_user_skills_per_user_limit":   "user_skills_cap:",
	} {
		var def string
		err := sqlDB.QueryRowContext(ctx,
			`SELECT pg_get_functiondef($1::regproc)`, function,
		).Scan(&def)
		require.NoError(t, err)
		require.Contains(t, def,
			"pg_advisory_xact_lock(hashtextextended('"+keyPrefix+"' || NEW.user_id::text, 0))",
			"%s must serialize on the registered advisory key", function)
		require.NotContains(t, def, "FOR UPDATE",
			"%s must not lock the users row", function)
	}
}

// TestUserSecretsCapConcurrentUpdates verifies the per-user advisory lock
// serializes concurrent user_secrets updates so the byte caps hold: two
// transactions growing different rows of the same user must not both pass
// the pre-statement aggregate check.
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
	// together exceed it. The second update must block on the advisory
	// lock held by the first transaction, then recount against its
	// committed state and fail the cap.
	bigValue := strings.Repeat("x", 150000)
	err := runLockRace(ctx, t, sqlDB,
		[]stmt{{`UPDATE user_secrets SET value = $1 WHERE id = $2`, []any{bigValue, secretA}}},
		stmt{`UPDATE user_secrets SET value = $1 WHERE id = $2`, []any{bigValue, secretB}},
		nil,
	)
	require.Error(t, err, "the second update must not bypass the byte cap")
	require.True(t, database.IsCheckViolation(err, database.CheckUserSecretsPerUserTotalBytesLimit),
		"expected the total-bytes cap violation, got: %v", err)

	var totalBytes int64
	err = sqlDB.QueryRowContext(ctx,
		`SELECT coalesce(sum(octet_length(value)), 0) FROM user_secrets WHERE user_id = $1`, user.ID,
	).Scan(&totalBytes)
	require.NoError(t, err)
	require.LessOrEqual(t, totalBytes, int64(204800), "the committed total must respect the cap")
}

// TestUserSkillsCapOwnerReassignment pins the UPDATE leg of the skills cap:
// a same-owner update at the cap succeeds (the trigger's WHEN clause skips
// it; the count cannot change), while reassigning a row onto an owner
// already at the cap fails, closing the UPDATE ... SET user_id bypass.
func TestUserSkillsCapOwnerReassignment(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.SkipNow()
	}

	db, _, sqlDB := dbtestutil.NewDBWithSQLDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)
	fullUser := dbgen.User(t, db, database.User{})
	otherUser := dbgen.User(t, db, database.User{})

	// Fill fullUser to the cap of 100.
	_, err := sqlDB.ExecContext(ctx, `
		INSERT INTO user_skills (id, user_id, name, description, content)
		SELECT gen_random_uuid(), $1, 'cap-skill-' || g, '', 'content'
		FROM generate_series(1, 100) AS g
	`, fullUser.ID)
	require.NoError(t, err)

	// A same-owner update at the cap must succeed: the WHEN clause keeps
	// it out of the trigger function entirely.
	_, err = sqlDB.ExecContext(ctx, `
		UPDATE user_skills SET description = 'edited'
		WHERE user_id = $1 AND name = 'cap-skill-1'
	`, fullUser.ID)
	require.NoError(t, err, "a same-owner update at the cap must not trip the cap")

	// Reassigning another user's row onto the full owner must fail.
	movingSkill := uuid.New()
	_, err = sqlDB.ExecContext(ctx, `
		INSERT INTO user_skills (id, user_id, name, description, content)
		VALUES ($1, $2, 'moving-skill', '', 'content')
	`, movingSkill, otherUser.ID)
	require.NoError(t, err)
	_, err = sqlDB.ExecContext(ctx,
		`UPDATE user_skills SET user_id = $1 WHERE id = $2`, fullUser.ID, movingSkill)
	require.Error(t, err, "reassigning onto a full owner must fail the cap")
	require.True(t, database.IsCheckViolation(err, database.CheckUserSkillsPerUserLimit),
		"expected the skill cap violation, got: %v", err)

	// Reassigning onto an owner with room succeeds: the moving row still
	// belongs to the old owner while counting, so the count is exact.
	roomUser := dbgen.User(t, db, database.User{})
	_, err = sqlDB.ExecContext(ctx,
		`UPDATE user_skills SET user_id = $1 WHERE id = $2`, roomUser.ID, movingSkill)
	require.NoError(t, err, "reassigning onto an owner with room must succeed")
}

// TestUserSkillsCapConcurrentInserts proves the advisory lock is
// load-bearing for the count cap: with one slot left, two racing inserts
// serialize on the per-user advisory lock and exactly one lands.
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
		nil,
	)
	require.Error(t, err, "the racing insert must recount and fail the cap")
	require.True(t, database.IsCheckViolation(err, database.CheckUserSkillsPerUserLimit),
		"expected the skill cap violation, got: %v", err)

	var count int
	require.NoError(t, sqlDB.QueryRowContext(ctx,
		`SELECT count(*) FROM user_skills WHERE user_id = $1`, user.ID,
	).Scan(&count))
	require.Equal(t, 100, count, "the cap must hold at exactly 100")
}

// TestUserSkillsCapConcurrentReassignment covers the leg where the advisory
// lock alone is load-bearing: an insert and an owner reassignment racing
// for the last slot of the same target owner serialize on the advisory
// lock, and the loser recounts against the winner's committed row.
func TestUserSkillsCapConcurrentReassignment(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.SkipNow()
	}

	db, _, sqlDB := dbtestutil.NewDBWithSQLDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)
	target := dbgen.User(t, db, database.User{})
	source := dbgen.User(t, db, database.User{})

	_, err := sqlDB.ExecContext(ctx, `
		INSERT INTO user_skills (id, user_id, name, description, content)
		SELECT gen_random_uuid(), $1, 'seed-skill-' || g, '', 'content'
		FROM generate_series(1, 99) AS g
	`, target.ID)
	require.NoError(t, err)

	movingSkill := uuid.New()
	_, err = sqlDB.ExecContext(ctx, `
		INSERT INTO user_skills (id, user_id, name, description, content)
		VALUES ($1, $2, 'moving-skill', '', 'content')
	`, movingSkill, source.ID)
	require.NoError(t, err)

	// The blocking insert takes the target owner's advisory lock; the
	// racing reassignment onto the same owner must wait on it (it touches
	// no row the insert wrote, so only the advisory lock can order them)
	// and then fail the recount.
	err = runLockRace(ctx, t, sqlDB,
		[]stmt{{`
			INSERT INTO user_skills (id, user_id, name, description, content)
			VALUES ($1, $2, 'winner-skill', '', 'content')
		`, []any{uuid.New(), target.ID}}},
		stmt{`UPDATE user_skills SET user_id = $1 WHERE id = $2`, []any{target.ID, movingSkill}},
		nil,
	)
	require.Error(t, err, "the racing reassignment must recount and fail the cap")
	require.True(t, database.IsCheckViolation(err, database.CheckUserSkillsPerUserLimit),
		"expected the skill cap violation, got: %v", err)

	var count int
	require.NoError(t, sqlDB.QueryRowContext(ctx,
		`SELECT count(*) FROM user_skills WHERE user_id = $1`, target.ID,
	).Scan(&count))
	require.Equal(t, 100, count, "the cap must hold at exactly 100")
}

// TestUserCapsIsolationContract pins the stated (not trigger-enforced)
// isolation contract: cap writes are race-free at the default READ
// COMMITTED level, and transactions at stronger levels are accepted rather
// than rejected. dbcrypt rotation rewrites user_secrets values under
// REPEATABLE READ and must keep working; there is deliberately no runtime
// isolation gate because it would turn a deployment-level
// default_transaction_isolation setting into a total outage of secret and
// skill writes.
func TestUserCapsIsolationContract(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.SkipNow()
	}

	db, _, sqlDB := dbtestutil.NewDBWithSQLDB(t)

	t.Run("RepeatableReadSameOwnerUpdate", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		user := dbgen.User(t, db, database.User{})

		secretID := uuid.New()
		_, err := sqlDB.ExecContext(ctx, `
			INSERT INTO user_secrets (id, user_id, name, description, value, env_name, file_path)
			VALUES ($1, $2, 'rotate-secret', '', 'ciphertext-old', '', '/tmp/rotate-secret')
		`, secretID, user.ID)
		require.NoError(t, err)

		// The dbcrypt rotation shape: rewrite the value in place under
		// REPEATABLE READ.
		tx, err := sqlDB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
		require.NoError(t, err)
		defer tx.Rollback() //nolint:errcheck // no-op after commit
		_, err = tx.ExecContext(ctx,
			`UPDATE user_secrets SET value = 'ciphertext-new' WHERE id = $1`, secretID)
		require.NoError(t, err, "same-owner updates under REPEATABLE READ must keep working")
		require.NoError(t, tx.Commit())
	})

	t.Run("RepeatableReadInsert", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		user := dbgen.User(t, db, database.User{})

		tx, err := sqlDB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
		require.NoError(t, err)
		defer tx.Rollback() //nolint:errcheck // no-op after commit
		_, err = tx.ExecContext(ctx, `
			INSERT INTO user_skills (id, user_id, name, description, content)
			VALUES ($1, $2, 'rr-skill', '', 'content')
		`, uuid.New(), user.ID)
		require.NoError(t, err, "inserts at stronger isolation are accepted, not gated")
		require.NoError(t, tx.Commit())
	})
}
