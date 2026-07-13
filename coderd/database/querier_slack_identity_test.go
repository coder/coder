package database_test

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/testutil"
)

// slackOAuthExtra builds the oauth_extra payload the Slack external
// auth provider persists, carrying the authed_user object.
func slackOAuthExtra(t *testing.T, authedUserID any) pqtype.NullRawMessage {
	t.Helper()
	extra, err := json.Marshal(map[string]any{
		"authed_user": map[string]any{"id": authedUserID},
	})
	require.NoError(t, err)
	return pqtype.NullRawMessage{RawMessage: extra, Valid: true}
}

func linkSlackUser(t *testing.T, db database.Store, providerID string, userID uuid.UUID, extra pqtype.NullRawMessage) {
	t.Helper()
	dbgen.ExternalAuthLink(t, db, database.ExternalAuthLink{
		ProviderID: providerID,
		UserID:     userID,
		OAuthExtra: extra,
	})
}

func TestGetUsersByExternalAuthProviderUserID(t *testing.T) {
	t.Parallel()

	t.Run("SingleMatch", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitLong)

		user := dbgen.User(t, db, database.User{})
		linkSlackUser(t, db, "slack", user.ID, slackOAuthExtra(t, "U123"))
		// A different Slack identity under the same provider does
		// not match.
		other := dbgen.User(t, db, database.User{})
		linkSlackUser(t, db, "slack", other.ID, slackOAuthExtra(t, "U999"))

		users, err := db.GetUsersByExternalAuthProviderUserID(ctx, database.GetUsersByExternalAuthProviderUserIDParams{
			ProviderID:     "slack",
			ExternalUserID: "U123",
		})
		require.NoError(t, err)
		require.Len(t, users, 1)
		require.Equal(t, user.ID, users[0].ID)
	})

	t.Run("NoMatch", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitLong)

		user := dbgen.User(t, db, database.User{})
		linkSlackUser(t, db, "slack", user.ID, slackOAuthExtra(t, "U123"))

		users, err := db.GetUsersByExternalAuthProviderUserID(ctx, database.GetUsersByExternalAuthProviderUserIDParams{
			ProviderID:     "slack",
			ExternalUserID: "UNOBODY",
		})
		require.NoError(t, err)
		require.Empty(t, users)
	})

	t.Run("DifferentProviderDoesNotMatch", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitLong)

		user := dbgen.User(t, db, database.User{})
		linkSlackUser(t, db, "other-slack", user.ID, slackOAuthExtra(t, "U123"))

		users, err := db.GetUsersByExternalAuthProviderUserID(ctx, database.GetUsersByExternalAuthProviderUserIDParams{
			ProviderID:     "slack",
			ExternalUserID: "U123",
		})
		require.NoError(t, err)
		require.Empty(t, users)

		// The same identity is still visible under its own provider.
		users, err = db.GetUsersByExternalAuthProviderUserID(ctx, database.GetUsersByExternalAuthProviderUserIDParams{
			ProviderID:     "other-slack",
			ExternalUserID: "U123",
		})
		require.NoError(t, err)
		require.Len(t, users, 1)
		require.Equal(t, user.ID, users[0].ID)
	})

	t.Run("MalformedOAuthExtraDoesNotMatch", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitLong)

		cases := []struct {
			name  string
			extra pqtype.NullRawMessage
		}{
			{"NullOAuthExtra", pqtype.NullRawMessage{}},
			{"NoAuthedUser", pqtype.NullRawMessage{RawMessage: []byte(`{"scope":"chat:write"}`), Valid: true}},
			{"AuthedUserNotObject", pqtype.NullRawMessage{RawMessage: []byte(`{"authed_user":"U123"}`), Valid: true}},
			{"NullID", slackOAuthExtra(t, nil)},
			{"EmptyID", slackOAuthExtra(t, "")},
			{"NumericID", slackOAuthExtra(t, 42)},
		}
		for _, tc := range cases {
			user := dbgen.User(t, db, database.User{})
			linkSlackUser(t, db, "slack", user.ID, tc.extra)
		}

		users, err := db.GetUsersByExternalAuthProviderUserID(ctx, database.GetUsersByExternalAuthProviderUserIDParams{
			ProviderID:     "slack",
			ExternalUserID: "U123",
		})
		require.NoError(t, err)
		require.Empty(t, users)
	})

	t.Run("MultipleLinkedAccountsAllReturned", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitLong)

		// Multiple Coder accounts may link the same Slack identity
		// under one provider; no uniqueness constraint exists and the
		// lookup returns every linked account.
		userA := dbgen.User(t, db, database.User{})
		userB := dbgen.User(t, db, database.User{})
		linkSlackUser(t, db, "slack", userA.ID, slackOAuthExtra(t, "U123"))
		linkSlackUser(t, db, "slack", userB.ID, slackOAuthExtra(t, "U123"))

		users, err := db.GetUsersByExternalAuthProviderUserID(ctx, database.GetUsersByExternalAuthProviderUserIDParams{
			ProviderID:     "slack",
			ExternalUserID: "U123",
		})
		require.NoError(t, err)
		require.Len(t, users, 2)
		ids := []uuid.UUID{users[0].ID, users[1].ID}
		require.ElementsMatch(t, []uuid.UUID{userA.ID, userB.ID}, ids)
	})

	t.Run("UnusableUsersRemainVisible", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitLong)

		// Deleted, suspended, dormant, and system users must be
		// returned so callers can reject them (or detect ambiguity)
		// instead of silently skipping them.
		deleted := dbgen.User(t, db, database.User{Deleted: true})
		suspended := dbgen.User(t, db, database.User{Status: database.UserStatusSuspended})
		dormant := dbgen.User(t, db, database.User{Status: database.UserStatusDormant})
		expected := []uuid.UUID{deleted.ID, suspended.ID, dormant.ID, database.PrebuildsSystemUserID}
		for _, id := range expected {
			linkSlackUser(t, db, "slack", id, slackOAuthExtra(t, "U123"))
		}

		users, err := db.GetUsersByExternalAuthProviderUserID(ctx, database.GetUsersByExternalAuthProviderUserIDParams{
			ProviderID:     "slack",
			ExternalUserID: "U123",
		})
		require.NoError(t, err)
		var got []uuid.UUID
		for _, u := range users {
			got = append(got, u.ID)
		}
		require.ElementsMatch(t, expected, got)
	})
}

