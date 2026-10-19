package migrations_test

import (
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/testutil"
)

// The up migration gives existing and newly inserted sub-chats their root's
// project, and the down migration clears it.
func TestMigration000630SubChatProjectID(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.SkipNow()
	}

	sqlDB := testSQLDB(t)
	stepTo(t, sqlDB, 629)
	ctx := testutil.Context(t, testutil.WaitLong)

	db := database.New(sqlDB)
	org := dbgen.Organization(t, db, database.Organization{})
	owner := dbgen.User(t, db, database.User{})
	dbgen.ChatProvider(t, db, database.ChatProvider{Provider: "openai", DisplayName: "OpenAI"})
	model := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{
		Model:                "test-model",
		CreatedBy:            uuid.NullUUID{UUID: owner.ID, Valid: true},
		UpdatedBy:            uuid.NullUUID{UUID: owner.ID, Valid: true},
		IsDefault:            true,
		CompressionThreshold: 80,
	})
	projectID := uuid.New()
	_, err := sqlDB.ExecContext(ctx, `INSERT INTO chat_projects (id, organization_id, owner_id, name) VALUES ($1, $2, $3, 'project')`,
		projectID, org.ID, owner.ID)
	require.NoError(t, err)

	nullID := func(id uuid.UUID) uuid.NullUUID { return uuid.NullUUID{UUID: id, Valid: id != uuid.Nil} }
	insertChat := func(project, parent, root uuid.UUID) uuid.UUID {
		id := uuid.New()
		_, err := sqlDB.ExecContext(ctx, `INSERT INTO chats (id, organization_id, owner_id, last_model_config_id, title, project_id, parent_chat_id, root_chat_id)
			VALUES ($1, $2, $3, $4, 'chat', $5, $6, $7)`, id, org.ID, owner.ID, model.ID, nullID(project), nullID(parent), nullID(root))
		require.NoError(t, err)
		return id
	}
	projectOf := func(id uuid.UUID) uuid.NullUUID {
		var project uuid.NullUUID
		require.NoError(t, sqlDB.QueryRowContext(ctx, "SELECT project_id FROM chats WHERE id = $1", id).Scan(&project))
		return project
	}
	migrate := func(file string) {
		migration, err := os.ReadFile(file)
		require.NoError(t, err)
		_, err = sqlDB.ExecContext(ctx, string(migration))
		require.NoError(t, err)
	}

	root := insertChat(projectID, uuid.Nil, uuid.Nil)
	child := insertChat(uuid.Nil, root, root)
	grandchild := insertChat(uuid.Nil, child, root)
	// A deleted parent leaves only root_chat_id set.
	orphan := insertChat(uuid.Nil, uuid.Nil, root)
	unrelated := insertChat(uuid.Nil, uuid.Nil, uuid.Nil)
	unrelatedChild := insertChat(uuid.Nil, unrelated, unrelated)

	migrate("000630_chat_project_deletion.up.sql")
	inProject := nullID(projectID)
	for name, id := range map[string]uuid.UUID{"root": root, "child": child, "grandchild": grandchild, "orphan": orphan} {
		require.Equal(t, inProject, projectOf(id), name)
	}
	require.False(t, projectOf(unrelatedChild).Valid)

	insertedChild := insertChat(uuid.Nil, root, root)
	require.Equal(t, inProject, projectOf(insertedChild), "the trigger gives a new sub-chat its root's project")
	insertedUnrelatedChild := insertChat(projectID, unrelated, unrelated)
	require.False(t, projectOf(insertedUnrelatedChild).Valid, "a sub-chat cannot join a project its root is not in")

	migrate("000630_chat_project_deletion.down.sql")
	require.Equal(t, inProject, projectOf(root))
	for name, id := range map[string]uuid.UUID{"child": child, "grandchild": grandchild, "orphan": orphan, "insertedChild": insertedChild} {
		require.False(t, projectOf(id).Valid, name)
	}
	insertedAfterDown := insertChat(uuid.Nil, root, root)
	require.False(t, projectOf(insertedAfterDown).Valid, "the down migration drops the trigger")
}
