package slackd

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/slack-go/slack/slackevents"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/x/chatd"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

const testProviderID = "slack"

// appMention builds the AppMentionEvent handleMention receives.
func appMention(slackUserID, channel, ts, threadTS string) *slackevents.AppMentionEvent {
	return &slackevents.AppMentionEvent{
		Type:            "app_mention",
		User:            slackUserID,
		Text:            "<@UBOT> hello",
		TimeStamp:       ts,
		ThreadTimeStamp: threadTS,
		Channel:         channel,
	}
}

// seedThreadChat persists a slackd chat bound to threadKey for the
// given owner.
func seedThreadChat(t *testing.T, db database.Store, org database.Organization, ownerID uuid.UUID, threadKey string) database.Chat {
	t.Helper()
	return dbgen.Chat(t, db, database.Chat{
		OrganizationID:    org.ID,
		OwnerID:           ownerID,
		Title:             "Slack thread " + threadKey,
		LastModelConfigID: dbgen.ChatModelConfig(t, db, database.ChatModelConfig{Model: "chat-model"}).ID,
		Labels: database.StringMap{
			LabelSlackd:      "true",
			LabelSlackThread: threadKey,
		},
	})
}

// requireAPIKeyOwnedBy asserts the delegated API key exists and is
// owned by the expected user.
func requireAPIKeyOwnedBy(t *testing.T, db database.Store, apiKeyID string, ownerID uuid.UUID) {
	t.Helper()
	ctx := testutil.Context(t, testutil.WaitShort)
	key, err := db.GetAPIKeyByID(ctx, apiKeyID)
	require.NoError(t, err)
	require.Equal(t, ownerID, key.UserID)
}

func TestHandleMentionLinkedSenderOwnsChat(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)
	fallback, _ := seedOwner(t, db)

	// The linked user belongs to their own single organization,
	// distinct from the fallback owner's.
	linkedOrg := dbgen.Organization(t, db, database.Organization{})
	linked := seedMember(t, db, linkedOrg)
	linkSlackIdentity(t, db, testProviderID, linked.ID, "ULINKED")

	chat := newFakeChatSubmitter()
	chat.createChat = database.Chat{ID: uuid.New(), OwnerID: linked.ID}
	server, webAPI := newTestServerWithWebAPI(t, db, chat, fallback.ID, testProviderID, newFakeSocketClient())
	webAPI.setReplies(threadMsg("100.1", "ULINKED", "<@UBOT> hello"))

	require.NoError(t, server.handleMention(ctx, "Ev1", appMention("ULINKED", "C1", "100.1", "")))

	creates, sends := chat.snapshot()
	require.Len(t, creates, 1)
	require.Empty(t, sends)
	create := creates[0]
	assert.Equal(t, linked.ID, create.OwnerID)
	assert.Equal(t, linkedOrg.ID, create.OrganizationID)
	assert.False(t, create.DedupAcrossOwners)
	requireAPIKeyOwnedBy(t, db, create.APIKeyID, linked.ID)
	// Linked senders get the individual-mode suffix scoped to their
	// Slack identity, including propose_mcp_server guidance. The shared
	// label is omitted so chatd enables the propose tool.
	assert.NotContains(t, create.Labels, LabelSlackShared)
	assert.Contains(t, create.SystemPrompt, "ULINKED")
	assert.Contains(t, create.SystemPrompt, "propose_mcp_server")
	assert.Contains(t, create.SystemPrompt, "Always find a reliable source for its configuration")
	assert.Contains(t, create.SystemPrompt, `"streamable_http" over "sse"`)
	assert.NotContains(t, create.SystemPrompt, "you're running in shared mode")
}

func TestHandleMentionUnlinkedSenderUsesFallback(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)
	fallback, org := seedOwner(t, db)

	chat := newFakeChatSubmitter()
	chat.createChat = database.Chat{ID: uuid.New(), OwnerID: fallback.ID}
	server, webAPI := newTestServerWithWebAPI(t, db, chat, fallback.ID, testProviderID, newFakeSocketClient())
	webAPI.setReplies(threadMsg("100.1", "UNOLINK", "<@UBOT> hello"))

	require.NoError(t, server.handleMention(ctx, "Ev1", appMention("UNOLINK", "C1", "100.1", "")))

	creates, _ := chat.snapshot()
	require.Len(t, creates, 1)
	assert.Equal(t, fallback.ID, creates[0].OwnerID)
	assert.Equal(t, org.ID, creates[0].OrganizationID)
	requireAPIKeyOwnedBy(t, db, creates[0].APIKeyID, fallback.ID)
	assert.Equal(t, "true", creates[0].Labels[LabelSlackShared])
	assert.Contains(t, creates[0].SystemPrompt, "you're running in shared mode")
	assert.Contains(t, creates[0].SystemPrompt, "https://coder.example.com/settings/external-auth")
	assert.NotContains(t, creates[0].SystemPrompt, "propose_mcp_server")
}

