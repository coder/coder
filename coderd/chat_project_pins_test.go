package coderd_test

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"maps"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

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
	"github.com/coder/coder/v2/coderd/util/slice"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestChatProjectPins(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client, _ := newChatProjectClient(t)
	firstUser := coderdtest.CreateFirstUser(t, client.Client)
	orgID := firstUser.OrganizationID

	// New projects start pinned at the end of the creator's list.
	a := createChatProject(t, client, orgID, "Pin A")
	b := createChatProject(t, client, orgID, "Pin B")
	c := createChatProject(t, client, orgID, "Pin C")
	requirePinOrders(ctx, t, client, map[uuid.UUID]int32{a.ID: 1, b.ID: 2, c.ID: 3})

	require.NoError(t, client.PinChatProject(ctx, orgID, c.ID, codersdk.PinChatProjectRequest{PinOrder: ptr.Ref(int32(1))}))
	requirePinOrders(ctx, t, client, map[uuid.UUID]int32{c.ID: 1, a.ID: 2, b.ID: 3})

	// Unpinning compacts the remaining pins, and repeating it changes nothing.
	require.NoError(t, client.UnpinChatProject(ctx, orgID, a.ID))
	require.NoError(t, client.UnpinChatProject(ctx, orgID, a.ID))
	requirePinOrders(ctx, t, client, map[uuid.UUID]int32{c.ID: 1, b.ID: 2, a.ID: 0})

	// Without an order, a re-pinned project goes to the end and an
	// existing pin keeps its place.
	require.NoError(t, client.PinChatProject(ctx, orgID, a.ID, codersdk.PinChatProjectRequest{}))
	require.NoError(t, client.PinChatProject(ctx, orgID, c.ID, codersdk.PinChatProjectRequest{}))
	requirePinOrders(ctx, t, client, map[uuid.UUID]int32{c.ID: 1, b.ID: 2, a.ID: 3})

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
	// Role-based read access is not enough: the project would be missing
	// from the site owner's summaries.
	otherOwn := createChatProject(t, other, orgID, "Pin Other")
	requireSDKError(t, client.PinChatProject(ctx, orgID, otherOwn.ID, codersdk.PinChatProjectRequest{}), http.StatusNotFound)
	requireSDKError(t, client.UnpinChatProject(ctx, orgID, otherOwn.ID), http.StatusNotFound)
	setRole := func(role codersdk.ChatProjectRole) {
		require.NoError(t, client.UpdateChatProjectACL(ctx, orgID, a.ID, codersdk.UpdateChatProjectACL{
			UserRoles: map[string]codersdk.ChatProjectRole{otherUser.ID.String(): role},
		}))
	}
	setRole(codersdk.ChatProjectRoleUse)
	require.False(t, chatProjectSummariesByID(ctx, t, other)[a.ID].Pinned)
	require.NoError(t, other.PinChatProject(ctx, orgID, a.ID, codersdk.PinChatProjectRequest{PinOrder: ptr.Ref(int32(1))}))
	requirePinOrders(ctx, t, other, map[uuid.UUID]int32{a.ID: 1, otherOwn.ID: 2})
	setRole(codersdk.ChatProjectRoleDeleted)
	require.NotContains(t, chatProjectSummariesByID(ctx, t, other), a.ID)
	requireSDKError(t, other.UnpinChatProject(ctx, orgID, a.ID), http.StatusNotFound)
	// The hidden pin takes no position, either in summaries or in
	// requested positions.
	requirePinOrders(ctx, t, other, map[uuid.UUID]int32{otherOwn.ID: 1})
	otherNew := createChatProject(t, other, orgID, "Pin Other New")
	require.NoError(t, other.PinChatProject(ctx, orgID, otherOwn.ID, codersdk.PinChatProjectRequest{PinOrder: ptr.Ref(int32(2))}))
	requirePinOrders(ctx, t, other, map[uuid.UUID]int32{otherNew.ID: 1, otherOwn.ID: 2})
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

	readOnly := newScopedClient("chat_project:read")
	requireSDKError(t, readOnly.PinChatProject(ctx, orgID, project.ID, codersdk.PinChatProjectRequest{}), http.StatusForbidden)
	requireSDKError(t, readOnly.UnpinChatProject(ctx, orgID, project.ID), http.StatusForbidden)

	// The same key with user:update_personal may change the pin, so that
	// scope is the one the read-only key lacks.
	personal := newScopedClient("chat_project:read", "user:update_personal")
	require.NoError(t, personal.UnpinChatProject(ctx, orgID, project.ID))
	require.NoError(t, personal.PinChatProject(ctx, orgID, project.ID, codersdk.PinChatProjectRequest{}))

	// Creating a project pins it for its creator, so a key that may create
	// projects also needs both of the pin's permissions.
	createReq := codersdk.CreateChatProjectRequest{Name: "Scoped Create"}
	_, err := newScopedClient("chat_project:create", "chat_project:read").CreateChatProject(ctx, orgID, createReq)
	requireSDKError(t, err, http.StatusForbidden)
	_, err = newScopedClient("chat_project:create", "user:update_personal").CreateChatProject(ctx, orgID, createReq)
	requireSDKError(t, err, http.StatusForbidden)
	_, err = newScopedClient("chat_project:create", "chat_project:read", "user:update_personal").CreateChatProject(ctx, orgID, createReq)
	require.NoError(t, err)

	// Summaries carry the caller's pins, so listing them needs
	// user:read_personal. So does pinning at a position, which reads the
	// caller's other pins.
	_, err = readOnly.ListChatProjectSummaries(ctx)
	requireSDKError(t, err, http.StatusForbidden)
	first := codersdk.PinChatProjectRequest{PinOrder: ptr.Ref(int32(1))}
	requireSDKError(t, personal.PinChatProject(ctx, orgID, project.ID, first), http.StatusForbidden)
	reader := newScopedClient("chat_project:read", "user:read_personal", "user:update_personal")
	_, err = reader.ListChatProjectSummaries(ctx)
	require.NoError(t, err)
	require.NoError(t, reader.PinChatProject(ctx, orgID, project.ID, first))
}

