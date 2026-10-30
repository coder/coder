package coderd

import (
	"context"
	"io"
	"sync"
	"time"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/quartz"
)

const (
	// chatStreamPacerTarget is how long the bytes queued ahead of a ping
	// may take to drain. WSWatcher fails a probe whose ping takes over
	// 15 s, which includes the round trip and up to 5 s waiting behind the
	// message being written.
	chatStreamPacerTarget = 4 * time.Second
	// chatStreamPacerMinWindow is two of the largest chunk messages, so one
	// can be written while the previous one is still unconfirmed.
	chatStreamPacerMinWindow = 2 * (2*codersdk.ChatStreamChunkBytes + 64)
	// chatStreamPacerMaxPings bounds the pings in flight. Pings are spaced a
	// quarter window apart, so about five are in flight on a full window.
	chatStreamPacerMaxPings = 8
)

// chatStreamPaceState bounds the bytes queued between the chat stream and
// its client, so that the WSWatcher ping, which waits behind every queued
// byte, gets its pong in time on a slow link with deep buffers.
//
// Contract. A pong for a ping sent when the stream had written offset bytes
// confirms that the client has read those bytes. The pong delay is
// RTT + queued/rate, and bytes in the network pipe are not queued. With
// unconfirmed bytes (written minus confirmed) at most
// window = rate * (RTT + target), the queue is at most rate * target, so a
// WSWatcher pong arrives within RTT + target plus the lock wait behind one
// message. Nothing else is limited: a link that drains the window keeps
// receiving data at its full rate.
//
// Estimates. RTT is the smallest pong delay since the stream started, which
// some ping measures with an empty queue, for example after a keepalive.
// The rate is the bytes read between two pongs divided by the time between
// them. When the application had nothing to write during that read, the
// sample is a lower bound, so it only raises the estimate. Two pongs for
// bytes written back to back arrive spaced by the time the link took to
// send the bytes between them, so their sample is the link rate even while
// the window holds the writer back.
//
// Assumptions. Write is not called concurrently and is the only writer of
// data messages. A ping records the offset when it starts; if a message is
// written before the ping frame, the pong confirms more than recorded, which
// overstates unconfirmed bytes now and the next rate sample by at most one
// message. The rate does not fall by more than about half within one window;
// a larger drop queues more than the target until the next pong. After the
// writer has been idle for target, the link may have slowed unseen, so the
// window returns to chatStreamPacerMinWindow and the queue is at most that.
//
// Cost. With pings a quarter window apart, the link stays busy while
// RTT < 3 * target. A transfer that starts at the minimum window, at stream
// start or after target of idle, waits about one round trip for the first
// two pongs, after which the window covers the link; in the tests a 14 MB
// batch at 100 Mbit/s takes 0.06 s longer at 100 ms RTT and 4 s longer at
// 4 s RTT. At 1 Mbit/s the window stays at the minimum, up to about 8.4 s of
// data, so with RTT and one message of lock wait the WSWatcher pong stays
// under 15 s down to about 0.84 Mbit/s. Each window/4 written, and each
// keepalive, sends one ping.
type chatStreamPaceState struct {
	written   int64
	confirmed int64
	rate      float64 // bytes per second; zero until the first sample
	rtt       time.Duration
	rttKnown  bool

	pings          []chatStreamPing
	lastPingOffset int64
	lastPong       time.Time
	// gapOffsets are offsets at which the writer paused long enough for
	// the queue to drain, so the read interval containing them is
	// app-limited.
	gapOffsets   []int64
	lastWriteEnd time.Time
}

type chatStreamPing struct {
	offset int64
	sentAt time.Time
}

func (s *chatStreamPaceState) window() int64 {
	if s.rate == 0 || !s.rttKnown {
		return chatStreamPacerMinWindow
	}
	w := int64(s.rate * (s.rtt + chatStreamPacerTarget).Seconds())
	return max(w, chatStreamPacerMinWindow)
}

// beginWrite records the pause since the previous write.
func (s *chatStreamPaceState) beginWrite(now time.Time) {
	if s.lastWriteEnd.IsZero() {
		s.lastPong = now
		return
	}
	gap := now.Sub(s.lastWriteEnd)
	if gap >= chatStreamPacerTarget {
		s.rate = 0
	}
	queued := time.Duration(0)
	if s.rate > 0 {
		unconfirmed := float64(s.written - s.confirmed)
		queued = time.Duration(unconfirmed/s.rate*float64(time.Second)) - s.rtt
	}
	if gap > max(queued, 0) {
		s.gapOffsets = append(s.gapOffsets, s.written)
	}
}