func TestHandleMentionNoProviderIgnoresLinks(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)
	fallback, _ := seedOwner(t, db)

	linkedOrg := dbgen.Organization(t, db, database.Organization{})
	linked := seedMember(t, db, linkedOrg)
	linkSlackIdentity(t, db, testProviderID, linked.ID, "ULINKED")

	chat := newFakeChatSubmitter()
	chat.createChat = database.Chat{ID: uuid.New(), OwnerID: fallback.ID}
	// No provider configured: legacy fixed-owner behavior for every
	// sender, even linked ones.
	server, webAPI := newTestServerWithWebAPI(t, db, chat, fallback.ID, "", newFakeSocketClient())
	webAPI.setReplies(threadMsg("100.1", "ULINKED", "<@UBOT> hello"))

	require.NoError(t, server.handleMention(ctx, "Ev1", appMention("ULINKED", "C1", "100.1", "")))

	creates, _ := chat.snapshot()
	require.Len(t, creates, 1)
	assert.Equal(t, fallback.ID, creates[0].OwnerID)
}

func TestHandleMentionOtherProviderLinkIgnored(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)
	fallback, _ := seedOwner(t, db)

	linkedOrg := dbgen.Organization(t, db, database.Organization{})
	linked := seedMember(t, db, linkedOrg)
	// The identity exists, but under a different provider.
	linkSlackIdentity(t, db, "other-slack", linked.ID, "ULINKED")

	chat := newFakeChatSubmitter()
	chat.createChat = database.Chat{ID: uuid.New(), OwnerID: fallback.ID}
	server, webAPI := newTestServerWithWebAPI(t, db, chat, fallback.ID, testProviderID, newFakeSocketClient())
	webAPI.setReplies(threadMsg("100.1", "ULINKED", "<@UBOT> hello"))

	require.NoError(t, server.handleMention(ctx, "Ev1", appMention("ULINKED", "C1", "100.1", "")))

	creates, _ := chat.snapshot()
	require.Len(t, creates, 1)
	assert.Equal(t, fallback.ID, creates[0].OwnerID)
}

func TestHandleMentionUnusableLinkedUserFailsClosed(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		user func(t *testing.T, db database.Store, org database.Organization) uuid.UUID
	}{
		{"Suspended", func(t *testing.T, db database.Store, org database.Organization) uuid.UUID {
			u := dbgen.User(t, db, database.User{Status: database.UserStatusSuspended})
			dbgen.OrganizationMember(t, db, database.OrganizationMember{UserID: u.ID, OrganizationID: org.ID})
			return u.ID
		}},
		{"Dormant", func(t *testing.T, db database.Store, org database.Organization) uuid.UUID {
			u := dbgen.User(t, db, database.User{Status: database.UserStatusDormant})
			dbgen.OrganizationMember(t, db, database.OrganizationMember{UserID: u.ID, OrganizationID: org.ID})
			return u.ID
		}},
		{"Deleted", func(t *testing.T, db database.Store, _ database.Organization) uuid.UUID {
			u := dbgen.User(t, db, database.User{Deleted: true})
			return u.ID
		}},
		{"System", func(*testing.T, database.Store, database.Organization) uuid.UUID {
			return database.PrebuildsSystemUserID
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			db, _ := dbtestutil.NewDB(t)
			ctx := testutil.Context(t, testutil.WaitLong)
			fallback, org := seedOwner(t, db)
			linkSlackIdentity(t, db, testProviderID, tc.user(t, db, org), "UBAD")

			chat := newFakeChatSubmitter()
			server := newTestServerWithProvider(t, db, chat, fallback.ID, testProviderID, newFakeSocketClient())

			// An unusable linked user fails the event; it must not
			// fall back to the configured owner.
			err := server.handleMention(ctx, "Ev1", appMention("UBAD", "C1", "100.1", ""))
			require.Error(t, err)
			creates, sends := chat.snapshot()
			require.Empty(t, creates)
			require.Empty(t, sends)
		})
	}
}

