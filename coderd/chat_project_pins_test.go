package coderd_test

import (
	"cmp"
	"context"
	"database/sql"
	"fmt"
	"maps"
	"net/http"
	"slices"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestChatProjectPins(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client, sqlDB := newChatProjectPinsClient(t)
	firstUser := coderdtest.CreateFirstUser(t, client.Client)
	orgID, me := firstUser.OrganizationID, firstUser.UserID
	reorder := func(client *codersdk.ExperimentalClient, projectIDs ...uuid.UUID) error {
		return client.ReorderChatProjectPins(ctx, codersdk.ReorderChatProjectPinsRequest{ProjectIDs: projectIDs})
	}

	// New projects start pinned at the end of the creator's list.
	a := createChatProject(t, client, orgID, "Pin A")
	b := createChatProject(t, client, orgID, "Pin B")
	c := createChatProject(t, client, orgID, "Pin C")
	requirePinnedProjects(ctx, t, sqlDB, me, a.ID, b.ID, c.ID)

	require.NoError(t, reorder(client, c.ID, a.ID, b.ID))
	requirePinnedProjects(ctx, t, sqlDB, me, c.ID, a.ID, b.ID)
	// Pins left out of the list follow the listed ones in their order.
	require.NoError(t, reorder(client, b.ID))
	requirePinnedProjects(ctx, t, sqlDB, me, b.ID, c.ID, a.ID)

	// Unpinning keeps the other pins in order, and repeating it changes
	// nothing.
	require.NoError(t, client.UnpinChatProject(ctx, orgID, c.ID))
	require.NoError(t, client.UnpinChatProject(ctx, orgID, c.ID))
	requirePinnedProjects(ctx, t, sqlDB, me, b.ID, a.ID)
	// A reorder skips a project that is not pinned, such as one another
	// tab just unpinned, and does not pin it.
	require.NoError(t, reorder(client, c.ID, a.ID, b.ID))
	requirePinnedProjects(ctx, t, sqlDB, me, a.ID, b.ID)

	// A re-pinned project goes to the end and an existing pin keeps its
	// place.
	require.NoError(t, client.PinChatProject(ctx, orgID, c.ID))
	require.NoError(t, client.PinChatProject(ctx, orgID, a.ID))
	requirePinnedProjects(ctx, t, sqlDB, me, a.ID, b.ID, c.ID)

	// Pins need read access. Sharing does not pin, and a pin that outlives
	// access stays hidden.
	other, otherUser := newChatProjectMember(t, client, orgID)
	requireSDKError(t, other.PinChatProject(ctx, orgID, a.ID), http.StatusNotFound)
	// Role-based read access is not enough: the project is missing from the
	// site owner's project list.
	otherOwn := createChatProject(t, other, orgID, "Pin Other")
	requireSDKError(t, client.PinChatProject(ctx, orgID, otherOwn.ID), http.StatusNotFound)
	requireSDKError(t, client.UnpinChatProject(ctx, orgID, otherOwn.ID), http.StatusNotFound)
	setRole := func(role codersdk.ChatProjectRole) {
		require.NoError(t, client.UpdateChatProjectACL(ctx, orgID, a.ID, codersdk.UpdateChatProjectACL{
			UserRoles: map[string]codersdk.ChatProjectRole{otherUser.ID.String(): role},
		}))
	}
	setRole(codersdk.ChatProjectRoleUse)
	requirePinnedProjects(ctx, t, sqlDB, otherUser.ID, otherOwn.ID)
	require.NoError(t, other.PinChatProject(ctx, orgID, a.ID))
	requirePinnedProjects(ctx, t, sqlDB, otherUser.ID, otherOwn.ID, a.ID)
	setRole(codersdk.ChatProjectRoleDeleted)
	requireSDKError(t, other.UnpinChatProject(ctx, orgID, a.ID), http.StatusNotFound)
	// The caller cannot list a pin they no longer see, so a reorder keeps
	// it after the listed pins.
	otherNew := createChatProject(t, other, orgID, "Pin Other New")
	require.NoError(t, reorder(other, otherNew.ID, otherOwn.ID))
	requirePinnedProjects(ctx, t, sqlDB, otherUser.ID, otherNew.ID, otherOwn.ID, a.ID)
}

// TestChatProjectPinsTokenScope checks that a pin, as the caller's personal
// data, cannot be written by an API key that lacks user:update_personal or
// cannot read the project.
func TestChatProjectPinsTokenScope(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client, db := newChatProjectClient(t)
	firstUser := coderdtest.CreateFirstUser(t, client.Client)
	orgID := firstUser.OrganizationID
	project := createChatProject(t, client, orgID, "Pin Scope")

	// Chat project scopes are not user-mintable, so seed scoped keys directly.
	newScopedClient := func(scopes ...database.APIKeyScope) *codersdk.ExperimentalClient {
		_, token := dbgen.APIKey(t, db, database.APIKey{
			UserID: firstUser.UserID,
			Scopes: append(database.APIKeyScopes{"organization:read"}, scopes...),
		})
		scoped := codersdk.New(client.URL)
		scoped.SetSessionToken(token)
		return codersdk.NewExperimentalClient(scoped)
	}

	order := codersdk.ReorderChatProjectPinsRequest{ProjectIDs: []uuid.UUID{project.ID}}
	readOnly := newScopedClient("chat_project:read")
	requireSDKError(t, readOnly.PinChatProject(ctx, orgID, project.ID), http.StatusForbidden)
	requireSDKError(t, readOnly.UnpinChatProject(ctx, orgID, project.ID), http.StatusForbidden)
	requireSDKError(t, readOnly.ReorderChatProjectPins(ctx, order), http.StatusForbidden)

	// The same key with user:update_personal may change the pins, so that
	// scope is the one the read-only key lacks.
	personal := newScopedClient("chat_project:read", "user:update_personal")
	require.NoError(t, personal.UnpinChatProject(ctx, orgID, project.ID))
	require.NoError(t, personal.PinChatProject(ctx, orgID, project.ID))
	require.NoError(t, personal.ReorderChatProjectPins(ctx, order))

	// Creating a project pins it for its creator, so a key that may create
	// projects also needs both of the pin's permissions.
	createReq := codersdk.CreateChatProjectRequest{Name: "Scoped Create"}
	_, err := newScopedClient("chat_project:create", "chat_project:read").CreateChatProject(ctx, orgID, createReq)
	requireSDKError(t, err, http.StatusForbidden)
	_, err = newScopedClient("chat_project:create", "user:update_personal").CreateChatProject(ctx, orgID, createReq)
	requireSDKError(t, err, http.StatusForbidden)
	_, err = newScopedClient("chat_project:create", "chat_project:read", "user:update_personal").CreateChatProject(ctx, orgID, createReq)
	require.NoError(t, err)
}

func TestChatProjectPinsConcurrentWrites(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client, sqlDB := newChatProjectPinsClient(t)
	firstUser := coderdtest.CreateFirstUser(t, client.Client)
	orgID := firstUser.OrganizationID
	// Sixteen projects make overlapping writes collide reliably.
	projectIDs := make([]uuid.UUID, 16)
	want := make([]int32, len(projectIDs))
	for i := range projectIDs {
		projectIDs[i] = createChatProject(t, client, orgID, fmt.Sprintf("Concurrent %d", i)).ID
		want[i] = int32(i + 1) //nolint:gosec // Bounded by the number of projects.
	}
	// Every pin holds its own position, with none skipped.
	requireDistinctPositions := func() {
		t.Helper()
		pins := chatProjectPinOrders(ctx, t, sqlDB, firstUser.UserID)
		require.ElementsMatch(t, want, slices.Collect(maps.Values(pins)))
	}

	for round := range 10 {
		// Overlapping reorders rewrite the same rows in different orders.
		var reorders errgroup.Group
		for i := range projectIDs {
			reorders.Go(func() error {
				order := append(slices.Clone(projectIDs[i:]), projectIDs[:i]...)
				if (i+round)%2 == 0 {
					slices.Reverse(order)
				}
				return client.ReorderChatProjectPins(ctx, codersdk.ReorderChatProjectPinsRequest{ProjectIDs: order})
			})
		}
		require.NoError(t, reorders.Wait())
		requireDistinctPositions()

		// Overlapping pins each read the last position before taking the
		// next one.
		for _, projectID := range projectIDs {
			require.NoError(t, client.UnpinChatProject(ctx, orgID, projectID))
		}
		var pins errgroup.Group
		for _, projectID := range projectIDs {
			pins.Go(func() error {
				return client.PinChatProject(ctx, orgID, projectID)
			})
		}
		require.NoError(t, pins.Wait())
		requireDistinctPositions()
	}
}

//nolint:paralleltest // Disabling chat sharing flips a process-wide RBAC switch.
func TestChatProjectsSharingDisabled(t *testing.T) {
	previous := rbac.ChatACLDisabled()
	t.Cleanup(func() {
		rbac.ReloadBuiltinRoles(nil)
		rbac.SetChatACLDisabled(previous)
	})

	ctx := testutil.Context(t, testutil.WaitLong)
	db, ps := dbtestutil.NewDB(t)
	client, _ := newChatProjectClientWithAPI(t, func(options *coderdtest.Options) {
		options.Database, options.Pubsub = db, ps
		options.DeploymentValues.DisableChatSharing = true
	})
	firstUser := coderdtest.CreateFirstUser(t, client.Client)
	orgID := firstUser.OrganizationID
	model := createChatModel(t, client)
	member, memberUser := newChatProjectMember(t, client, orgID)
	project := createChatProject(t, client, orgID, "Members Disabled")
	// Grants left from before sharing was disabled do not count.
	require.NoError(t, db.UpdateChatProjectACLByID(ctx, database.UpdateChatProjectACLByIDParams{
		ID:       project.ID,
		UserACL:  database.ChatACL{},
		GroupACL: database.ChatACL{orgID.String(): {Permissions: []policy.Action{policy.ActionRead}}},
	}))

	// The member cannot open the project, so no project list reaches their
	// chat in it and the main list keeps it.
	chat := dbgen.Chat(t, db, database.Chat{
		OrganizationID: orgID, OwnerID: memberUser.ID, LastModelConfigID: model.ID,
		ProjectID: uuid.NullUUID{UUID: project.ID, Valid: true},
	})
	memberChats, err := member.ListChats(ctx, &codersdk.ListChatsOptions{ExcludeOwnProjectChats: true})
	require.NoError(t, err)
	require.Contains(t, chatIDs(memberChats), chat.ID)

	// Only the owner can pin a project, even when a role reads it and a
	// grant from before sharing was disabled names the caller.
	memberProject := createChatProject(t, member, orgID, "Member Project")
	require.NoError(t, db.UpdateChatProjectACLByID(ctx, database.UpdateChatProjectACLByIDParams{
		ID:       memberProject.ID,
		UserACL:  database.ChatACL{firstUser.UserID.String(): {Permissions: []policy.Action{policy.ActionRead}}},
		GroupACL: database.ChatACL{},
	}))
	requireSDKError(t, client.PinChatProject(ctx, orgID, memberProject.ID), http.StatusNotFound)
	require.NoError(t, member.PinChatProject(ctx, orgID, memberProject.ID))
}

func TestListChatsExcludeOwnProjectChats(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client, _ := newChatProjectClient(t)
	firstUser := coderdtest.CreateFirstUser(t, client.Client)
	orgID := firstUser.OrganizationID
	_ = createChatModel(t, client)
	pinned := createChatProject(t, client, orgID, "Sidebar Pinned")
	unpinned := createChatProject(t, client, orgID, "Sidebar Unpinned")
	require.NoError(t, client.UnpinChatProject(ctx, orgID, unpinned.ID))

	unfiled := createChatInProject(t, client, orgID, nil)
	// Project pins do not matter: chats in pinned and unpinned projects
	// are both left out.
	inPinned := createChatInProject(t, client, orgID, &pinned.ID)
	pinnedChat := createChatInProject(t, client, orgID, &unpinned.ID)
	require.NoError(t, client.UpdateChat(ctx, pinnedChat.ID, codersdk.UpdateChatRequest{PinOrder: ptr.Ref(int32(1))}))
	// The newest chat is left out, so filtering after the query would
	// return an empty second page.
	inUnpinned := createChatInProject(t, client, orgID, &unpinned.ID)

	opts := &codersdk.ListChatsOptions{ExcludeOwnProjectChats: true, Pagination: codersdk.Pagination{Limit: 1}}
	firstPage, err := client.ListChats(ctx, opts)
	require.NoError(t, err)
	require.Len(t, firstPage, 1)
	require.Equal(t, pinnedChat.ID, firstPage[0].ID)
	opts.AfterID = firstPage[0].ID
	secondPage, err := client.ListChats(ctx, opts)
	require.NoError(t, err)
	require.Equal(t, []uuid.UUID{unfiled.ID}, chatIDs(secondPage))

	// A chat in a project the viewer can no longer use stays listed,
	// because no project list reaches it.
	member, memberUser := newChatProjectMember(t, client, orgID)
	setRole := func(role codersdk.ChatProjectRole) {
		require.NoError(t, client.UpdateChatProjectACL(ctx, orgID, unpinned.ID, codersdk.UpdateChatProjectACL{
			UserRoles: map[string]codersdk.ChatProjectRole{memberUser.ID.String(): role},
		}))
	}
	setRole(codersdk.ChatProjectRoleUse)
	revoked := createChatInProject(t, member, orgID, &unpinned.ID)
	// Grants count: the member can use the project, so its chats are left
	// out whether or not the member pinned it.
	memberOpts := &codersdk.ListChatsOptions{ExcludeOwnProjectChats: true}
	for _, pin := range []bool{false, true} {
		if pin {
			require.NoError(t, member.PinChatProject(ctx, orgID, unpinned.ID))
		}
		memberChats, err := member.ListChats(ctx, memberOpts)
		require.NoError(t, err)
		require.NotContains(t, chatIDs(memberChats), revoked.ID)
	}
	setRole(codersdk.ChatProjectRoleDeleted)
	memberChats, err := member.ListChats(ctx, memberOpts)
	require.NoError(t, err)
	require.Contains(t, chatIDs(memberChats), revoked.ID)

	// Without the option, every chat is listed.
	all, err := client.ListChats(ctx, nil)
	require.NoError(t, err)
	require.Subset(t, chatIDs(all), []uuid.UUID{unfiled.ID, inPinned.ID, pinnedChat.ID, inUnpinned.ID})
}

// A chat pin belongs to the chat's owner, so pinning a shared chat must not
// change how the chat is listed for anyone it is shared with, and project
// pins never hide a shared chat.
func TestListChatsIgnoresOtherUsersChatPins(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client, _ := newChatProjectClient(t)
	firstUser := coderdtest.CreateFirstUser(t, client.Client)
	orgID := firstUser.OrganizationID
	_ = createChatModel(t, client)
	member, memberUser := newChatProjectMember(t, client, orgID)
	project := createChatProject(t, client, orgID, "Shared Project")
	require.NoError(t, client.UpdateChatProjectACL(ctx, orgID, project.ID, codersdk.UpdateChatProjectACL{
		UserRoles: map[string]codersdk.ChatProjectRole{memberUser.ID.String(): codersdk.ChatProjectRoleUse},
	}))

	memberPinned := createChatInProject(t, member, orgID, nil)
	require.NoError(t, member.UpdateChat(ctx, memberPinned.ID, codersdk.UpdateChatRequest{PinOrder: ptr.Ref(int32(1))}))
	ownerPinned := createChatInProject(t, client, orgID, &project.ID)
	newer := createChatInProject(t, client, orgID, nil)
	// Shared in creation order so the newer chat stays the most recently
	// updated even if sharing bumps updated_at.
	for _, chat := range []codersdk.Chat{ownerPinned, newer} {
		require.NoError(t, client.UpdateChatACL(ctx, chat.ID, codersdk.UpdateChatACL{
			UserRoles: map[string]codersdk.ChatRole{memberUser.ID.String(): codersdk.ChatRoleRead},
		}))
	}
	require.NoError(t, client.UpdateChat(ctx, ownerPinned.ID, codersdk.UpdateChatRequest{PinOrder: ptr.Ref(int32(1))}))

	// The sidebar lists owned and shared chats together. Only the member's
	// own pin sorts first. Paging one chat at a time checks that the cursor
	// uses the same order.
	const ownedAndShared = "source:created_by_me,shared_with_me"
	var listed []uuid.UUID
	opts := &codersdk.ListChatsOptions{Query: ownedAndShared, Pagination: codersdk.Pagination{Limit: 1}}
	for {
		page, err := member.ListChats(ctx, opts)
		require.NoError(t, err)
		if len(page) == 0 {
			break
		}
		listed = append(listed, chatIDs(page)...)
		opts.AfterID = page[len(page)-1].ID
	}
	require.Equal(t, []uuid.UUID{memberPinned.ID, newer.ID, ownerPinned.ID}, listed)

	// The owner's own list keeps the pinned chat in the project.
	ownerChats, err := client.ListChats(ctx, &codersdk.ListChatsOptions{ExcludeOwnProjectChats: true})
	require.NoError(t, err)
	require.NotEmpty(t, ownerChats)
	require.Equal(t, ownerPinned.ID, ownerChats[0].ID)

	// Chats shared with the member are listed whether or not anyone pinned
	// the chat, although the member can use its project.
	pinnedOpts := &codersdk.ListChatsOptions{Query: ownedAndShared, ExcludeOwnProjectChats: true}
	want := []uuid.UUID{memberPinned.ID, newer.ID, ownerPinned.ID}
	memberChats, err := member.ListChats(ctx, pinnedOpts)
	require.NoError(t, err)
	require.ElementsMatch(t, want, chatIDs(memberChats))
	require.NoError(t, client.UpdateChat(ctx, ownerPinned.ID, codersdk.UpdateChatRequest{PinOrder: ptr.Ref(int32(0))}))
	memberChats, err = member.ListChats(ctx, pinnedOpts)
	require.NoError(t, err)
	require.ElementsMatch(t, want, chatIDs(memberChats))
}

// newChatProjectPinsClient returns a client and a connection to its
// database for reading pin rows.
func newChatProjectPinsClient(t testing.TB) (*codersdk.ExperimentalClient, *sql.DB) {
	t.Helper()
	db, ps, sqlDB := dbtestutil.NewDBWithSQLDB(t)
	client, _ := newChatProjectClient(t, func(options *coderdtest.Options) {
		options.Database, options.Pubsub = db, ps
	})
	return client, sqlDB
}

// chatProjectPinOrders reads the stored order of each of the user's pins.
func chatProjectPinOrders(ctx context.Context, t testing.TB, sqlDB *sql.DB, userID uuid.UUID) map[uuid.UUID]int32 {
	t.Helper()
	rows, err := sqlDB.QueryContext(ctx, `SELECT project_id, pin_order FROM chat_project_pins WHERE user_id = $1`, userID)
	require.NoError(t, err)
	defer rows.Close()
	pins := map[uuid.UUID]int32{}
	for rows.Next() {
		var projectID uuid.UUID
		var order int32
		require.NoError(t, rows.Scan(&projectID, &order))
		pins[projectID] = order
	}
	require.NoError(t, rows.Err())
	return pins
}

// requirePinnedProjects checks which projects the user has pinned, in pin
// order. Pins on projects the user can no longer see count too.
func requirePinnedProjects(ctx context.Context, t testing.TB, sqlDB *sql.DB, userID uuid.UUID, want ...uuid.UUID) {
	t.Helper()
	pins := chatProjectPinOrders(ctx, t, sqlDB, userID)
	got := slices.SortedFunc(maps.Keys(pins), func(a, b uuid.UUID) int {
		return cmp.Compare(pins[a], pins[b])
	})
	require.Equal(t, want, got)
}
