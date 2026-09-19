package coderd_test

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/serpent"
)

// chatTreeExperiments enables the chat runtime experiments plus chat-tree.
func chatTreeExperiments() func(*coderdtest.Options) {
	return func(o *coderdtest.Options) {
		o.DeploymentValues.Experiments = serpent.StringArray{
			string(codersdk.ExperimentChatAdvisor),
			string(codersdk.ExperimentChatVirtualDesktop),
			string(codersdk.ExperimentAgentLifecycleHooks),
			string(codersdk.ExperimentChatTree),
		}
	}
}

func createTreeChat(t *testing.T, client *codersdk.ExperimentalClient, orgID uuid.UUID, text string, parentID *uuid.UUID) codersdk.Chat {
	t.Helper()
	ctx := testutil.Context(t, testutil.WaitLong)
	chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
		OrganizationID: orgID,
		Content:        []codersdk.ChatInputPart{{Type: codersdk.ChatInputPartTypeText, Text: text}},
		ParentChatID:   parentID,
	})
	require.NoError(t, err)
	return chat
}

func requireChatAPIError(t *testing.T, err error, status int, message string) {
	t.Helper()
	var apiErr *codersdk.Error
	require.ErrorAs(t, err, &apiErr)
	require.Equal(t, status, apiErr.StatusCode())
	if message != "" {
		require.Contains(t, apiErr.Message, message)
	}
}

func TestChatTree_ExperimentOff(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client, db := newChatClientWithDatabase(t)
	firstUser := coderdtest.CreateFirstUser(t, client.Client)
	_ = createChatModel(t, client)

	_, err := client.ChatTree(ctx, firstUser.OrganizationID, nil)
	requireChatAPIError(t, err, http.StatusNotFound, "")

	_, err = client.CreateChat(ctx, codersdk.CreateChatRequest{
		OrganizationID: firstUser.OrganizationID,
		Content:        []codersdk.ChatInputPart{{Type: codersdk.ChatInputPartTypeText, Text: "hello"}},
		ParentChatID:   ptr.Ref(uuid.New()),
	})
	requireChatAPIError(t, err, http.StatusBadRequest, "parent_chat_id requires the chat-tree experiment")

	// A plain create stays parentless and no root row appears.
	chat := createTreeChat(t, client, firstUser.OrganizationID, "hello", nil)
	require.Nil(t, chat.ParentChatID)
	require.Equal(t, codersdk.ChatKindChat, chat.Kind)
	state, err := db.GetChatTreeRootStateByOwnerAndOrganization(dbauthz.AsSystemRestricted(ctx), database.GetChatTreeRootStateByOwnerAndOrganizationParams{
		OwnerID:        firstUser.UserID,
		OrganizationID: firstUser.OrganizationID,
	})
	require.NoError(t, err)
	require.Equal(t, uuid.Nil, state.RootChatID)
	require.EqualValues(t, 1, state.AdoptableCount)
}

