package slackd

import (
	"context"
	"encoding/json"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"
	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/x/chatd"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m, testutil.GoleakOptions...)
}

// fakeSocketClient scripts RunContext results and delivers events.
type fakeSocketClient struct {
	events chan socketmode.Event
	// runErr is returned from RunContext after runRelease is closed
	// (or immediately when nil).
	runCalls   atomic.Int64
	runResults chan error
	acked      chan socketmode.Request
}

func newFakeSocketClient() *fakeSocketClient {
	return &fakeSocketClient{
		events:     make(chan socketmode.Event, 16),
		runResults: make(chan error, 16),
		acked:      make(chan socketmode.Request, 16),
	}
}

func (f *fakeSocketClient) RunContext(ctx context.Context) error {
	f.runCalls.Add(1)
	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-f.runResults:
		return err
	}
}

func (f *fakeSocketClient) EventsChannel() <-chan socketmode.Event {
	return f.events
}

func (f *fakeSocketClient) Ack(req socketmode.Request, _ ...any) error {
	f.acked <- req
	return nil
}

// fakeWebAPI serves canned Slack identities and scripted thread
// replies.
type fakeWebAPI struct {
	botUID string
	users  map[string]*slack.User

	mu sync.Mutex
	// replies is returned from GetConversationRepliesContext, split
	// into pages of repliesPageSize (all at once when zero).
	replies         []slack.Message
	repliesPageSize int
	repliesErr      error
	repliesCalls    []slack.GetConversationRepliesParameters

	// updateCalls records UpdateMessageContext invocations;
	// ephemeralCalls records PostEphemeralContext invocations.
	updateCalls    []fakeMessageUpdate
	ephemeralCalls []fakeEphemeralPost
}

// fakeMessageUpdate captures one UpdateMessageContext call.
type fakeMessageUpdate struct {
	Channel string
	TS      string
	Options []slack.MsgOption
}

// fakeEphemeralPost captures one PostEphemeralContext call.
type fakeEphemeralPost struct {
	Channel string
	User    string
	Options []slack.MsgOption
}

func (f *fakeWebAPI) UpdateMessageContext(_ context.Context, channelID, timestamp string, options ...slack.MsgOption) (respChannel, respTS, respText string, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.updateCalls = append(f.updateCalls, fakeMessageUpdate{Channel: channelID, TS: timestamp, Options: options})
	return channelID, timestamp, "", nil
}

func (f *fakeWebAPI) PostEphemeralContext(_ context.Context, channelID, userID string, options ...slack.MsgOption) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ephemeralCalls = append(f.ephemeralCalls, fakeEphemeralPost{Channel: channelID, User: userID, Options: options})
	return "", nil
}

// updates returns a snapshot of the recorded message updates.
func (f *fakeWebAPI) updates() []fakeMessageUpdate {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]fakeMessageUpdate(nil), f.updateCalls...)
}

// ephemerals returns a snapshot of the recorded ephemeral posts.
func (f *fakeWebAPI) ephemerals() []fakeEphemeralPost {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]fakeEphemeralPost(nil), f.ephemeralCalls...)
}

func (f *fakeWebAPI) AuthTestContext(context.Context) (*slack.AuthTestResponse, error) {
	return &slack.AuthTestResponse{UserID: f.botUID}, nil
}

func (f *fakeWebAPI) GetUserInfoContext(_ context.Context, user string) (*slack.User, error) {
	if u, ok := f.users[user]; ok {
		return u, nil
	}
	return nil, xerrors.New("user not found")
}

func (f *fakeWebAPI) GetConversationRepliesContext(_ context.Context, params *slack.GetConversationRepliesParameters) ([]slack.Message, bool, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.repliesCalls = append(f.repliesCalls, *params)
	if f.repliesErr != nil {
		return nil, false, "", f.repliesErr
	}
	start := 0
	if params.Cursor != "" {
		var err error
		start, err = strconv.Atoi(params.Cursor)
		if err != nil {
			return nil, false, "", err
		}
	}
	end := len(f.replies)
	if f.repliesPageSize > 0 && start+f.repliesPageSize < end {
		end = start + f.repliesPageSize
	}
	page := f.replies[start:end]
	if end < len(f.replies) {
		return page, true, strconv.Itoa(end), nil
	}
	return page, false, "", nil
}

// setReplies replaces the scripted thread replies.
func (f *fakeWebAPI) setReplies(msgs ...slack.Message) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.replies = msgs
}

// threadMsg builds a scripted Slack thread message.
func threadMsg(ts, user, text string) slack.Message {
	return slack.Message{Msg: slack.Msg{Timestamp: ts, User: user, Text: text}}
}

// fakeChatSubmitter records chatd calls and returns scripted results.
type fakeChatSubmitter struct {
	mu             sync.Mutex
	createCalls    []chatd.CreateOptions
	sendCalls      []chatd.SendMessageOptions
	interruptCalls []database.Chat
	// ops records the order of chatd calls ("interrupt", "create",
	// "send") so tests can assert siblings are interrupted before
	// message submission.
	ops []string

	createChat   database.Chat
	createErr    error
	sendErr      error
	interruptErr error

	called chan struct{}
}

