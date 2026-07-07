package chatd_test

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/x/chatd"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func TestActiveServer_RetryStatePersistedDuringBackoff(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	db, ps := dbtestutil.NewDB(t)
	clock := quartz.NewMock(t).WithLogger(quartz.NoOpLogger)
	sink := testutil.NewFakeSink(t)
	var calls atomic.Int32
	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("title")
		}
		if calls.Add(1) == 1 {
			return chattest.OpenAIRateLimitResponse()
		}
		return chattest.OpenAIStreamingResponse(openAITextChunksWithStop("recovered")...)
	})
	user, org, model := seedChatDependenciesWithProvider(t, db, "openai", openAIURL)
	factory := chattest.NewMockAIBridgeTransport(t, openAIURL)
	server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
		cfg.Clock = clock
		cfg.Logger = sink.Logger()
		cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(factory)
	})

	chat := createChatThroughServer(ctx, t, db, server, org.ID, user.ID, model.ID, "hello")
	withRetry := waitForChatRetryState(ctx, t, db, chat.ID)
	require.Equal(t, database.ChatStatusRunning, withRetry.Status)
	require.True(t, withRetry.RetryState.Valid)
	require.Equal(t, withRetry.SnapshotVersion, withRetry.RetryStateVersion)
	require.Equal(t, int64(1), withRetry.GenerationAttempt)

	var retryPayload codersdk.ChatStreamRetry
	require.NoError(t, json.Unmarshal(withRetry.RetryState.RawMessage, &retryPayload))
	require.Equal(t, 1, retryPayload.Attempt)
	require.Equal(t, int64(1000), retryPayload.DelayMs)
	require.Equal(t, "OpenAI is rate limiting requests.", retryPayload.Error)
	require.Equal(t, codersdk.ChatErrorKindRateLimit, retryPayload.Kind)
	require.Equal(t, "openai", retryPayload.Provider)
	require.Equal(t, 429, retryPayload.StatusCode)
	require.False(t, retryPayload.RetryingAt.IsZero())

	advanceToNextTimer(ctx, clock)
	advanceUntilProviderCall(ctx, clock, &calls, 2)
	waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusWaiting)
	require.Equal(t, int32(2), calls.Load())
	latest, err := db.GetChatByID(ctx, chat.ID)
	require.NoError(t, err)
	require.False(t, latest.RetryState.Valid)
	entries := retryEntriesWithMessage(sink, "chat generation retrying")
	require.Len(t, entries, 1)
	require.Equal(t, "generate_assistant", retrySinkFieldValue(t, entries[0].Fields, "action"))
	require.Equal(t, "openai", retrySinkFieldValue(t, entries[0].Fields, "provider"))
	require.Equal(t, "429", retrySinkFieldValue(t, entries[0].Fields, "status_code"))
	require.Greater(t, latest.RetryStateVersion, withRetry.RetryStateVersion)
	messages, err := db.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{ChatID: chat.ID})
	require.NoError(t, err)
	requireTextPart(t, messages[len(messages)-1], "recovered")
}

func TestActiveServer_RetryStreamSilenceTimeoutAndClassification(t *testing.T) {
	t.Parallel()

	t.Run("rate limit retry recovers and records metric", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		db, ps := dbtestutil.NewDB(t)
		reg := prometheus.NewRegistry()
		var calls atomic.Int32
		openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
			if !req.Stream {
				return chattest.OpenAINonStreamingResponse("title")
			}
			if calls.Add(1) == 1 {
				return chattest.OpenAIRateLimitResponse()
			}
			return chattest.OpenAIStreamingResponse(openAITextChunksWithStop("recovered")...)
		})
		user, org, _ := seedChatDependenciesWithProvider(t, db, "openai", openAIURL)
		model := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{
			Model:     "gpt-4o",
			Enabled:   true,
			CreatedBy: uuid.NullUUID{UUID: user.ID, Valid: true},
			UpdatedBy: uuid.NullUUID{UUID: user.ID, Valid: true},
		})
		factory := chattest.NewMockAIBridgeTransport(t, openAIURL)
		server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
			cfg.PrometheusRegistry = reg
			cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(factory)
		})

		chat := createChatThroughServer(ctx, t, db, server, org.ID, user.ID, model.ID, "hello")
		waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusWaiting)
		require.Equal(t, int32(2), calls.Load())
		messages, err := db.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{ChatID: chat.ID})
		require.NoError(t, err)
		requireTextPart(t, messages[len(messages)-1], "recovered")
		requireRetryCounter(t, reg, "coderd_chatd_stream_retries_total", 1, map[string]string{
			"provider": "openai",
			"model":    "gpt-4o",
			"kind":     string(codersdk.ChatErrorKindRateLimit),
		})
	})

	t.Run("silent stream generation retry recovers", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		db, ps := dbtestutil.NewDB(t)
		reg := prometheus.NewRegistry()
		clock := quartz.NewMock(t).WithLogger(quartz.NoOpLogger)
		streamGuardTrap := clock.Trap().AfterFunc("streamSilenceGuard")
		defer streamGuardTrap.Close()
		retryTrap := clock.Trap().NewTimer("chatworker", "generation-retry")
		defer retryTrap.Close()
		sink := testutil.NewFakeSink(t)
		var calls atomic.Int32
		openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
			if !req.Stream {
				return chattest.OpenAINonStreamingResponse("title")
			}
			if calls.Add(1) == 1 {
				<-req.Request.Context().Done()
				return chattest.OpenAIStreamingResponse(openAITextChunksWithStop("timed out")...)
			}
			return chattest.OpenAIStreamingResponse(openAITextChunksWithStop("recovered")...)
		})
		user, org, model := seedChatDependenciesWithProvider(t, db, "openai", openAIURL)
		factory := chattest.NewMockAIBridgeTransport(t, openAIURL)
		server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
			cfg.Clock = clock
			cfg.Logger = sink.Logger()
			cfg.PrometheusRegistry = reg
			cfg.PendingChatAcquireInterval = 30 * time.Minute
			cfg.ChatHeartbeatInterval = 30 * time.Minute
			cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(factory)
		})

		chat := createChatThroughServer(ctx, t, db, server, org.ID, user.ID, model.ID, "hello")
		firstGuard := streamGuardTrap.MustWait(ctx)
		firstGuard.MustRelease(ctx)
		waitUntilProviderCall(ctx, t, &calls, 1)
		advanceMockClockBy(ctx, t, clock, firstGuard.Duration)
		retryTimer := retryTrap.MustWait(ctx)
		retryTimer.MustRelease(ctx)
		advanceMockClockBy(ctx, t, clock, retryTimer.Duration)
		secondGuard := streamGuardTrap.MustWait(ctx)
		secondGuard.MustRelease(ctx)
		waitUntilProviderCall(ctx, t, &calls, 2)
		waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusWaiting)
		require.Equal(t, int32(2), calls.Load())
		messages, err := db.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{ChatID: chat.ID})
		require.NoError(t, err)
		requireTextPart(t, messages[len(messages)-1], "recovered")
		require.Empty(t, retryEntriesWithMessage(sink, "chatworker task retrying"))
		entries := retryEntriesWithMessage(sink, "chat generation retrying")
		require.NotEmpty(t, entries)
		require.Equal(t, "generate_assistant", retrySinkFieldValue(t, entries[0].Fields, "action"))
		require.Equal(t, string(codersdk.ChatErrorKindStreamSilenceTimeout), retrySinkFieldValue(t, entries[0].Fields, "error_kind"))
		require.Equal(t, "openai", retrySinkFieldValue(t, entries[0].Fields, "provider"))
		requireRetryCounter(t, reg, "coderd_chatd_stream_retries_total", 1, map[string]string{
			"provider": "openai",
			"model":    model.Model,
			"kind":     string(codersdk.ChatErrorKindStreamSilenceTimeout),
		})
	})
}

