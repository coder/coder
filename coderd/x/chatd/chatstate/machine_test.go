package chatstate_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"slices"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	coderdpubsub "github.com/coder/coder/v2/coderd/pubsub"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

// testFixture bundles the resources every integration test needs:
// a database, a publisher recorder, a user/org/model triple, and
// helper accessors. It is intentionally NOT a generic chatd test
// fixture; tests outside this package should not depend on it.
type testFixture struct {
	DB    database.Store
	Pub   *recordingPubsub
	User  database.User
	Org   database.Organization
	Model database.ChatModelConfig
}

func newTestFixture(t *testing.T) *testFixture {
	t.Helper()
	db, _ := dbtestutil.NewDB(t)
	user := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})
	dbgen.OrganizationMember(t, db, database.OrganizationMember{
		UserID:         user.ID,
		OrganizationID: org.ID,
	})
	dbgen.ChatProvider(t, db, database.ChatProvider{
		Provider:    "openai",
		DisplayName: "openai",
		BaseUrl:     "http://example.invalid",
	})
	model := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{
		OrganizationID: org.ID,
		IsDefault:      true,
	})
	pub := newRecordingPubsub()
	return &testFixture{
		DB:    db,
		Pub:   pub,
		User:  user,
		Org:   org,
		Model: model,
	}
}

// readChat re-reads the chat from the database. Tests use this to
// verify post-transition state because transition results no longer
// carry the chat snapshot.
func (f *testFixture) readChat(ctx context.Context, t *testing.T, chatID uuid.UUID) database.Chat {
	t.Helper()
	chat, err := f.DB.GetChatByID(ctx, chatID)
	require.NoError(t, err)
	return chat
}

// classify reads the chat plus queue cardinality and returns the
// execution state.
func (f *testFixture) classify(ctx context.Context, t *testing.T, chatID uuid.UUID) chatstate.ExecutionState {
	t.Helper()
	chat := f.readChat(ctx, t, chatID)
	count, err := f.DB.CountChatQueuedMessages(ctx, chatID)
	require.NoError(t, err)
	return chatstate.ClassifyExecutionState(chat, count > 0, true)
}

// recordingPubsub captures every Publish call so tests can assert on
// the chatstate notifications without needing a live subscriber. The
// mutex makes it safe to use from concurrent tests that race multiple
// goroutines through the same publisher (see TestConcurrentUpdatesSerializeOnChatRow).
type recordingPubsub struct {
	mu       sync.Mutex
	channels []string
	payloads [][]byte
}

func newRecordingPubsub() *recordingPubsub { return &recordingPubsub{} }

func (r *recordingPubsub) Publish(channel string, payload []byte) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.channels = append(r.channels, channel)
	r.payloads = append(r.payloads, slices.Clone(payload))
	return nil
}

// expectChatUpdate finds the most recent chat:update message on the
// per-chat channel and asserts that it has snapshot_version == want.
func (r *recordingPubsub) expectChatUpdate(t *testing.T, chatID uuid.UUID, wantSnapshot int64) {
	t.Helper()
	channel := coderdpubsub.ChatStateUpdateChannel(chatID)
	for i := len(r.channels) - 1; i >= 0; i-- {
		if r.channels[i] != channel {
			continue
		}
		var msg coderdpubsub.ChatStateUpdateMessage
		require.NoError(t, json.Unmarshal(r.payloads[i], &msg))
		require.Equal(t, wantSnapshot, msg.SnapshotVersion)
		return
	}
	t.Fatalf("no chat:update on %s", channel)
}

func (r *recordingPubsub) hasOwnership() bool {
	for _, c := range r.channels {
		if c == coderdpubsub.ChatStateOwnershipChannel {
			return true
		}
	}
	return false
}

func userTextMessage(text string, createdBy uuid.UUID, modelConfigID uuid.UUID) chatstate.Message {
	parts := []codersdk.ChatMessagePart{codersdk.ChatMessageText(text)}
	raw, err := chatprompt.MarshalParts(parts)
	if err != nil {
		panic(err)
	}
	return chatstate.Message{
		Role:           database.ChatMessageRoleUser,
		Content:        raw,
		Visibility:     database.ChatMessageVisibilityBoth,
		ContentVersion: chatprompt.CurrentContentVersion,
		CreatedBy:      uuid.NullUUID{UUID: createdBy, Valid: true},
		ModelConfigID:  uuid.NullUUID{UUID: modelConfigID, Valid: true},
	}
}

