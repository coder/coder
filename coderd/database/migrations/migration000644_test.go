package migrations_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/testutil"
)

// Migration 000644 moves chats out of projects their owners can no longer
// use and leaves the rest.
func TestMigration000644RemovesChatsFromUnusableChatProjects(t *testing.T) {
	t.Parallel()

	sqlDB := testSQLDB(t)
	// 000644 changes only data, so the schema at 000643 is the current one.
	stepTo(t, sqlDB, 643)
	db := database.New(sqlDB)
	ctx := testutil.Context(t, testutil.WaitLong)

	org := dbgen.Organization(t, db, database.Organization{})
	member := func() database.User {
		u := dbgen.User(t, db, database.User{})
		dbgen.OrganizationMember(t, db, database.OrganizationMember{UserID: u.ID, OrganizationID: org.ID})
		return u
	}
	projectOwner, direct, viaGroup, revoked := member(), member(), member(), member()
	group := dbgen.Group(t, db, database.Group{OrganizationID: org.ID})
	dbgen.GroupMember(t, db, database.GroupMemberTable{UserID: viaGroup.ID, GroupID: group.ID})
	project := dbgen.ChatProject(t, db, database.ChatProject{OrganizationID: org.ID, OwnerID: projectOwner.ID})
	require.NoError(t, db.UpdateChatProjectACLByID(ctx, database.UpdateChatProjectACLByIDParams{
		ID:       project.ID,
		UserACL:  database.ChatACL{direct.ID.String(): {Permissions: []policy.Action{policy.ActionRead}}},
		GroupACL: database.ChatACL{group.ID.String(): {Permissions: []policy.Action{policy.ActionRead}}},
	}))
	_ = dbgen.ChatProvider(t, db, database.ChatProvider{Provider: "openai", DisplayName: "OpenAI"})
	modelConfig := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{Model: "test-model", IsDefault: true, CompressionThreshold: 80})
	chats := map[uuid.UUID]bool{}
	for _, tc := range []struct {
		owner     database.User
		inProject bool
	}{{projectOwner, true}, {direct, true}, {viaGroup, true}, {revoked, false}} {
		chat := dbgen.Chat(t, db, database.Chat{
			OrganizationID:    org.ID,
			OwnerID:           tc.owner.ID,
			LastModelConfigID: modelConfig.ID,
			ProjectID:         uuid.NullUUID{UUID: project.ID, Valid: true},
		})
		chats[chat.ID] = tc.inProject
	}

	stepTo(t, sqlDB, 644)

	for id, inProject := range chats {
		got, err := db.GetChatByID(ctx, id)
		require.NoError(t, err)
		require.Equal(t, inProject, got.ProjectID.Valid, "chat of %s", got.OwnerID)
	}
}
