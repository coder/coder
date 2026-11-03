package coderd_test

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd"
	"github.com/coder/coder/v2/coderd/aibridgedtest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

// proofBModelCall is one streaming model request. The fake provider
// holds the request open until the test sends a response, so the test
// decides when each model call ends.
type proofBModelCall struct {
	Model   string
	Body    []byte
	respond chan chattest.OpenAIResponse
}

func (c *proofBModelCall) has(marker string) bool {
	return bytes.Contains(c.Body, []byte(marker))
}

func (c *proofBModelCall) text(t *testing.T, text string) {
	t.Helper()
	c.respond <- chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks(text)...)
}

func (c *proofBModelCall) fail(t *testing.T) {
	t.Helper()
	// 400 invalid_request_error is not retryable, so the worker runs
	// FinishError.
	c.respond <- chattest.OpenAIErrorResponse(http.StatusBadRequest, "invalid_request_error", "proofB non-retryable failure")
}

type proofBEnv struct {
	client *codersdk.ExperimentalClient
	api    *coderd.API
	user   codersdk.CreateFirstUserResponse
	// calls receives every streaming model request in arrival order.
	calls      chan *proofBModelCall
	providerID uuid.UUID
	model      codersdk.ChatModel
}

// proofBSetup starts coderd with the real chat worker and AI Gateway
// daemon, backed by a fake OpenAI-compatible provider whose streaming
// requests block until the test responds.
func proofBSetup(t *testing.T, overrides ...func(*coderdtest.Options)) *proofBEnv {
	t.Helper()
	env := &proofBEnv{calls: make(chan *proofBModelCall)}
	stop := make(chan struct{})
	baseURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse(`{"title": "proofB"}`)
		}
		call := &proofBModelCall{
			Model:   req.Model,
			Body:    bytes.Clone(req.RawBody),
			respond: make(chan chattest.OpenAIResponse, 1),
		}
		cancelled := chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("cancelled")...)
		select {
		case env.calls <- call:
		case <-req.Context().Done():
			return cancelled
		case <-stop:
			return cancelled
		}
		select {
		case resp := <-call.respond:
			return resp
		case <-req.Context().Done():
			return cancelled
		case <-stop:
			return cancelled
		}
	})

	client, _, api := newChatClientWithoutAIBridge(t, overrides...)
	// Registered after coderd, so it runs first and unblocks held
	// provider requests before coderd shuts down.
	t.Cleanup(func() { close(stop) })
	env.client = client
	env.api = api
	env.user = coderdtest.CreateFirstUser(t, client.Client)

	ctx := testutil.Context(t, testutil.WaitLong)
	provider, err := client.CreateAIProvider(ctx, codersdk.CreateAIProviderRequest{
		Type:    codersdk.AIProviderType(coderdtest.TestChatProviderOpenAICompat),
		Name:    "proofb-" + uuid.NewString(),
		BaseURL: baseURL,
		Enabled: true,
		APIKeys: []string{coderdtest.TestChatProviderAPIKey},
	})
	require.NoError(t, err)
	env.providerID = provider.ID
	env.model = proofBCreateModel(t, env, "proofb-model-a", true)
	aibridgedtest.StartTestAIBridgeDaemon(t.Context(), t, api, nil)
	return env
}

func proofBCreateModel(t *testing.T, env *proofBEnv, name string, isDefault bool) codersdk.ChatModel {
	t.Helper()
	ctx := testutil.Context(t, testutil.WaitLong)
	org, err := env.client.OrganizationByName(ctx, codersdk.DefaultOrganization)
	require.NoError(t, err)
	model, err := env.client.CreateChatModel(ctx, org.ID, codersdk.CreateChatModelRequest{
		AIProviderID: &env.providerID,
		Model:        name,
		ContextLimit: ptr.Ref(int64(4096)),
		IsDefault:    &isDefault,
	})
	require.NoError(t, err)
	return model
}

func proofBCreateChat(ctx context.Context, t *testing.T, env *proofBEnv, text string) codersdk.Chat {
	t.Helper()
	chat, err := env.client.CreateChat(ctx, codersdk.CreateChatRequest{
		OrganizationID: env.user.OrganizationID,
		ModelConfigID:  &env.model.ID,
		Content:        []codersdk.ChatInputPart{{Type: codersdk.ChatInputPartTypeText, Text: text}},
	})
	require.NoError(t, err)
	return chat
}