// createTestChat is the standard "fresh R0 chat" helper used by other
// tests. It exercises CreateChat itself.
func createTestChat(t *testing.T, f *testFixture) chatstate.CreateChatResult {
	t.Helper()
	ctx := testutil.Context(t, testutil.WaitShort)
	res, err := chatstate.CreateChat(ctx, f.DB, f.Pub, chatstate.CreateChatInput{
		OrganizationID:    f.Org.ID,
		OwnerID:           f.User.ID,
		LastModelConfigID: f.Model.ID,
		Title:             "test",
		ClientType:        database.ChatClientTypeApi,
		InitialStatus:     database.ChatStatusRunning,
		InitialMessages: []chatstate.Message{
			userTextMessage("hello", f.User.ID, f.Model.ID),
		},
	})
	require.NoError(t, err)
	return res
}

func TestChatMachine_Update_RejectsMissingChat(t *testing.T) {
	t.Parallel()
	f := newTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	m := chatstate.NewChatMachine(f.DB, f.Pub, uuid.New())
	err := m.Update(ctx, func(tx *chatstate.Tx, store database.Store) error { return nil })
	require.ErrorIs(t, err, chatstate.ErrChatNotFound)
	require.Empty(t, f.Pub.channels)
}

func TestChatMachine_Lock_DoesNotBumpSnapshot(t *testing.T) {
	t.Parallel()
	f := newTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	created := createTestChat(t, f)
	m := chatstate.NewChatMachine(f.DB, f.Pub, created.Chat.ID)

	before := f.readChat(ctx, t, created.Chat.ID)
	publishedBefore := len(f.Pub.channels)

	require.NoError(t, m.Lock(ctx, func(_ database.Store) error {
		return nil
	}))
	after := f.readChat(ctx, t, created.Chat.ID)
	require.Equal(t, before.SnapshotVersion, after.SnapshotVersion)
	require.Equal(t, publishedBefore, len(f.Pub.channels), "Lock must not publish")
}

func TestChatMachine_ReadSnapshot_DoesNotBumpSnapshot(t *testing.T) {
	t.Parallel()
	f := newTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	created := createTestChat(t, f)
	m := chatstate.NewChatMachine(f.DB, f.Pub, created.Chat.ID)

	before := f.readChat(ctx, t, created.Chat.ID)
	publishedBefore := len(f.Pub.channels)

	var called bool
	require.NoError(t, m.ReadSnapshot(func(_ database.Store) error {
		called = true
		return nil
	}))
	require.True(t, called, "ReadSnapshot must invoke the callback")
	after := f.readChat(ctx, t, created.Chat.ID)
	require.Equal(t, before.SnapshotVersion, after.SnapshotVersion)
	require.Equal(t, publishedBefore, len(f.Pub.channels), "ReadSnapshot must not publish")
}

func TestChatMachine_ReadSnapshot_PropagatesMissingChat(t *testing.T) {
	t.Parallel()
	f := newTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	chatID := uuid.New()
	m := chatstate.NewChatMachine(f.DB, f.Pub, chatID)
	err := m.ReadSnapshot(func(store database.Store) error {
		_, err := store.GetChatByID(ctx, chatID)
		return err
	})
	require.ErrorIs(t, err, sql.ErrNoRows)
	require.Empty(t, f.Pub.channels)
}