func newFakeChatSubmitter() *fakeChatSubmitter {
	return &fakeChatSubmitter{called: make(chan struct{}, 16)}
}

func (f *fakeChatSubmitter) CreateChat(_ context.Context, opts chatd.CreateOptions) (database.Chat, error) {
	f.mu.Lock()
	f.createCalls = append(f.createCalls, opts)
	f.ops = append(f.ops, "create")
	f.mu.Unlock()
	f.called <- struct{}{}
	return f.createChat, f.createErr
}

func (f *fakeChatSubmitter) SendMessage(_ context.Context, opts chatd.SendMessageOptions) (chatd.SendMessageResult, error) {
	f.mu.Lock()
	f.sendCalls = append(f.sendCalls, opts)
	f.ops = append(f.ops, "send")
	f.mu.Unlock()
	f.called <- struct{}{}
	return chatd.SendMessageResult{}, f.sendErr
}

func (f *fakeChatSubmitter) InterruptChat(_ context.Context, chat database.Chat) (database.Chat, error) {
	f.mu.Lock()
	f.interruptCalls = append(f.interruptCalls, chat)
	f.ops = append(f.ops, "interrupt")
	f.mu.Unlock()
	return chat, f.interruptErr
}

func (f *fakeChatSubmitter) snapshot() ([]chatd.CreateOptions, []chatd.SendMessageOptions) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]chatd.CreateOptions(nil), f.createCalls...),
		append([]chatd.SendMessageOptions(nil), f.sendCalls...)
}

// interrupts returns the interrupted chats and the ordered op log.
func (f *fakeChatSubmitter) interrupts() ([]database.Chat, []string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]database.Chat(nil), f.interruptCalls...),
		append([]string(nil), f.ops...)
}

func newTestServer(t *testing.T, db database.Store, chat ChatSubmitter, owner uuid.UUID, socket *fakeSocketClient) *Server {
	t.Helper()
	return newTestServerWithProvider(t, db, chat, owner, "", socket)
}

func newTestServerWithProvider(t *testing.T, db database.Store, chat ChatSubmitter, owner uuid.UUID, providerID string, socket *fakeSocketClient) *Server {
	t.Helper()
	server, _ := newTestServerWithWebAPI(t, db, chat, owner, providerID, socket)
	return server
}

func newTestServerWithWebAPI(t *testing.T, db database.Store, chat ChatSubmitter, owner uuid.UUID, providerID string, socket *fakeSocketClient) (*Server, *fakeWebAPI) {
	t.Helper()
	webAPI := &fakeWebAPI{
		botUID: "UBOT",
		users: map[string]*slack.User{
			"USENDER": {Name: "sender", RealName: "Sender Name"},
			"UBOT":    {Name: "bot", RealName: "Bot App"},
		},
	}
	server, err := New(Options{
		Logger:                 slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug),
		Database:               db,
		Chat:                   chat,
		ChatOwnerUserID:        owner,
		ExternalAuthProviderID: providerID,
		AccessURL:              &url.URL{Scheme: "https", Host: "coder.example.com"},
		SocketClient:           socket,
		WebAPI:                 webAPI,
	})
	require.NoError(t, err)
	t.Cleanup(server.Close)
	return server, webAPI
}

func mentionEvent(eventID, channel, ts, threadTS, text string) socketmode.Event {
	return mentionEventFrom(eventID, "USENDER", channel, ts, threadTS, text)
}

func mentionEventFrom(eventID, slackUserID, channel, ts, threadTS, text string) socketmode.Event {
	return socketmode.Event{
		Type:    socketmode.EventTypeEventsAPI,
		Request: &socketmode.Request{EnvelopeID: eventID},
		Data: slackevents.EventsAPIEvent{
			Type: slackevents.CallbackEvent,
			Data: &slackevents.EventsAPICallbackEvent{EventID: eventID},
			InnerEvent: slackevents.EventsAPIInnerEvent{
				Data: &slackevents.AppMentionEvent{
					Type:            "app_mention",
					User:            slackUserID,
					Text:            text,
					TimeStamp:       ts,
					ThreadTimeStamp: threadTS,
					Channel:         channel,
				},
			},
		},
	}
}

func directMessageEvent(eventID, slackUserID, channel, ts, threadTS, text string) socketmode.Event {
	return socketmode.Event{
		Type:    socketmode.EventTypeEventsAPI,
		Request: &socketmode.Request{EnvelopeID: eventID},
		Data: slackevents.EventsAPIEvent{
			Type: slackevents.CallbackEvent,
			Data: &slackevents.EventsAPICallbackEvent{EventID: eventID},
			InnerEvent: slackevents.EventsAPIInnerEvent{
				Data: &slackevents.MessageEvent{
					Type:            "message",
					User:            slackUserID,
					Text:            text,
					TimeStamp:       ts,
					ThreadTimeStamp: threadTS,
					Channel:         channel,
					ChannelType:     "im",
				},
			},
		},
	}
}

