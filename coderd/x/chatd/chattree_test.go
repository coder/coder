package chatd_test

import (
	"context"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/x/chatd"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func staticModelResolver(id uuid.UUID) func(context.Context) (uuid.UUID, error) {
	return func(context.Context) (uuid.UUID, error) { return id, nil }
}

func TestEnsureChatTreeRoot(t *testing.T) {
	t.Parallel()

	t.Run("ConcurrentCallersShareOneRoot", func(t *testing.T) {
		t.Parallel()

		db, ps := dbtestutil.NewDB(t)
		server := newTestServer(t, db, ps, uuid.New())
		ctx := testutil.Context(t, testutil.WaitLong)
		user, org, model := seedChatDependencies(t, db)

		legacy := dbgen.Chat(t, db, database.Chat{
			OrganizationID:    org.ID,
			OwnerID:           user.ID,
			LastModelConfigID: model.ID,
			Title:             "legacy",
		})

		const callers = 8
		roots := make([]database.Chat, callers)
		created := make([]bool, callers)
		errs := make([]error, callers)
		var wg sync.WaitGroup
		for i := 0; i < callers; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				roots[i], created[i], errs[i] = server.EnsureChatTreeRoot(ctx, chatd.EnsureChatTreeRootOptions{
					OwnerID:              user.ID,
					OrganizationID:       org.ID,
					ResolveModelConfigID: staticModelResolver(model.ID),
				})
			}(i)
		}
		wg.Wait()

		createdCount := 0
		for i := 0; i < callers; i++ {
			require.NoError(t, errs[i])
			require.Equal(t, roots[0].ID, roots[i].ID)
			if created[i] {
				createdCount++
			}
		}
		require.Equal(t, 1, createdCount)
		require.Equal(t, database.ChatKindRoot, roots[0].Kind)
		require.Equal(t, database.ChatStatusWaiting, roots[0].Status)
		require.Equal(t, chatd.ChatTreeRootTitle, roots[0].Title)
		require.False(t, roots[0].WorkspaceID.Valid)

		adopted, err := db.GetChatByID(ctx, legacy.ID)
		require.NoError(t, err)
		require.Equal(t, roots[0].ID, adopted.ParentChatID.UUID)
		require.Equal(t, database.ChatKindChat, adopted.Kind)
		require.Equal(t, legacy.UpdatedAt.UTC(), adopted.UpdatedAt.UTC())

		// The root itself has no messages and no owner hint, so no worker
		// would pick it up.
		messages, err := db.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{ChatID: roots[0].ID})
		require.NoError(t, err)
		require.Empty(t, messages)
	})

	t.Run("SubagentsAndOtherOrganizationsUntouched", func(t *testing.T) {
		t.Parallel()

		db, ps := dbtestutil.NewDB(t)
		server := newTestServer(t, db, ps, uuid.New())
		ctx := testutil.Context(t, testutil.WaitLong)
		user, org, model := seedChatDependencies(t, db)

		parent := dbgen.Chat(t, db, database.Chat{
			OrganizationID:    org.ID,
			OwnerID:           user.ID,
			LastModelConfigID: model.ID,
			Title:             "parent",
		})
		subagent := dbgen.Chat(t, db, database.Chat{
			OrganizationID:    org.ID,
			OwnerID:           user.ID,
			LastModelConfigID: model.ID,
			ParentChatID:      uuid.NullUUID{UUID: parent.ID, Valid: true},
			RootChatID:        uuid.NullUUID{UUID: parent.ID, Valid: true},
			Title:             "subagent",
		})
		otherOrg := dbgen.Organization(t, db, database.Organization{})
		otherOrgChat := dbgen.Chat(t, db, database.Chat{
			OrganizationID:    otherOrg.ID,
			OwnerID:           user.ID,
			LastModelConfigID: model.ID,
			Title:             "other org",
		})

		root, created, err := server.EnsureChatTreeRoot(ctx, chatd.EnsureChatTreeRootOptions{
			OwnerID:              user.ID,
			OrganizationID:       org.ID,
			ResolveModelConfigID: staticModelResolver(model.ID),
		})
		require.NoError(t, err)
		require.True(t, created)

		gotSubagent, err := db.GetChatByID(ctx, subagent.ID)
		require.NoError(t, err)
		require.Equal(t, parent.ID, gotSubagent.ParentChatID.UUID)
		require.Equal(t, parent.ID, gotSubagent.RootChatID.UUID)
		gotOther, err := db.GetChatByID(ctx, otherOrgChat.ID)
		require.NoError(t, err)
		require.False(t, gotOther.ParentChatID.Valid)
		gotParent, err := db.GetChatByID(ctx, parent.ID)
		require.NoError(t, err)
		require.Equal(t, root.ID, gotParent.ParentChatID.UUID)
	})

	t.Run("NoModelConfig", func(t *testing.T) {
		t.Parallel()

		db, ps := dbtestutil.NewDB(t)
		server := newTestServer(t, db, ps, uuid.New())
		ctx := testutil.Context(t, testutil.WaitLong)
		user, org, _ := seedChatDependencies(t, db)

		_, _, err := server.EnsureChatTreeRoot(ctx, chatd.EnsureChatTreeRootOptions{
			OwnerID:        user.ID,
			OrganizationID: org.ID,
			ResolveModelConfigID: func(context.Context) (uuid.UUID, error) {
				return uuid.Nil, xerrors.New("no model")
			},
		})
		require.ErrorIs(t, err, chatd.ErrChatTreeRootUnavailable)

		state, err := db.GetChatTreeRootStateByOwnerAndOrganization(ctx, database.GetChatTreeRootStateByOwnerAndOrganizationParams{
			OwnerID:        user.ID,
			OrganizationID: org.ID,
		})
		require.NoError(t, err)
		require.Equal(t, uuid.Nil, state.RootChatID)
	})
}