func TestChatTree_LazyRootAndAdoption(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client, db, api := newChatClientWithAPIAndDatabase(t, chatTreeExperiments())
	firstUser := coderdtest.CreateFirstUser(t, client.Client)
	modelConfig := createChatModel(t, client)

	// A legacy parentless chat, an archived one, and a pinned one exist
	// before the tree is materialized.
	legacy := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    firstUser.OrganizationID,
		OwnerID:           firstUser.UserID,
		LastModelConfigID: modelConfig.ID,
		Title:             "legacy",
	})
	archived := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    firstUser.OrganizationID,
		OwnerID:           firstUser.UserID,
		LastModelConfigID: modelConfig.ID,
		Title:             "archived legacy",
	})
	_, err := db.UpdateChatExecutionState(dbauthz.AsSystemRestricted(ctx), database.UpdateChatExecutionStateParams{
		ID:       archived.ID,
		Status:   archived.Status,
		Archived: true,
	})
	require.NoError(t, err)
	otherUserClient, otherUser := coderdtest.CreateAnotherUser(t, client.Client, firstUser.OrganizationID)
	otherChat := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    firstUser.OrganizationID,
		OwnerID:           otherUser.ID,
		LastModelConfigID: modelConfig.ID,
		Title:             "other owner",
	})

	tree, err := client.ChatTree(ctx, firstUser.OrganizationID, nil)
	require.NoError(t, err)
	require.NotNil(t, tree.RootChatID)
	require.Len(t, tree.Chats, 2, "root plus the unarchived legacy chat")
	root := tree.Chats[0]
	require.Equal(t, *tree.RootChatID, root.ID)
	require.Equal(t, codersdk.ChatKindRoot, root.Kind)
	require.Equal(t, "Root", root.Title)
	require.NotNil(t, root.Depth)
	require.Equal(t, 1, *root.Depth)
	require.NotNil(t, root.ChildChatCount)
	require.Equal(t, 2, *root.ChildChatCount, "archived children count too")
	require.Equal(t, legacy.ID, tree.Chats[1].ID)
	require.Equal(t, 2, *tree.Chats[1].Depth)
	require.Equal(t, root.ID, *tree.Chats[1].ParentChatID)

	// Adoption changes only parent_chat_id.
	adopted, err := db.GetChatByID(dbauthz.AsSystemRestricted(ctx), legacy.ID)
	require.NoError(t, err)
	require.Equal(t, root.ID, adopted.ParentChatID.UUID)
	require.False(t, adopted.RootChatID.Valid)
	require.Equal(t, legacy.UpdatedAt.UTC(), adopted.UpdatedAt.UTC())
	require.Equal(t, legacy.SnapshotVersion, adopted.SnapshotVersion)
	adoptedArchived, err := db.GetChatByID(dbauthz.AsSystemRestricted(ctx), archived.ID)
	require.NoError(t, err)
	require.Equal(t, root.ID, adoptedArchived.ParentChatID.UUID)
	require.True(t, adoptedArchived.Archived)

	// Other owners are untouched until they read their own tree.
	untouched, err := db.GetChatByID(dbauthz.AsSystemRestricted(ctx), otherChat.ID)
	require.NoError(t, err)
	require.False(t, untouched.ParentChatID.Valid)

	// Archived view returns the root plus archived rows only.
	archivedTree, err := client.ChatTree(ctx, firstUser.OrganizationID, &codersdk.ChatTreeOptions{Archived: true})
	require.NoError(t, err)
	require.Len(t, archivedTree.Chats, 2)
	require.Equal(t, root.ID, archivedTree.Chats[0].ID)
	require.Equal(t, archived.ID, archivedTree.Chats[1].ID)

	// A second read is idempotent: same root, nothing new.
	again, err := client.ChatTree(ctx, firstUser.OrganizationID, nil)
	require.NoError(t, err)
	require.Equal(t, root.ID, *again.RootChatID)
	require.Len(t, again.Chats, 2)

	// The other owner gets a separate root in the same organization.
	otherTree, err := codersdk.NewExperimentalClient(otherUserClient).ChatTree(ctx, firstUser.OrganizationID, nil)
	require.NoError(t, err)
	require.NotEqual(t, root.ID, *otherTree.RootChatID)
	require.Len(t, otherTree.Chats, 2)

	// GET /chats never lists the root; the root exposes its tree position.
	listed, err := client.ListChats(ctx, nil)
	require.NoError(t, err)
	for _, c := range listed {
		require.NotEqual(t, codersdk.ChatKindRoot, c.Kind)
	}
	gotRoot, err := client.GetChat(ctx, root.ID)
	require.NoError(t, err)
	require.Equal(t, codersdk.ChatKindRoot, gotRoot.Kind)
	require.Equal(t, 1, *gotRoot.Depth)
	require.Equal(t, 2, *gotRoot.ChildChatCount)

	// New chats default to the root as parent.
	created := createTreeChat(t, client, firstUser.OrganizationID, "under root", nil)
	require.NotNil(t, created.ParentChatID)
	require.Equal(t, root.ID, *created.ParentChatID)
	coderdtest.WaitForChatSettled(ctx, t, api, created.ID)
}