func TestHandleMentionAmbiguousLinkFailsClosed(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)
	fallback, org := seedOwner(t, db)

	// Two Coder accounts link the same Slack identity; one of them is
	// suspended, but the mapping is still ambiguous.
	linkedA := seedMember(t, db, org)
	linkedB := dbgen.User(t, db, database.User{Status: database.UserStatusSuspended})
	linkSlackIdentity(t, db, testProviderID, linkedA.ID, "UDUP")
	linkSlackIdentity(t, db, testProviderID, linkedB.ID, "UDUP")

	chat := newFakeChatSubmitter()
	server := newTestServerWithProvider(t, db, chat, fallback.ID, testProviderID, newFakeSocketClient())

	err := server.handleMention(ctx, "Ev1", appMention("UDUP", "C1", "100.1", ""))
	require.Error(t, err)
	creates, sends := chat.snapshot()
	require.Empty(t, creates)
	require.Empty(t, sends)
}

func TestHandleMentionOtherSenderGetsOwnChat(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)
	fallback, org := seedOwner(t, db)

	// The thread already has ownerA's chat. A different linked Slack
	// user replies: they get their own chat for the thread, and
	// ownerA's chat is interrupted before anything is submitted.
	ownerA := seedMember(t, db, org)
	existing := seedThreadChat(t, db, org, ownerA.ID, "C1:100.1")

	linkedOrg := dbgen.Organization(t, db, database.Organization{})
	linkedB := seedMember(t, db, linkedOrg)
	linkSlackIdentity(t, db, testProviderID, linkedB.ID, "UOTHER")

	chat := newFakeChatSubmitter()
	chat.createChat = database.Chat{ID: uuid.New(), OwnerID: linkedB.ID}
	server, webAPI := newTestServerWithWebAPI(t, db, chat, fallback.ID, testProviderID, newFakeSocketClient())
	webAPI.setReplies(threadMsg("105.0", "UOTHER", "<@UBOT> hello"))

	require.NoError(t, server.handleMention(ctx, "Ev2", appMention("UOTHER", "C1", "105.0", "100.1")))

	creates, sends := chat.snapshot()
	require.Len(t, creates, 1)
	require.Empty(t, sends)
	assert.Equal(t, linkedB.ID, creates[0].OwnerID)
	assert.Equal(t, linkedOrg.ID, creates[0].OrganizationID)
	requireAPIKeyOwnedBy(t, db, creates[0].APIKeyID, linkedB.ID)

	interrupted, ops := chat.interrupts()
	require.Len(t, interrupted, 1)
	assert.Equal(t, existing.ID, interrupted[0].ID)
	assert.Equal(t, []string{"interrupt", "create"}, ops)
}

func TestHandleMentionOwnerReplyReusesChatAndInterruptsSiblings(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)
	fallback, org := seedOwner(t, db)

	// The thread has two chats: ownerA's and linkedB's. linkedB
	// replies: the message reuses linkedB's chat and ownerA's chat is
	// interrupted first. ownerA's chat receives no message.
	ownerA := seedMember(t, db, org)
	sibling := seedThreadChat(t, db, org, ownerA.ID, "C1:100.1")

	linkedB := seedMember(t, db, org)
	linkSlackIdentity(t, db, testProviderID, linkedB.ID, "UOTHER")
	own := seedThreadChat(t, db, org, linkedB.ID, "C1:100.1")

	chat := newFakeChatSubmitter()
	server, webAPI := newTestServerWithWebAPI(t, db, chat, fallback.ID, testProviderID, newFakeSocketClient())
	webAPI.setReplies(threadMsg("105.0", "UOTHER", "<@UBOT> hello"))

	require.NoError(t, server.handleMention(ctx, "Ev2", appMention("UOTHER", "C1", "105.0", "100.1")))

	creates, sends := chat.snapshot()
	require.Empty(t, creates)
	require.Len(t, sends, 1)
	assert.Equal(t, own.ID, sends[0].ChatID)
	assert.Equal(t, linkedB.ID, sends[0].CreatedBy)
	requireAPIKeyOwnedBy(t, db, sends[0].APIKeyID, linkedB.ID)

	interrupted, ops := chat.interrupts()
	require.Len(t, interrupted, 1)
	assert.Equal(t, sibling.ID, interrupted[0].ID)
	assert.Equal(t, []string{"interrupt", "send"}, ops)
}

