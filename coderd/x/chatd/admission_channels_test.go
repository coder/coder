package chatd_test

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/x/chatd"
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