func seedOwner(t *testing.T, db database.Store) (database.User, database.Organization) {
	t.Helper()
	user := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})
	dbgen.OrganizationMember(t, db, database.OrganizationMember{UserID: user.ID, OrganizationID: org.ID})
	dbgen.ChatModelConfig(t, db, database.ChatModelConfig{Model: "test-model", IsDefault: true})
	return user, org
}

// seedMember creates an active user that belongs to org.
func seedMember(t *testing.T, db database.Store, org database.Organization) database.User {
	t.Helper()
	user := dbgen.User(t, db, database.User{})
	dbgen.OrganizationMember(t, db, database.OrganizationMember{UserID: user.ID, OrganizationID: org.ID})
	return user
}

// linkSlackIdentity links a Coder user to a Slack user id under the
// given external auth provider, mirroring the oauth_extra payload the
// Slack provider stores.
func linkSlackIdentity(t *testing.T, db database.Store, providerID string, userID uuid.UUID, slackUserID string) {
	t.Helper()
	extra, err := json.Marshal(map[string]any{
		"authed_user": map[string]any{"id": slackUserID},
	})
	require.NoError(t, err)
	dbgen.ExternalAuthLink(t, db, database.ExternalAuthLink{
		ProviderID: providerID,
		UserID:     userID,
		OAuthExtra: pqtype.NullRawMessage{RawMessage: extra, Valid: true},
	})
}

func TestNewSlackUserResolverUsesSlackdAuthorization(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	owner, _ := seedOwner(t, db)
	const (
		providerID  = "slack-test"
		slackUserID = "USENDER"
	)
	linkSlackIdentity(t, db, providerID, owner.ID, slackUserID)

	authorizedDB := dbauthz.New(
		db,
		rbac.NewStrictCachingAuthorizer(prometheus.NewRegistry()),
		slogtest.Make(t, nil),
		nil,
	)
	resolver := NewSlackUserResolver(
		slogtest.Make(t, nil),
		authorizedDB,
		providerID,
		uuid.New(),
	)

	resolvedID, err := resolver(dbauthz.AsChatd(t.Context()), slackUserID)
	require.NoError(t, err)
	require.Equal(t, owner.ID, resolvedID)
}

func TestHandleMentionCreatesChatForNewThread(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)
	owner, org := seedOwner(t, db)

	chat := newFakeChatSubmitter()
	chat.createChat = database.Chat{ID: uuid.New()}
	socket := newFakeSocketClient()
	server, webAPI := newTestServerWithWebAPI(t, db, chat, owner.ID, "", socket)
	webAPI.setReplies(threadMsg("100.1", "USENDER", "<@UBOT> hello <@USENDER>"))
	server.Start(ctx)

	socket.events <- mentionEvent("Ev1", "C1", "100.1", "", "<@UBOT> hello <@USENDER>")

	_ = testutil.TryReceive(ctx, t, socket.acked)
	_ = testutil.TryReceive(ctx, t, chat.called)

	creates, sends := chat.snapshot()
	require.Len(t, creates, 1)
	require.Empty(t, sends)
	create := creates[0]
	assert.Equal(t, owner.ID, create.OwnerID)
	assert.Equal(t, org.ID, create.OrganizationID)
	assert.NotEmpty(t, create.APIKeyID)
	assert.NotEqual(t, uuid.Nil, create.ModelConfigID)
	assert.Equal(t, "true", create.Labels[LabelSlackd])
	assert.Equal(t, "C1:100.1", create.Labels[LabelSlackThread])
	assert.Equal(t, "true", create.Labels[LabelSlackShared])
	// Unlinked senders (no external auth provider configured) get the
	// shared-mode suffix, plus the bot's own Slack identity so the
	// model can recognize inline @bot mentions as referring to itself.
	assert.Contains(t, create.SystemPrompt, "You appear in Slack as @bot (user id <@UBOT>)")
	assert.Contains(t, create.SystemPrompt, "you are in shared mode")
	assert.Contains(t, create.SystemPrompt, "https://coder.example.com/settings/external-auth")
	assert.Contains(t, create.SystemPrompt, "propose_mcp_server")
	assert.Equal(t, map[string]string{
		LabelSlackd:      "true",
		LabelSlackThread: "C1:100.1",
	}, create.DedupLabels)
	assert.False(t, create.DedupAcrossOwners)
	require.Len(t, create.InitialUserContent, 2)
	header := create.InitialUserContent[0]
	assert.Equal(t, "Ev1", header.Metadata[MetadataKeySlackEventID])
	assert.Contains(t, header.Text, "Channel ID: C1")
	assert.Contains(t, header.Text, "N/A (new thread)")
	block := create.InitialUserContent[1]
	assert.Equal(t, "100.1", block.Metadata[MetadataKeySlackMessageTS])
	assert.True(t, strings.HasPrefix(block.Text, "<slack-message>\n"))
	assert.True(t, strings.HasSuffix(block.Text, "</slack-message>\n"))
	assert.Contains(t, block.Text, "<timestamp-raw>100.1</timestamp-raw>\n")
	assert.Contains(t, block.Text, "<timestamp>1970-01-01T00:01:40.1Z</timestamp>\n")
	assert.Contains(t, block.Text, "<from-user>sender (<@USENDER>) (Sender Name)</from-user>\n")
	// Mentions are rendered inline the way Slack displays them.
	assert.Contains(t, block.Text, "<content>\n@bot hello @sender\n</content>\n")
	assert.NotContains(t, block.Text, "<@UBOT>")
}