func retryEntriesWithMessage(sink *testutil.FakeSink, message string) []slog.SinkEntry {
	return sink.Entries(func(e slog.SinkEntry) bool { return e.Message == message })
}

func retrySinkFieldValue(t *testing.T, fields slog.Map, name string) string {
	t.Helper()
	value, ok := sinkFieldValue(fields, name)
	require.True(t, ok, "missing log field %q", name)
	return fmt.Sprint(value)
}

func requireRetryCounter(t *testing.T, reg *prometheus.Registry, name string, wantValue float64, wantLabels map[string]string) {
	t.Helper()
	require.True(t, hasRetryCounter(t, reg, name, wantValue, wantLabels), "metric %s not found", name)
}

func hasRetryCounter(t *testing.T, reg *prometheus.Registry, name string, wantValue float64, wantLabels map[string]string) bool {
	t.Helper()

	families, err := reg.Gather()
	require.NoError(t, err)
	for _, family := range families {
		if family.GetName() != name {
			continue
		}
		for _, metric := range family.GetMetric() {
			if metric.GetCounter().GetValue() != wantValue {
				continue
			}
			labels := map[string]string{}
			for _, label := range metric.GetLabel() {
				labels[label.GetName()] = label.GetValue()
			}
			matches := true
			for key, want := range wantLabels {
				if labels[key] != want {
					matches = false
					break
				}
			}
			if matches {
				return true
			}
		}
		return false
	}
	return false
}

func waitForChatRetryState(ctx context.Context, t *testing.T, db database.Store, chatID uuid.UUID) database.Chat {
	t.Helper()
	var chat database.Chat
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		latest, err := db.GetChatByID(ctx, chatID)
		if err != nil {
			return false
		}
		chat = latest
		return latest.RetryState.Valid
	}, testutil.IntervalFast)
	return chat
}

func waitUntilProviderCall(ctx context.Context, t *testing.T, calls *atomic.Int32, want int32) {
	t.Helper()
	testutil.Eventually(ctx, t, func(context.Context) bool {
		return calls.Load() >= want
	}, testutil.IntervalFast)
}

func advanceMockClockBy(ctx context.Context, t *testing.T, clock *quartz.Mock, d time.Duration) {
	t.Helper()
	for remaining := d; remaining > 0; {
		next, ok := clock.Peek()
		require.True(t, ok, "no pending clock event while advancing %s", remaining)
		if next > remaining {
			clock.Advance(remaining).MustWait(ctx)
			return
		}
		_, waiter := clock.AdvanceNext()
		waiter.MustWait(ctx)
		remaining -= next
	}
}

func advanceUntilProviderCall(ctx context.Context, clock *quartz.Mock, calls *atomic.Int32, want int32) {
	for calls.Load() < want {
		advanceToNextTimer(ctx, clock)
	}
}

func advanceToNextTimer(ctx context.Context, clock *quartz.Mock) {
	_, waiter := clock.AdvanceNext()
	waiter.MustWait(ctx)
}

func openAITextChunksWithStop(deltas ...string) []chattest.OpenAIChunk {
	chunks := chattest.OpenAITextChunks(deltas...)
	if len(chunks) == 0 {
		return nil
	}
	chunks[len(chunks)-1].Choices[0].FinishReason = "stop"
	return chunks
}