func TestMessageRevisionTrigger_AssignsRevisionFromSnapshot(t *testing.T) {
	t.Parallel()
	f := newTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	created := createTestChat(t, f) // snapshot 1, history_version 1 via trigger

	// CommitStep an assistant message; it should land with revision = chat.snapshot_version after the bump.
	m := chatstate.NewChatMachine(f.DB, f.Pub, created.Chat.ID)
	var step chatstate.CommitStepResult
	require.NoError(t, m.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
		assistant := userTextMessage("assistant", f.User.ID, f.Model.ID)
		assistant.Role = database.ChatMessageRoleAssistant
		var err error
		step, err = tx.CommitStep(chatstate.CommitStepInput{
			Messages: []chatstate.Message{assistant},
		})
		return err
	}))
	require.Len(t, step.InsertedMessages, 1)
	after := f.readChat(ctx, t, created.Chat.ID)
	// The Update call bumps snapshot_version once before the trigger
	// runs, so the new revision should equal the bumped snapshot.
	require.Equal(t, after.SnapshotVersion, step.InsertedMessages[0].Revision)
	require.Equal(t, after.SnapshotVersion, after.HistoryVersion)
	require.Equal(t, int64(0), after.GenerationAttempt, "trigger resets generation_attempt to 0")
}

func TestQueueVersionTrigger_AdvancesOnInsert(t *testing.T) {
	t.Parallel()
	f := newTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	created := createTestChat(t, f) // queue_version starts at 0

	m := chatstate.NewChatMachine(f.DB, f.Pub, created.Chat.ID)
	require.NoError(t, m.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
		_, err := tx.SendMessage(chatstate.SendMessageInput{
			Message:      userTextMessage("queue", f.User.ID, f.Model.ID),
			BusyBehavior: chatstate.BusyBehaviorQueue,
			MaxQueueSize: codersdk.DefaultChatMaxQueuedMessagesPerChat,
		})
		return err
	}))
	after := f.readChat(ctx, t, created.Chat.ID)
	require.Equal(t, after.SnapshotVersion, after.QueueVersion)
	require.Greater(t, after.QueueVersion, int64(0))
}

func TestQueueVersionTrigger_StableForNonQueueMutations(t *testing.T) {
	t.Parallel()
	f := newTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	created := createTestChat(t, f)
	m := chatstate.NewChatMachine(f.DB, f.Pub, created.Chat.ID)

	require.NoError(t, m.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
		assistant := userTextMessage("assistant", f.User.ID, f.Model.ID)
		assistant.Role = database.ChatMessageRoleAssistant
		_, err := tx.CommitStep(chatstate.CommitStepInput{
			Messages: []chatstate.Message{assistant},
		})
		return err
	}))
	// queue_version must remain unchanged from initial 0.
	require.Equal(t, int64(0), f.readChat(ctx, t, created.Chat.ID).QueueVersion)
}

// TestUpdateFlushesBufferedPublicationsAfterCommit verifies that
// ChatMachine.Update owns the PublishBuffer lifecycle: nothing
// reaches the inner publisher until after the transaction commits,
// and at commit the buffered chat:update is forwarded.
func TestUpdateFlushesBufferedPublicationsAfterCommit(t *testing.T) {
	t.Parallel()
	f := newTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	created := createTestChat(t, f)

	channel := coderdpubsub.ChatStateUpdateChannel(created.Chat.ID)
	baseline := countChannel(f.Pub.channels, channel)

	m := chatstate.NewChatMachine(f.DB, f.Pub, created.Chat.ID)

	// During the callback, no new chat:update for this chat may have
	// reached the inner publisher because the buffer holds it.
	require.NoError(t, m.Update(ctx, func(_ *chatstate.Tx, _ database.Store) error {
		require.Equal(t, baseline, countChannel(f.Pub.channels, channel),
			"inner publisher saw chat:update before transaction committed")
		return nil
	}))

	require.Equal(t, baseline+1, countChannel(f.Pub.channels, channel),
		"exactly one new chat:update reached the inner publisher after commit")
}

func countChannel(channels []string, channel string) int {
	c := 0
	for _, ch := range channels {
		if ch == channel {
			c++
		}
	}
	return c
}

// fakeLocalAcquirer records LocalAcquirer calls. StartAcquired captures
// the committed row and the publications made so far, so tests can check
// it runs after commit and publication.
type fakeLocalAcquirer struct {
	db      database.Store
	pub     *recordingPubsub
	decline bool
	// prepare, when set, runs at the start of PrepareAcquire.
	prepare func(ctx context.Context, store database.Store) error

	mu       sync.Mutex
	prepares int
	inputs   []chatstate.AcquireInput
	starts   []fakeLocalStart
}

type fakeLocalStart struct {
	chatID    uuid.UUID
	input     chatstate.AcquireInput
	committed database.Chat
	published []string
}