func TestHandleDirectMessageCreatesChatForNewThread(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)
	owner, org := seedOwner(t, db)

	chat := newFakeChatSubmitter()
	chat.createChat = database.Chat{ID: uuid.New()}
	socket := newFakeSocketClient()
	server, webAPI := newTestServerWithWebAPI(t, db, chat, owner.ID, "", socket)
	webAPI.setReplies(threadMsg("100.1", "USENDER", "hello without a mention"))

	server.handleEvent(ctx, directMessageEvent("EvDM1", "USENDER", "D1", "100.1", "", "hello without a mention"))
	_ = testutil.TryReceive(ctx, t, socket.acked)
	_ = testutil.TryReceive(ctx, t, chat.called)

	creates, sends := chat.snapshot()
	require.Len(t, creates, 1)
	require.Empty(t, sends)
	create := creates[0]
	assert.Equal(t, owner.ID, create.OwnerID)
	assert.Equal(t, org.ID, create.OrganizationID)
	requireAPIKeyOwnedBy(t, db, create.APIKeyID, owner.ID)
	assert.Equal(t, "D1:100.1", create.Labels[LabelSlackThread])
	assert.Equal(t, "true", create.Labels[LabelSlackShared])
	assert.Equal(t, map[string]string{
		LabelSlackd:      "true",
		LabelSlackThread: "D1:100.1",
	}, create.DedupLabels)
	require.Len(t, create.InitialUserContent, 2)
	assert.Equal(t, "EvDM1", create.InitialUserContent[0].Metadata[MetadataKeySlackEventID])
	assert.Contains(t, create.InitialUserContent[0].Text, "N/A (new thread)")
	assert.Contains(t, create.InitialUserContent[1].Text, "hello without a mention")
}

func TestHandleDirectMessageSendsToExistingChat(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)
	owner, org := seedOwner(t, db)
	existing := seedThreadChat(t, db, org, owner.ID, "D1:100.1")

	chat := newFakeChatSubmitter()
	socket := newFakeSocketClient()
	server, webAPI := newTestServerWithWebAPI(t, db, chat, owner.ID, "", socket)
	webAPI.setReplies(
		threadMsg("100.1", "USENDER", "thread start"),
		threadMsg("105.0", "USENDER", "follow-up"),
	)

	server.handleEvent(ctx, directMessageEvent("EvDM2", "USENDER", "D1", "105.0", "100.1", "follow-up"))
	_ = testutil.TryReceive(ctx, t, socket.acked)
	_ = testutil.TryReceive(ctx, t, chat.called)

	creates, sends := chat.snapshot()
	require.Empty(t, creates)
	require.Len(t, sends, 1)
	assert.Equal(t, existing.ID, sends[0].ChatID)
	assert.Equal(t, MetadataKeySlackEventID, sends[0].DedupMetadataKey)
	assert.Equal(t, []string{"100.1", "105.0"}, blockTimestamps(sends[0].Content))
}

func TestHandleDirectMessageLinkedSenderOwnsChatAndInterruptsSibling(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)
	fallback, fallbackOrg := seedOwner(t, db)
	linkedOrg := dbgen.Organization(t, db, database.Organization{})
	linked := seedMember(t, db, linkedOrg)
	linkSlackIdentity(t, db, testProviderID, linked.ID, "ULINKED")
	sibling := seedThreadChat(t, db, fallbackOrg, fallback.ID, "D1:100.1")

	chat := newFakeChatSubmitter()
	chat.createChat = database.Chat{ID: uuid.New(), OwnerID: linked.ID}
	socket := newFakeSocketClient()
	server, webAPI := newTestServerWithWebAPI(t, db, chat, fallback.ID, testProviderID, socket)
	webAPI.setReplies(threadMsg("100.1", "ULINKED", "hello"))

	server.handleEvent(ctx, directMessageEvent("EvDMOwner", "ULINKED", "D1", "100.1", "", "hello"))
	_ = testutil.TryReceive(ctx, t, socket.acked)
	_ = testutil.TryReceive(ctx, t, chat.called)

	creates, sends := chat.snapshot()
	require.Len(t, creates, 1)
	require.Empty(t, sends)
	assert.Equal(t, linked.ID, creates[0].OwnerID)
	assert.Equal(t, linkedOrg.ID, creates[0].OrganizationID)
	assert.NotContains(t, creates[0].Labels, LabelSlackShared)
	requireAPIKeyOwnedBy(t, db, creates[0].APIKeyID, linked.ID)
	interrupted, ops := chat.interrupts()
	require.Len(t, interrupted, 1)
	assert.Equal(t, sibling.ID, interrupted[0].ID)
	assert.Equal(t, []string{"interrupt", "create"}, ops)
}

