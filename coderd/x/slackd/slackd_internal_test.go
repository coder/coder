package slackd

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/x/chatd"
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

// fakeUserInfoAPI serves canned Slack identities.
type fakeUserInfoAPI struct {
	botUID string
	users  map[string]*slack.User
}

func (f *fakeUserInfoAPI) AuthTestContext(context.Context) (*slack.AuthTestResponse, error) {
	return &slack.AuthTestResponse{UserID: f.botUID}, nil
}

func (f *fakeUserInfoAPI) GetUserInfoContext(_ context.Context, user string) (*slack.User, error) {
	if u, ok := f.users[user]; ok {
		return u, nil
	}
	return nil, xerrors.New("user not found")
}

// fakeChatSubmitter records chatd calls and returns scripted results.
type fakeChatSubmitter struct {
	mu          sync.Mutex
	createCalls []chatd.CreateOptions
	sendCalls   []chatd.SendMessageOptions

	createChat database.Chat
	createErr  error
	sendErr    error

	called chan struct{}
}

func newFakeChatSubmitter() *fakeChatSubmitter {
	return &fakeChatSubmitter{called: make(chan struct{}, 16)}
}

func (f *fakeChatSubmitter) CreateChat(_ context.Context, opts chatd.CreateOptions) (database.Chat, error) {
	f.mu.Lock()
	f.createCalls = append(f.createCalls, opts)
	f.mu.Unlock()
	f.called <- struct{}{}
	return f.createChat, f.createErr
}

func (f *fakeChatSubmitter) SendMessage(_ context.Context, opts chatd.SendMessageOptions) (chatd.SendMessageResult, error) {
	f.mu.Lock()
	f.sendCalls = append(f.sendCalls, opts)
	f.mu.Unlock()
	f.called <- struct{}{}
	return chatd.SendMessageResult{}, f.sendErr
}

func (f *fakeChatSubmitter) snapshot() ([]chatd.CreateOptions, []chatd.SendMessageOptions) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]chatd.CreateOptions(nil), f.createCalls...),
		append([]chatd.SendMessageOptions(nil), f.sendCalls...)
}

func newTestServer(t *testing.T, db database.Store, chat ChatSubmitter, owner uuid.UUID, socket *fakeSocketClient) *Server {
	t.Helper()
	server, err := New(Options{
		Logger:          slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug),
		Database:        db,
		Chat:            chat,
		ChatOwnerUserID: owner,
		SocketClient:    socket,
		UserInfoAPI: &fakeUserInfoAPI{
			botUID: "UBOT",
			users: map[string]*slack.User{
				"USENDER": {Name: "sender", RealName: "Sender Name"},
			},
		},
	})
	require.NoError(t, err)
	t.Cleanup(server.Close)
	return server
}

func mentionEvent(eventID, channel, ts, threadTS, text string) socketmode.Event {
	return socketmode.Event{
		Type:    socketmode.EventTypeEventsAPI,
		Request: &socketmode.Request{EnvelopeID: eventID},
		Data: slackevents.EventsAPIEvent{
			Type: slackevents.CallbackEvent,
			Data: &slackevents.EventsAPICallbackEvent{EventID: eventID},
			InnerEvent: slackevents.EventsAPIInnerEvent{
				Data: &slackevents.AppMentionEvent{
					Type:            "app_mention",
					User:            "USENDER",
					Text:            text,
					TimeStamp:       ts,
					ThreadTimeStamp: threadTS,
					Channel:         channel,
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

func TestHandleMentionCreatesChatForNewThread(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)
	owner, org := seedOwner(t, db)

	chat := newFakeChatSubmitter()
	chat.createChat = database.Chat{ID: uuid.New()}
	socket := newFakeSocketClient()
	server := newTestServer(t, db, chat, owner.ID, socket)
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
	assert.Equal(t, map[string]string{
		LabelSlackd:      "true",
		LabelSlackThread: "C1:100.1",
	}, create.DedupLabels)
	require.Len(t, create.InitialUserContent, 1)
	part := create.InitialUserContent[0]
	assert.Equal(t, "Ev1", part.Metadata[MetadataKeySlackEventID])
	assert.Contains(t, part.Text, "hello")
	assert.Contains(t, part.Text, "Channel ID: C1")
	assert.Contains(t, part.Text, "sender")
	assert.NotContains(t, part.Text, "<@UBOT> hello")
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
	server := newTestServer(t, db, chat, owner.ID, socket)
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
	require.Len(t, send.Content, 1)
	assert.Equal(t, "Ev2", send.Content[0].Metadata[MetadataKeySlackEventID])
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
	chat.createChat = database.Chat{ID: winnerChatID}
	chat.createErr = chatd.ErrChatAlreadyExists
	chat.sendErr = chatd.ErrDuplicateMessage
	socket := newFakeSocketClient()
	server := newTestServer(t, db, chat, owner.ID, socket)
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