func TestCreateChat_TreeParent(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	server := newTestServer(t, db, ps, uuid.New())
	ctx := testutil.Context(t, testutil.WaitLong)
	user, org, model := seedChatDependencies(t, db)

	root, _, err := server.EnsureChatTreeRoot(ctx, chatd.EnsureChatTreeRootOptions{
		OwnerID:              user.ID,
		OrganizationID:       org.ID,
		ResolveModelConfigID: staticModelResolver(model.ID),
	})
	require.NoError(t, err)

	create := func(parent uuid.UUID) (database.Chat, error) {
		return server.CreateChat(ctx, chatd.CreateOptions{
			OrganizationID:     org.ID,
			OwnerID:            user.ID,
			Title:              "child",
			ModelConfigID:      model.ID,
			ParentChatID:       uuid.NullUUID{UUID: parent, Valid: true},
			InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("hi")},
		})
	}

	parent := root.ID
	for depth := 2; depth <= codersdk.ChatTreeMaxDepth; depth++ {
		chat, err := create(parent)
		require.NoError(t, err, "depth %d", depth)
		require.Equal(t, database.ChatKindChat, chat.Kind)
		require.Equal(t, parent, chat.ParentChatID.UUID)
		stored, err := db.GetChatByID(ctx, chat.ID)
		require.NoError(t, err)
		require.False(t, stored.RootChatID.Valid, "named children carry no root_chat_id")
		got, err := db.GetChatTreeDepthByID(ctx, chat.ID)
		require.NoError(t, err)
		require.EqualValues(t, depth, got)
		parent = chat.ID
	}
	_, err = create(parent)
	require.ErrorIs(t, err, chatd.ErrChatTreeDepthExceeded)

	_, err = create(uuid.New())
	require.ErrorIs(t, err, chatd.ErrChatTreeParentMismatch)

	otherUser := dbgen.User(t, db, database.User{})
	otherChat := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    org.ID,
		OwnerID:           otherUser.ID,
		LastModelConfigID: model.ID,
		Title:             "other",
	})
	_, err = create(otherChat.ID)
	require.ErrorIs(t, err, chatd.ErrChatTreeParentMismatch)

	subagent := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    org.ID,
		OwnerID:           user.ID,
		LastModelConfigID: model.ID,
		ParentChatID:      uuid.NullUUID{UUID: root.ID, Valid: true},
		RootChatID:        uuid.NullUUID{UUID: root.ID, Valid: true},
		Title:             "subagent",
	})
	_, err = create(subagent.ID)
	require.ErrorIs(t, err, chatd.ErrChatTreeParentIsSubagent)

	// An archived parent rejects children; the tree root cannot be archived.
	archivedParent := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    org.ID,
		OwnerID:           user.ID,
		LastModelConfigID: model.ID,
		Kind:              database.ChatKindChat,
		ParentChatID:      uuid.NullUUID{UUID: root.ID, Valid: true},
		Title:             "archived parent",
	})
	require.NoError(t, server.ArchiveChat(ctx, archivedParent))
	_, err = create(archivedParent.ID)
	require.ErrorIs(t, err, chatd.ErrChatTreeParentArchived)
	require.Error(t, server.ArchiveChat(ctx, root))
}