func TestHandleDirectMessageDuplicateEventDoesNotInterruptSibling(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)
	fallback, org := seedOwner(t, db)
	siblingOwner := seedMember(t, db, org)
	seedThreadChat(t, db, org, siblingOwner.ID, "D1:100.1")
	own := seedThreadChat(t, db, org, fallback.ID, "D1:100.1")
	content, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{{
		Type:     codersdk.ChatMessagePartTypeText,
		Text:     "original",
		Metadata: map[string]string{MetadataKeySlackEventID: "EvDMDuplicate"},
	}})
	require.NoError(t, err)
	dbgen.ChatMessage(t, db, database.ChatMessage{
		ChatID:    own.ID,
		CreatedBy: uuid.NullUUID{UUID: fallback.ID, Valid: true},
		Content:   content,
	})

	chat := newFakeChatSubmitter()
	socket := newFakeSocketClient()
	server := newTestServer(t, db, chat, fallback.ID, socket)

	server.handleEvent(ctx, directMessageEvent("EvDMDuplicate", "USENDER", "D1", "105.0", "100.1", "duplicate"))
	_ = testutil.TryReceive(ctx, t, socket.acked)
	server.wg.Wait()

	creates, sends := chat.snapshot()
	require.Empty(t, creates)
	require.Empty(t, sends)
	interrupted, _ := chat.interrupts()
	require.Empty(t, interrupted)
}

func TestHandleEventIgnoresUnsupportedDirectMessages(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(*slackevents.MessageEvent)
	}{
		{
			name: "BotAuthored",
			mutate: func(ev *slackevents.MessageEvent) {
				ev.BotID = "B1"
			},
		},
		{
			name: "SelfAuthored",
			mutate: func(ev *slackevents.MessageEvent) {
				ev.User = "UBOT"
			},
		},
		{
			name: "Subtype",
			mutate: func(ev *slackevents.MessageEvent) {
				ev.SubType = "message_changed"
			},
		},
		{
			name: "Malformed",
			mutate: func(ev *slackevents.MessageEvent) {
				ev.TimeStamp = ""
			},
		},
		{
			name: "Channel",
			mutate: func(ev *slackevents.MessageEvent) {
				ev.ChannelType = "channel"
			},
		},
		{
			name: "MultipartyDirectMessage",
			mutate: func(ev *slackevents.MessageEvent) {
				ev.ChannelType = "mpim"
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			db, _ := dbtestutil.NewDB(t)
			ctx := testutil.Context(t, testutil.WaitLong)
			owner, _ := seedOwner(t, db)
			chat := newFakeChatSubmitter()
			socket := newFakeSocketClient()
			server := newTestServer(t, db, chat, owner.ID, socket)
			evt := directMessageEvent("EvIgnored", "USENDER", "D1", "100.1", "", "hello")
			message := evt.Data.(slackevents.EventsAPIEvent).InnerEvent.Data.(*slackevents.MessageEvent)
			tt.mutate(message)

			server.handleEvent(ctx, evt)
			_ = testutil.TryReceive(ctx, t, socket.acked)
			server.wg.Wait()

			creates, sends := chat.snapshot()
			require.Empty(t, creates)
			require.Empty(t, sends)
		})
	}
}

func TestFormatSlackTimestamp(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
		want string
		ok   bool
	}{
		{name: "Seconds", raw: "1721651696", want: "2024-07-22T12:34:56Z", ok: true},
		{name: "Microseconds", raw: "1721651696.123456", want: "2024-07-22T12:34:56.123456Z", ok: true},
		{name: "Malformed", raw: "not-a-timestamp"},
		{name: "ExcessPrecision", raw: "1721651696.1234567890"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, ok := formatSlackTimestamp(tt.raw)
			assert.Equal(t, tt.ok, ok)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestHandleMentionSendsToExistingChat(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)
	owner, org := seedOwner(t, db)

	existing := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    org.ID,
		OwnerID:           owner.ID,
		Title:             "existing slack chat",
		LastModelConfigID: dbgen.ChatModelConfig(t, db, database.ChatModelConfig{Model: "chat-model"}).ID,
		Labels: database.StringMap{
			LabelSlackd:      "true",
			LabelSlackThread: "C1:100.1",
		},
	})

	chat := newFakeChatSubmitter()
	socket := newFakeSocketClient()
	server, webAPI := newTestServerWithWebAPI(t, db, chat, owner.ID, "", socket)
	webAPI.setReplies(threadMsg("105.0", "USENDER", "<@UBOT> follow-up"))
	server.Start(ctx)

	socket.events <- mentionEvent("Ev2", "C1", "105.0", "100.1", "<@UBOT> follow-up")
	_ = testutil.TryReceive(ctx, t, chat.called)

	creates, sends := chat.snapshot()
	require.Empty(t, creates)
	require.Len(t, sends, 1)
	send := sends[0]
	assert.Equal(t, existing.ID, send.ChatID)
	assert.Equal(t, owner.ID, send.CreatedBy)
	assert.NotEmpty(t, send.APIKeyID)
	assert.Equal(t, chatd.SendMessageBusyBehaviorInterrupt, send.BusyBehavior)
	assert.Equal(t, MetadataKeySlackEventID, send.DedupMetadataKey)
	require.Len(t, send.Content, 2)
	assert.Equal(t, "Ev2", send.Content[0].Metadata[MetadataKeySlackEventID])
	assert.Equal(t, "105.0", send.Content[1].Metadata[MetadataKeySlackMessageTS])
}

