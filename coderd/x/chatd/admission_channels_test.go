package chatd_test

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/x/chatd"
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