func proofBSend(
	ctx context.Context,
	env *proofBEnv,
	chatID uuid.UUID,
	text string,
	behavior codersdk.ChatBusyBehavior,
) (codersdk.CreateChatMessageResponse, error) {
	return env.client.CreateChatMessage(ctx, chatID, codersdk.CreateChatMessageRequest{
		Content:      []codersdk.ChatInputPart{{Type: codersdk.ChatInputPartTypeText, Text: text}},
		BusyBehavior: behavior,
	})
}

func proofBWaitStatus(
	ctx context.Context,
	t *testing.T,
	env *proofBEnv,
	chatID uuid.UUID,
	done func(codersdk.ChatStatus) bool,
) codersdk.Chat {
	t.Helper()
	var got codersdk.Chat
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		chat, err := env.client.GetChat(ctx, chatID)
		if err != nil {
			return false
		}
		got = chat
		return done(chat.Status)
	}, testutil.IntervalFast)
	return got
}

func proofBStatusIs(want codersdk.ChatStatus) func(codersdk.ChatStatus) bool {
	return func(got codersdk.ChatStatus) bool { return got == want }
}

func proofBQueuedTexts(ctx context.Context, t *testing.T, env *proofBEnv, chatID uuid.UUID) []string {
	t.Helper()
	got, err := env.client.GetChatMessages(ctx, chatID, nil)
	require.NoError(t, err)
	texts := make([]string, 0, len(got.QueuedMessages))
	for _, q := range got.QueuedMessages {
		if len(q.Content) > 0 {
			texts = append(texts, q.Content[0].Text)
		}
	}
	return texts
}

// TestProofBInterruptThenSendDeliversOlderQueueRowFirst follows the
// documented replacement for busy_behavior=interrupt: POST /interrupt,
// wait until the chat leaves interrupting, then POST the correction. An
// older queue-lane message Q is delivered by the interruption, so the
// first model call after the interrupt answers Q, not the correction.
func TestProofBInterruptThenSendDeliversOlderQueueRowFirst(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	env := proofBSetup(t)
	id := uuid.NewString()
	initial, older, correction := "proofB-initial-"+id, "proofB-older-queued-"+id, "proofB-correction-"+id

	chat := proofBCreateChat(ctx, t, env, initial)
	call1 := testutil.RequireReceive(ctx, t, env.calls)
	require.True(t, call1.has(initial))

	queued, err := proofBSend(ctx, env, chat.ID, older, codersdk.ChatBusyBehaviorQueue)
	require.NoError(t, err)
	require.True(t, queued.Queued)

	interrupted, err := env.client.InterruptChat(ctx, chat.ID)
	require.NoError(t, err)
	require.Equal(t, codersdk.ChatStatusInterrupting, interrupted.Status)
	left := proofBWaitStatus(ctx, t, env, chat.ID, func(s codersdk.ChatStatus) bool {
		return s != codersdk.ChatStatusInterrupting
	})
	t.Logf("status after the interruption finished: %s", left.Status)

	sent, err := proofBSend(ctx, env, chat.ID, correction, "")
	require.NoError(t, err)
	t.Logf("correction send: queued=%v", sent.Queued)

	call2 := testutil.RequireReceive(ctx, t, env.calls)
	t.Logf("first model call after the interrupt: has older=%v has correction=%v older precedes correction=%v",
		call2.has(older), call2.has(correction),
		bytes.Index(call2.Body, []byte(older)) < bytes.Index(call2.Body, []byte(correction)))
	t.Logf("queue after the first post-interrupt call started: %q", proofBQueuedTexts(ctx, t, env, chat.ID))
	require.False(t, call2.has(older),
		"the first model call after interrupt-then-send must not answer the older queued message")
	require.True(t, call2.has(correction),
		"the first model call after interrupt-then-send must carry the correction")
}

