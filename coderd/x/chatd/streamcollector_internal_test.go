package chatd

import (
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/x/chatd/chatloop"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

// TestStreamStateCollector exercises the four gauges emitted by
// streamStateCollector against representative map states.
func TestStreamStateCollector(t *testing.T) {
	t.Parallel()

	t.Run("EmptyMap", func(t *testing.T) {
		t.Parallel()

		reg := prometheus.NewRegistry()
		server := &Server{}
		reg.MustRegister(&streamStateCollector{server: server})

		assertGauges(t, reg, gaugeExpectations{
			active:      0,
			bufferMax:   0,
			bufferTotal: 0,
			subscribers: 0,
		})
	})

	t.Run("PopulatedMap", func(t *testing.T) {
		t.Parallel()

		reg := prometheus.NewRegistry()
		server := &Server{}

		server.chatStreams.Store(uuid.New(), &chatStreamState{
			buffer:      make([]codersdk.ChatStreamEvent, 10),
			subscribers: newSubscribers(t, 2),
		})
		server.chatStreams.Store(uuid.New(), &chatStreamState{
			buffer:      make([]codersdk.ChatStreamEvent, 25),
			subscribers: map[uuid.UUID]chan codersdk.ChatStreamEvent{},
		})
		server.chatStreams.Store(uuid.New(), &chatStreamState{
			buffer:      nil,
			subscribers: newSubscribers(t, 1),
		})

		reg.MustRegister(&streamStateCollector{server: server})

		assertGauges(t, reg, gaugeExpectations{
			active:      3,
			bufferMax:   25,
			bufferTotal: 35,
			subscribers: 3,
		})
	})

	t.Run("SkipsWrongType", func(t *testing.T) {
		t.Parallel()

		reg := prometheus.NewRegistry()
		server := &Server{}

		server.chatStreams.Store(uuid.New(), "garbage")
		server.chatStreams.Store(uuid.New(), &chatStreamState{
			buffer:      make([]codersdk.ChatStreamEvent, 5),
			subscribers: newSubscribers(t, 1),
		})

		reg.MustRegister(&streamStateCollector{server: server})

		// The non-matching entry is silently skipped. Only the
		// valid chatStreamState counts.
		assertGauges(t, reg, gaugeExpectations{
			active:      1,
			bufferMax:   5,
			bufferTotal: 5,
			subscribers: 1,
		})
	})

	// Runs Collect concurrently with state.mu mutations; catches
	// missing lock acquisition under `go test -race`.
	t.Run("LockContentionSmoke", func(t *testing.T) {
		t.Parallel()

		server := &Server{}
		state := &chatStreamState{
			buffer:      make([]codersdk.ChatStreamEvent, 0, 100),
			subscribers: newSubscribers(t, 1),
		}
		server.chatStreams.Store(uuid.New(), state)
		collector := &streamStateCollector{server: server}

		const iterations = 100
		var wg sync.WaitGroup

		// Mutator: grows and shrinks the buffer under state.mu.
		wg.Go(func() {
			for range iterations {
				state.mu.Lock()
				state.buffer = append(state.buffer, codersdk.ChatStreamEvent{})
				if len(state.buffer) > 50 {
					state.buffer = state.buffer[10:]
				}
				state.mu.Unlock()
			}
		})

		// Scraper: repeatedly invokes Collect into a discard
		// channel. A panic or race here fails the test.
		wg.Go(func() {
			ctx := testutil.Context(t, 10*time.Second)
			for range iterations {
				ch := make(chan prometheus.Metric, 4)
				collector.Collect(ch)
				// Drain all metrics the collector wrote.
				for range 4 {
					testutil.SoftTryReceive(ctx, t, ch)
				}
			}
		})

		wg.Wait()
	})
}

type gaugeExpectations struct {
	active      float64
	bufferMax   float64
	bufferTotal float64
	subscribers float64
}

func assertGauges(t *testing.T, reg *prometheus.Registry, want gaugeExpectations) {
	t.Helper()
	families, err := reg.Gather()
	require.NoError(t, err)

	got := map[string]float64{}
	for _, f := range families {
		require.Len(t, f.GetMetric(), 1, "metric %q should have exactly one sample", f.GetName())
		got[f.GetName()] = f.GetMetric()[0].GetGauge().GetValue()
	}

	assert.Equal(t, want.active, got["coderd_chatd_streams_active"], "streams_active")
	assert.Equal(t, want.bufferMax, got["coderd_chatd_stream_buffer_size_max"], "buffer_size_max")
	assert.Equal(t, want.bufferTotal, got["coderd_chatd_stream_buffer_events"], "buffer_events")
	assert.Equal(t, want.subscribers, got["coderd_chatd_stream_subscribers"], "subscribers")
}

func newSubscribers(t *testing.T, n int) map[uuid.UUID]chan codersdk.ChatStreamEvent {
	t.Helper()
	subs := make(map[uuid.UUID]chan codersdk.ChatStreamEvent, n)
	for range n {
		subs[uuid.New()] = make(chan codersdk.ChatStreamEvent, 1)
	}
	return subs
}

// TestStreamStateCollector_BufferDroppedIncrementsOnCapacity pre-fills
// a buffer to capacity and asserts stream_buffer_dropped_total
// increments on each subsequent publishToStream drop.
func TestStreamStateCollector_BufferDroppedIncrementsOnCapacity(t *testing.T) {
	t.Parallel()

	reg := prometheus.NewRegistry()
	server := &Server{
		logger:  slog.Make(),
		clock:   quartz.NewMock(t),
		metrics: chatloop.NewMetrics(reg),
	}

	chatID := uuid.New()
	server.chatStreams.Store(chatID, &chatStreamState{
		buffering: true,
		buffer:    make([]codersdk.ChatStreamEvent, maxStreamBufferSize),
	})

	partEvent := codersdk.ChatStreamEvent{
		Type:        codersdk.ChatStreamEventTypeMessagePart,
		MessagePart: &codersdk.ChatStreamMessagePart{},
	}

	server.publishToStream(chatID, partEvent)
	assert.Equal(t, float64(1), counterValue(t, reg, "coderd_chatd_stream_buffer_dropped_total"))

	server.publishToStream(chatID, partEvent)
	assert.Equal(t, float64(2), counterValue(t, reg, "coderd_chatd_stream_buffer_dropped_total"))
}

func counterValue(t *testing.T, reg *prometheus.Registry, name string) float64 {
	t.Helper()
	families, err := reg.Gather()
	require.NoError(t, err)
	for _, f := range families {
		if f.GetName() != name {
			continue
		}
		require.Len(t, f.GetMetric(), 1, "counter %q should have exactly one sample", name)
		return f.GetMetric()[0].GetCounter().GetValue()
	}
	t.Fatalf("counter %q not registered", name)
	return 0
}
