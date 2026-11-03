package coderd_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/aibridgedtest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/google/uuid"
)

// proofAReply is how the test answers a blocked model call: a
// list_templates tool call, which makes the turn take another step,
// or a final text answer.
type proofAReply struct {
	toolCall bool
}

// proofACall is one streaming model call held by the fake provider
// until the test replies or the worker cancels the request.
type proofACall struct {
	userTexts []string
	reply     chan proofAReply
	// outcome receives "replied" or "cancelled" once.
	outcome chan string
}

// proofAModel is a chattest OpenAI handler that blocks every streaming
// call until the test replies.
type proofAModel struct {
	calls chan *proofACall
	done  chan struct{}
}

func (m *proofAModel) handle(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
	if !req.Stream {
		return chattest.OpenAINonStreamingResponse(`{"title": "proof"}`)
	}
	call := &proofACall{
		reply:   make(chan proofAReply, 1),
		outcome: make(chan string, 1),
	}
	for _, msg := range req.Messages {
		if msg.Role == "user" {
			call.userTexts = append(call.userTexts, msg.Content)
		}
	}
	chunks := make(chan chattest.OpenAIChunk, 8)
	select {
	case <-m.done:
		close(chunks)
		return chattest.OpenAIResponse{StreamingChunks: chunks}
	case m.calls <- call:
	}
	go func() {
		defer close(chunks)
		select {
		case r := <-call.reply:
			call.outcome <- "replied"
			if r.toolCall {
				chunks <- chattest.OpenAIToolCallChunk("list_templates", `{}`)
				return
			}
			for _, c := range chattest.OpenAITextChunks("done") {
				chunks <- c
			}
		case <-req.Context().Done():
			call.outcome <- "cancelled"
		case <-m.done:
		}
	}()
	return chattest.OpenAIResponse{StreamingChunks: chunks}
}

func (m *proofAModel) next(ctx context.Context, t *testing.T) *proofACall {
	t.Helper()
	return testutil.TryReceive(ctx, t, m.calls)
}

// proofASetup starts coderd with the real chatd worker and a model whose
// streaming calls block until the test replies. It returns a running
// chat whose first model call is in flight.
func proofASetup(ctx context.Context, t *testing.T) (*codersdk.ExperimentalClient, *proofAModel, codersdk.Chat, *proofACall) {
	t.Helper()
	model := &proofAModel{calls: make(chan *proofACall, 16), done: make(chan struct{})}
	baseURL := chattest.NewOpenAI(t, model.handle)
	client, _, api := newChatClientWithoutAIBridge(t)
	user := coderdtest.CreateFirstUser(t, client.Client)
	_ = createChatModelWithBaseURL(t, client, baseURL)
	aibridgedtest.StartTestAIBridgeDaemon(t.Context(), t, api, nil)
	// Registered last so it runs first and unblocks held calls.
	t.Cleanup(func() { close(model.done) })

	chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
		OrganizationID: user.OrganizationID,
		Content:        []codersdk.ChatInputPart{{Type: codersdk.ChatInputPartTypeText, Text: "hello"}},
	})
	require.NoError(t, err)
	first := model.next(ctx, t)
	return client, model, chat, first
}

// proofASend sends text with the given busy behavior ("" is the default)
// and returns the queued row.
func proofASend(ctx context.Context, t *testing.T, client *codersdk.ExperimentalClient, chatID uuid.UUID, text string, behavior codersdk.ChatBusyBehavior) codersdk.ChatQueuedMessage {
	t.Helper()
	resp, err := client.CreateChatMessage(ctx, chatID, codersdk.CreateChatMessageRequest{
		Content:      []codersdk.ChatInputPart{{Type: codersdk.ChatInputPartTypeText, Text: text}},
		BusyBehavior: behavior,
	})
	require.NoError(t, err)
	require.True(t, resp.Queued, "send %q to a running chat must queue", text)
	require.NotNil(t, resp.QueuedMessage)
	return *resp.QueuedMessage
}

func proofAQueueRequest(ctx context.Context, t *testing.T, client *codersdk.ExperimentalClient, method string, chatID uuid.UUID, queuedID int64, suffix string) (int, string) {
	t.Helper()
	res, err := client.Request(ctx, method, fmt.Sprintf("/api/v2/chats/%s/queue/%d%s", chatID, queuedID, suffix), nil)
	require.NoError(t, err)
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	require.NoError(t, err)
	return res.StatusCode, strings.Join(strings.Fields(string(body)), " ")
}

func proofAPromote(ctx context.Context, t *testing.T, client *codersdk.ExperimentalClient, chatID uuid.UUID, queuedID int64) (int, string) {
	t.Helper()
	return proofAQueueRequest(ctx, t, client, http.MethodPost, chatID, queuedID, "/promote")
}

func proofADelete(ctx context.Context, t *testing.T, client *codersdk.ExperimentalClient, chatID uuid.UUID, queuedID int64) (int, string) {
	t.Helper()
	return proofAQueueRequest(ctx, t, client, http.MethodDelete, chatID, queuedID, "")
}

