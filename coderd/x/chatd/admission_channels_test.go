package chatd_test

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/x/chatd"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/x/agenthooks"
	"github.com/coder/coder/v2/testutil"
)

// userPromptSubmitRecorder is a signing hook consumer that records every
// user_prompt_submit payload it is asked to admit.
type userPromptSubmitRecorder struct {
	mu     sync.Mutex
	events []agenthooks.UserPromptSubmitData
}

func (r *userPromptSubmitRecorder) record(data agenthooks.UserPromptSubmitData) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, data)
}

func (r *userPromptSubmitRecorder) all() []agenthooks.UserPromptSubmitData {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]agenthooks.UserPromptSubmitData(nil), r.events...)
}

func newUserPromptSubmitRecorder(t *testing.T) (*httptest.Server, *userPromptSubmitRecorder) {
	t.Helper()
	recorder := &userPromptSubmitRecorder{}
	server := httptest.NewUnstartedServer(nil)
	server.Config.Handler = agenthooks.NewHTTPHandler(
		[]byte("test-hook-secret-32-bytes-minimum!!"),
		"http://"+server.Listener.Addr().String(),
		agenthooks.Hooks{
			UserPromptSubmit: func(_ context.Context, _ agenthooks.Meta, data agenthooks.UserPromptSubmitData) (agenthooks.Response, error) {
				recorder.record(data)
				return agenthooks.Response{}, nil
			},
		},
	)
	server.Start()
	t.Cleanup(server.Close)
	return server, recorder
}

// TestUserPromptSubmitCarriesInstructionChannels pins that the admission
// event exposes every caller-controlled instruction channel: the per-chat
// system prompt and dynamic tool definitions at create, and the owner's
// stored custom prompt on every prompt admission. A policy shown only the
// prompt text would otherwise admit these channels sight unseen.
func TestUserPromptSubmitCarriesInstructionChannels(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)
	user, org, model := seedChatDependencies(t, db)

	_, err := db.UpdateUserChatCustomPrompt(ctx, database.UpdateUserChatCustomPromptParams{
		UserID:           user.ID,
		ChatCustomPrompt: "stored custom prompt",
	})
	require.NoError(t, err)

	consumer, recorder := newUserPromptSubmitRecorder(t)
	server := newHookTestServer(t, db, ps, consumer)

	dynamicTools := json.RawMessage(`[{"name":"dyn_tool","description":"attacker instructions","input_schema":{"type":"object"}}]`)
	chat, err := server.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID:     org.ID,
		OwnerID:            user.ID,
		Title:              "admission channels",
		ModelConfigID:      model.ID,
		SystemPrompt:       "per-chat system prompt",
		DynamicTools:       dynamicTools,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("hi")},
	})
	require.NoError(t, err)

	events := recorder.all()
	require.Len(t, events, 1)
	created := events[0]
	require.Equal(t, "hi", created.Prompt)
	require.Contains(t, created.SystemPrompt, "per-chat system prompt")
	require.Contains(t, created.CustomPrompt, "stored custom prompt")
	require.JSONEq(t, string(dynamicTools), string(created.DynamicTools))

	// Interrupt-free follow-up: the custom prompt is injected every turn,
	// so every admission repeats it; the create-only channels are not.
	_, err = server.SendMessage(ctx, chatd.SendMessageOptions{
		ChatID:    chat.ID,
		CreatedBy: user.ID,
		Content:   []codersdk.ChatMessagePart{codersdk.ChatMessageText("again")},
	})
	require.NoError(t, err)

	events = recorder.all()
	require.Len(t, events, 2)
	sent := events[1]
	require.Equal(t, "again", sent.Prompt)
	require.Empty(t, sent.SystemPrompt)
	require.Empty(t, sent.DynamicTools)
	require.Contains(t, sent.CustomPrompt, "stored custom prompt")
}