func TestHandleMentionCreateRaceFallsBackToSend(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)
	owner, _ := seedOwner(t, db)

	// Another replica won chat creation: CreateChat returns the
	// existing chat with ErrChatAlreadyExists, and the follow-up
	// SendMessage is dropped as a duplicate. Both are success paths.
	winnerChatID := uuid.New()
	chat := newFakeChatSubmitter()
	chat.createChat = database.Chat{ID: winnerChatID, OwnerID: owner.ID}
	chat.createErr = chatd.ErrChatAlreadyExists
	chat.sendErr = chatd.ErrDuplicateMessage
	socket := newFakeSocketClient()
	server, webAPI := newTestServerWithWebAPI(t, db, chat, owner.ID, "", socket)
	webAPI.setReplies(threadMsg("200.1", "USENDER", "<@UBOT> race"))
	server.Start(ctx)

	socket.events <- mentionEvent("Ev3", "C2", "200.1", "", "<@UBOT> race")
	_ = testutil.TryReceive(ctx, t, chat.called) // CreateChat
	_ = testutil.TryReceive(ctx, t, chat.called) // SendMessage

	_, sends := chat.snapshot()
	require.Len(t, sends, 1)
	assert.Equal(t, winnerChatID, sends[0].ChatID)
	assert.Equal(t, MetadataKeySlackEventID, sends[0].DedupMetadataKey)
}

func TestRunLoopReconnectsWithBackoff(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)
	owner, _ := seedOwner(t, db)

	chat := newFakeChatSubmitter()
	socket := newFakeSocketClient()
	server := newTestServer(t, db, chat, owner.ID, socket)
	server.backoffFloor = time.Millisecond
	server.backoffCeil = 5 * time.Millisecond
	server.Start(ctx)

	// Fail the connection several times; the loop must keep
	// reconnecting.
	for range 3 {
		socket.runResults <- xerrors.New("connection lost")
	}
	require.Eventually(t, func() bool {
		return socket.runCalls.Load() >= 4
	}, testutil.WaitShort, testutil.IntervalFast)

	// Close stops the loop even while RunContext is blocked.
	server.Close()
}

func TestExtractMentions(t *testing.T) {
	t.Parallel()

	ids := extractMentions("<@U1> hi <@U2> and <@U1> again")
	require.Equal(t, []string{"U1", "U2"}, ids)
	require.Empty(t, extractMentions("no mentions here"))
}

func TestSlackTSCompare(t *testing.T) {
	t.Parallel()

	assert.Negative(t, slackTSCompare("100.1", "100.2"))
	assert.Positive(t, slackTSCompare("101.1", "100.2"))
	assert.Zero(t, slackTSCompare("100.1", "100.1"))
	// Numeric, not lexicographic: 9 < 10 in both segments.
	assert.Negative(t, slackTSCompare("9.0", "10.0"))
	assert.Negative(t, slackTSCompare("100.9", "100.10"))
	// Empty and malformed timestamps sort before well-formed ones.
	assert.Negative(t, slackTSCompare("", "100.1"))
	assert.Negative(t, slackTSCompare("garbage", "100.1"))
	assert.Positive(t, slackTSCompare("100.1", ""))
	// Suffix is optional.
	assert.Zero(t, slackTSCompare("100", "100.0"))
}

// seedIngestedMessage persists a user message stamped with
// slack_message_ts values, simulating an earlier catch-up submission.
func seedIngestedMessage(t *testing.T, db database.Store, chatID uuid.UUID, createdBy uuid.UUID, tss ...string) {
	t.Helper()
	parts := make([]codersdk.ChatMessagePart, 0, len(tss))
	for _, ts := range tss {
		parts = append(parts, codersdk.ChatMessagePart{
			Type:     codersdk.ChatMessagePartTypeText,
			Text:     "<slack-message>seeded</slack-message>",
			Metadata: map[string]string{MetadataKeySlackMessageTS: ts},
		})
	}
	content, err := chatprompt.MarshalParts(parts)
	require.NoError(t, err)
	dbgen.ChatMessage(t, db, database.ChatMessage{
		ChatID:    chatID,
		CreatedBy: uuid.NullUUID{UUID: createdBy, Valid: true},
		Content:   content,
	})
}

// seedPostedMessage persists a tool message whose tool-result part is
// stamped with slack_posted_message_ts, simulating a reply the chat
// posted via slack_send_message.
func seedPostedMessage(t *testing.T, db database.Store, chatID uuid.UUID, ts string) {
	t.Helper()
	part := codersdk.ChatMessageToolResult("call1", "slack_send_message", json.RawMessage(`{"ok":true,"ts":"`+ts+`"}`), false, false)
	part.Metadata = map[string]string{MetadataKeySlackPostedMessageTS: ts}
	content, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{part})
	require.NoError(t, err)
	dbgen.ChatMessage(t, db, database.ChatMessage{
		ChatID:  chatID,
		Role:    database.ChatMessageRoleTool,
		Content: content,
	})
}