// TestProofBSteerThenPromoteDeliversCorrectionFirst sends the correction
// as a steer message and promotes it, which steers and interrupts in one
// transaction. The interruption delivers the steer lane, so the
// correction reaches the first model call after the interrupt and the
// older queue-lane message stays queued until that turn ends.
func TestProofBSteerThenPromoteDeliversCorrectionFirst(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	env := proofBSetup(t)
	id := uuid.NewString()
	initial, older, correction := "proofB-initial-"+id, "proofB-older-queued-"+id, "proofB-correction-"+id

	chat := proofBCreateChat(ctx, t, env, initial)
	call1 := testutil.RequireReceive(ctx, t, env.calls)
	require.True(t, call1.has(initial))

	queued, err := proofBSend(ctx, env, chat.ID, older, codersdk.ChatBusyBehaviorQueue)
	require.NoError(t, err)
	require.True(t, queued.Queued)

	steered, err := proofBSend(ctx, env, chat.ID, correction, "")
	require.NoError(t, err)
	require.True(t, steered.Queued)
	require.NotNil(t, steered.QueuedMessage)
	require.Equal(t, codersdk.ChatBusyBehaviorSteer, steered.QueuedMessage.BusyBehavior)

	res, err := env.client.Request(ctx, http.MethodPost,
		fmt.Sprintf("/api/v2/chats/%s/queue/%d/promote", chat.ID, steered.QueuedMessage.ID), nil)
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, http.StatusAccepted, res.StatusCode)

	call2 := testutil.RequireReceive(ctx, t, env.calls)
	t.Logf("first model call after the promote: has older=%v has correction=%v", call2.has(older), call2.has(correction))
	require.True(t, call2.has(correction), "the first model call after the promote carries the correction")
	require.False(t, call2.has(older), "the older queued message waits for the correction's turn")
	require.Equal(t, []string{older}, proofBQueuedTexts(ctx, t, env, chat.ID), "the older message stays queued")

	call2.text(t, "answered the correction")
	call3 := testutil.RequireReceive(ctx, t, env.calls)
	require.True(t, call3.has(older), "the older message is delivered when the correction's turn ends")
	call3.text(t, "answered the older message")
	proofBWaitStatus(ctx, t, env, chat.ID, proofBStatusIs(codersdk.ChatStatusWaiting))
}

// TestProofBSteerSendToFullBlockedQueue fills the queue to the default
// cap with real sends during a blocked model call, begins an edit on the
// head, and ends the turn so the chat is paused (P) or errored with the
// blocked head (E1P). A steer send then starts the steer lane when the
// head is a queue-lane row, so it is delivered at once and the queue
// does not grow. When the head is a steer row, the new row waits behind
// it and the cap applies.
func TestProofBSteerSendToFullBlockedQueue(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name      string
		lane      codersdk.ChatBusyBehavior
		failTurn  bool
		wantState codersdk.ChatStatus
		wantSent  bool
	}{
		{name: "PausedQueueLaneHead", lane: codersdk.ChatBusyBehaviorQueue, wantState: codersdk.ChatStatusPaused, wantSent: true},
		{name: "ErrorQueueLaneHead", lane: codersdk.ChatBusyBehaviorQueue, failTurn: true, wantState: codersdk.ChatStatusError, wantSent: true},
		{name: "PausedSteerLaneHead", lane: codersdk.ChatBusyBehaviorSteer, wantState: codersdk.ChatStatusPaused},
		{name: "ErrorSteerLaneHead", lane: codersdk.ChatBusyBehaviorSteer, failTurn: true, wantState: codersdk.ChatStatusError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitLong)
			env := proofBSetup(t)
			id := uuid.NewString()
			chat := proofBCreateChat(ctx, t, env, "proofB-initial-"+id)
			call1 := testutil.RequireReceive(ctx, t, env.calls)

			var head *codersdk.ChatQueuedMessage
			for i := range codersdk.DefaultChatMaxQueuedMessagesPerChat {
				resp, err := proofBSend(ctx, env, chat.ID, fmt.Sprintf("proofB-fill-%d-%s", i, id), tc.lane)
				require.NoError(t, err)
				require.True(t, resp.Queued)
				if head == nil {
					head = resp.QueuedMessage
				}
			}
			require.NoError(t, env.client.EditChatQueuedMessage(ctx, chat.ID, head.ID, codersdk.EditChatQueuedMessageRequest{Editing: ptr.Ref(true)}))

			if tc.failTurn {
				call1.fail(t)
			} else {
				call1.text(t, "turn done")
			}
			proofBWaitStatus(ctx, t, env, chat.ID, proofBStatusIs(tc.wantState))
			listed, err := env.client.GetChatMessages(ctx, chat.ID, nil)
			require.NoError(t, err)
			require.Len(t, listed.QueuedMessages, codersdk.DefaultChatMaxQueuedMessagesPerChat)
			require.Equal(t, head.ID, listed.QueuedMessages[0].ID)
			require.NotNil(t, listed.QueuedMessages[0].EditingSince)

			steer := "proofB-steer-" + id
			resp, err := proofBSend(ctx, env, chat.ID, steer, codersdk.ChatBusyBehaviorSteer)
			if !tc.wantSent {
				sdkErr := requireSDKError(t, err, http.StatusTooManyRequests)
				require.Equal(t, "Message queue is full.", sdkErr.Message)
				return
			}
			require.NoError(t, err, "a steer send behind a queue-lane head under edit is delivered at once")
			require.False(t, resp.Queued)
			call2 := testutil.RequireReceive(ctx, t, env.calls)
			require.True(t, call2.has(steer), "the chat runs with the steer message")
			require.Len(t, proofBQueuedTexts(ctx, t, env, chat.ID), codersdk.DefaultChatMaxQueuedMessagesPerChat,
				"the queue did not grow")
			call2.text(t, "steer answered")
		})
	}
}