func (a *fakeLocalAcquirer) PrepareAcquire(ctx context.Context, store database.Store, _ database.Chat) (chatstate.AcquireInput, bool, error) {
	a.mu.Lock()
	a.prepares++
	a.mu.Unlock()
	if a.prepare != nil {
		if err := a.prepare(ctx, store); err != nil {
			return chatstate.AcquireInput{}, false, err
		}
	}
	if a.decline {
		return chatstate.AcquireInput{}, false, nil
	}
	input := chatstate.AcquireInput{WorkerID: uuid.New(), RunnerID: uuid.New()}
	a.mu.Lock()
	a.inputs = append(a.inputs, input)
	a.mu.Unlock()
	return input, true, nil
}

func (a *fakeLocalAcquirer) StartAcquired(chatID uuid.UUID, input chatstate.AcquireInput) {
	committed, _ := a.db.GetChatByID(context.Background(), chatID)
	a.pub.mu.Lock()
	published := slices.Clone(a.pub.channels)
	a.pub.mu.Unlock()
	a.mu.Lock()
	defer a.mu.Unlock()
	a.starts = append(a.starts, fakeLocalStart{chatID: chatID, input: input, committed: committed, published: published})
}

func (a *fakeLocalAcquirer) counts() (prepares, starts int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.prepares, len(a.starts)
}

// requireLocallyAcquired checks that a started the chat once, after the
// commit that made input its owner and after the owned chat:update, and
// returns the chat as stored.
func requireLocallyAcquired(t *testing.T, f *testFixture, a *fakeLocalAcquirer, chatID uuid.UUID) database.Chat {
	t.Helper()
	ctx := testutil.Context(t, testutil.WaitShort)
	a.mu.Lock()
	defer a.mu.Unlock()
	require.Len(t, a.inputs, 1)
	require.Len(t, a.starts, 1)
	start := a.starts[0]
	input := a.inputs[0]
	require.Equal(t, chatID, start.chatID)
	require.Equal(t, input, start.input)
	require.Equal(t, uuid.NullUUID{UUID: input.WorkerID, Valid: true}, start.committed.WorkerID)
	require.Equal(t, uuid.NullUUID{UUID: input.RunnerID, Valid: true}, start.committed.RunnerID)
	require.Contains(t, start.published, coderdpubsub.ChatStateUpdateChannel(chatID))
	stored := f.readChat(ctx, t, chatID)
	require.Equal(t, start.committed, stored)

	_, err := f.DB.GetChatHeartbeat(ctx, database.GetChatHeartbeatParams{ChatID: chatID, RunnerID: input.RunnerID})
	require.NoError(t, err)

	f.Pub.mu.Lock()
	defer f.Pub.mu.Unlock()
	channel := coderdpubsub.ChatStateUpdateChannel(chatID)
	for i := len(f.Pub.channels) - 1; i >= 0; i-- {
		if f.Pub.channels[i] != channel {
			continue
		}
		var msg coderdpubsub.ChatStateUpdateMessage
		require.NoError(t, json.Unmarshal(f.Pub.payloads[i], &msg))
		require.NotNil(t, msg.WorkerID)
		require.NotNil(t, msg.RunnerID)
		require.Equal(t, input.WorkerID, *msg.WorkerID)
		require.Equal(t, input.RunnerID, *msg.RunnerID)
		return stored
	}
	t.Fatalf("no chat:update on %s", channel)
	return database.Chat{}
}

