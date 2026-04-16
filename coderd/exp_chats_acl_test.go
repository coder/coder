package coderd_test

import (
	"context"
	"encoding/json"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	coderdpubsub "github.com/coder/coder/v2/coderd/pubsub"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

// createSharedChat creates a chat owned by the admin client for use in
// sharing tests. Callers must have already invoked
// createChatModelConfig(t, ownerClient). The returned chat has no ACL
// entries yet; share it by calling ownerClient.UpdateChatACL.
func createSharedChat(
	ctx context.Context,
	t *testing.T,
	ownerClient *codersdk.ExperimentalClient,
	orgID uuid.UUID,
	title string,
) codersdk.Chat {
	t.Helper()

	chat, err := ownerClient.CreateChat(ctx, codersdk.CreateChatRequest{
		OrganizationID: orgID,
		Content: []codersdk.ChatInputPart{
			{
				Type: codersdk.ChatInputPartTypeText,
				Text: title,
			},
		},
	})
	require.NoError(t, err)
	return chat
}

func TestPatchChatACL_AddsUserAndGroup(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	ownerClient, db := newChatClientWithDatabase(t)
	firstUser := coderdtest.CreateFirstUser(t, ownerClient.Client)
	_ = createChatModelConfig(t, ownerClient)

	_, viewer := coderdtest.CreateAnotherUser(t, ownerClient.Client, firstUser.OrganizationID)

	group := dbgen.Group(t, db, database.Group{OrganizationID: firstUser.OrganizationID})
	dbgen.GroupMember(t, db, database.GroupMemberTable{GroupID: group.ID, UserID: viewer.ID})

	chat := createSharedChat(ctx, t, ownerClient, firstUser.OrganizationID, "acl patch add user+group")

	err := ownerClient.UpdateChatACL(ctx, chat.ID, codersdk.UpdateChatACL{
		UserRoles: map[string]codersdk.ChatRole{
			viewer.ID.String(): codersdk.ChatRoleRead,
		},
		GroupRoles: map[string]codersdk.ChatRole{
			group.ID.String(): codersdk.ChatRoleRead,
		},
	})
	require.NoError(t, err)

	acl, err := ownerClient.ChatACL(ctx, chat.ID)
	require.NoError(t, err)

	require.Len(t, acl.Users, 1)
	require.Equal(t, viewer.ID, acl.Users[0].ID)
	require.Equal(t, codersdk.ChatRoleRead, acl.Users[0].Role)

	require.Len(t, acl.Groups, 1)
	require.Equal(t, group.ID, acl.Groups[0].ID)
	require.Equal(t, codersdk.ChatRoleRead, acl.Groups[0].Role)
}

func TestPatchChatACL_RejectsNonReadRole(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	ownerClient := newChatClient(t)
	firstUser := coderdtest.CreateFirstUser(t, ownerClient.Client)
	_ = createChatModelConfig(t, ownerClient)

	_, viewer := coderdtest.CreateAnotherUser(t, ownerClient.Client, firstUser.OrganizationID)

	chat := createSharedChat(ctx, t, ownerClient, firstUser.OrganizationID, "acl patch bad role")

	err := ownerClient.UpdateChatACL(ctx, chat.ID, codersdk.UpdateChatACL{
		UserRoles: map[string]codersdk.ChatRole{
			// "admin" is not a valid ChatRole in v1 (only "read" and
			// the empty delete sentinel are accepted).
			viewer.ID.String(): codersdk.ChatRole("admin"),
		},
	})
	requireSDKError(t, err, http.StatusBadRequest)
}

func TestPatchChatACL_SubChatRejected(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	ownerClient, db := newChatClientWithDatabase(t)
	firstUser := coderdtest.CreateFirstUser(t, ownerClient.Client)
	modelConfig := createChatModelConfig(t, ownerClient)

	_, viewer := coderdtest.CreateAnotherUser(t, ownerClient.Client, firstUser.OrganizationID)

	parent := createSharedChat(ctx, t, ownerClient, firstUser.OrganizationID, "root chat for sub-chat patch")

	// Insert a sub-chat directly so ParentChatID/RootChatID are set.
	subChat, err := db.InsertChat(dbauthz.AsSystemRestricted(ctx), database.InsertChatParams{
		OrganizationID:    firstUser.OrganizationID,
		Status:            database.ChatStatusWaiting,
		ClientType:        database.ChatClientTypeUi,
		OwnerID:           firstUser.UserID,
		LastModelConfigID: modelConfig.ID,
		Title:             "sub-chat",
		ParentChatID:      uuid.NullUUID{UUID: parent.ID, Valid: true},
		RootChatID:        uuid.NullUUID{UUID: parent.ID, Valid: true},
	})
	require.NoError(t, err)

	err = ownerClient.UpdateChatACL(ctx, subChat.ID, codersdk.UpdateChatACL{
		UserRoles: map[string]codersdk.ChatRole{
			viewer.ID.String(): codersdk.ChatRoleRead,
		},
	})
	sdkErr := requireSDKError(t, err, http.StatusBadRequest)
	require.Contains(t, sdkErr.Message, "root chats")
}

func TestPatchChatACL_RequiresToolConfirmation(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	ownerClient, db := newChatClientWithDatabase(t)
	firstUser := coderdtest.CreateFirstUser(t, ownerClient.Client)
	modelConfig := createChatModelConfig(t, ownerClient)

	_, viewer := coderdtest.CreateAnotherUser(t, ownerClient.Client, firstUser.OrganizationID)

	chat := createSharedChat(ctx, t, ownerClient, firstUser.OrganizationID, "acl patch tool confirm")

	// Insert an assistant message with a tool-call part so the chat
	// is flagged as containing visible tool calls.
	toolCallPart := codersdk.ChatMessageToolCall(
		"call_abc",
		"demo_tool",
		json.RawMessage(`{"arg":"value"}`),
	)
	content, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{toolCallPart})
	require.NoError(t, err)

	_, err = db.InsertChatMessages(dbauthz.AsSystemRestricted(ctx), database.InsertChatMessagesParams{
		ChatID:              chat.ID,
		CreatedBy:           []uuid.UUID{uuid.Nil},
		ModelConfigID:       []uuid.UUID{modelConfig.ID},
		Role:                []database.ChatMessageRole{database.ChatMessageRoleAssistant},
		ContentVersion:      []int16{chatprompt.CurrentContentVersion},
		Content:             []string{string(content.RawMessage)},
		Visibility:          []database.ChatMessageVisibility{database.ChatMessageVisibilityBoth},
		InputTokens:         []int64{0},
		OutputTokens:        []int64{0},
		TotalTokens:         []int64{0},
		ReasoningTokens:     []int64{0},
		CacheCreationTokens: []int64{0},
		CacheReadTokens:     []int64{0},
		ContextLimit:        []int64{0},
		Compressed:          []bool{false},
		TotalCostMicros:     []int64{0},
		RuntimeMs:           []int64{0},
	})
	require.NoError(t, err)

	// First PATCH without the confirmation flag: must be rejected
	// with 400 and a validation pointing at confirm_share_tool_calls.
	err = ownerClient.UpdateChatACL(ctx, chat.ID, codersdk.UpdateChatACL{
		UserRoles: map[string]codersdk.ChatRole{
			viewer.ID.String(): codersdk.ChatRoleRead,
		},
	})
	sdkErr := requireSDKError(t, err, http.StatusBadRequest)

	foundConfirmField := false
	for _, v := range sdkErr.Validations {
		if v.Field == "confirm_share_tool_calls" {
			foundConfirmField = true
			break
		}
	}
	require.True(t, foundConfirmField,
		"expected validation error on confirm_share_tool_calls, got: %+v", sdkErr.Validations)

	// Second PATCH with the confirmation flag set: must succeed.
	err = ownerClient.UpdateChatACL(ctx, chat.ID, codersdk.UpdateChatACL{
		UserRoles: map[string]codersdk.ChatRole{
			viewer.ID.String(): codersdk.ChatRoleRead,
		},
		ConfirmShareToolCalls: true,
	})
	require.NoError(t, err)

	acl, err := ownerClient.ChatACL(ctx, chat.ID)
	require.NoError(t, err)
	require.Len(t, acl.Users, 1)
	require.Equal(t, viewer.ID, acl.Users[0].ID)
}