// blockTimestamps returns the slack_message_ts metadata of each part
// after the header, in order.
func blockTimestamps(parts []codersdk.ChatMessagePart) []string {
	tss := make([]string, 0, len(parts))
	for _, part := range parts[1:] {
		tss = append(tss, part.Metadata[MetadataKeySlackMessageTS])
	}
	return tss
}

func TestHandleMentionNewChatIngestsFullThread(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)
	owner, _ := seedOwner(t, db)

	chat := newFakeChatSubmitter()
	chat.createChat = database.Chat{ID: uuid.New()}
	server, webAPI := newTestServerWithWebAPI(t, db, chat, owner.ID, "", newFakeSocketClient())
	// Paginate to prove the cursor loop is followed.
	webAPI.repliesPageSize = 2
	webAPI.setReplies(
		threadMsg("100.1", "USENDER", "thread start"),
		threadMsg("101.0", "UOTHER", "a reply from another app"),
		threadMsg("105.0", "USENDER", "<@UBOT> hello"),
	)

	require.NoError(t, server.handleMention(ctx, "Ev1", appMention("USENDER", "C1", "105.0", "100.1")))

	creates, sends := chat.snapshot()
	require.Empty(t, sends)
	require.Len(t, creates, 1)
	content := creates[0].InitialUserContent
	require.Len(t, content, 4)
	assert.Equal(t, "Ev1", content[0].Metadata[MetadataKeySlackEventID])
	assert.Equal(t, []string{"100.1", "101.0", "105.0"}, blockTimestamps(content))
	assert.Contains(t, content[1].Text, "thread start")
	assert.Contains(t, content[2].Text, "a reply from another app")
	// The triggering mention's bot reference is rendered inline.
	assert.NotContains(t, content[3].Text, "<@UBOT>")
	assert.Contains(t, content[3].Text, "@bot hello")

	webAPI.mu.Lock()
	defer webAPI.mu.Unlock()
	require.Len(t, webAPI.repliesCalls, 2)
	assert.Equal(t, "2", webAPI.repliesCalls[1].Cursor)
}

func TestHandleMentionIngestsOnlyUnseenMessages(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)
	owner, org := seedOwner(t, db)
	existing := seedThreadChat(t, db, org, owner.ID, "C1:100.1")
	// The chat has ingested up to 102.0 and posted its own reply at
	// 104.0. The posted ts must not advance the watermark: the human
	// message at 103.0 is still unseen.
	seedIngestedMessage(t, db, existing.ID, owner.ID, "100.1", "102.0")
	seedPostedMessage(t, db, existing.ID, "104.0")

	chat := newFakeChatSubmitter()
	server, webAPI := newTestServerWithWebAPI(t, db, chat, owner.ID, "", newFakeSocketClient())
	webAPI.setReplies(
		threadMsg("100.1", "USENDER", "thread start"),
		threadMsg("102.0", "USENDER", "already seen"),
		threadMsg("103.0", "UOTHER", "unseen human message"),
		threadMsg("104.0", "UBOT", "our own posted reply"),
		threadMsg("105.0", "USENDER", "<@UBOT> hello"),
	)

	require.NoError(t, server.handleMention(ctx, "Ev2", appMention("USENDER", "C1", "105.0", "100.1")))

	creates, sends := chat.snapshot()
	require.Empty(t, creates)
	require.Len(t, sends, 1)
	assert.Equal(t, existing.ID, sends[0].ChatID)
	assert.Equal(t, []string{"103.0", "105.0"}, blockTimestamps(sends[0].Content))
}

func TestHandleMentionQueuedMessagesCountTowardWatermark(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)
	owner, org := seedOwner(t, db)
	existing := seedThreadChat(t, db, org, owner.ID, "C1:100.1")

	// A queued (not yet promoted) catch-up already carries 103.0; a
	// busy chat must not double-ingest it.
	queued, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{{
		Type:     codersdk.ChatMessagePartTypeText,
		Text:     "<slack-message>queued</slack-message>",
		Metadata: map[string]string{MetadataKeySlackMessageTS: "103.0"},
	}})
	require.NoError(t, err)
	_, err = db.InsertChatQueuedMessage(ctx, database.InsertChatQueuedMessageParams{
		ChatID:  existing.ID,
		Content: queued.RawMessage,
	})
	require.NoError(t, err)

	chat := newFakeChatSubmitter()
	server, webAPI := newTestServerWithWebAPI(t, db, chat, owner.ID, "", newFakeSocketClient())
	webAPI.setReplies(
		threadMsg("103.0", "UOTHER", "queued already"),
		threadMsg("105.0", "USENDER", "<@UBOT> hello"),
	)

	require.NoError(t, server.handleMention(ctx, "Ev3", appMention("USENDER", "C1", "105.0", "100.1")))

	_, sends := chat.snapshot()
	require.Len(t, sends, 1)
	assert.Equal(t, []string{"105.0"}, blockTimestamps(sends[0].Content))
}

