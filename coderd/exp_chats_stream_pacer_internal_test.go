package coderd

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
)

// paceSimLink is a bottleneck link with a FIFO queue in front of it. Data
// and ping frames share the queue, and the client answers a ping after it
// has read every byte before it.
type paceSimLink struct {
	now       time.Time
	rate      float64 // bytes per second
	rtt       time.Duration
	busyUntil time.Time
	// maxQueued is the most bytes queued at once, which a WSWatcher ping
	// sent at that moment would wait behind.
	maxQueued float64
	delivered []paceSimDelivery
	pongs     []paceSimPong
	state     chatStreamPaceState
}

type paceSimDelivery struct {
	at    time.Time
	bytes int
}

type paceSimPong struct {
	at   time.Time
	ping chatStreamPing
}

func newPaceSimLink(rate float64, rtt time.Duration) *paceSimLink {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	return &paceSimLink{now: start, busyUntil: start, rate: rate, rtt: rtt}
}

func (l *paceSimLink) seconds(bytes float64) time.Duration {
	return time.Duration(bytes / l.rate * float64(time.Second))
}

func (l *paceSimLink) queued() float64 {
	if !l.busyUntil.After(l.now) {
		return 0
	}
	return l.busyUntil.Sub(l.now).Seconds() * l.rate
}

func (l *paceSimLink) transmit(n int) {
	start := l.busyUntil
	if l.now.After(start) {
		start = l.now
	}
	l.busyUntil = start.Add(l.seconds(float64(n)))
	l.maxQueued = max(l.maxQueued, l.queued())
	l.delivered = append(l.delivered, paceSimDelivery{at: l.busyUntil.Add(l.rtt / 2), bytes: n})
}

func (l *paceSimLink) startPings() {
	for l.state.shouldPing() {
		ping := l.state.startPing(l.now)
		at := l.busyUntil
		if l.now.After(at) {
			at = l.now
		}
		l.pongs = append(l.pongs, paceSimPong{at: at.Add(l.rtt), ping: ping})
	}
}

// nextPong delivers the earliest pong and reports whether there was one.
func (l *paceSimLink) nextPong() bool {
	if len(l.pongs) == 0 {
		return false
	}
	p := l.pongs[0]
	l.pongs = l.pongs[1:]
	if p.at.After(l.now) {
		l.now = p.at
	}
	l.state.pong(p.ping, l.now)
	l.startPings()
	return true
}

// idle lets d pass with nothing to write.
func (l *paceSimLink) idle(d time.Duration) {
	until := l.now.Add(d)
	for len(l.pongs) > 0 && !l.pongs[0].at.After(until) {
		l.nextPong()
	}
	l.now = until
}

// write writes messages back to back, waiting for pongs while the window
// is full.
func (l *paceSimLink) write(t *testing.T, messages []int) {
	t.Helper()
	for _, n := range messages {
		l.state.beginWrite(l.now)
		for !l.state.admit(int64(n)) {
			require.True(t, l.nextPong(), "window full with no ping in flight")
		}
		l.transmit(n)
		l.state.wrote(int64(n), l.now)
		l.startPings()
	}
	for l.nextPong() {
	}
}

// throughput is the delivery rate over the second half of the bytes, which
// leaves out the window's growth from its minimum.
func (l *paceSimLink) throughput() float64 {
	total := 0
	for _, d := range l.delivered {
		total += d.bytes
	}
	var half time.Time
	sum, counted := 0, 0
	for _, d := range l.delivered {
		sum += d.bytes
		if sum < total/2 {
			continue
		}
		if half.IsZero() {
			half = d.at
			continue
		}
		counted += d.bytes
	}
	return float64(counted) / l.delivered[len(l.delivered)-1].at.Sub(half).Seconds()
}

// imageChunkMessages returns the message sizes writeChatStreamBatch writes
// for a batch holding a 10 MiB inline image.
func imageChunkMessages(t *testing.T) []int {
	t.Helper()
	raw := make([]byte, codersdk.MaxChatFileSizeBytes)
	_, _ = rand.Read(raw)
	result, err := json.Marshal(map[string]string{"data": base64.StdEncoding.EncodeToString(raw), "mime_type": "image/png"})
	require.NoError(t, err)
	part := codersdk.ChatMessageToolResult("call", "computer", result, false, true)
	batch := []codersdk.ChatStreamEvent{{
		Type:        codersdk.ChatStreamEventTypeMessagePart,
		MessagePart: &codersdk.ChatStreamMessagePart{Part: part},
	}}
	var rec paceSimRecorder
	require.NoError(t, writeChatStreamBatch(&rec, batch, codersdk.ChatStreamChunkBytes))
	require.Greater(t, len(rec), 40)
	return rec
}

type paceSimRecorder []int

func (r *paceSimRecorder) Write(b []byte) (int, error) {
	*r = append(*r, len(b))
	return len(b), nil
}

func TestChatStreamPacer(t *testing.T) {
	t.Parallel()

	messages := imageChunkMessages(t)
	largest := 0
	for _, n := range messages {
		largest = max(largest, n)
	}
	const mbit = 1e6 / 8

	t.Run("FastLinkThroughput", func(t *testing.T) {
		t.Parallel()
		for _, rtt := range []time.Duration{100 * time.Millisecond, 600 * time.Millisecond, 2 * time.Second, 4 * time.Second} {
			t.Run(rtt.String(), func(t *testing.T) {
				t.Parallel()
				link := newPaceSimLink(100*mbit, rtt)
				var long []int
				for range 40 {
					long = append(long, messages...)
				}
				link.write(t, long)
				require.GreaterOrEqual(t, link.throughput(), 0.9*link.rate)
			})
		}
	})

	t.Run("SlowLinkQueue", func(t *testing.T) {
		t.Parallel()
		for _, rtt := range []time.Duration{0, 600 * time.Millisecond} {
			t.Run(rtt.String(), func(t *testing.T) {
				t.Parallel()
				link := newPaceSimLink(mbit, rtt)
				link.write(t, messages)
				require.LessOrEqual(t, link.maxQueued, float64(chatStreamPacerMinWindow))
				// A WSWatcher ping waits behind the queue and one message.
				pongDelay := link.seconds(link.maxQueued+float64(largest)) + rtt
				require.Less(t, pongDelay, 15*time.Second)
			})
		}
	})

	t.Run("RateDropAfterIdle", func(t *testing.T) {
		t.Parallel()
		link := newPaceSimLink(100*mbit, 100*time.Millisecond)
		link.write(t, messages)
		require.Greater(t, link.state.window(), int64(10*chatStreamPacerMinWindow))
		link.idle(chatStreamPacerTarget)
		link.rate = mbit
		link.maxQueued = 0
		link.write(t, messages)
		require.LessOrEqual(t, link.maxQueued, float64(chatStreamPacerMinWindow))
	})

	t.Run("AppLimitedSampleDoesNotLowerRate", func(t *testing.T) {
		t.Parallel()
		link := newPaceSimLink(100*mbit, 100*time.Millisecond)
		link.write(t, messages)
		rate := link.state.rate
		// Small messages with pauses between them read slower than the link.
		for range 5 {
			link.idle(time.Second)
			link.write(t, []int{1024})
		}
		require.Equal(t, rate, link.state.rate)
	})
}