func TestChatMachine_Update_LocalAcquirer(t *testing.T) {
	t.Parallel()

	sendMessage := func(f *testFixture) func(*chatstate.Tx, database.Store) error {
		return func(tx *chatstate.Tx, _ database.Store) error {
			_, err := tx.SendMessage(chatstate.SendMessageInput{
				Message:      userTextMessage("next", f.User.ID, f.Model.ID),
				BusyBehavior: chatstate.BusyBehaviorQueue,
				MaxQueueSize: codersdk.DefaultChatMaxQueuedMessagesPerChat,
			})
			return err
		}
	}
	// waitingChat returns an unowned chat in W, so SendMessage makes it
	// runnable and unowned.
	waitingChat := func(t *testing.T, f *testFixture) uuid.UUID {
		created := createTestChat(t, f)
		landInW(t, f, chatstate.NewChatMachine(f.DB, f.Pub, created.Chat.ID))
		return created.Chat.ID
	}

	t.Run("AcquiresRunnableUnownedChat", func(t *testing.T) {
		t.Parallel()
		f := newTestFixture(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		chatID := waitingChat(t, f)
		hints := f.Pub.ownershipPublishCount()

		a := &fakeLocalAcquirer{db: f.DB, pub: f.Pub}
		m := chatstate.NewChatMachine(f.DB, f.Pub, chatID).WithLocalAcquirer(a)
		require.NoError(t, m.Update(ctx, sendMessage(f)))
		stored := requireLocallyAcquired(t, f, a, chatID)
		require.Equal(t, database.ChatStatusRunning, stored.Status)
		require.Equal(t, hints, f.Pub.ownershipPublishCount(), "a local acquire publishes no ownership hint")
	})

	t.Run("DeclinedPublishesHint", func(t *testing.T) {
		t.Parallel()
		f := newTestFixture(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		chatID := waitingChat(t, f)
		hints := f.Pub.ownershipPublishCount()

		a := &fakeLocalAcquirer{db: f.DB, pub: f.Pub, decline: true}
		m := chatstate.NewChatMachine(f.DB, f.Pub, chatID).WithLocalAcquirer(a)
		require.NoError(t, m.Update(ctx, sendMessage(f)))
		prepares, starts := a.counts()
		require.Equal(t, 1, prepares)
		require.Zero(t, starts)
		require.False(t, f.readChat(ctx, t, chatID).WorkerID.Valid)
		require.Equal(t, hints+1, f.Pub.ownershipPublishCount())
	})

	t.Run("OwnedChatSkipsPrepare", func(t *testing.T) {
		t.Parallel()
		f := newTestFixture(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		created := createTestChat(t, f)
		owner := chatstate.AcquireInput{WorkerID: uuid.New(), RunnerID: uuid.New()}
		require.NoError(t, chatstate.NewChatMachine(f.DB, f.Pub, created.Chat.ID).Update(ctx, func(tx *chatstate.Tx, _ database.Store) error {
			_, err := tx.Acquire(owner)
			return err
		}))

		a := &fakeLocalAcquirer{db: f.DB, pub: f.Pub}
		m := chatstate.NewChatMachine(f.DB, f.Pub, created.Chat.ID).WithLocalAcquirer(a)
		require.NoError(t, m.Update(ctx, sendMessage(f)))
		prepares, starts := a.counts()
		require.Zero(t, prepares)
		require.Zero(t, starts)
		require.Equal(t, owner.RunnerID, f.readChat(ctx, t, created.Chat.ID).RunnerID.UUID)
	})

	t.Run("NestedUpdateSkipsPrepare", func(t *testing.T) {
		t.Parallel()
		f := newTestFixture(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		chatID := waitingChat(t, f)

		a := &fakeLocalAcquirer{db: f.DB, pub: f.Pub}
		outer := chatstate.NewChatMachine(f.DB, f.Pub, chatID)
		require.NoError(t, outer.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
			inner := chatstate.NewChatMachine(store, f.Pub, chatID).WithLocalAcquirer(a)
			return inner.Update(tx.Ctx(), sendMessage(f))
		}))
		prepares, starts := a.counts()
		require.Zero(t, prepares)
		require.Zero(t, starts)
		require.False(t, f.readChat(ctx, t, chatID).WorkerID.Valid)
	})

	t.Run("CallbackErrorStartsNothing", func(t *testing.T) {
		t.Parallel()
		f := newTestFixture(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		chatID := waitingChat(t, f)

		a := &fakeLocalAcquirer{db: f.DB, pub: f.Pub}
		m := chatstate.NewChatMachine(f.DB, f.Pub, chatID).WithLocalAcquirer(a)
		wantErr := xerrors.New("callback failed")
		err := m.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
			if err := sendMessage(f)(tx, store); err != nil {
				return err
			}
			return wantErr
		})
		require.ErrorIs(t, err, wantErr)
		prepares, starts := a.counts()
		require.Zero(t, prepares)
		require.Zero(t, starts)
	})

	t.Run("PrepareErrorRollsBack", func(t *testing.T) {
		t.Parallel()
		f := newTestFixture(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		chatID := waitingChat(t, f)
		before := f.readChat(ctx, t, chatID)

		wantErr := xerrors.New("prepare failed")
		a := &fakeLocalAcquirer{db: f.DB, pub: f.Pub, prepare: func(context.Context, database.Store) error {
			return wantErr
		}}
		m := chatstate.NewChatMachine(f.DB, f.Pub, chatID).WithLocalAcquirer(a)
		err := m.Update(ctx, sendMessage(f))
		require.ErrorIs(t, err, wantErr)
		_, starts := a.counts()
		require.Zero(t, starts)
		require.Equal(t, before.SnapshotVersion, f.readChat(ctx, t, chatID).SnapshotVersion)
	})
}

func TestCreateChatWithID_LocalAcquirer(t *testing.T) {
	t.Parallel()

	create := func(t *testing.T, f *testFixture, a chatstate.LocalAcquirer, status database.ChatStatus) (uuid.UUID, chatstate.CreateChatResult, error) {
		t.Helper()
		chatID := uuid.New()
		res, err := chatstate.CreateChatWithID(testutil.Context(t, testutil.WaitShort), f.DB, f.Pub, chatID, chatstate.CreateChatInput{
			OrganizationID:    f.Org.ID,
			OwnerID:           f.User.ID,
			LastModelConfigID: f.Model.ID,
			Title:             "test",
			ClientType:        database.ChatClientTypeApi,
			InitialStatus:     status,
			InitialMessages: []chatstate.Message{
				userTextMessage("hello", f.User.ID, f.Model.ID),
			},
			LocalAcquirer: a,
		})
		return chatID, res, err
	}

	t.Run("AcquiresRunnableChat", func(t *testing.T) {
		t.Parallel()
		f := newTestFixture(t)
		a := &fakeLocalAcquirer{db: f.DB, pub: f.Pub}
		chatID, res, err := create(t, f, a, database.ChatStatusRunning)
		require.NoError(t, err)
		stored := requireLocallyAcquired(t, f, a, chatID)
		require.Equal(t, stored, res.Chat, "the returned row is the owned row")
		require.Equal(t, int64(1), res.Chat.SnapshotVersion)
		require.False(t, f.Pub.hasOwnership())
	})

	t.Run("DeclinedPublishesHint", func(t *testing.T) {
		t.Parallel()
		f := newTestFixture(t)
		a := &fakeLocalAcquirer{db: f.DB, pub: f.Pub, decline: true}
		_, res, err := create(t, f, a, database.ChatStatusRunning)
		require.NoError(t, err)
		prepares, starts := a.counts()
		require.Equal(t, 1, prepares)
		require.Zero(t, starts)
		require.False(t, res.Chat.WorkerID.Valid)
		require.True(t, f.Pub.hasOwnership())
	})

	t.Run("WaitingChatSkipsPrepare", func(t *testing.T) {
		t.Parallel()
		f := newTestFixture(t)
		a := &fakeLocalAcquirer{db: f.DB, pub: f.Pub}
		_, res, err := create(t, f, a, database.ChatStatusWaiting)
		require.NoError(t, err)
		prepares, starts := a.counts()
		require.Zero(t, prepares)
		require.Zero(t, starts)
		require.False(t, res.Chat.WorkerID.Valid)
	})

	t.Run("PrepareErrorCreatesNothing", func(t *testing.T) {
		t.Parallel()
		f := newTestFixture(t)
		wantErr := xerrors.New("prepare failed")
		a := &fakeLocalAcquirer{db: f.DB, pub: f.Pub, prepare: func(context.Context, database.Store) error {
			return wantErr
		}}
		chatID, _, err := create(t, f, a, database.ChatStatusRunning)
		require.ErrorIs(t, err, wantErr)
		_, starts := a.counts()
		require.Zero(t, starts)
		_, err = f.DB.GetChatByID(testutil.Context(t, testutil.WaitShort), chatID)
		require.ErrorIs(t, err, sql.ErrNoRows)
		require.Empty(t, f.Pub.channels)
	})
}
