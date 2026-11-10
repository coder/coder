package coderd_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

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

	// Without an order, a re-pinned tombstone goes to the end and an
	// existing pin keeps its place.
	require.NoError(t, client.PinChatProject(ctx, orgID, a.ID, codersdk.PinChatProjectRequest{}))
	require.NoError(t, client.PinChatProject(ctx, orgID, c.ID, codersdk.PinChatProjectRequest{}))
	requirePinOrders(ctx, t, client, map[uuid.UUID]int32{c.ID: 1, b.ID: 2, a.ID: 3})

	err := client.PinChatProject(ctx, orgID, a.ID, codersdk.PinChatProjectRequest{PinOrder: ptr.Ref(int32(0))})
	requireSDKError(t, err, http.StatusBadRequest)

	// Pins need read access. Sharing does not pin, and a pin that outlives
	// access stays hidden.
	other, otherUser := newChatProjectMember(t, client, orgID)
	requireSDKError(t, other.PinChatProject(ctx, orgID, a.ID, codersdk.PinChatProjectRequest{}), http.StatusNotFound)
	setRole := func(role codersdk.ChatProjectRole) {
		require.NoError(t, client.UpdateChatProjectACL(ctx, orgID, a.ID, codersdk.UpdateChatProjectACL{
			UserRoles: map[string]codersdk.ChatProjectRole{otherUser.ID.String(): role},
		}))
	}
	setRole(codersdk.ChatProjectRoleUse)
	require.False(t, chatProjectSummariesByID(ctx, t, other)[a.ID].Pinned)
	require.NoError(t, other.PinChatProject(ctx, orgID, a.ID, codersdk.PinChatProjectRequest{}))
	require.True(t, chatProjectSummariesByID(ctx, t, other)[a.ID].Pinned)
	setRole(codersdk.ChatProjectRoleDeleted)
	require.NotContains(t, chatProjectSummariesByID(ctx, t, other), a.ID)
	requireSDKError(t, other.UnpinChatProject(ctx, orgID, a.ID), http.StatusNotFound)
}

func TestChatProjectSummaries(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client, api := newChatProjectClientWithAPI(t)
	firstUser := coderdtest.CreateFirstUser(t, client.Client)
	orgID, me := firstUser.OrganizationID, firstUser.UserID
	model := createChatModel(t, client)
	_, alice := newChatProjectMember(t, client, orgID)
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
	// The Everyone group's ID is the organization ID.
	everyone := share("Summary Everyone", nil, []uuid.UUID{orgID})

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
	for project, want := range map[uuid.UUID]summary{
		// The owner plus alice and bob, who are granted directly and through
		// the group.
		busy.ID: {3, 1, 3, true, true, latestUpdate(unread, running, paused)},
		// Carol left the organization.
		quiet.ID: {1, 0, 1, false, false, latestUpdate(read)},
		// Organization members, without system users or carol.
		everyone.ID: {0, 0, 3, false, false, time.Time{}},
	} {
		got := summaries[project]
		require.True(t, got.Pinned)
		require.Equal(t, codersdk.ChatProjectPermissions{Update: true, Delete: true, Share: true}, got.Project.Permissions)
		lastActivity := ptr.NilToDefault(got.LastActivityAt, time.Time{}).UTC()
		gotSummary := summary{got.OwnChatCount, got.OwnUnreadCount, got.MemberCount, got.OwnRunning, got.OwnNeedsAttention, lastActivity}
		require.Equal(t, want, gotSummary, got.Project.Name)
	}
}

//nolint:paralleltest // Disabling chat sharing flips a process-wide RBAC switch.
func TestChatProjectSummariesSharingDisabled(t *testing.T) {
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
	_, _ = newChatProjectMember(t, client, firstUser.OrganizationID)
	project := createChatProject(t, client, firstUser.OrganizationID, "Members Disabled")
	// Grants left from before sharing was disabled do not count.
	require.NoError(t, db.UpdateChatProjectACLByID(ctx, database.UpdateChatProjectACLByIDParams{
		ID:       project.ID,
		UserACL:  database.ChatACL{},
		GroupACL: database.ChatACL{firstUser.OrganizationID.String(): {Permissions: []policy.Action{policy.ActionRead}}},
	}))

	summaries := chatProjectSummariesByID(ctx, t, client)
	require.EqualValues(t, 1, summaries[project.ID].MemberCount)
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

	// Other callers still see every chat.
	all, err := client.ListChats(ctx, nil)
	require.NoError(t, err)
	require.Contains(t, chatIDs(all), hidden.ID)
}

func chatProjectSummariesByID(ctx context.Context, t testing.TB, client *codersdk.ExperimentalClient) map[uuid.UUID]codersdk.ChatProjectSummary {
	t.Helper()
	summaries, err := client.ListChatProjectSummaries(ctx)
	require.NoError(t, err)
	byID := make(map[uuid.UUID]codersdk.ChatProjectSummary, len(summaries))
	for _, summary := range summaries {
		byID[summary.Project.ID] = summary
	}
	return byID
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