// TestProofBSteerBatchKeepsPerMessageModel sends two steer messages with
// different model_config_id and reasoning_effort during one model call.
// Both are delivered together, so one model call answers both. The test
// asserts the non-surprising behavior that the first message is answered
// by its own model.
func TestProofBSteerBatchKeepsPerMessageModel(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	env := proofBSetup(t)
	modelB := proofBCreateModel(t, env, "proofb-model-b", false)
	id := uuid.NewString()
	s1, s2 := "proofB-s1-model-a-"+id, "proofB-s2-model-b-"+id

	chat := proofBCreateChat(ctx, t, env, "proofB-initial-"+id)
	call1 := testutil.RequireReceive(ctx, t, env.calls)
	require.Equal(t, "proofb-model-a", call1.Model)

	for _, m := range []struct {
		text   string
		model  uuid.UUID
		effort string
	}{{s1, env.model.ID, "high"}, {s2, modelB.ID, "low"}} {
		resp, err := env.client.CreateChatMessage(ctx, chat.ID, codersdk.CreateChatMessageRequest{
			Content:         []codersdk.ChatInputPart{{Type: codersdk.ChatInputPartTypeText, Text: m.text}},
			ModelConfigID:   ptr.Ref(m.model),
			ReasoningEffort: ptr.Ref(m.effort),
		})
		require.NoError(t, err)
		require.True(t, resp.Queued)
		require.Equal(t, codersdk.ChatBusyBehaviorSteer, resp.QueuedMessage.BusyBehavior)
	}

	call1.text(t, "first turn done")
	call2 := testutil.RequireReceive(ctx, t, env.calls)
	got, err := env.client.GetChat(ctx, chat.ID)
	require.NoError(t, err)
	effort := "<nil>"
	if got.LastReasoningEffort != nil {
		effort = *got.LastReasoningEffort
	}
	t.Logf("next model call: model=%q has s1=%v has s2=%v; chat last_model_config_id is model B=%v, last_reasoning_effort=%s",
		call2.Model, call2.has(s1), call2.has(s2), got.LastModelConfigID == modelB.ID, effort)
	messages, err := env.client.GetChatMessages(ctx, chat.ID, nil)
	require.NoError(t, err)
	for _, msg := range messages.Messages {
		if msg.Role != codersdk.ChatMessageRoleUser || len(msg.Content) == 0 {
			continue
		}
		modelID := "<nil>"
		if msg.ModelConfigID != nil {
			modelID = msg.ModelConfigID.String()
		}
		t.Logf("history user message %q: model_config_id=%s", msg.Content[0].Text, modelID)
	}
	t.Logf("model A=%s model B=%s", env.model.ID, modelB.ID)
	t.Logf("next model call body has effort \"high\"=%v \"low\"=%v",
		call2.has(`"high"`), call2.has(`"low"`))

	require.True(t, call2.has(s1))
	require.Equal(t, "proofb-model-a", call2.Model,
		"the model call that first answers s1 uses s1's model")
}