func TestDeleteChatACL_ClearsEntriesAndPublishesInvalidation(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	rawClient, _, api := coderdtest.NewWithAPI(t, &coderdtest.Options{
		DeploymentValues: chatDeploymentValues(t),
	})
	ownerClient := codersdk.NewExperimentalClient(rawClient)
	firstUser := coderdtest.CreateFirstUser(t, ownerClient.Client)
	_ = createChatModelConfig(t, ownerClient)

	_, viewer := coderdtest.CreateAnotherUser(t, ownerClient.Client, firstUser.OrganizationID)

	chat := createSharedChat(ctx, t, ownerClient, firstUser.OrganizationID, "delete acl invalidation")

	err := ownerClient.UpdateChatACL(ctx, chat.ID, codersdk.UpdateChatACL{
		UserRoles: map[string]codersdk.ChatRole{
			viewer.ID.String(): codersdk.ChatRoleRead,
		},
	})
	require.NoError(t, err)

	// Subscribe to the chat's ACL invalidation channel BEFORE
	// triggering the DELETE so the publish cannot race the
	// subscription.
	received := make(chan struct{}, 1)
	cancelSub, err := api.Pubsub.SubscribeWithErr(
		coderdpubsub.ChatACLInvalidationChannel(chat.ID),
		func(_ context.Context, _ []byte, _ error) {
			select {
			case received <- struct{}{}:
			default:
			}
		},
	)
	require.NoError(t, err)
	defer cancelSub()

	err = ownerClient.DeleteChatACL(ctx, chat.ID)
	require.NoError(t, err)

	acl, err := ownerClient.ChatACL(ctx, chat.ID)
	require.NoError(t, err)
	require.Empty(t, acl.Users)
	require.Empty(t, acl.Groups)

	select {
	case <-received:
	case <-ctx.Done():
		t.Fatalf("timed out waiting for chat acl invalidation publish: %v", ctx.Err())
	}
}

