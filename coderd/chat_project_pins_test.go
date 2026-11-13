package coderd_test

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"strings"
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

	// New projects start pinned at the end of the creator's list.
	a := createChatProject(t, client, orgID, "Pin A")
	b := createChatProject(t, client, orgID, "Pin B")
	c := createChatProject(t, client, orgID, "Pin C")
	requirePinOrders(ctx, t, sqlDB, me, map[uuid.UUID]int32{a.ID: 1, b.ID: 2, c.ID: 3})

	require.NoError(t, client.PinChatProject(ctx, orgID, c.ID, codersdk.PinChatProjectRequest{PinOrder: ptr.Ref(int32(1))}))
	requirePinOrders(ctx, t, sqlDB, me, map[uuid.UUID]int32{c.ID: 1, a.ID: 2, b.ID: 3})

	// Unpinning compacts the remaining pins, and repeating it changes nothing.
	require.NoError(t, client.UnpinChatProject(ctx, orgID, a.ID))
	require.NoError(t, client.UnpinChatProject(ctx, orgID, a.ID))
	requirePinOrders(ctx, t, sqlDB, me, map[uuid.UUID]int32{c.ID: 1, b.ID: 2, a.ID: 0})

	// Without an order, a re-pinned tombstone goes to the end and an
	// existing pin keeps its place.
	require.NoError(t, client.PinChatProject(ctx, orgID, a.ID, codersdk.PinChatProjectRequest{}))
	require.NoError(t, client.PinChatProject(ctx, orgID, c.ID, codersdk.PinChatProjectRequest{}))
	requirePinOrders(ctx, t, sqlDB, me, map[uuid.UUID]int32{c.ID: 1, b.ID: 2, a.ID: 3})

	err := client.PinChatProject(ctx, orgID, a.ID, codersdk.PinChatProjectRequest{PinOrder: ptr.Ref(int32(0))})
	requireSDKError(t, err, http.StatusBadRequest)
	putPin := func(body io.Reader, opts ...codersdk.RequestOption) int {
		res, err := client.Request(ctx, http.MethodPut, fmt.Sprintf("/api/experimental/organizations/%s/chats/projects/%s/pin", orgID, b.ID), body, opts...)
		require.NoError(t, err)
		_ = res.Body.Close()
		return res.StatusCode
	}
	// The body is optional, also in a chunked request that cannot announce
	// its empty length.
	require.Equal(t, http.StatusNoContent, putPin(nil))
	require.Equal(t, http.StatusNoContent, putPin(nil, func(req *http.Request) {
		req.Body, req.ContentLength, req.TransferEncoding = io.NopCloser(strings.NewReader("")), -1, []string{"chunked"}
	}))
	require.Equal(t, http.StatusBadRequest, putPin(strings.NewReader("{")))

	// Pins need read access. Sharing does not pin, and a pin that outlives
	// access stays hidden.
	other, otherUser := newChatProjectMember(t, client, orgID)
	requireSDKError(t, other.PinChatProject(ctx, orgID, a.ID, codersdk.PinChatProjectRequest{}), http.StatusNotFound)
	// Role-based read access is not enough: the project is missing from the
	// site owner's project list.
	otherOwn := createChatProject(t, other, orgID, "Pin Other")
	requireSDKError(t, client.PinChatProject(ctx, orgID, otherOwn.ID, codersdk.PinChatProjectRequest{}), http.StatusNotFound)
	requireSDKError(t, client.UnpinChatProject(ctx, orgID, otherOwn.ID), http.StatusNotFound)
	setRole := func(role codersdk.ChatProjectRole) {
		require.NoError(t, client.UpdateChatProjectACL(ctx, orgID, a.ID, codersdk.UpdateChatProjectACL{
			UserRoles: map[string]codersdk.ChatProjectRole{otherUser.ID.String(): role},
		}))
	}
	setRole(codersdk.ChatProjectRoleUse)
	requirePinOrders(ctx, t, sqlDB, otherUser.ID, map[uuid.UUID]int32{a.ID: 0})
	require.NoError(t, other.PinChatProject(ctx, orgID, a.ID, codersdk.PinChatProjectRequest{PinOrder: ptr.Ref(int32(1))}))
	requirePinOrders(ctx, t, sqlDB, otherUser.ID, map[uuid.UUID]int32{a.ID: 1, otherOwn.ID: 2})
	setRole(codersdk.ChatProjectRoleDeleted)
	requireSDKError(t, other.UnpinChatProject(ctx, orgID, a.ID), http.StatusNotFound)
	// The hidden pin takes no position in requested positions.
	otherNew := createChatProject(t, other, orgID, "Pin Other New")
	require.NoError(t, other.PinChatProject(ctx, orgID, otherOwn.ID, codersdk.PinChatProjectRequest{PinOrder: ptr.Ref(int32(2))}))
	requirePinOrders(ctx, t, sqlDB, otherUser.ID, map[uuid.UUID]int32{otherNew.ID: 1, otherOwn.ID: 2})
}