// TestAdmittedCustomPromptFixedForTurn pins the admission snapshot: the
// turn injects the custom prompt persisted with its user_prompt_submit
// admission, not the live per-user config. The owner rewrites the stored
// prompt between admission and generation, and a second replica (with a
// cold config cache, so it would resolve the rewrite) generates the turn;
// the model must still receive the admitted bytes.
func TestAdmittedCustomPromptFixedForTurn(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	var (
		promptsMu     sync.Mutex
		systemPrompts []string
	)
	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("title")
		}
		promptsMu.Lock()
		for _, message := range req.Messages {
			if message.Role == "system" {
				systemPrompts = append(systemPrompts, message.Content)
			}
		}
		promptsMu.Unlock()
		return chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("done")...)
	})
	user, org, model := seedChatDependenciesWithProvider(t, db, "openai-compat", openAIURL)

	_, err := db.UpdateUserChatCustomPrompt(ctx, database.UpdateUserChatCustomPromptParams{
		UserID:           user.ID,
		ChatCustomPrompt: "admitted instruction marker",
	})
	require.NoError(t, err)

	consumer, recorder := newUserPromptSubmitRecorder(t)

	// Admit the create on an API-only replica whose worker never runs, so
	// the stored prompt can change before any replica generates the turn.
	apiServer := newHookTestServer(t, db, ps, consumer)
	chat, err := apiServer.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID:     org.ID,
		OwnerID:            user.ID,
		Title:              "admitted custom prompt",
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("hi")},
	})
	require.NoError(t, err)

	// The admission persisted its snapshot atomically with the create.
	stored, err := db.GetChatByID(ctx, chat.ID)
	require.NoError(t, err)
	require.True(t, stored.AdmittedCustomPrompt.Valid)
	require.Contains(t, stored.AdmittedCustomPrompt.String, "admitted instruction marker")

	events := recorder.all()
	require.Len(t, events, 1)
	admitted := events[0].CustomPrompt
	require.Contains(t, admitted, "admitted instruction marker")

	// The owner rewrites the stored prompt after admission but before any
	// worker picks the turn up.
	_, err = db.UpdateUserChatCustomPrompt(ctx, database.UpdateUserChatCustomPromptParams{
		UserID:           user.ID,
		ChatCustomPrompt: "unadmitted instruction marker",
	})
	require.NoError(t, err)

	genServer := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
		cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(chattest.NewMockAIBridgeTransport(t, openAIURL))
		cfg.HookDispatcher = newHookDispatcher(t, db, consumer)
	})
	waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusWaiting)

	promptsMu.Lock()
	joined := strings.Join(systemPrompts, "\n")
	promptsMu.Unlock()
	// The injected value is byte-for-byte what the policy admitted; the
	// rewrite the policy never saw stays out of the model's context.
	require.Contains(t, joined, admitted)
	require.NotContains(t, joined, "unadmitted instruction marker")

	// The next admission sees the rewrite and replaces the snapshot
	// synchronously with the send: the latest admitted value wins.
	_, err = genServer.SendMessage(ctx, chatd.SendMessageOptions{
		ChatID:    chat.ID,
		CreatedBy: user.ID,
		Content:   []codersdk.ChatMessagePart{codersdk.ChatMessageText("again")},
	})
	require.NoError(t, err)
	stored, err = db.GetChatByID(ctx, chat.ID)
	require.NoError(t, err)
	require.Contains(t, stored.AdmittedCustomPrompt.String, "unadmitted instruction marker")
	events = recorder.all()
	require.Len(t, events, 2)
	require.Contains(t, events[1].CustomPrompt, "unadmitted instruction marker")
}