func TestChatProjectPinsConcurrentWrites(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client, _ := newChatProjectClient(t)
	orgID := coderdtest.CreateFirstUser(t, client.Client).OrganizationID
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
		summaries := chatProjectSummariesByID(ctx, t, client)
		for i, project := range projects {
			require.Equal(t, (i+round)%2 != 0, summaries[project.ID].Pinned, project.Name)
		}
	}
}

func TestChatProjectSummaries(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	db, ps := dbtestutil.NewDB(t)
	client, api := newChatProjectClientWithAPI(t, func(options *coderdtest.Options) {
		options.Database, options.Pubsub = db, ps
	})
	firstUser := coderdtest.CreateFirstUser(t, client.Client)
	orgID, me := firstUser.OrganizationID, firstUser.UserID
	model := createChatModel(t, client)
	aliceClient, alice := newChatProjectMember(t, client, orgID)
	_, bob := newChatProjectMember(t, client, orgID)
	_, carol := newChatProjectMember(t, client, orgID)
	group := dbgen.Group(t, api.Database, database.Group{OrganizationID: orgID})
	for _, user := range []codersdk.User{alice, bob} {
		dbgen.GroupMember(t, api.Database, database.GroupMemberTable{GroupID: group.ID, UserID: user.ID})
	}
	newChat := func(project codersdk.ChatProject, owner uuid.UUID, status database.ChatStatus, reply bool, parent uuid.NullUUID) database.Chat {
		chat := dbgen.Chat(t, api.Database, database.Chat{
			OrganizationID: orgID, OwnerID: owner, ProjectID: uuid.NullUUID{UUID: project.ID, Valid: true},
			LastModelConfigID: model.ID, Status: status, ParentChatID: parent, RootChatID: parent,
		})
		if reply {
			dbgen.ChatMessage(t, api.Database, database.ChatMessage{ChatID: chat.ID, Role: database.ChatMessageRoleAssistant})
		}
		return chat
	}
	share := func(name string, users, groups []uuid.UUID) codersdk.ChatProject {
		project := createChatProject(t, client, orgID, name)
		acl := codersdk.UpdateChatProjectACL{UserRoles: map[string]codersdk.ChatProjectRole{}, GroupRoles: map[string]codersdk.ChatProjectRole{}}
		for _, id := range users {
			acl.UserRoles[id.String()] = codersdk.ChatProjectRoleUse
		}
		for _, id := range groups {
			acl.GroupRoles[id.String()] = codersdk.ChatProjectRoleUse
		}
		require.NoError(t, client.UpdateChatProjectACL(ctx, orgID, project.ID, acl))
		return project
	}
	none := uuid.NullUUID{}

	busy := share("Summary Busy", []uuid.UUID{alice.ID}, []uuid.UUID{group.ID})
	// The ACL API keeps groups from other organizations it finds stored, but
	// they reach only members of the project's organization, so dave is not
	// counted.
	otherOrg := dbgen.Organization(t, db, database.Organization{})
	dave := dbgen.User(t, db, database.User{})
	dbgen.OrganizationMember(t, db, database.OrganizationMember{UserID: dave.ID, OrganizationID: otherOrg.ID})
	foreignGroup := dbgen.Group(t, db, database.Group{OrganizationID: otherOrg.ID})
	dbgen.GroupMember(t, db, database.GroupMemberTable{GroupID: foreignGroup.ID, UserID: dave.ID})
	stored, err := db.GetChatProjectByID(ctx, busy.ID)
	require.NoError(t, err)
	stored.GroupACL[foreignGroup.ID.String()] = database.ChatACLEntry{Permissions: []policy.Action{policy.ActionRead}}
	require.NoError(t, db.UpdateChatProjectACLByID(ctx, database.UpdateChatProjectACLByIDParams{ID: busy.ID, UserACL: stored.UserACL, GroupACL: stored.GroupACL}))
	unread := newChat(busy, me, database.ChatStatusWaiting, true, none)
	running := newChat(busy, me, database.ChatStatusRunning, false, none)
	paused := newChat(busy, me, database.ChatStatusPaused, false, none)
	// Child chats, archived chats and other users' chats are not counted.
	quiet := share("Summary Quiet", []uuid.UUID{carol.ID}, nil)
	read := newChat(quiet, me, database.ChatStatusWaiting, false, none)
	newChat(quiet, me, database.ChatStatusError, false, uuid.NullUUID{UUID: read.ID, Valid: true})
	archived := newChat(quiet, me, database.ChatStatusError, true, none)
	require.NoError(t, client.UpdateChat(ctx, archived.ID, codersdk.UpdateChatRequest{Archived: ptr.Ref(true)}))
	newChat(quiet, carol.ID, database.ChatStatusRunning, true, none)
	require.NoError(t, client.DeleteOrganizationMember(ctx, orgID, carol.Username))
	// A chat that is being stopped still counts as running. A grant without
	// read does not make alice a member.
	stopping := share("Summary Stopping", nil, nil)
	require.NoError(t, db.UpdateChatProjectACLByID(ctx, database.UpdateChatProjectACLByIDParams{
		ID:       stopping.ID,
		UserACL:  database.ChatACL{alice.ID.String(): {Permissions: []policy.Action{policy.ActionShare}}},
		GroupACL: database.ChatACL{},
	}))
	interrupting := newChat(stopping, me, database.ChatStatusInterrupting, false, none)
	// The Everyone group's ID is the organization ID.
	everyone := share("Summary Everyone", nil, []uuid.UUID{orgID})
	// Its members are counted per organization, so an owner outside the
	// organization must be added once.
	outsider := dbgen.ChatProject(t, db, database.ChatProject{OrganizationID: orgID, OwnerID: dave.ID})
	require.NoError(t, db.UpdateChatProjectACLByID(ctx, database.UpdateChatProjectACLByIDParams{
		ID:       outsider.ID,
		UserACL:  database.ChatACL{},
		GroupACL: database.ChatACL{orgID.String(): {Permissions: []policy.Action{policy.ActionRead}}},
	}))
	dbgen.ChatProjectPin(t, db, database.ChatProjectPin{UserID: me, ProjectID: outsider.ID})
	// Suspended users cannot use any project.
	_, err = client.UpdateUserStatus(ctx, bob.Username, codersdk.UserStatusSuspended)
	require.NoError(t, err)

	type summary struct {
		chats, unread, members int64
		running, attention     bool
		lastActivity           time.Time
	}
	latestUpdate := func(chats ...database.Chat) time.Time {
		var latest time.Time
		for _, chat := range chats {
			fetched, err := client.GetChat(ctx, chat.ID)
			require.NoError(t, err)
			if fetched.UpdatedAt.After(latest) {
				latest = fetched.UpdatedAt.UTC()
			}
		}
		return latest
	}
	summaries := chatProjectSummariesByID(ctx, t, client)
	memberCounts := chatProjectMemberCountsByID(ctx, t, client)
	for project, want := range map[uuid.UUID]summary{
		// The owner plus alice, who is granted directly and through the group.
		busy.ID: {3, 1, 2, true, true, latestUpdate(unread, running, paused)},
		// Carol left the organization.
		quiet.ID:    {1, 0, 1, false, false, latestUpdate(read)},
		stopping.ID: {1, 0, 1, true, false, latestUpdate(interrupting)},
		// Organization members, without system users, carol or bob.
		everyone.ID: {0, 0, 2, false, false, time.Time{}},
		outsider.ID: {0, 0, 3, false, false, time.Time{}},
	} {
		got := summaries[project]
		require.True(t, got.Pinned)
		lastActivity := ptr.NilToDefault(got.LastActivityAt, time.Time{}).UTC()
		gotSummary := summary{got.OwnChatCount, got.OwnUnreadCount, memberCounts[project], got.OwnRunning, got.OwnNeedsAttention, lastActivity}
		require.Equal(t, want, gotSummary, project)
	}
	requireSummariesMatchList(ctx, t, aliceClient)

	// Without project sharing, as with its experiment off in a release
	// build, grants still decide which projects are visible, as in the
	// project list, but only owners are counted.
	rows, err := db.GetChatProjectMemberCountsByUserID(ctx, database.GetChatProjectMemberCountsByUserIDParams{UserID: alice.ID})
	require.NoError(t, err)
	ownerOnly := make(map[uuid.UUID]int64, len(rows))
	for _, row := range rows {
		ownerOnly[row.ProjectID] = row.MemberCount
	}
	require.EqualValues(t, 1, ownerOnly[busy.ID])

	// Alice keeps a pin on stopping, whose grant lacks read, so it is not
	// returned and leaves no gap in the pin orders she sees.
	require.NoError(t, aliceClient.PinChatProject(ctx, orgID, busy.ID, codersdk.PinChatProjectRequest{}))
	dbgen.ChatProjectPin(t, db, database.ChatProjectPin{UserID: alice.ID, ProjectID: stopping.ID})
	require.NoError(t, aliceClient.PinChatProject(ctx, orgID, everyone.ID, codersdk.PinChatProjectRequest{}))
	requirePinOrders(ctx, t, aliceClient, map[uuid.UUID]int32{busy.ID: 1, everyone.ID: 2})
	// Requested positions count only the pins she sees, so the hidden pin
	// stored between busy and everyone does not absorb the move.
	require.NoError(t, aliceClient.PinChatProject(ctx, orgID, busy.ID, codersdk.PinChatProjectRequest{PinOrder: ptr.Ref(int32(2))}))
	requirePinOrders(ctx, t, aliceClient, map[uuid.UUID]int32{everyone.ID: 1, busy.ID: 2})
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

	require.EqualValues(t, 1, chatProjectMemberCountsByID(ctx, t, client)[project.ID])

	// The member cannot open the project, so no project list reaches their
	// chat in it and the main list keeps it.
	chat := dbgen.Chat(t, db, database.Chat{
		OrganizationID: orgID, OwnerID: memberUser.ID, LastModelConfigID: model.ID,
		ProjectID: uuid.NullUUID{UUID: project.ID, Valid: true},
	})
	memberChats, err := member.ListChats(ctx, &codersdk.ListChatsOptions{ExcludeOwnProjectChats: true})
	require.NoError(t, err)
	require.Contains(t, chatIDs(memberChats), chat.ID)

	// A pin from before sharing was disabled takes no visible position.
	require.NoError(t, db.PinChatProject(ctx, database.PinChatProjectParams{
		UserID: memberUser.ID, ProjectID: project.ID, PinOrder: sql.NullInt32{Int32: 1, Valid: true},
	}))
	x := createChatProject(t, member, orgID, "Hidden Pin X")
	y := createChatProject(t, member, orgID, "Hidden Pin Y")
	requirePinOrders(ctx, t, member, map[uuid.UUID]int32{x.ID: 1, y.ID: 2})
	require.NotContains(t, chatProjectSummariesByID(ctx, t, member), project.ID)
	requireSummariesMatchList(ctx, t, member)
	require.NoError(t, member.PinChatProject(ctx, orgID, y.ID, codersdk.PinChatProjectRequest{PinOrder: ptr.Ref(int32(1))}))
	requirePinOrders(ctx, t, member, map[uuid.UUID]int32{y.ID: 1, x.ID: 2})
	require.NoError(t, member.UnpinChatProject(ctx, orgID, x.ID))
	require.NoError(t, member.PinChatProject(ctx, orgID, x.ID, codersdk.PinChatProjectRequest{}))
	requirePinOrders(ctx, t, member, map[uuid.UUID]int32{y.ID: 1, x.ID: 2})

	// The owner role still reads a project that keeps a grant for its
	// holder, so the summaries list it, as the project list does.
	require.NoError(t, db.UpdateChatProjectACLByID(ctx, database.UpdateChatProjectACLByIDParams{
		ID:       x.ID,
		UserACL:  database.ChatACL{firstUser.UserID.String(): {Permissions: []policy.Action{policy.ActionRead}}},
		GroupACL: database.ChatACL{},
	}))
	require.Contains(t, chatProjectSummariesByID(ctx, t, client), x.ID)
	requireSummariesMatchList(ctx, t, client)
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
			require.NoError(t, member.PinChatProject(ctx, orgID, unpinned.ID, codersdk.PinChatProjectRequest{}))
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

func chatProjectSummariesByID(ctx context.Context, t testing.TB, client *codersdk.ExperimentalClient) map[uuid.UUID]codersdk.ChatProjectSummary {
	t.Helper()
	summaries, err := client.ListChatProjectSummaries(ctx)
	require.NoError(t, err)
	byID := make(map[uuid.UUID]codersdk.ChatProjectSummary, len(summaries))
	for _, summary := range summaries {
		byID[summary.ProjectID] = summary
	}
	return byID
}

func chatProjectMemberCountsByID(ctx context.Context, t testing.TB, client *codersdk.ExperimentalClient) map[uuid.UUID]int64 {
	t.Helper()
	counts, err := client.ListChatProjectMemberCounts(ctx)
	require.NoError(t, err)
	byID := make(map[uuid.UUID]int64, len(counts))
	for _, count := range counts {
		byID[count.ProjectID] = count.MemberCount
	}
	return byID
}

// requireSummariesMatchList checks that the summaries and the member counts,
// which clients join to the project list by ID, cover exactly its projects.
func requireSummariesMatchList(ctx context.Context, t testing.TB, client *codersdk.ExperimentalClient) {
	t.Helper()
	projects, err := client.ListChatProjects(ctx)
	require.NoError(t, err)
	projectIDs := slice.List(projects, func(project codersdk.ChatProject) uuid.UUID { return project.ID })
	require.ElementsMatch(t, projectIDs, slices.Collect(maps.Keys(chatProjectSummariesByID(ctx, t, client))))
	require.ElementsMatch(t, projectIDs, slices.Collect(maps.Keys(chatProjectMemberCountsByID(ctx, t, client))))
}

// requirePinOrders checks each project's pin order; 0 means not pinned.
func requirePinOrders(ctx context.Context, t testing.TB, client *codersdk.ExperimentalClient, want map[uuid.UUID]int32) {
	t.Helper()
	summaries := chatProjectSummariesByID(ctx, t, client)
	got := make(map[uuid.UUID]int32, len(want))
	for id := range want {
		require.Equal(t, summaries[id].PinOrder > 0, summaries[id].Pinned)
		got[id] = summaries[id].PinOrder
	}
	require.Equal(t, want, got)
}
