package chatstate_test

import (
	"context"
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
	"github.com/coder/coder/v2/coderd/database/pubsub"
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
	DB     database.Store
	PubSub pubsub.Pubsub
	Pub    *recordingPubsub
	User   database.User
	Org    database.Organization
	Model  database.ChatModelConfig
}

func newTestFixture(t *testing.T) *testFixture {
	t.Helper()
	db, ps := dbtestutil.NewDB(t)
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
		IsDefault: true,
	})
	pub := newRecordingPubsub()
	return &testFixture{
		DB:     db,
		PubSub: ps,
		Pub:    pub,
		User:   user,
		Org:    org,
		Model:  model,
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

func TestChatMachine_ReadLock_DoesNotBumpSnapshot(t *testing.T) {
	t.Parallel()
	f := newTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	created := createTestChat(t, f)
	m := chatstate.NewChatMachine(f.DB, f.Pub, created.Chat.ID)

	before := f.readChat(ctx, t, created.Chat.ID)
	publishedBefore := len(f.Pub.channels)

	var called bool
	require.NoError(t, m.ReadLock(ctx, func(_ database.Store) error {
		called = true
		return nil
	}))
	require.True(t, called, "ReadLock must invoke the callback")
	after := f.readChat(ctx, t, created.Chat.ID)
	require.Equal(t, before.SnapshotVersion, after.SnapshotVersion)
	require.Equal(t, publishedBefore, len(f.Pub.channels), "ReadLock must not publish")
}

func TestChatMachine_ReadLock_RejectsMissingChat(t *testing.T) {
	t.Parallel()
	f := newTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	m := chatstate.NewChatMachine(f.DB, f.Pub, uuid.New())
	err := m.ReadLock(ctx, func(_ database.Store) error {
		t.Fatal("callback must not run when the chat is missing")
		return nil
	})
	require.ErrorIs(t, err, chatstate.ErrChatNotFound)
	require.Empty(t, f.Pub.channels)
}

func TestChatMachine_UpdatePublishesAfterCommit(t *testing.T) {
	t.Parallel()
	f := newTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	created := createTestChat(t, f)
	m := chatstate.NewChatMachine(f.DB, f.Pub, created.Chat.ID)

	publishedBefore := len(f.Pub.channels)
	// Run a no-op Update; snapshot bump still happens, one update message
	// should follow the commit.
	require.NoError(t, m.Update(ctx, func(_ *chatstate.Tx, _ database.Store) error { return nil }))
	channel := coderdpubsub.ChatStateUpdateChannel(created.Chat.ID)
	var found bool
	for _, c := range f.Pub.channels[publishedBefore:] {
		if c == channel {
			found = true
			break
		}
	}
	require.True(t, found, "expected one chat:update message after commit")
}

func TestChatMachine_FailedUpdate_PublishesNothing(t *testing.T) {
	t.Parallel()
	f := newTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	created := createTestChat(t, f)
	m := chatstate.NewChatMachine(f.DB, f.Pub, created.Chat.ID)

	before := f.readChat(ctx, t, created.Chat.ID)
	channelsBefore := len(f.Pub.channels)
	expected := newSentinel()
	cbErr := m.Update(ctx, func(_ *chatstate.Tx, _ database.Store) error { return expected })
	require.ErrorIs(t, cbErr, expected)
	require.Equal(t, channelsBefore, len(f.Pub.channels), "failed update should not publish")
	// snapshot_version should not have advanced.
	after := f.readChat(ctx, t, created.Chat.ID)
	require.Equal(t, before.SnapshotVersion, after.SnapshotVersion)
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

// TestUpdateDiscardsBufferedPublicationsOnCallbackError verifies the
// deferred Discard path: when the callback returns an error the
// transaction rolls back and no buffered messages reach the inner
// publisher.
func TestUpdateDiscardsBufferedPublicationsOnCallbackError(t *testing.T) {
	t.Parallel()
	f := newTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	created := createTestChat(t, f)
	m := chatstate.NewChatMachine(f.DB, f.Pub, created.Chat.ID)

	before := f.readChat(ctx, t, created.Chat.ID)
	channelsBefore := len(f.Pub.channels)

	sentinel := xerrors.New("callback boom")
	err := m.Update(ctx, func(_ *chatstate.Tx, _ database.Store) error { return sentinel })
	require.ErrorIs(t, err, sentinel)

	require.Equal(t, channelsBefore, len(f.Pub.channels),
		"failed update must not flush any buffered publications")
	after := f.readChat(ctx, t, created.Chat.ID)
	require.Equal(t, before.SnapshotVersion, after.SnapshotVersion,
		"snapshot bump rolled back when callback returns error")
}

type sentinelError struct{ msg string }

func (s *sentinelError) Error() string { return s.msg }

func newSentinel() error { return &sentinelError{msg: "sentinel"} }

func countChannel(channels []string, channel string) int {
	c := 0
	for _, ch := range channels {
		if ch == channel {
			c++
		}
	}
	return c
}