func TestHandleMentionUnlinkedSenderGetsFallbackChat(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)
	fallback, org := seedOwner(t, db)

	// The thread has a linked owner's chat. An unlinked sender routes
	// to a fallback-owned chat created on demand, interrupting the
	// linked owner's chat.
	ownerA := seedMember(t, db, org)
	existing := seedThreadChat(t, db, org, ownerA.ID, "C1:100.1")

	chat := newFakeChatSubmitter()
	chat.createChat = database.Chat{ID: uuid.New(), OwnerID: fallback.ID}
	server, webAPI := newTestServerWithWebAPI(t, db, chat, fallback.ID, testProviderID, newFakeSocketClient())
	webAPI.setReplies(threadMsg("105.0", "UNOLINK", "<@UBOT> hello"))

	require.NoError(t, server.handleMention(ctx, "Ev2", appMention("UNOLINK", "C1", "105.0", "100.1")))

	creates, sends := chat.snapshot()
	require.Len(t, creates, 1)
	require.Empty(t, sends)
	assert.Equal(t, fallback.ID, creates[0].OwnerID)

	interrupted, ops := chat.interrupts()
	require.Len(t, interrupted, 1)
	assert.Equal(t, existing.ID, interrupted[0].ID)
	assert.Equal(t, []string{"interrupt", "create"}, ops)
}

func TestHandleMentionUnlinkedSendersShareFallbackChat(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)
	fallback, org := seedOwner(t, db)

	// The fallback-owned chat for the thread already exists. Two
	// different unlinked senders both route to it.
	shared := seedThreadChat(t, db, org, fallback.ID, "C1:100.1")

	chat := newFakeChatSubmitter()
	server, webAPI := newTestServerWithWebAPI(t, db, chat, fallback.ID, testProviderID, newFakeSocketClient())

	webAPI.setReplies(threadMsg("105.0", "UNOLINK1", "<@UBOT> hello"))
	require.NoError(t, server.handleMention(ctx, "Ev1", appMention("UNOLINK1", "C1", "105.0", "100.1")))
	webAPI.setReplies(threadMsg("106.0", "UNOLINK2", "<@UBOT> hello"))
	require.NoError(t, server.handleMention(ctx, "Ev2", appMention("UNOLINK2", "C1", "106.0", "100.1")))

	creates, sends := chat.snapshot()
	require.Empty(t, creates)
	require.Len(t, sends, 2)
	assert.Equal(t, shared.ID, sends[0].ChatID)
	assert.Equal(t, shared.ID, sends[1].ChatID)
	assert.Equal(t, fallback.ID, sends[0].CreatedBy)
	assert.Equal(t, fallback.ID, sends[1].CreatedBy)

	interrupted, _ := chat.interrupts()
	require.Empty(t, interrupted)
}

func TestHandleMentionDuplicateEventDoesNotInterruptSiblings(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)
	fallback, org := seedOwner(t, db)

	// The fallback-owned chat already processed event Ev1 and a
	// sibling chat exists on the thread. A redelivery of Ev1 must be
	// dropped before sibling interruption: otherwise the replay would
	// interrupt the sibling's active run and then have nothing to
	// submit.
	ownerA := seedMember(t, db, org)
	seedThreadChat(t, db, org, ownerA.ID, "C1:100.1")
	own := seedThreadChat(t, db, org, fallback.ID, "C1:100.1")

	content, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{{
		Type:     codersdk.ChatMessagePartTypeText,
		Text:     "original",
		Metadata: map[string]string{MetadataKeySlackEventID: "Ev1"},
	}})
	require.NoError(t, err)
	dbgen.ChatMessage(t, db, database.ChatMessage{
		ChatID:    own.ID,
		CreatedBy: uuid.NullUUID{UUID: fallback.ID, Valid: true},
		Content:   content,
	})

	chat := newFakeChatSubmitter()
	server := newTestServerWithProvider(t, db, chat, fallback.ID, testProviderID, newFakeSocketClient())

	require.NoError(t, server.handleMention(ctx, "Ev1", appMention("UNOLINK", "C1", "105.0", "100.1")))

	creates, sends := chat.snapshot()
	require.Empty(t, creates)
	require.Empty(t, sends)
	interrupted, _ := chat.interrupts()
	require.Empty(t, interrupted)
}