func TestListChats_SharedFilter(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	ownerClient := newChatClient(t)
	firstUser := coderdtest.CreateFirstUser(t, ownerClient.Client)
	_ = createChatModelConfig(t, ownerClient)

	viewerRaw, viewer := coderdtest.CreateAnotherUser(t, ownerClient.Client, firstUser.OrganizationID)
	viewerClient := codersdk.NewExperimentalClient(viewerRaw)

	// Owner creates two chats; share the second one with viewer.
	ownedOnly := createSharedChat(ctx, t, ownerClient, firstUser.OrganizationID, "owned only")
	sharedChat := createSharedChat(ctx, t, ownerClient, firstUser.OrganizationID, "owned and shared")

	err := ownerClient.UpdateChatACL(ctx, sharedChat.ID, codersdk.UpdateChatACL{
		UserRoles: map[string]codersdk.ChatRole{
			viewer.ID.String(): codersdk.ChatRoleRead,
		},
	})
	require.NoError(t, err)

	// Viewer has no owned chats, so the default (no shared filter)
	// must return an empty list.
	defaultList, err := viewerClient.ListChats(ctx, nil)
	require.NoError(t, err)
	require.Empty(t, defaultList, "default list should only include owned chats")

	// ?shared=include returns owned + shared.
	includeList, err := viewerClient.ListChats(ctx, &codersdk.ListChatsOptions{
		Shared: codersdk.ChatSharedFilterInclude,
	})
	require.NoError(t, err)
	includeIDs := chatIDSet(includeList)
	require.Contains(t, includeIDs, sharedChat.ID)
	require.NotContains(t, includeIDs, ownedOnly.ID, "viewer does not own or share the first chat")

	// ?shared=only returns only chats the caller does not own but
	// has access to via ACL.
	onlyList, err := viewerClient.ListChats(ctx, &codersdk.ListChatsOptions{
		Shared: codersdk.ChatSharedFilterOnly,
	})
	require.NoError(t, err)
	onlyIDs := chatIDSet(onlyList)
	require.Contains(t, onlyIDs, sharedChat.ID)
	require.Len(t, onlyIDs, 1, "viewer has exactly one shared chat")

	// Owner sees both chats under the default filter.
	ownerList, err := ownerClient.ListChats(ctx, nil)
	require.NoError(t, err)
	ownerIDs := chatIDSet(ownerList)
	require.Contains(t, ownerIDs, ownedOnly.ID)
	require.Contains(t, ownerIDs, sharedChat.ID)

	// Unknown shared filter values return 400. The SDK wrapper
	// guards the known values, so issue the request directly.
	res, err := viewerClient.Request(ctx, http.MethodGet, "/api/experimental/chats?shared=wat", nil)
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, http.StatusBadRequest, res.StatusCode)
}