func proofAQueue(ctx context.Context, t *testing.T, client *codersdk.ExperimentalClient, chatID uuid.UUID) []codersdk.ChatQueuedMessage {
	t.Helper()
	res, err := client.GetChatMessages(ctx, chatID, nil)
	require.NoError(t, err)
	return res.QueuedMessages
}

func proofAQueueIDs(queue []codersdk.ChatQueuedMessage) []int64 {
	ids := make([]int64, 0, len(queue))
	for _, q := range queue {
		ids = append(ids, q.ID)
	}
	return ids
}

func proofAStatus(ctx context.Context, t *testing.T, client *codersdk.ExperimentalClient, chatID uuid.UUID) codersdk.ChatStatus {
	t.Helper()
	chat, err := client.GetChat(ctx, chatID)
	require.NoError(t, err)
	return chat.Status
}

// proofAWaitStatus waits until the chat status satisfies ok and returns it.
func proofAWaitStatus(ctx context.Context, t *testing.T, client *codersdk.ExperimentalClient, chatID uuid.UUID, ok func(codersdk.ChatStatus) bool) codersdk.ChatStatus {
	t.Helper()
	var status codersdk.ChatStatus
	require.True(t, testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		chat, err := client.GetChat(ctx, chatID)
		if err != nil {
			return false
		}
		status = chat.Status
		return ok(status)
	}, testutil.IntervalFast))
	return status
}

func proofANot(s codersdk.ChatStatus) func(codersdk.ChatStatus) bool {
	return func(got codersdk.ChatStatus) bool { return got != s }
}

// Item 1. Owner sends S1 (steer), begins editing S1, sends S2 (steer),
// then presses Send now on S2 while the turn's model call is in flight.
// If Send now interrupts the turn, S2 must be delivered; otherwise the
// in-flight model call must be left alone.
func TestProofSendNowBehindEditedSteerHead(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	client, _, chat, first := proofASetup(ctx, t)

	s1 := proofASend(ctx, t, client, chat.ID, "s1", "")
	require.Equal(t, codersdk.ChatBusyBehaviorSteer, s1.BusyBehavior)
	require.NoError(t, client.EditChatQueuedMessage(ctx, chat.ID, s1.ID, codersdk.EditChatQueuedMessageRequest{Editing: ptr.Ref(true)}))
	s2 := proofASend(ctx, t, client, chat.ID, "s2", "")
	require.Equal(t, codersdk.ChatBusyBehaviorSteer, s2.BusyBehavior)

	queue := proofAQueue(ctx, t, client, chat.ID)
	require.Equal(t, []int64{s1.ID, s2.ID}, proofAQueueIDs(queue))
	require.NotNil(t, queue[0].EditingSince, "S1 is under edit")
	require.Equal(t, codersdk.ChatStatusRunning, proofAStatus(ctx, t, client, chat.ID))

	code, body := proofAPromote(ctx, t, client, chat.ID, s2.ID)
	require.Equal(t, http.StatusAccepted, code, body)

	status := proofAWaitStatus(ctx, t, client, chat.ID, proofANot(codersdk.ChatStatusInterrupting))
	queued := proofAQueueIDs(proofAQueue(ctx, t, client, chat.ID))
	t.Logf("after Send now on S2: status=%s queue=%v (S1=%d S2=%d)", status, queued, s1.ID, s2.ID)
	if slices.Contains(queued, s2.ID) {
		if status != codersdk.ChatStatusRunning {
			t.Logf("first model call outcome: %s", testutil.TryReceive(ctx, t, first.outcome))
		}
		require.Equal(t, codersdk.ChatStatusRunning, status,
			"Send now on S2 interrupted the turn but delivered nothing: chat is %s with S2 still queued", status)
		first.reply <- proofAReply{}
		require.Equal(t, "replied", testutil.TryReceive(ctx, t, first.outcome),
			"S2 was not delivered, so Send now must not cancel the in-flight model call")
	}
}

// Item 8 (question). Send now on a row already in the deliverable steer
// lane interrupts the running chat, although the row would be delivered
// before the next model call anyway. This characterization asserts that
// Send now leaves the in-flight call alone.
func TestProofSendNowOnDeliverableSteerRow(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	client, model, chat, first := proofASetup(ctx, t)

	s := proofASend(ctx, t, client, chat.ID, "s", "")
	require.Equal(t, codersdk.ChatBusyBehaviorSteer, s.BusyBehavior)

	code, body := proofAPromote(ctx, t, client, chat.ID, s.ID)
	require.Equal(t, http.StatusAccepted, code, body)

	// Read status first: once FinishInterruption runs, S has left the
	// queue, so "running with S queued" holds only if nothing
	// interrupted the turn.
	status := proofAStatus(ctx, t, client, chat.ID)
	queued := proofAQueueIDs(proofAQueue(ctx, t, client, chat.ID))
	t.Logf("after Send now on S: status=%s queue=%v (S=%d)", status, queued, s.ID)
	if status != codersdk.ChatStatusRunning || !slices.Contains(queued, s.ID) {
		t.Logf("first model call outcome: %s", testutil.TryReceive(ctx, t, first.outcome))
	}
	require.Equal(t, codersdk.ChatStatusRunning, status, "Send now on a deliverable steer row interrupted the turn")
	require.Contains(t, queued, s.ID, "S was delivered by an interruption, not at the next model call")

	first.reply <- proofAReply{toolCall: true}
	require.Equal(t, "replied", testutil.TryReceive(ctx, t, first.outcome))
	second := model.next(ctx, t)
	require.Contains(t, second.userTexts, "s", "S is delivered before the next model call")
	second.reply <- proofAReply{}
}

