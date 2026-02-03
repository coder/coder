package database_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
)

func TestChats_InsertAndListMessages(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	ctx := context.Background()

	owner := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})

	now := dbtime.Now()
	chatID := uuid.New()
	chat, err := db.InsertChat(ctx, database.InsertChatParams{
		ID:             chatID,
		CreatedAt:      now,
		UpdatedAt:      now,
		OrganizationID: org.ID,
		OwnerID:        owner.ID,
		WorkspaceID:    uuid.NullUUID{},
		Title:          sql.NullString{},
		Provider:       "openai",
		Model:          "gpt-4o-mini",
		Metadata:       json.RawMessage(`{}`),
	})
	require.NoError(t, err)
	require.Equal(t, chatID, chat.ID)
	require.Equal(t, owner.ID, chat.OwnerID)
	require.Equal(t, org.ID, chat.OrganizationID)
	require.False(t, chat.WorkspaceID.Valid)

	msg1, err := db.InsertChatMessage(ctx, database.InsertChatMessageParams{
		ChatID:    chatID,
		CreatedAt: now,
		Role:      "user",
		Content:   json.RawMessage(`{"type":"text","text":"hello"}`),
	})
	require.NoError(t, err)
	require.Equal(t, chatID, msg1.ChatID)
	require.Equal(t, "user", msg1.Role)

	msg2, err := db.InsertChatMessage(ctx, database.InsertChatMessageParams{
		ChatID:    chatID,
		CreatedAt: now,
		Role:      "assistant",
		Content:   json.RawMessage(`{"type":"text","text":"hi"}`),
	})
	require.NoError(t, err)
	require.Greater(t, msg2.ID, msg1.ID)

	msgs, err := db.ListChatMessages(ctx, chatID)
	require.NoError(t, err)
	require.Len(t, msgs, 2)
	require.Equal(t, msg1.ID, msgs[0].ID)
	require.Equal(t, msg2.ID, msgs[1].ID)
}

func TestChats_MessageRoleCheckConstraint(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	ctx := context.Background()

	owner := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})

	now := dbtime.Now()
	chat, err := db.InsertChat(ctx, database.InsertChatParams{
		ID:             uuid.New(),
		CreatedAt:      now,
		UpdatedAt:      now,
		OrganizationID: org.ID,
		OwnerID:        owner.ID,
		WorkspaceID:    uuid.NullUUID{},
		Title:          sql.NullString{},
		Provider:       "anthropic",
		Model:          "claude-3-5-sonnet",
		Metadata:       json.RawMessage(`{}`),
	})
	require.NoError(t, err)

	_, err = db.InsertChatMessage(ctx, database.InsertChatMessageParams{
		ChatID:    chat.ID,
		CreatedAt: now,
		Role:      "tool", // invalid; must be tool_call or tool_result.
		Content:   json.RawMessage(`{}`),
	})
	require.Error(t, err)
	require.True(t, database.IsCheckViolation(err, database.CheckChatMessagesRole))
}

func TestChats_UpdateWorkspaceIDSetOnce(t *testing.T) {
	t.Parallel()

	db, _, sqlDB := dbtestutil.NewDBWithSQLDB(t)
	ctx := context.Background()

	owner := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})
	tpl := dbgen.Template(t, db, database.Template{
		OrganizationID: org.ID,
		CreatedBy:      owner.ID,
	})
	ws := dbgen.Workspace(t, db, database.WorkspaceTable{
		OrganizationID: org.ID,
		OwnerID:        owner.ID,
		TemplateID:     tpl.ID,
	})

	now := dbtime.Now()
	chat, err := db.InsertChat(ctx, database.InsertChatParams{
		ID:             uuid.New(),
		CreatedAt:      now,
		UpdatedAt:      now,
		OrganizationID: org.ID,
		OwnerID:        owner.ID,
		WorkspaceID:    uuid.NullUUID{},
		Title:          sql.NullString{},
		Provider:       "openai",
		Model:          "gpt-4o-mini",
		Metadata:       json.RawMessage(`{}`),
	})
	require.NoError(t, err)
	require.False(t, chat.WorkspaceID.Valid)

	updated, err := db.UpdateChatWorkspaceID(ctx, database.UpdateChatWorkspaceIDParams{
		ID:          chat.ID,
		WorkspaceID: uuid.NullUUID{UUID: ws.ID, Valid: true},
		UpdatedAt:   now,
	})
	require.NoError(t, err)
	require.True(t, updated.WorkspaceID.Valid)
	require.Equal(t, ws.ID, updated.WorkspaceID.UUID)

	// Ensure the link is set-once.
	_, err = db.UpdateChatWorkspaceID(ctx, database.UpdateChatWorkspaceIDParams{
		ID:          chat.ID,
		WorkspaceID: uuid.NullUUID{UUID: uuid.New(), Valid: true},
		UpdatedAt:   now,
	})
	require.Error(t, err)
	require.True(t, errors.Is(err, sql.ErrNoRows))

	// Verify ON DELETE RESTRICT on chats.workspace_id.
	_, err = sqlDB.ExecContext(ctx, "DELETE FROM workspaces WHERE id = $1", ws.ID)
	require.Error(t, err)
	require.True(t, database.IsForeignKeyViolation(err, database.ForeignKeyChatsWorkspaceID))
}