// TestQueuedAdmissionSnapshotsPromotePerMessage pins that each queued send
// carries its own admission snapshot and promotion installs the promoted
// message's value, not the latest admission's. Two messages are queued on a
// busy chat under different stored custom prompts; when the first promotes,
// the turn must use the first admission's value even though a later
// admission happened in between.
func TestQueuedAdmissionSnapshotsPromotePerMessage(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)
	user, org, model := seedChatDependencies(t, db)

	consumer, recorder := newUserPromptSubmitRecorder(t)

	// A running chat queues sends instead of inserting them directly.
	chat := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    org.ID,
		OwnerID:           user.ID,
		LastModelConfigID: model.ID,
		Status:            database.ChatStatusRunning,
	})

	// Each send goes through its own server (and so its own config cache)
	// to make the admissions observe the two stored prompts without
	// waiting out the cache TTL.
	_, err := db.UpdateUserChatCustomPrompt(ctx, database.UpdateUserChatCustomPromptParams{
		UserID:           user.ID,
		ChatCustomPrompt: "first admission marker",
	})
	require.NoError(t, err)
	firstServer := newHookTestServer(t, db, ps, consumer)
	firstSend, err := firstServer.SendMessage(ctx, chatd.SendMessageOptions{
		ChatID:    chat.ID,
		CreatedBy: user.ID,
		Content:   []codersdk.ChatMessagePart{codersdk.ChatMessageText("first queued")},
	})
	require.NoError(t, err)
	require.True(t, firstSend.Queued)

	_, err = db.UpdateUserChatCustomPrompt(ctx, database.UpdateUserChatCustomPromptParams{
		UserID:           user.ID,
		ChatCustomPrompt: "second admission marker",
	})
	require.NoError(t, err)
	secondServer := newHookTestServer(t, db, ps, consumer)
	secondSend, err := secondServer.SendMessage(ctx, chatd.SendMessageOptions{
		ChatID:    chat.ID,
		CreatedBy: user.ID,
		Content:   []codersdk.ChatMessagePart{codersdk.ChatMessageText("second queued")},
	})
	require.NoError(t, err)
	require.True(t, secondSend.Queued)

	events := recorder.all()
	require.Len(t, events, 2)
	require.Contains(t, events[0].CustomPrompt, "first admission marker")
	require.Contains(t, events[1].CustomPrompt, "second admission marker")

	// Each queued row carries the snapshot its own admission was shown.
	queued, err := db.GetChatQueuedMessages(ctx, chat.ID)
	require.NoError(t, err)
	require.Len(t, queued, 2)
	require.Contains(t, queued[0].AdmittedCustomPrompt.String, "first admission marker")
	require.Contains(t, queued[1].AdmittedCustomPrompt.String, "second admission marker")

	// The in-flight turn finishes: the head promotes and its own
	// admission value becomes the chat-level snapshot, even though a
	// later admission happened after it was queued.
	machine := chatstate.NewChatMachine(db, ps, chat.ID)
	require.NoError(t, machine.Update(ctx, func(tx *chatstate.Tx, _ database.Store) error {
		_, err := tx.FinishTurn(chatstate.FinishTurnInput{})
		return err
	}))
	stored, err := db.GetChatByID(ctx, chat.ID)
	require.NoError(t, err)
	require.Contains(t, stored.AdmittedCustomPrompt.String, "first admission marker")
	require.NotContains(t, stored.AdmittedCustomPrompt.String, "second admission marker")

	// The next promotion installs the second admission's value.
	require.NoError(t, machine.Update(ctx, func(tx *chatstate.Tx, _ database.Store) error {
		_, err := tx.FinishTurn(chatstate.FinishTurnInput{})
		return err
	}))
	stored, err = db.GetChatByID(ctx, chat.ID)
	require.NoError(t, err)
	require.Contains(t, stored.AdmittedCustomPrompt.String, "second admission marker")
}

// TestHookBypassingTurnClearsAdmittedSnapshot pins that a turn admitted
// while hooks are disabled stamps a NULL snapshot instead of preserving an
// earlier admitted turn's value. Without this, re-enabling hooks before
// the bypassing turn generates (replicas can briefly run different rollout
// configurations) would inject stale admitted instructions into a turn
// whose admission never saw them.
func TestHookBypassingTurnClearsAdmittedSnapshot(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)
	user, org, model := seedChatDependencies(t, db)

	_, err := db.UpdateUserChatCustomPrompt(ctx, database.UpdateUserChatCustomPromptParams{
		UserID:           user.ID,
		ChatCustomPrompt: "hooked admission marker",
	})
	require.NoError(t, err)

	consumer, _ := newUserPromptSubmitRecorder(t)
	hookedServer := newHookTestServer(t, db, ps, consumer)
	chat, err := hookedServer.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID:     org.ID,
		OwnerID:            user.ID,
		Title:              "hook bypass",
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("hi")},
	})
	require.NoError(t, err)
	stored, err := db.GetChatByID(ctx, chat.ID)
	require.NoError(t, err)
	require.True(t, stored.AdmittedCustomPrompt.Valid)

	// Move the chat out of the created running state so the next send
	// inserts directly instead of queueing.
	machine := chatstate.NewChatMachine(db, ps, chat.ID)
	require.NoError(t, machine.Update(ctx, func(tx *chatstate.Tx, _ database.Store) error {
		_, err := tx.FinishTurn(chatstate.FinishTurnInput{})
		return err
	}))

	// A send on a hooks-disabled replica bypasses admission entirely, so
	// it must clear the snapshot rather than inherit the earlier one.
	plainServer := newTestServer(t, db, ps, uuid.New())
	_, err = plainServer.SendMessage(ctx, chatd.SendMessageOptions{
		ChatID:    chat.ID,
		CreatedBy: user.ID,
		Content:   []codersdk.ChatMessagePart{codersdk.ChatMessageText("unadmitted send")},
	})
	require.NoError(t, err)
	stored, err = db.GetChatByID(ctx, chat.ID)
	require.NoError(t, err)
	require.False(t, stored.AdmittedCustomPrompt.Valid,
		"a hook-bypassing send must stamp NULL, not preserve an earlier admitted snapshot")
}