// Item 4 (server side). The worker delivers the last steer row S before
// the next model call; that call is held so the chat stays running.
// Send now and Delete with S's id must return 404, the status the site
// tolerates for a row that is gone.
func TestProofQueuedRowDeliveredMidTurnNotFound(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		// keepQueued sends a queue row that stays queued after S is
		// delivered, so the chat is R1 instead of R0.
		keepQueued bool
	}{
		{name: "EmptyQueueR0"},
		{name: "RowsRemainR1", keepQueued: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitLong)
			client, model, chat, first := proofASetup(ctx, t)

			s := proofASend(ctx, t, client, chat.ID, "s", "")
			require.Equal(t, codersdk.ChatBusyBehaviorSteer, s.BusyBehavior)
			remaining := []int64{}
			if tc.keepQueued {
				q := proofASend(ctx, t, client, chat.ID, "q", codersdk.ChatBusyBehaviorQueue)
				remaining = append(remaining, q.ID)
			}

			first.reply <- proofAReply{toolCall: true}
			require.Equal(t, "replied", testutil.TryReceive(ctx, t, first.outcome))
			second := model.next(ctx, t)
			require.Contains(t, second.userTexts, "s", "the worker delivered S before the second model call")

			require.Equal(t, codersdk.ChatStatusRunning, proofAStatus(ctx, t, client, chat.ID))
			require.Equal(t, remaining, proofAQueueIDs(proofAQueue(ctx, t, client, chat.ID)))

			promoteCode, promoteBody := proofAPromote(ctx, t, client, chat.ID, s.ID)
			deleteCode, deleteBody := proofADelete(ctx, t, client, chat.ID, s.ID)
			t.Logf("promote delivered S: %d %s", promoteCode, promoteBody)
			t.Logf("delete delivered S: %d %s", deleteCode, deleteBody)
			assert.Equal(t, http.StatusNotFound, promoteCode, "Send now on a delivered row")
			assert.Equal(t, http.StatusNotFound, deleteCode, "Delete on a delivered row")

			second.reply <- proofAReply{}
		})
	}
}

// Item 9 (doc). ARCHITECTURE.md says a steer send from R1P makes the new
// message the queue head, so the chat lands in R1. With a steer row S1
// under edit at the head, a new steer send S2 queues behind S1: S2 is not
// delivered at the next model call and the chat stays R1P.
func TestProofSteerSendBehindEditedSteerHeadStaysPaused(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	client, model, chat, first := proofASetup(ctx, t)

	s1 := proofASend(ctx, t, client, chat.ID, "s1", "")
	require.NoError(t, client.EditChatQueuedMessage(ctx, chat.ID, s1.ID, codersdk.EditChatQueuedMessageRequest{Editing: ptr.Ref(true)}))
	s2 := proofASend(ctx, t, client, chat.ID, "s2", "")
	require.Equal(t, codersdk.ChatBusyBehaviorSteer, s2.BusyBehavior)

	first.reply <- proofAReply{toolCall: true}
	require.Equal(t, "replied", testutil.TryReceive(ctx, t, first.outcome))
	second := model.next(ctx, t)
	t.Logf("second model call user texts: %q", second.userTexts)
	require.NotContains(t, second.userTexts, "s2", "S2 is not delivered at the next model call")
	require.NotContains(t, second.userTexts, "s1")

	queue := proofAQueue(ctx, t, client, chat.ID)
	t.Logf("queue at second model call: %v, head editing_since=%v", proofAQueueIDs(queue), queue[0].EditingSince)
	require.Equal(t, []int64{s1.ID, s2.ID}, proofAQueueIDs(queue), "S1 stays the queue head")
	require.NotNil(t, queue[0].EditingSince, "the head is still under edit (R1P)")
	require.Equal(t, codersdk.ChatStatusRunning, proofAStatus(ctx, t, client, chat.ID))

	second.reply <- proofAReply{}
	status := proofAWaitStatus(ctx, t, client, chat.ID, proofANot(codersdk.ChatStatusRunning))
	t.Logf("status after the turn ends: %s", status)
	require.Equal(t, codersdk.ChatStatusPaused, status, "R1P ends the turn in P")
}