func TestHandleMentionInterruptErrorsDoNotBlockSubmission(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		err  error
	}{
		{"TransitionNotAllowed", xerrors.Errorf("interrupt: %w", chatstate.ErrTransitionNotAllowed)},
		{"ChatNotFound", xerrors.Errorf("interrupt: %w", chatstate.ErrChatNotFound)},
		{"Unexpected", xerrors.New("boom")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			db, _ := dbtestutil.NewDB(t)
			ctx := testutil.Context(t, testutil.WaitLong)
			fallback, org := seedOwner(t, db)

			ownerA := seedMember(t, db, org)
			seedThreadChat(t, db, org, ownerA.ID, "C1:100.1")
			own := seedThreadChat(t, db, org, fallback.ID, "C1:100.1")

			chat := newFakeChatSubmitter()
			chat.interruptErr = tc.err
			server, webAPI := newTestServerWithWebAPI(t, db, chat, fallback.ID, testProviderID, newFakeSocketClient())
			webAPI.setReplies(threadMsg("105.0", "UNOLINK", "<@UBOT> hello"))

			require.NoError(t, server.handleMention(ctx, "Ev2", appMention("UNOLINK", "C1", "105.0", "100.1")))

			_, sends := chat.snapshot()
			require.Len(t, sends, 1)
			assert.Equal(t, own.ID, sends[0].ChatID)
			interrupted, _ := chat.interrupts()
			require.Len(t, interrupted, 1)
		})
	}
}

func TestHandleMentionCreateRaceResolvesToSameOwnerChat(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)
	fallback, _ := seedOwner(t, db)

	// Another replica already created this owner's chat for the
	// thread. Dedup is per owner, so the winning chat has the same
	// owner and the message is sent to it.
	linkedOrg := dbgen.Organization(t, db, database.Organization{})
	linked := seedMember(t, db, linkedOrg)
	linkSlackIdentity(t, db, testProviderID, linked.ID, "ULINKED")

	chat := newFakeChatSubmitter()
	chat.createChat = database.Chat{ID: uuid.New(), OwnerID: linked.ID}
	chat.createErr = chatd.ErrChatAlreadyExists
	server, webAPI := newTestServerWithWebAPI(t, db, chat, fallback.ID, testProviderID, newFakeSocketClient())
	webAPI.setReplies(threadMsg("200.1", "ULINKED", "<@UBOT> hello"))

	require.NoError(t, server.handleMention(ctx, "Ev3", appMention("ULINKED", "C2", "200.1", "")))

	creates, sends := chat.snapshot()
	require.Len(t, creates, 1)
	assert.Equal(t, linked.ID, creates[0].OwnerID)
	require.Len(t, sends, 1)
	assert.Equal(t, chat.createChat.ID, sends[0].ChatID)
	assert.Equal(t, linked.ID, sends[0].CreatedBy)
	requireAPIKeyOwnedBy(t, db, sends[0].APIKeyID, linked.ID)
}

func TestHandleMentionAmbiguousLinkFailsClosedOnExistingThread(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)
	fallback, org := seedOwner(t, db)

	// The thread already has a chat, but the sender's link is
	// ambiguous: the event fails closed without touching any chat.
	seedThreadChat(t, db, org, fallback.ID, "C1:100.1")

	linkedA := seedMember(t, db, org)
	linkedB := seedMember(t, db, org)
	linkSlackIdentity(t, db, testProviderID, linkedA.ID, "UDUP")
	linkSlackIdentity(t, db, testProviderID, linkedB.ID, "UDUP")

	chat := newFakeChatSubmitter()
	server := newTestServerWithProvider(t, db, chat, fallback.ID, testProviderID, newFakeSocketClient())

	err := server.handleMention(ctx, "Ev2", appMention("UDUP", "C1", "105.0", "100.1"))
	require.Error(t, err)
	creates, sends := chat.snapshot()
	require.Empty(t, creates)
	require.Empty(t, sends)
	interrupted, _ := chat.interrupts()
	require.Empty(t, interrupted)
}