func chatIDSet(chats []codersdk.Chat) map[uuid.UUID]struct{} {
	ids := make(map[uuid.UUID]struct{}, len(chats))
	for _, c := range chats {
		ids[c.ID] = struct{}{}
	}
	return ids
}

// TestChatStream_SharedViewerReceivesMessages covers plan id
// [stream-shared]: a user added to a chat's ACL can open the stream
// and receive the snapshot of historical user messages, confirming
// authz gates read correctly for shared viewers.
func TestChatStream_SharedViewerReceivesMessages(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	ownerClient := newChatClient(t)
	firstUser := coderdtest.CreateFirstUser(t, ownerClient.Client)
	_ = createChatModelConfig(t, ownerClient)

	viewerRaw, viewer := coderdtest.CreateAnotherUser(t, ownerClient.Client, firstUser.OrganizationID)
	viewerClient := codersdk.NewExperimentalClient(viewerRaw)

	const initialMessage = "shared stream initial message"
	chat, err := ownerClient.CreateChat(ctx, codersdk.CreateChatRequest{
		OrganizationID: firstUser.OrganizationID,
		Content: []codersdk.ChatInputPart{
			{
				Type: codersdk.ChatInputPartTypeText,
				Text: initialMessage,
			},
		},
	})
	require.NoError(t, err)

	err = ownerClient.UpdateChatACL(ctx, chat.ID, codersdk.UpdateChatACL{
		UserRoles: map[string]codersdk.ChatRole{
			viewer.ID.String(): codersdk.ChatRoleRead,
		},
	})
	require.NoError(t, err)

	events, closer, err := viewerClient.StreamChat(ctx, chat.ID, nil)
	require.NoError(t, err)
	defer closer.Close()

	hasTextPart := func(parts []codersdk.ChatMessagePart, want string) bool {
		for _, part := range parts {
			if part.Type == codersdk.ChatMessagePartTypeText && part.Text == want {
				return true
			}
		}
		return false
	}

	for {
		select {
		case <-ctx.Done():
			require.FailNow(t, "timed out waiting for shared viewer stream event")
		case event, ok := <-events:
			require.True(t, ok, "stream closed before expected event")
			require.Equal(t, chat.ID, event.ChatID)
			require.NotEqual(t, codersdk.ChatStreamEventTypeError, event.Type)
			if event.Type == codersdk.ChatStreamEventTypeMessage &&
				event.Message != nil &&
				event.Message.Role == codersdk.ChatMessageRoleUser &&
				hasTextPart(event.Message.Content, initialMessage) {
				return
			}
		}
	}
}