func TestChatProjectPinsConcurrentWrites(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client, sqlDB := newChatProjectPinsClient(t)
	firstUser := coderdtest.CreateFirstUser(t, client.Client)
	orgID := firstUser.OrganizationID
	// Eight projects make overlapping writes collide reliably.
	projects := make([]codersdk.ChatProject, 8)
	for i := range projects {
		projects[i] = createChatProject(t, client, orgID, fmt.Sprintf("Concurrent %d", i))
	}

	// Each round pins half of the projects at the top and unpins the other
	// half at the same time, so the writes renumber the same rows.
	for round := range 10 {
		var eg errgroup.Group
		for i, project := range projects {
			eg.Go(func() error {
				if (i+round)%2 == 0 {
					return client.UnpinChatProject(ctx, orgID, project.ID)
				}
				return client.PinChatProject(ctx, orgID, project.ID, codersdk.PinChatProjectRequest{PinOrder: ptr.Ref(int32(1))})
			})
		}
		require.NoError(t, eg.Wait())
		pins := chatProjectPinOrders(ctx, t, sqlDB, firstUser.UserID)
		for i, project := range projects {
			require.Equal(t, (i+round)%2 != 0, pins[project.ID] > 0, project.Name)
		}
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
	db, ps, sqlDB := dbtestutil.NewDBWithSQLDB(t)
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

	// The member cannot open the project, so the sidebar keeps their chat
	// in it although nothing is pinned.
	chat := dbgen.Chat(t, db, database.Chat{
		OrganizationID: orgID, OwnerID: memberUser.ID, LastModelConfigID: model.ID,
		ProjectID: uuid.NullUUID{UUID: project.ID, Valid: true},
	})
	memberChats, err := member.ListChats(ctx, &codersdk.ListChatsOptions{PinnedProjectsOnly: true})
	require.NoError(t, err)
	require.Contains(t, chatIDs(memberChats), chat.ID)

	// A pin from before sharing was disabled takes no visible position.
	require.NoError(t, db.PinChatProject(ctx, database.PinChatProjectParams{
		UserID: memberUser.ID, ProjectID: project.ID, ACLEnabled: true, PinOrder: sql.NullInt32{Int32: 1, Valid: true},
	}))
	x := createChatProject(t, member, orgID, "Hidden Pin X")
	y := createChatProject(t, member, orgID, "Hidden Pin Y")
	require.NoError(t, member.PinChatProject(ctx, orgID, y.ID, codersdk.PinChatProjectRequest{PinOrder: ptr.Ref(int32(1))}))
	requirePinOrders(ctx, t, sqlDB, memberUser.ID, map[uuid.UUID]int32{y.ID: 1, x.ID: 2})
	require.NoError(t, member.UnpinChatProject(ctx, orgID, x.ID))
	require.NoError(t, member.PinChatProject(ctx, orgID, x.ID, codersdk.PinChatProjectRequest{}))
	requirePinOrders(ctx, t, sqlDB, memberUser.ID, map[uuid.UUID]int32{y.ID: 1, x.ID: 2})
}

func TestListChatsPinnedProjectsOnly(t *testing.T) {
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
	inPinned := createChatInProject(t, client, orgID, &pinned.ID)
	pinnedChat := createChatInProject(t, client, orgID, &unpinned.ID)
	require.NoError(t, client.UpdateChat(ctx, pinnedChat.ID, codersdk.UpdateChatRequest{PinOrder: ptr.Ref(int32(1))}))
	// The newest chat is hidden, so filtering after the query would leave
	// the first page short.
	hidden := createChatInProject(t, client, orgID, &unpinned.ID)

	opts := &codersdk.ListChatsOptions{PinnedProjectsOnly: true, Pagination: codersdk.Pagination{Limit: 2}}
	firstPage, err := client.ListChats(ctx, opts)
	require.NoError(t, err)
	require.Len(t, firstPage, 2)
	require.Equal(t, pinnedChat.ID, firstPage[0].ID)
	opts.AfterID = firstPage[1].ID
	secondPage, err := client.ListChats(ctx, opts)
	require.NoError(t, err)
	require.ElementsMatch(t,
		[]uuid.UUID{pinnedChat.ID, inPinned.ID, unfiled.ID},
		chatIDs(append(firstPage, secondPage...)),
	)

	// A chat in a project the viewer can no longer use stays listed,
	// because the sidebar is the only place left to find it.
	member, memberUser := newChatProjectMember(t, client, orgID)
	setRole := func(role codersdk.ChatProjectRole) {
		require.NoError(t, client.UpdateChatProjectACL(ctx, orgID, unpinned.ID, codersdk.UpdateChatProjectACL{
			UserRoles: map[string]codersdk.ChatProjectRole{memberUser.ID.String(): role},
		}))
	}
	setRole(codersdk.ChatProjectRoleUse)
	revoked := createChatInProject(t, member, orgID, &unpinned.ID)
	// Grants count: the member sees the project, so its chats are listed
	// only after the member pins it.
	memberChats, err := member.ListChats(ctx, &codersdk.ListChatsOptions{PinnedProjectsOnly: true})
	require.NoError(t, err)
	require.NotContains(t, chatIDs(memberChats), revoked.ID)
	require.NoError(t, member.PinChatProject(ctx, orgID, unpinned.ID, codersdk.PinChatProjectRequest{}))
	memberChats, err = member.ListChats(ctx, &codersdk.ListChatsOptions{PinnedProjectsOnly: true})
	require.NoError(t, err)
	require.Contains(t, chatIDs(memberChats), revoked.ID)
	require.NoError(t, member.UnpinChatProject(ctx, orgID, unpinned.ID))
	setRole(codersdk.ChatProjectRoleDeleted)
	memberChats, err = member.ListChats(ctx, &codersdk.ListChatsOptions{PinnedProjectsOnly: true})
	require.NoError(t, err)
	require.Contains(t, chatIDs(memberChats), revoked.ID)

	// Other callers still see every chat.
	all, err := client.ListChats(ctx, nil)
	require.NoError(t, err)
	require.Contains(t, chatIDs(all), hidden.ID)
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

	// The owner's own list still honors the pin with the project unpinned.
	require.NoError(t, client.UnpinChatProject(ctx, orgID, project.ID))
	ownerChats, err := client.ListChats(ctx, &codersdk.ListChatsOptions{PinnedProjectsOnly: true})
	require.NoError(t, err)
	require.NotEmpty(t, ownerChats)
	require.Equal(t, ownerPinned.ID, ownerChats[0].ID)

	// Chats shared with the member are listed whether or not anyone pinned
	// the chat or its project. The member can use the project but has not
	// pinned it.
	pinnedOpts := &codersdk.ListChatsOptions{Query: ownedAndShared, PinnedProjectsOnly: true}
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

// chatProjectPinOrders reads the stored order of each of the user's active
// pins.
func chatProjectPinOrders(ctx context.Context, t testing.TB, sqlDB *sql.DB, userID uuid.UUID) map[uuid.UUID]int32 {
	t.Helper()
	rows, err := sqlDB.QueryContext(ctx, `SELECT project_id, pin_order FROM chat_project_pins WHERE user_id = $1 AND unpinned_at IS NULL`, userID)
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

// requirePinOrders checks each project's pin order; 0 means not pinned.
func requirePinOrders(ctx context.Context, t testing.TB, sqlDB *sql.DB, userID uuid.UUID, want map[uuid.UUID]int32) {
	t.Helper()
	pins := chatProjectPinOrders(ctx, t, sqlDB, userID)
	got := make(map[uuid.UUID]int32, len(want))
	for id := range want {
		got[id] = pins[id]
	}
	require.Equal(t, want, got)
}