func TestHandleMentionObservesOrganizationChanges(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)
	fallback, org1 := seedOwner(t, db)

	chat := newFakeChatSubmitter()
	chat.createChat = database.Chat{ID: uuid.New(), OwnerID: fallback.ID}
	server, webAPI := newTestServerWithWebAPI(t, db, chat, fallback.ID, "", newFakeSocketClient())

	webAPI.setReplies(threadMsg("100.1", "USENDER", "<@UBOT> hello"))
	require.NoError(t, server.handleMention(ctx, "Ev1", appMention("USENDER", "C1", "100.1", "")))

	// Move the owner to a new organization between messages. The
	// next new thread must observe the change without a restart
	// because slackd does not cache the organization.
	org2 := dbgen.Organization(t, db, database.Organization{})
	dbgen.OrganizationMember(t, db, database.OrganizationMember{UserID: fallback.ID, OrganizationID: org2.ID})
	err := db.DeleteOrganizationMember(ctx, database.DeleteOrganizationMemberParams{
		OrganizationID: org1.ID,
		UserID:         fallback.ID,
	})
	require.NoError(t, err)

	webAPI.setReplies(threadMsg("200.1", "USENDER", "<@UBOT> hello"))
	require.NoError(t, server.handleMention(ctx, "Ev2", appMention("USENDER", "C2", "200.1", "")))

	creates, _ := chat.snapshot()
	require.Len(t, creates, 2)
	assert.Equal(t, org1.ID, creates[0].OrganizationID)
	assert.Equal(t, org2.ID, creates[1].OrganizationID)
}

func TestEnsureAPIKeyID(t *testing.T) {
	t.Parallel()

	newServer := func(t *testing.T, db database.Store) *Server {
		return newTestServer(t, db, newFakeChatSubmitter(), uuid.New(), newFakeSocketClient())
	}

	t.Run("ReusesExistingSlackdKey", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitLong)
		owner := dbgen.User(t, db, database.User{})
		existing, _ := dbgen.APIKey(t, db, database.APIKey{
			UserID:    owner.ID,
			LoginType: database.LoginTypeToken,
			TokenName: "slackd-existing",
			ExpiresAt: time.Now().Add(10 * 24 * time.Hour),
		})
		// Unrelated user tokens are never reused.
		dbgen.APIKey(t, db, database.APIKey{
			UserID:    owner.ID,
			LoginType: database.LoginTypeToken,
			TokenName: "my-personal-token",
			ExpiresAt: time.Now().Add(20 * 24 * time.Hour),
		})

		server := newServer(t, db)
		keyID, err := server.ensureAPIKeyID(ctx, owner.ID)
		require.NoError(t, err)
		require.Equal(t, existing.ID, keyID)
	})

	t.Run("NearExpiryKeyIsReplaced", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitLong)
		owner := dbgen.User(t, db, database.User{})
		nearExpiry, _ := dbgen.APIKey(t, db, database.APIKey{
			UserID:    owner.ID,
			LoginType: database.LoginTypeToken,
			TokenName: "slackd-nearexpiry",
			ExpiresAt: time.Now().Add(time.Hour),
		})

		server := newServer(t, db)
		keyID, err := server.ensureAPIKeyID(ctx, owner.ID)
		require.NoError(t, err)
		require.NotEqual(t, nearExpiry.ID, keyID)
		requireAPIKeyOwnedBy(t, db, keyID, owner.ID)
	})

	t.Run("DeterministicSelection", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitLong)
		owner := dbgen.User(t, db, database.User{})
		dbgen.APIKey(t, db, database.APIKey{
			UserID:    owner.ID,
			LoginType: database.LoginTypeToken,
			TokenName: "slackd-older",
			ExpiresAt: time.Now().Add(5 * 24 * time.Hour),
		})
		latest, _ := dbgen.APIKey(t, db, database.APIKey{
			UserID:    owner.ID,
			LoginType: database.LoginTypeToken,
			TokenName: "slackd-latest",
			ExpiresAt: time.Now().Add(15 * 24 * time.Hour),
		})

		server := newServer(t, db)
		// The latest-expiring key is always selected.
		for range 3 {
			keyID, err := server.ensureAPIKeyID(ctx, owner.ID)
			require.NoError(t, err)
			require.Equal(t, latest.ID, keyID)
		}
	})

	t.Run("MintsWhenNoneReusable", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitLong)
		owner := dbgen.User(t, db, database.User{})

		server := newServer(t, db)
		keyID, err := server.ensureAPIKeyID(ctx, owner.ID)
		require.NoError(t, err)
		requireAPIKeyOwnedBy(t, db, keyID, owner.ID)

		// The minted key is discovered from the database on the next
		// call rather than being cached in process memory.
		again, err := server.ensureAPIKeyID(ctx, owner.ID)
		require.NoError(t, err)
		require.Equal(t, keyID, again)
	})
}
