package chatd_test

import (
	"bytes"
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/x/chatd"
	"github.com/coder/coder/v2/coderd/x/chatd/chatloop"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

// proofCModel serves a two-step turn: the first streaming call blocks
// until release and then asks for a tool call, so the turn continues
// with a second model call. Each streaming request body is kept so the
// test can see which messages each call carried.
type proofCModel struct {
	started chan struct{}
	release chan struct{}
	calls   atomic.Int32
	mu      sync.Mutex
	bodies  [][]byte
}

func (m *proofCModel) respond(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
	if !req.Stream {
		return chattest.OpenAINonStreamingResponse("title")
	}
	m.mu.Lock()
	m.bodies = append(m.bodies, req.RawBody)
	m.mu.Unlock()
	if m.calls.Add(1) == 1 {
		close(m.started)
		select {
		case <-m.release:
		case <-req.Context().Done():
		}
		return chattest.OpenAIStreamingResponse(chattest.OpenAIToolCallChunk("list_templates", `{}`))
	}
	return chattest.OpenAIStreamingResponse(openAITextChunksWithStop("done")...)
}

func (m *proofCModel) streamBodies() [][]byte {
	m.mu.Lock()
	defer m.mu.Unlock()
	return slicesClone(m.bodies)
}

func slicesClone(in [][]byte) [][]byte {
	out := make([][]byte, len(in))
	copy(out, in)
	return out
}

type proofCHarness struct {
	db       database.Store
	server   *chatd.Server
	recorder *tracetest.SpanRecorder
	registry *prometheus.Registry
	model    *proofCModel
	chatID   [16]byte
}

// proofCSendDuringFirstCall starts a chat through the real worker,
// sends a second user message with busyBehavior through
// chatd.Server.SendMessage while the first model call is blocked, then
// lets the turn run to completion.
func proofCSendDuringFirstCall(
	ctx context.Context,
	t *testing.T,
	busyBehavior chatd.SendMessageBusyBehavior,
) (proofCHarness, chatd.SendMessageResult) {
	t.Helper()
	db, ps := dbtestutil.NewDB(t)
	provider, recorder := newRecordingTracerProvider(t)
	registry := prometheus.NewRegistry()
	model := &proofCModel{started: make(chan struct{}), release: make(chan struct{})}
	openAIURL := chattest.NewOpenAI(t, model.respond)
	user, org, modelConfig := seedChatDependenciesWithProvider(t, db, "openai", openAIURL)
	server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
		cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(chattest.NewMockAIBridgeTransport(t, openAIURL))
		cfg.TracerProvider = provider
		cfg.PrometheusRegistry = registry
	})

	chat := createChatThroughServer(ctx, t, db, server, org.ID, user.ID, modelConfig.ID, "hello")
	testutil.TryReceive(ctx, t, model.started)
	sent, err := server.SendMessage(ctx, chatd.SendMessageOptions{
		ChatID:       chat.ID,
		CreatedBy:    user.ID,
		Content:      []codersdk.ChatMessagePart{codersdk.ChatMessageText("proofc follow-up")},
		BusyBehavior: busyBehavior,
	})
	require.NoError(t, err)
	require.True(t, sent.Queued, "the chat is running, so the message is queued")
	require.NotNil(t, sent.QueuedMessage)
	close(model.release)
	waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusWaiting)

	return proofCHarness{
		db:       db,
		server:   server,
		recorder: recorder,
		registry: registry,
		model:    model,
		chatID:   chat.ID,
	}, sent
}

// proofCEndedSpans waits until n chat_turn spans have ended, because a
// turn span can end after the chat status is visible, and returns every
// ended span with the given name.
func proofCEndedSpans(ctx context.Context, t *testing.T, recorder *tracetest.SpanRecorder, turns int, stage chatloop.Stage) []sdktrace.ReadOnlySpan {
	t.Helper()
	testutil.Eventually(ctx, t, func(context.Context) bool {
		ended := 0
		for _, span := range recorder.Ended() {
			if span.Name() == string(chatloop.StageChatTurn) {
				ended++
			}
		}
		return ended >= turns
	}, testutil.IntervalFast)
	var found []sdktrace.ReadOnlySpan
	for _, span := range recorder.Ended() {
		if span.Name() == string(stage) {
			found = append(found, span)
		}
	}
	return found
}