func TestHandleMentionFetchIsAuthoritative(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)
	owner, org := seedOwner(t, db)
	seedThreadChat(t, db, org, owner.ID, "C1:100.1")

	chat := newFakeChatSubmitter()
	server, webAPI := newTestServerWithWebAPI(t, db, chat, owner.ID, "", newFakeSocketClient())
	// The fetch does not return the triggering mention. It is not
	// synthesized from the event payload: only the fetched thread
	// content is ingested.
	webAPI.setReplies(
		threadMsg("103.0", "UOTHER", "an earlier reply"),
	)

	require.NoError(t, server.handleMention(ctx, "Ev4", appMention("USENDER", "C1", "105.0", "100.1")))

	_, sends := chat.snapshot()
	require.Len(t, sends, 1)
	assert.Equal(t, []string{"103.0"}, blockTimestamps(sends[0].Content))
}

func TestHandleMentionLateEventAlreadyIngestedSkips(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)
	owner, org := seedOwner(t, db)
	existing := seedThreadChat(t, db, org, owner.ID, "C1:100.1")
	// An earlier event's catch-up already ingested the mention at
	// 103.0 and a later message at 105.0.
	seedIngestedMessage(t, db, existing.ID, owner.ID, "100.1", "103.0", "105.0")

	chat := newFakeChatSubmitter()
	server, webAPI := newTestServerWithWebAPI(t, db, chat, owner.ID, "", newFakeSocketClient())
	webAPI.setReplies(
		threadMsg("100.1", "USENDER", "thread start"),
		threadMsg("103.0", "USENDER", "<@UBOT> late-delivered mention"),
		threadMsg("105.0", "USENDER", "<@UBOT> newer mention"),
	)

	// The mention at 103.0 arrives late with a fresh event id, after
	// its content was ingested and the watermark moved past it. It
	// must not be re-ingested.
	require.NoError(t, server.handleMention(ctx, "Ev4-late", appMention("USENDER", "C1", "103.0", "100.1")))

	creates, sends := chat.snapshot()
	require.Empty(t, creates)
	require.Empty(t, sends)
}

func TestHandleMentionRepliesFetchFailureFallsBackToMentionOnly(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)
	owner, org := seedOwner(t, db)
	seedThreadChat(t, db, org, owner.ID, "C1:100.1")

	chat := newFakeChatSubmitter()
	server, webAPI := newTestServerWithWebAPI(t, db, chat, owner.ID, "", newFakeSocketClient())
	webAPI.repliesErr = xerrors.New("slack is down")

	require.NoError(t, server.handleMention(ctx, "Ev5", appMention("USENDER", "C1", "105.0", "100.1")))

	_, sends := chat.snapshot()
	require.Len(t, sends, 1)
	content := sends[0].Content
	require.Len(t, content, 2)
	assert.Equal(t, "Ev5", content[0].Metadata[MetadataKeySlackEventID])
	assert.Equal(t, "105.0", content[1].Metadata[MetadataKeySlackMessageTS])
	assert.Contains(t, content[1].Text, "hello")
}

func TestWithThreadLock(t *testing.T) {
	t.Parallel()

	t.Run("SerializesSameThread", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitLong)
		owner, _ := seedOwner(t, db)
		server := newTestServer(t, db, newFakeChatSubmitter(), owner.ID, newFakeSocketClient())

		firstEntered := make(chan struct{})
		firstRelease := make(chan struct{})
		firstDone := make(chan error, 1)
		go func() {
			firstDone <- server.withThreadLock(ctx, "C1:100.1", func() error {
				close(firstEntered)
				<-firstRelease
				return nil
			})
		}()
		_ = testutil.TryReceive(ctx, t, firstEntered)

		secondEntered := make(chan struct{})
		secondDone := make(chan error, 1)
		go func() {
			secondDone <- server.withThreadLock(ctx, "C1:100.1", func() error {
				close(secondEntered)
				return nil
			})
		}()

		// While the first holder is inside its critical section, the
		// second must not enter.
		require.Never(t, func() bool {
			select {
			case <-secondEntered:
				return true
			default:
				return false
			}
		}, time.Second, testutil.IntervalFast)

		close(firstRelease)
		require.NoError(t, testutil.TryReceive(ctx, t, firstDone))
		_ = testutil.TryReceive(ctx, t, secondEntered)
		require.NoError(t, testutil.TryReceive(ctx, t, secondDone))
	})

	t.Run("DifferentThreadsIndependent", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitLong)
		owner, _ := seedOwner(t, db)
		server := newTestServer(t, db, newFakeChatSubmitter(), owner.ID, newFakeSocketClient())

		firstEntered := make(chan struct{})
		firstRelease := make(chan struct{})
		firstDone := make(chan error, 1)
		go func() {
			firstDone <- server.withThreadLock(ctx, "C1:100.1", func() error {
				close(firstEntered)
				<-firstRelease
				return nil
			})
		}()
		_ = testutil.TryReceive(ctx, t, firstEntered)

		// A different thread's lock is not contended.
		require.NoError(t, server.withThreadLock(ctx, "C2:200.1", func() error {
			return nil
		}))

		close(firstRelease)
		require.NoError(t, testutil.TryReceive(ctx, t, firstDone))
	})
}
