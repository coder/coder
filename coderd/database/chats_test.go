package database_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/testutil"
)

func TestGetChatsByIDs(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitMedium)
	user := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})
	dbgen.OrganizationMember(t, db, database.OrganizationMember{UserID: user.ID, OrganizationID: org.ID})
	model := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{OrganizationID: org.ID})
	newChat := func() database.Chat {
		return dbgen.Chat(t, db, database.Chat{OrganizationID: org.ID, OwnerID: user.ID, LastModelConfigID: model.ID})
	}
	first, second, third := newChat(), newChat(), newChat()

	chats, err := db.GetChatsByIDs(ctx, []uuid.UUID{first.ID, third.ID, uuid.New()})
	require.NoError(t, err)
	gotIDs := make([]uuid.UUID, 0, len(chats))
	for _, c := range chats {
		gotIDs = append(gotIDs, c.ID)
	}
	require.ElementsMatch(t, []uuid.UUID{first.ID, third.ID}, gotIDs)
	require.NotContains(t, gotIDs, second.ID)

	chats, err = db.GetChatsByIDs(ctx, []uuid.UUID{})
	require.NoError(t, err)
	require.Empty(t, chats)
}