// admit reports whether a message of n bytes may be written now. A message
// larger than the window is written when nothing is unconfirmed.
func (s *chatStreamPaceState) admit(n int64) bool {
	unconfirmed := s.written - s.confirmed
	return unconfirmed == 0 || unconfirmed+n <= s.window()
}

func (s *chatStreamPaceState) wrote(n int64, now time.Time) {
	s.written += n
	s.lastWriteEnd = now
}

// shouldPing reports whether a ping should start now.
func (s *chatStreamPaceState) shouldPing() bool {
	if s.written == s.lastPingOffset || len(s.pings) >= chatStreamPacerMaxPings {
		return false
	}
	return len(s.pings) == 0 || s.written-s.lastPingOffset >= s.window()/4
}

func (s *chatStreamPaceState) startPing(now time.Time) chatStreamPing {
	p := chatStreamPing{offset: s.written, sentAt: now}
	s.lastPingOffset = s.written
	s.pings = append(s.pings, p)
	return p
}

func (s *chatStreamPaceState) pong(p chatStreamPing, now time.Time) {
	for len(s.pings) > 0 && s.pings[0].offset <= p.offset {
		s.pings = s.pings[1:]
	}
	if delay := now.Sub(p.sentAt); !s.rttKnown || delay < s.rtt {
		s.rtt = delay
		s.rttKnown = true
	}
	if p.offset <= s.confirmed {
		return
	}
	appLimited := false
	for len(s.gapOffsets) > 0 && s.gapOffsets[0] < p.offset {
		if s.gapOffsets[0] >= s.confirmed {
			appLimited = true
		}
		s.gapOffsets = s.gapOffsets[1:]
	}
	if elapsed := now.Sub(s.lastPong); elapsed > 0 {
		sample := float64(p.offset-s.confirmed) / elapsed.Seconds()
		if !appLimited || sample > s.rate {
			s.rate = sample
		}
	}
	s.confirmed = p.offset
	s.lastPong = now
}

// chatStreamPinger sends a WebSocket ping and returns when the pong
// arrives. *websocket.Conn implements it.
type chatStreamPinger interface {
	Ping(ctx context.Context) error
}

// chatStreamPacer writes chat stream messages under chatStreamPaceState.
// Pings run in their own goroutines, so Write waits only when the window is
// full. ctx is the stream's context; a pacer lives for one stream.
type chatStreamPacer struct {
	ctx    context.Context
	w      io.Writer
	pinger chatStreamPinger
	clock  quartz.Clock

	mu      sync.Mutex
	state   chatStreamPaceState
	err     error
	changed chan struct{}
}

func newChatStreamPacer(ctx context.Context, w io.Writer, pinger chatStreamPinger, clock quartz.Clock) *chatStreamPacer {
	return &chatStreamPacer{ctx: ctx, w: w, pinger: pinger, clock: clock, changed: make(chan struct{})}
}

// Write writes b as one message, first waiting while the window is full.
func (p *chatStreamPacer) Write(b []byte) (int, error) {
	p.mu.Lock()
	p.state.beginWrite(p.clock.Now("chatStreamPacer", "write"))
	for p.err == nil && !p.state.admit(int64(len(b))) {
		changed := p.changed
		p.mu.Unlock()
		select {
		case <-changed:
		case <-p.ctx.Done():
			return 0, p.ctx.Err()
		}
		p.mu.Lock()
	}
	if err := p.err; err != nil {
		p.mu.Unlock()
		return 0, err
	}
	p.mu.Unlock()

	n, err := p.w.Write(b)

	p.mu.Lock()
	defer p.mu.Unlock()
	p.state.wrote(int64(n), p.clock.Now("chatStreamPacer", "write"))
	p.startPingsLocked()
	return n, err
}

func (p *chatStreamPacer) startPingsLocked() {
	for p.state.shouldPing() {
		ping := p.state.startPing(p.clock.Now("chatStreamPacer", "ping"))
		go p.ping(ping)
	}
}

func (p *chatStreamPacer) ping(ping chatStreamPing) {
	err := p.pinger.Ping(p.ctx)
	p.mu.Lock()
	defer p.mu.Unlock()
	if err != nil {
		if p.err == nil {
			p.err = xerrors.Errorf("wait for chat stream client to read: %w", err)
		}
	} else {
		p.state.pong(ping, p.clock.Now("chatStreamPacer", "pong"))
		p.startPingsLocked()
	}
	close(p.changed)
	p.changed = make(chan struct{})
}