func proofCStageObservations(t *testing.T, registry *prometheus.Registry, stage chatloop.Stage) uint64 {
	t.Helper()
	families, err := registry.Gather()
	require.NoError(t, err)
	var count uint64
	for _, family := range families {
		if family.GetName() != "coderd_chatd_stage_duration_seconds" {
			continue
		}
		for _, metric := range family.GetMetric() {
			for _, label := range metric.GetLabel() {
				if label.GetName() == "stage" && label.GetValue() == string(stage) {
					count += metric.GetHistogram().GetSampleCount()
				}
			}
		}
	}
	return count
}

// TestProofSteerDeliveredMidTurnRecordsQueueWait sends a steer message
// while the first model call of a two-step turn is blocked. The worker
// delivers the steer row at the next step boundary
// (beginAssistantGenerationAttempt), so the second model call carries
// it. Every other path that moves a queued row into history records a
// queue_wait span and stage_duration_seconds sample from the row's
// created_at; this one should too.
func TestProofSteerDeliveredMidTurnRecordsQueueWait(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	h, sent := proofCSendDuringFirstCall(ctx, t, chatd.SendMessageBusyBehaviorSteer)

	turns := proofCEndedSpans(ctx, t, h.recorder, 1, chatloop.StageChatTurn)
	// The delivered row's created_at is a newer turn trigger, so the
	// runner closes the first chat_turn span and opens another
	// (runnerTurnSpan.Ensure). The model calls show the delivery happened
	// mid-turn: no third call, and the second call carries the row.
	t.Logf("chat_turn spans: %d", len(turns))
	bodies := h.model.streamBodies()
	require.Len(t, bodies, 2)
	require.False(t, bytes.Contains(bodies[0], []byte("proofc follow-up")))
	require.True(t, bytes.Contains(bodies[1], []byte("proofc follow-up")), "the second model call carries the delivered steer row")

	queueWaits := proofCEndedSpans(ctx, t, h.recorder, 1, chatloop.StageQueueWait)
	require.Len(t, queueWaits, 1, "a delivered steer row records one queue_wait span")
	require.Equal(t, sent.QueuedMessage.CreatedAt.UTC(), queueWaits[0].StartTime().UTC())
	require.Equal(t, uint64(1), proofCStageObservations(t, h.registry, chatloop.StageQueueWait))
}

// TestProofQueuedRowPromotedAtTurnEndRecordsQueueWait is the contrast:
// the same message with busy_behavior=queue is promoted when the turn
// ends and records queue_wait.
func TestProofQueuedRowPromotedAtTurnEndRecordsQueueWait(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	h, sent := proofCSendDuringFirstCall(ctx, t, chatd.SendMessageBusyBehaviorQueue)

	turns := proofCEndedSpans(ctx, t, h.recorder, 2, chatloop.StageChatTurn)
	require.Len(t, turns, 2, "the queued row starts a second turn")
	bodies := h.model.streamBodies()
	require.Len(t, bodies, 3)
	require.False(t, bytes.Contains(bodies[1], []byte("proofc follow-up")), "the queued row waits for the turn to end")
	require.True(t, bytes.Contains(bodies[2], []byte("proofc follow-up")))

	queueWaits := proofCEndedSpans(ctx, t, h.recorder, 2, chatloop.StageQueueWait)
	require.Len(t, queueWaits, 1)
	require.Equal(t, sent.QueuedMessage.CreatedAt.UTC(), queueWaits[0].StartTime().UTC())
	require.Equal(t, uint64(1), proofCStageObservations(t, h.registry, chatloop.StageQueueWait))
}

var _ = time.Time{}
