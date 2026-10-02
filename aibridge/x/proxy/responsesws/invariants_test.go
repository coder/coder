package responsesws_test

import (
	"context"
	"fmt"
	"io"
	"maps"
	"math/rand/v2"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	codertestutil "github.com/coder/coder/v2/testutil"
)

// TestRecordingInvariants drives random interleavings of client creates and
// steers, upstream write outcomes (success, failure, or blocked while server
// events arrive), server events, and session end. After each scenario:
//   - every terminal response's usage is recorded exactly once,
//   - every started interception is ended,
//   - no response is recorded on a create whose write had not succeeded
//     when the server announced the response.
func TestRecordingInvariants(t *testing.T) {
	t.Parallel()
	rng := rand.New(rand.NewPCG(1, 2)) //nolint:gosec // Deterministic test input.
	for i := range 300 {
		seed := rng.Uint64()
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			t.Parallel()
			runScenario(t, rand.New(rand.NewPCG(seed, 0))) //nolint:gosec // Deterministic test input.
		})
	}
}

type scenario struct {
	t   *testing.T
	rng *rand.Rand
	h   *harness

	// writeResult is what the next upstream write returns; block makes it
	// wait for release first.
	writeResult error
	block       bool
	entered     chan struct{}
	release     chan struct{}

	creates, responses int
	// written holds the models of creates whose write returned success.
	written map[string]bool
	// received holds per lane the creates the server received and has not
	// answered yet.
	received map[string][]string
	// inFlight lists announced responses without a terminal event.
	inFlight []string
	lanes    map[string]string
	// allowed holds, per announced response, the create models it may be
	// recorded on: those written when the server announced it.
	allowed   map[string]map[string]bool
	terminals []string
}

var errWrite = xerrors.New("broken pipe")