func TestChatTree_NamedChildren(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client, db, api := newChatClientWithAPIAndDatabase(t, chatTreeExperiments())
	firstUser := coderdtest.CreateFirstUser(t, client.Client)
	modelConfig := createChatModel(t, client)
	otherClient, otherUser := coderdtest.CreateAnotherUser(t, client.Client, firstUser.OrganizationID)

	tree, err := client.ChatTree(ctx, firstUser.OrganizationID, nil)
	require.NoError(t, err)
	rootID := *tree.RootChatID

	// Depth 2 through 5 succeed; depth 6 is rejected.
	parentID := rootID
	var chain []codersdk.Chat
	for depth := 2; depth <= codersdk.ChatTreeMaxDepth; depth++ {
		chat := createTreeChat(t, client, firstUser.OrganizationID, "level", &parentID)
		coderdtest.WaitForChatSettled(ctx, t, api, chat.ID)
		got, err := client.GetChat(ctx, chat.ID)
		require.NoError(t, err)
		require.Equal(t, depth, *got.Depth)
		chain = append(chain, chat)
		parentID = chat.ID
	}
	_, err = client.CreateChat(ctx, codersdk.CreateChatRequest{
		OrganizationID: firstUser.OrganizationID,
		Content:        []codersdk.ChatInputPart{{Type: codersdk.ChatInputPartTypeText, Text: "too deep"}},
		ParentChatID:   &parentID,
	})
	requireChatAPIError(t, err, http.StatusBadRequest, "depth limit")

	// Missing, cross-owner, and shared parents produce the same error.
	for name, parent := range map[string]uuid.UUID{
		"missing": uuid.New(),
	} {
		_, err = client.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: firstUser.OrganizationID,
			Content:        []codersdk.ChatInputPart{{Type: codersdk.ChatInputPartTypeText, Text: name}},
			ParentChatID:   &parent,
		})
		requireChatAPIError(t, err, http.StatusBadRequest, "Parent chat not found")
	}
	otherExp := codersdk.NewExperimentalClient(otherClient)
	_, err = otherExp.CreateChat(ctx, codersdk.CreateChatRequest{
		OrganizationID: firstUser.OrganizationID,
		Content:        []codersdk.ChatInputPart{{Type: codersdk.ChatInputPartTypeText, Text: "cross owner"}},
		ParentChatID:   &rootID,
	})
	requireChatAPIError(t, err, http.StatusBadRequest, "Parent chat not found")
	err = client.UpdateChatACL(ctx, chain[0].ID, codersdk.UpdateChatACL{
		UserRoles: map[string]codersdk.ChatRole{otherUser.ID.String(): codersdk.ChatRoleRead},
	})
	require.NoError(t, err)
	_, err = otherExp.CreateChat(ctx, codersdk.CreateChatRequest{
		OrganizationID: firstUser.OrganizationID,
		Content:        []codersdk.ChatInputPart{{Type: codersdk.ChatInputPartTypeText, Text: "shared parent"}},
		ParentChatID:   &chain[0].ID,
	})
	requireChatAPIError(t, err, http.StatusBadRequest, "Parent chat not found")

	// Subagent parents are rejected.
	subagent := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    firstUser.OrganizationID,
		OwnerID:           firstUser.UserID,
		LastModelConfigID: modelConfig.ID,
		ParentChatID:      uuid.NullUUID{UUID: chain[0].ID, Valid: true},
		RootChatID:        uuid.NullUUID{UUID: chain[0].ID, Valid: true},
		Title:             "subagent",
	})
	_, err = client.CreateChat(ctx, codersdk.CreateChatRequest{
		OrganizationID: firstUser.OrganizationID,
		Content:        []codersdk.ChatInputPart{{Type: codersdk.ChatInputPartTypeText, Text: "under subagent"}},
		ParentChatID:   &subagent.ID,
	})
	requireChatAPIError(t, err, http.StatusBadRequest, "Subagent chats cannot have child chats")

	// Root restrictions.
	err = client.UpdateChat(ctx, rootID, codersdk.UpdateChatRequest{Archived: ptr.Ref(true)})
	requireChatAPIError(t, err, http.StatusBadRequest, "Chat tree root cannot be archived")
	err = client.UpdateChat(ctx, rootID, codersdk.UpdateChatRequest{PinOrder: ptr.Ref(int32(1))})
	requireChatAPIError(t, err, http.StatusBadRequest, "Cannot pin the chat tree root")
	err = client.UpdateChatACL(ctx, rootID, codersdk.UpdateChatACL{
		UserRoles: map[string]codersdk.ChatRole{otherUser.ID.String(): codersdk.ChatRoleRead},
	})
	requireChatAPIError(t, err, http.StatusBadRequest, "Chat tree root cannot be shared")
	err = client.UpdateChat(ctx, rootID, codersdk.UpdateChatRequest{Title: ptr.Ref("My tree")})
	require.NoError(t, err)

	// Archiving level 2 cascades over the named descendants and the
	// subagent; the root stays.
	err = client.UpdateChat(ctx, chain[0].ID, codersdk.UpdateChatRequest{Archived: ptr.Ref(true)})
	require.NoError(t, err)
	for _, chat := range chain {
		got, err := client.GetChat(ctx, chat.ID)
		require.NoError(t, err)
		require.True(t, got.Archived, "chat %s", chat.Title)
	}
	gotSubagent, err := client.GetChat(ctx, subagent.ID)
	require.NoError(t, err)
	require.True(t, gotSubagent.Archived)
	gotRoot, err := client.GetChat(ctx, rootID)
	require.NoError(t, err)
	require.False(t, gotRoot.Archived)

	// Creating under an archived parent fails; unarchiving a child under
	// an archived parent fails.
	_, err = client.CreateChat(ctx, codersdk.CreateChatRequest{
		OrganizationID: firstUser.OrganizationID,
		Content:        []codersdk.ChatInputPart{{Type: codersdk.ChatInputPartTypeText, Text: "under archived"}},
		ParentChatID:   &chain[0].ID,
	})
	requireChatAPIError(t, err, http.StatusBadRequest, "archived parent")
	err = client.UpdateChat(ctx, chain[1].ID, codersdk.UpdateChatRequest{Archived: ptr.Ref(false)})
	requireChatAPIError(t, err, http.StatusBadRequest, "parent chat is archived")

	// Unarchiving the top restores it and its subagent, not the named
	// descendants.
	err = client.UpdateChat(ctx, chain[0].ID, codersdk.UpdateChatRequest{Archived: ptr.Ref(false)})
	require.NoError(t, err)
	gotTop, err := client.GetChat(ctx, chain[0].ID)
	require.NoError(t, err)
	require.False(t, gotTop.Archived)
	gotSubagent, err = client.GetChat(ctx, subagent.ID)
	require.NoError(t, err)
	require.False(t, gotSubagent.Archived)
	gotChild, err := client.GetChat(ctx, chain[1].ID)
	require.NoError(t, err)
	require.True(t, gotChild.Archived)

	// The tree view lists unarchived rows with their positions and the
	// archived view lists the rest.
	tree, err = client.ChatTree(ctx, firstUser.OrganizationID, nil)
	require.NoError(t, err)
	require.Len(t, tree.Chats, 2)
	require.Equal(t, "My tree", tree.Chats[0].Title)
	require.Equal(t, chain[0].ID, tree.Chats[1].ID)
	require.Equal(t, 1, *tree.Chats[1].ChildChatCount)
	archivedTree, err := client.ChatTree(ctx, firstUser.OrganizationID, &codersdk.ChatTreeOptions{Archived: true})
	require.NoError(t, err)
	require.Len(t, archivedTree.Chats, len(chain))
}