func TestGetChatsByLabels(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	org := dbgen.Organization(t, db, database.Organization{})
	model := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{Model: "test-model"})
	ownerA := dbgen.User(t, db, database.User{})
	ownerB := dbgen.User(t, db, database.User{})

	newChat := func(owner uuid.UUID, labels database.StringMap) database.Chat {
		return dbgen.Chat(t, db, database.Chat{
			OrganizationID:    org.ID,
			OwnerID:           owner,
			Title:             "labeled chat",
			LastModelConfigID: model.ID,
			Labels:            labels,
		})
	}
	threadLabels := database.StringMap{"slackd": "true", "slack_thread": "C1:100.1"}
	chatA := newChat(ownerA.ID, threadLabels)
	// A chat under a different owner with different thread labels.
	otherThread := newChat(ownerB.ID, database.StringMap{"slackd": "true", "slack_thread": "C2:200.2"})
	// A chat missing part of the filter.
	newChat(ownerB.ID, database.StringMap{"slackd": "true"})

	filter := func(labels map[string]string) json.RawMessage {
		raw, err := json.Marshal(labels)
		require.NoError(t, err)
		return raw
	}

	// The lookup is global: chatA is found without knowing its owner.
	chats, err := db.GetChatsByLabels(ctx, filter(threadLabels))
	require.NoError(t, err)
	require.Len(t, chats, 1)
	require.Equal(t, chatA.ID, chats[0].ID)
	require.Equal(t, ownerA.ID, chats[0].OwnerID)

	// A different thread key finds the other owner's chat.
	chats, err = db.GetChatsByLabels(ctx, filter(map[string]string{"slackd": "true", "slack_thread": "C2:200.2"}))
	require.NoError(t, err)
	require.Len(t, chats, 1)
	require.Equal(t, otherThread.ID, chats[0].ID)

	// Chats whose labels do not contain the complete filter are
	// excluded.
	chats, err = db.GetChatsByLabels(ctx, filter(map[string]string{"slackd": "true", "slack_thread": "C9:999.9"}))
	require.NoError(t, err)
	require.Empty(t, chats)

	// Archived chats are excluded.
	_, err = db.ArchiveChatByID(ctx, chatA.ID)
	require.NoError(t, err)
	chats, err = db.GetChatsByLabels(ctx, filter(threadLabels))
	require.NoError(t, err)
	require.Empty(t, chats)
}