// TestChatStream_ACLRevokePublishesInvalidationAndClosesStream covers
// plan id [stream-revoke-invalidation]: when the owner revokes access
// while a shared viewer's stream is open, the server publishes on the
// per-chat ACL invalidation channel and closes the viewer's stream.
func TestChatStream_ACLRevokePublishesInvalidationAndClosesStream(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	rawClient, _, api := coderdtest.NewWithAPI(t, &coderdtest.Options{
		DeploymentValues: chatDeploymentValues(t),
	})
	ownerClient := codersdk.NewExperimentalClient(rawClient)
	firstUser := coderdtest.CreateFirstUser(t, ownerClient.Client)
	_ = createChatModelConfig(t, ownerClient)

	viewerRaw, viewer := coderdtest.CreateAnotherUser(t, ownerClient.Client, firstUser.OrganizationID)
	viewerClient := codersdk.NewExperimentalClient(viewerRaw)

	chat := createSharedChat(ctx, t, ownerClient, firstUser.OrganizationID, "stream revoke")

	err := ownerClient.UpdateChatACL(ctx, chat.ID, codersdk.UpdateChatACL{
		UserRoles: map[string]codersdk.ChatRole{
			viewer.ID.String(): codersdk.ChatRoleRead,
		},
	})
	require.NoError(t, err)

	events, closer, err := viewerClient.StreamChat(ctx, chat.ID, nil)
	require.NoError(t, err)
	defer closer.Close()

	// Drain the snapshot so the stream is fully attached before we
	// trigger the revoke. The existing send loop only subscribes to
	// the ACL invalidation channel after Accept; draining at least
	// one event ensures the handler has progressed past Subscribe.
	select {
	case <-events:
	case <-ctx.Done():
		require.FailNow(t, "timed out waiting for initial snapshot event")
	}

	// Observe the invalidation publish to prove the handler fired
	// pubsub after the transaction committed.
	var invalidationCount atomic.Int32
	cancelSub, err := api.Pubsub.SubscribeWithErr(
		coderdpubsub.ChatACLInvalidationChannel(chat.ID),
		func(_ context.Context, _ []byte, _ error) {
			invalidationCount.Add(1)
		},
	)
	require.NoError(t, err)
	defer cancelSub()

	// Revoke the viewer's access.
	err = ownerClient.UpdateChatACL(ctx, chat.ID, codersdk.UpdateChatACL{
		UserRoles: map[string]codersdk.ChatRole{
			viewer.ID.String(): codersdk.ChatRoleDeleted,
		},
	})
	require.NoError(t, err)

	// The viewer's events channel must close once the server
	// re-authorizes and calls conn.Close(4403, ...). The SDK
	// StreamChat helper closes the channel when the websocket read
	// loop terminates, so we wait for that signal.
	testutil.Eventually(ctx, t, func(_ context.Context) bool {
		select {
		case _, ok := <-events:
			return !ok
		default:
			return false
		}
	}, testutil.IntervalFast, "viewer stream did not close after ACL revocation")

	require.GreaterOrEqual(t, invalidationCount.Load(), int32(1),
		"expected at least one publish on chat acl invalidation channel")
}