func runScenario(t *testing.T, rng *rand.Rand) {
	ctx := codertestutil.Context(t, codertestutil.WaitShort)
	s := &scenario{
		t: t, rng: rng, h: newHarness(ctx, t, nil),
		written: map[string]bool{}, received: map[string][]string{},
		lanes: map[string]string{}, allowed: map[string]map[string]bool{},
	}
	write := func(ctx context.Context) error {
		if s.block {
			close(s.entered)
			select {
			case <-s.release:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		return s.writeResult
	}
	s.h.conn.onWrite.Store(&write)

	for range 1 + rng.IntN(25) {
		switch n := rng.IntN(8); {
		case n < 3:
			s.create()
		case n < 4:
			s.steer()
		default:
			s.serverEvent()
		}
	}
	if rng.IntN(2) == 0 {
		require.NoError(t, s.h.sess.Close(nil))
	} else {
		close(s.h.conn.toClient)
		_, err := s.h.sess.Recv(ctx)
		require.ErrorIs(t, err, io.EOF)
	}
	s.check()
}

func (s *scenario) lane() string {
	return []string{"", "a"}[s.rng.IntN(2)]
}

// send runs one Send with the given write outcome. A blocked write lets the
// server emit events before it resolves; during the block the server may
// already have received the frame.
func (s *scenario) send(frame string, received func()) error {
	s.writeResult, s.block = nil, false
	switch s.rng.IntN(3) {
	case 1:
		s.writeResult = errWrite
	case 2:
		s.block = true
		s.entered, s.release = make(chan struct{}), make(chan struct{})
	}
	if !s.block {
		err := s.h.sess.Send(s.h.ctx, []byte(frame))
		if err == nil {
			received()
		}
		return err
	}
	sent := make(chan error, 1)
	go func() { sent <- s.h.sess.Send(s.h.ctx, []byte(frame)) }()
	_ = codertestutil.TryReceive(s.h.ctx, s.t, s.entered)
	// A write can reach the server before it returns, even when it then
	// reports failure.
	early := s.rng.IntN(2) == 0
	if early {
		received()
	}
	for range s.rng.IntN(3) {
		s.serverEvent()
	}
	if s.rng.IntN(2) == 0 {
		s.writeResult = errWrite
	}
	close(s.release)
	err := codertestutil.TryReceive(s.h.ctx, s.t, sent)
	if err == nil && !early {
		received()
	}
	return err
}

func (s *scenario) create() {
	lane, model := s.lane(), fmt.Sprintf("create-%d", s.creates)
	s.creates++
	err := s.send(create(lane, model, "hi"), func() {
		s.received[lane] = append(s.received[lane], model)
	})
	if err == nil {
		s.written[model] = true
	} else {
		require.ErrorIs(s.t, err, errWrite)
	}
}

func (s *scenario) steer() {
	if len(s.inFlight) == 0 {
		return
	}
	id := s.inFlight[s.rng.IntN(len(s.inFlight))]
	err := s.send(fmt.Sprintf(`{"type":"response.steer"%s,"previous_response_id":%q,"input":"more"}`, lane(s.lanes[id]), id), func() {})
	if err != nil {
		require.ErrorIs(s.t, err, errWrite)
	}
}

func (s *scenario) serverEvent() {
	switch s.rng.IntN(4) {
	case 0, 1:
		// Announce a response: the answer to the oldest received create on
		// a lane, or an unexplained response.
		lane := s.lane()
		if q := s.received[lane]; len(q) > 0 && s.rng.IntN(4) > 0 {
			s.received[lane] = q[1:]
		}
		id := fmt.Sprintf("resp_%d", s.responses)
		s.responses++
		s.allowed[id] = maps.Clone(s.written)
		s.lanes[id] = lane
		s.inFlight = append(s.inFlight, id)
		s.h.relay(created(lane, id, "srv-"+id))
	case 2:
		s.finish()
	default:
		// Reject the oldest received create on a lane.
		l := s.lane()
		if q := s.received[l]; len(q) > 0 {
			s.received[l] = q[1:]
			s.h.relay(fmt.Sprintf(`{"type":"error","status":400%s,"error":{"type":"invalid_request_error","code":"invalid","message":"bad"}}`, lane(l)))
		}
	}
}

// finish ends an in-flight response, or a response the session never saw
// announced.
func (s *scenario) finish() {
	var id string
	if len(s.inFlight) > 0 {
		i := s.rng.IntN(len(s.inFlight))
		id = s.inFlight[i]
		s.inFlight = slices.Delete(s.inFlight, i, i+1)
	} else {
		id = fmt.Sprintf("resp_%d", s.responses)
		s.responses++
		s.allowed[id] = map[string]bool{}
		s.lanes[id] = s.lane()
	}
	extra := fmt.Sprintf(`,"model":%q`, "srv-"+id)
	eventType := "response.completed"
	switch s.rng.IntN(4) {
	case 1:
		eventType, extra = "response.incomplete", extra+`,"incomplete_details":{"reason":"steered"}`
	case 2:
		eventType, extra = "response.incomplete", extra+`,"incomplete_details":{"reason":"max_output_tokens"}`
	case 3:
		eventType, extra = "response.failed", extra+`,"error":{"code":"server_error","message":"boom"}`
	}
	s.terminals = append(s.terminals, id)
	s.h.relay(terminal(s.lanes[id], eventType, id, extra))
}

func (s *scenario) check() {
	models := map[string]string{}
	for _, ic := range s.h.rec.RecordedInterceptions() {
		models[ic.ID] = ic.Model
		require.NotNil(s.t, s.h.rec.RecordedInterceptionEnd(ic.ID), "interception %s left open", ic.Model)
	}
	usages := usagesByResponse(s.h)
	require.Len(s.t, usages, len(s.terminals))
	for _, id := range s.terminals {
		ics := usages[id]
		require.Len(s.t, ics, 1, "usage of %s", id)
		model := models[ics[0]]
		if strings.HasPrefix(model, "create-") {
			require.True(s.t, s.allowed[id][model], "%s recorded on %s, which was not written when it was announced", id, model)
		} else {
			require.Equal(s.t, "srv-"+id, model)
		}
	}
}