// TestChatWatch_SharedViewerReceivesDualPublishLifecycle covers plan id
// [watchlist-dualpub]: an owner shares a chat with a viewer, then
// mutates the chat (archive) to trigger a lifecycle publish. The
// viewer subscribed to /chats/watch receives the event because the
// publisher dual-publishes to chat:owner:<owner> AND
// chat:watchlist:<viewer>.
func TestChatWatch_SharedViewerReceivesDualPublishLifecycle(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	ownerClient := newChatClient(t)
	firstUser := coderdtest.CreateFirstUser(t, ownerClient.Client)
	_ = createChatModelConfig(t, ownerClient)

	viewerRaw, viewer := coderdtest.CreateAnotherUser(t, ownerClient.Client, firstUser.OrganizationID)
	viewerClient := codersdk.NewExperimentalClient(viewerRaw)

	chat := createSharedChat(ctx, t, ownerClient, firstUser.OrganizationID, "watchlist dualpub")

	err := ownerClient.UpdateChatACL(ctx, chat.ID, codersdk.UpdateChatACL{
		UserRoles: map[string]codersdk.ChatRole{
			viewer.ID.String(): codersdk.ChatRoleRead,
		},
	})
	require.NoError(t, err)

	// Open the watch connection as the viewer. The viewer does not
	// own the chat; they can only see lifecycle events via the
	// watchlist channel fed by the publisher's dual-publish path.
	conn, err := viewerClient.Dial(ctx, "/api/experimental/chats/watch", nil)
	require.NoError(t, err)
	defer conn.Close(websocket.StatusNormalClosure, "done")

	// Trigger a lifecycle publish by archiving the shared chat.
	// ArchiveChat fans out a Deleted-kind event through
	// publishChatPubsubEvent which calls publishChatWatchlist.
	err = ownerClient.UpdateChat(ctx, chat.ID, codersdk.UpdateChatRequest{
		Archived: ptr.Ref(true),
	})
	require.NoError(t, err)

	// Drain events until we see one for the shared chat. The viewer
	// does not own the chat; receiving any lifecycle event proves the
	// publisher dual-published to the watchlist channel. Archive can
	// emit several event kinds (status_change, deleted) depending on
	// prior chat state, so accept any kind that references the chat.
	for {
		var payload codersdk.ChatWatchEvent
		err = wsjson.Read(ctx, conn, &payload)
		require.NoError(t, err, "viewer should receive dual-published lifecycle event")
		if payload.Chat.ID == chat.ID {
			return
		}
	}
}

// TestChatWatch_GroupMemberReceivesLifecycle covers plan id
// [watchlist-group]: a group is added to a chat's ACL. A member of
// the group subscribed to /chats/watch receives lifecycle events via
// their watchlist channel, proving the publisher resolves group
// members through GetGroupMembersByGroupID.
func TestChatWatch_GroupMemberReceivesLifecycle(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	ownerClient, db := newChatClientWithDatabase(t)
	firstUser := coderdtest.CreateFirstUser(t, ownerClient.Client)
	_ = createChatModelConfig(t, ownerClient)

	viewerRaw, viewer := coderdtest.CreateAnotherUser(t, ownerClient.Client, firstUser.OrganizationID)
	viewerClient := codersdk.NewExperimentalClient(viewerRaw)

	group := dbgen.Group(t, db, database.Group{OrganizationID: firstUser.OrganizationID})
	dbgen.GroupMember(t, db, database.GroupMemberTable{GroupID: group.ID, UserID: viewer.ID})

	chat := createSharedChat(ctx, t, ownerClient, firstUser.OrganizationID, "watchlist group")

	err := ownerClient.UpdateChatACL(ctx, chat.ID, codersdk.UpdateChatACL{
		GroupRoles: map[string]codersdk.ChatRole{
			group.ID.String(): codersdk.ChatRoleRead,
		},
	})
	require.NoError(t, err)

	conn, err := viewerClient.Dial(ctx, "/api/experimental/chats/watch", nil)
	require.NoError(t, err)
	defer conn.Close(websocket.StatusNormalClosure, "done")

	err = ownerClient.UpdateChat(ctx, chat.ID, codersdk.UpdateChatRequest{
		Archived: ptr.Ref(true),
	})
	require.NoError(t, err)

	// Any lifecycle event for the chat arriving on the group
	// member's socket proves the publisher resolved group members
	// and dual-published. See the SharedViewer test for notes on
	// why we accept multiple kinds.
	for {
		var payload codersdk.ChatWatchEvent
		err = wsjson.Read(ctx, conn, &payload)
		require.NoError(t, err, "group member should receive dual-published lifecycle event")
		if payload.Chat.ID == chat.ID {
			return
		}
	}
}
