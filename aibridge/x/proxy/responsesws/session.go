// Package responsesws records OpenAI Responses API WebSocket mode sessions
// while relaying every frame unchanged in both directions. It contains no
// transport code: callers supply the upstream text-message connection and
// relay client frames through a Session.
//
// Recording contract:
//
//   - Each admitted client response.create is recorded as an interception
//     before it is forwarded.
//   - Every response the provider announces or finishes is recorded on
//     exactly one interception of the session. A response.created binds to
//     the oldest create on its lane whose upstream write succeeded, else it
//     opens an interception of its own. A terminal event for a response no
//     interception owns also opens one, so terminal usage is never dropped.
//   - A create whose write fails is never bound to a response. A create whose
//     write was in flight when a response.created or error event on its lane
//     could not be attributed ends without a response, since that event may
//     have been its answer.
//   - Steering frames are relayed unchanged, and steer input is recorded as a
//     prompt on the interception owning the steered response. Automatic
//     continuation responses are not promised to share that interception:
//     they bind like any other response. Interceptions of one connection are
//     grouped by client session.
//   - An interception ends with its response's terminal event, an error event
//     rejecting its create, or session end. Session end ends every open
//     interception within one shared cleanup deadline.
//
// Accounting never delays forwarding. The reader binds frames to
// interceptions and forwards them; one accountant goroutine feeds the events
// an extractor acts on to each response's extract/responses extraction, in
// order, and ends the interception from its outcome. The accounting queue
// is bounded: on overload the reader drops accounting jobs, never frames,
// logs each drop, and the affected interception ends with an overload error.
package responsesws

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/openai/openai-go/v3/responses"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	aibcontext "github.com/coder/coder/v2/aibridge/context"
	"github.com/coder/coder/v2/aibridge/credential"
	respextract "github.com/coder/coder/v2/aibridge/extract/responses"
	"github.com/coder/coder/v2/aibridge/intercept"
	"github.com/coder/coder/v2/aibridge/interceptionerror"
	"github.com/coder/coder/v2/aibridge/recorder"
)

const (
	// frameBuffer bounds upstream frames read ahead of Recv. The reader stops
	// reading upstream while it is full.
	frameBuffer = 16
	// errorBuffer bounds synthesized error events waiting for Recv. It matches
	// the upstream limit of 16 in-flight responses per connection.
	errorBuffer = 16

	eventCreate     = "response.create"
	eventSteer      = "response.steer"
	eventCreated    = "response.created"
	eventCompleted  = "response.completed"
	eventFailed     = "response.failed"
	eventIncomplete = "response.incomplete"
	eventError      = "error"
)

// ErrClosed is returned after the session has been closed.
var ErrClosed = xerrors.New("responses websocket session closed")

// MessageConn is the upstream text-message connection. Read and Write must
// honor ctx. Read may reuse the returned buffer on the next call. Close must
// unblock pending Read and Write calls.
type MessageConn interface {
	Read(ctx context.Context) ([]byte, error)
	Write(ctx context.Context, msg []byte) error
	Close() error
}

// Provider identifies the upstream provider in interception records and
// categorizes its errors. provider.Provider satisfies it.
type Provider interface {
	Type() string
	Name() string
	interceptionerror.ErrorCategorizer
}

// AdmitFunc decides whether a client response.create for model may be
// forwarded. A returned *intercept.ResponseError is relayed to the client
// with its status, code, and message. Other errors are relayed as a generic
// 403 refusal so internal details are not exposed.
type AdmitFunc func(ctx context.Context, model string) error

// Options configures a Session.
type Options struct {
	Provider Provider
	Recorder recorder.Recorder
	// Admit is consulted before each client response.create. Nil admits all.
	Admit           AdmitFunc
	Logger          slog.Logger
	Client          string
	ClientSessionID *string
	UserAgent       string
	CredentialKind  credential.Kind
	CredentialHint  string
}

// Session relays one Responses WebSocket connection and records one
// interception per client response.create, plus one per response no create
// explains. It is safe for one concurrent Send caller and one concurrent Recv
// caller.
type Session struct {
	opts     Options
	actor    *aibcontext.Actor
	upstream MessageConn

	ctx    context.Context
	cancel context.CancelCauseFunc

	frames   chan []byte
	errors   chan []byte
	readDone chan struct{}
	readErr  error // Written before readDone is closed.

	closeOnce sync.Once
	closeErr  error

	// queue holds accounting jobs for the accountant, which closes
	// accountDone when it exits.
	queue       *jobQueue
	accountDone chan struct{}
	// cleanupCtx scopes every record call. It outlives the session and ends
	// at the shutdown cleanup deadline.
	cleanupCtx    context.Context
	cleanupCancel context.CancelFunc
	// cleanupDeadline cancels cleanupCtx once the session ended. Set by the
	// reader before readDone is closed.
	cleanupDeadline *time.Timer

	mu     sync.Mutex
	closed bool
	// endErr is the cause open interceptions end with at session end, and
	// endedAt the time the end was first observed, by Close or the reader.
	endErr  error
	endedAt time.Time
	open    map[string]*interception
	// pending holds admitted creates per lane, in write order, waiting for
	// response.created.
	pending map[string][]*interception
	// active maps a lane to the interception owning its in-flight response.
	active map[string]*interception
	// responses maps response IDs to their owning interception until the
	// response's terminal usage is recorded or the session ends.
	responses map[string]*interception
	// overloaded holds interceptions whose terminal accounting job was
	// dropped, in hand-off order, for the accountant to end. overloadWake
	// signals the accountant after a hand-off.
	overloaded   []overloadedEnd
	overloadWake chan struct{}
}

type interception struct {
	id   string
	lane string
	// prompt is the create's last user prompt, recorded once its response ID
	// is known. Empty for interceptions the server opened.
	prompt string
	// model and startedAt are recorded in the interception start record.
	// startedAt is when the create was sent or the server frame arrived,
	// not when the accountant records the start.
	model     string
	startedAt time.Time
	// started marks an interception whose start is recorded. Creates start
	// before they are forwarded; interceptions the server opened are
	// started by the accountant, which alone reads and writes it for them.
	started bool
	// written marks a create whose upstream write succeeded. Only a written
	// create binds a response. Guarded by Session.mu.
	written bool
	// raced marks a create whose write was in flight when a response.created
	// or error event on its lane could not be attributed. Guarded by
	// Session.mu.
	raced bool
	// racedAt is when the first event that raced the create's write
	// arrived. racedErr is the first error event that raced it, which
	// arrived at racedErrAt. The error becomes the create's end cause, else
	// the create ends without one at racedAt. Guarded by Session.mu.
	racedAt    time.Time
	racedErr   []byte
	racedErrAt time.Time
	// responseID is the bound response, if any. Guarded by Session.mu.
	responseID string
	// ended is set once, by whoever ends the interception. Guarded by
	// Session.mu.
	ended bool
	// lossy marks an interception that lost an accounting job. Guarded by
	// Session.mu.
	lossy bool
	// ext records the response through rec. Only the accountant uses them.
	ext *respextract.ResponseExtraction
	rec *boundedRecorder
}

// NewSession starts relaying upstream. ctx bounds the session and must carry
// the actor. The caller must call Close.
func NewSession(ctx context.Context, upstream MessageConn, opts Options) (*Session, error) {
	actor := aibcontext.ActorFromContext(ctx)
	switch {
	case actor == nil:
		return nil, xerrors.New("actor is required")
	case upstream == nil:
		return nil, xerrors.New("upstream connection is required")
	case opts.Provider == nil:
		return nil, xerrors.New("provider is required")
	case opts.Recorder == nil:
		return nil, xerrors.New("recorder is required")
	}
	sessionCtx, cancel := context.WithCancelCause(ctx)
	cleanupCtx, cleanupCancel := context.WithCancel(context.WithoutCancel(ctx))
	s := &Session{
		opts:          opts,
		actor:         actor,
		upstream:      upstream,
		ctx:           sessionCtx,
		cancel:        cancel,
		frames:        make(chan []byte, frameBuffer),
		errors:        make(chan []byte, errorBuffer),
		readDone:      make(chan struct{}),
		queue:         newJobQueue(),
		accountDone:   make(chan struct{}),
		cleanupCtx:    cleanupCtx,
		cleanupCancel: cleanupCancel,
		open:          make(map[string]*interception),
		pending:       make(map[string][]*interception),
		active:        make(map[string]*interception),
		responses:     make(map[string]*interception),
		overloadWake:  make(chan struct{}, 1),
	}
	go s.readLoop()
	go s.accountLoop()
	return s, nil
}

// Send forwards one client frame upstream unchanged. A refused or
// unrecordable response.create is not forwarded; the client receives an
// error event from Recv instead and the session stays open.
//
// Every blocking step of Send (admission, recording the interception start,
// queueing a synthesized error event, and the upstream write) runs under one
// operation context that ends when ctx or the session ends. When a step
// fails or is cut short, and after admission returns, Send classifies the
// outcome in this order:
//
//  1. The session ended: Send returns ErrClosed and records or synthesizes
//     nothing more. An interception it started ends with the session's
//     cause.
//  2. ctx ended: Send returns ctx's error and synthesizes nothing. An
//     interception it started ends with ctx's error.
//  3. Otherwise the step failed on its own. A refused create or a failed
//     interception start queues an error event and Send returns nil. A
//     failed write ends the interception Send started and Send returns the
//     write error.
//
// After a failed step, Send returns nil exactly when it queued an event.
func (s *Session) Send(ctx context.Context, frame []byte) error {
	// The time the client sent the frame, for the records it causes.
	sentAt := time.Now()
	if s.isClosed() {
		return ErrClosed
	}
	opCtx, cancelOp := context.WithCancel(ctx)
	defer cancelOp()
	stop := context.AfterFunc(s.ctx, cancelOp)
	defer stop()
	eventType := gjson.GetBytes(frame, "type").String()
	if eventType == eventCreate {
		return s.sendCreate(ctx, opCtx, frame, sentAt)
	}
	var steered *interception
	if eventType == eventSteer {
		s.mu.Lock()
		steered = s.responses[gjson.GetBytes(frame, "previous_response_id").String()]
		s.mu.Unlock()
	}
	if err := s.upstream.Write(opCtx, frame); err != nil {
		if aborted := s.sendAborted(ctx); aborted != nil {
			return aborted
		}
		return xerrors.Errorf("write upstream: %w", err)
	}
	if steered != nil {
		s.recordSteerPrompt(steered, frame, sentAt)
	}
	return nil
}

// recordSteerPrompt records a forwarded steer's input, sent at sentAt, as a
// prompt on the interception owning the steered response.
func (s *Session) recordSteerPrompt(ic *interception, frame []byte, sentAt time.Time) {
	facts, err := respextract.RequestExtractor{}.ExtractRequest(frame)
	if err != nil {
		s.opts.Logger.Debug(s.cleanupCtx, "steer prompt not fully parsed", slog.Error(err))
	}
	if facts.Prompt == "" {
		return
	}
	ctx, cancel := s.recordContext()
	defer cancel()
	if err := s.opts.Recorder.RecordPromptUsage(ctx, &recorder.PromptUsageRecord{
		CreatedAt:      sentAt.UTC(),
		InterceptionID: ic.id,
		MsgID:          gjson.GetBytes(frame, "previous_response_id").String(),
		Prompt:         facts.Prompt,
	}); err != nil {
		s.opts.Logger.Warn(ctx, "failed to record prompt usage", slog.Error(err), slog.F("interception_id", ic.id))
	}
}

// sendAborted reports why a Send step was cut short by its operation
// context: ErrClosed when the session ended, else ctx's error when the
// caller's context ended, else nil. The session is checked first so a close
// racing a caller cancellation reports the close.
func (s *Session) sendAborted(ctx context.Context) error {
	if s.isClosed() {
		return ErrClosed
	}
	return ctx.Err()
}

// isClosed reports whether Close was called or the reader ended the session.
func (s *Session) isClosed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed || s.ctx.Err() != nil
}

// Recv returns the next frame for the client: upstream frames unchanged, or
// synthesized error events. After upstream ends it drains buffered frames and
// then returns the terminal error.
func (s *Session) Recv(ctx context.Context) ([]byte, error) {
	select {
	case ev := <-s.errors:
		return ev, nil
	case frame := <-s.frames:
		return frame, nil
	case <-s.readDone:
		select {
		case ev := <-s.errors:
			return ev, nil
		case frame := <-s.frames:
			return frame, nil
		default:
			return nil, s.readErr
		}
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Close ends the session, closes upstream, and ends every open interception
// with err, or ErrClosed when err is nil. It waits for the reader and the
// accountant to exit, which takes at most the shutdown cleanup deadline when
// the recorder honors its context.
func (s *Session) Close(err error) error {
	s.closeOnce.Do(func() {
		cause := err
		if cause == nil {
			cause = ErrClosed
		}
		s.observeEnd(time.Now())
		s.cancel(cause)
		s.closeErr = s.upstream.Close()
		<-s.readDone
		<-s.accountDone
	})
	return s.closeErr
}

func (s *Session) readLoop() {
	defer close(s.readDone)
	for {
		frame, err := s.upstream.Read(s.ctx)
		if err == nil {
			arrived := time.Now()
			// route copies what accounting keeps, so the client owns this
			// copy and may modify it.
			frame = bytes.Clone(frame)
			s.route(frame, arrived)
			select {
			case s.frames <- frame:
				continue
			case <-s.ctx.Done():
			}
		}
		endedAt := time.Now()
		if s.ctx.Err() != nil {
			err = context.Cause(s.ctx)
		}
		s.readErr = err
		// Cancel the session so work bound to it, such as a Send waiting on
		// admission, stops when upstream ends. After Close this is a no-op
		// that keeps Close's cause.
		s.cancel(err)
		// The accountant ends the open interceptions once the reader exits.
		// The cleanup deadline starts now, so it also bounds a record call
		// the accountant is already blocked in.
		s.observeEnd(endedAt)
		s.mu.Lock()
		s.closed = true
		s.endErr = err
		s.cleanupDeadline = time.AfterFunc(recorder.DefaultAsyncTimeout, s.cleanupCancel)
		s.mu.Unlock()
		return
	}
}

// observeEnd records at as the session end time unless an earlier end was
// observed.
func (s *Session) observeEnd(at time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.endedAt.IsZero() || at.Before(s.endedAt) {
		s.endedAt = at
	}
}

// sessionEndTime returns when the session end was observed, or now when
// the caller observed it before Close or the reader did.
func (s *Session) sessionEndTime(now time.Time) time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.endedAt.IsZero() {
		return now
	}
	return s.endedAt
}

// sendCreate implements Send for response.create, sent at sentAt. ctx is
// the caller's context and opCtx the Send operation context.
func (s *Session) sendCreate(ctx, opCtx context.Context, frame []byte, sentAt time.Time) error {
	lane := gjson.GetBytes(frame, "stream_id").String()
	facts, factsErr := respextract.RequestExtractor{}.ExtractRequest(frame)
	model := facts.Model
	log := s.opts.Logger.With(slog.F("model", model), slog.F("stream_id", lane))
	if factsErr != nil {
		log.Debug(ctx, "response.create not fully parsed", slog.Error(factsErr))
	}
	if s.opts.Admit != nil {
		err := s.opts.Admit(opCtx, model)
		// A create admitted or refused after the session or the caller
		// ended is never forwarded, so nothing is recorded or relayed for
		// it. Any other error is a refusal, including a context error from
		// admission's own deadline.
		if aborted := s.sendAborted(ctx); aborted != nil {
			return aborted
		}
		if err != nil {
			log.Info(ctx, "response.create refused by admission", slog.Error(err))
			if respErr, ok := errors.AsType[*intercept.ResponseError](err); ok && respErr.ErrorObject != nil {
				return s.enqueueError(ctx, opCtx, lane, respErr.StatusCode, respErr.ErrorObject.Type, respErr.ErrorObject.Code, respErr.ErrorObject.Message)
			}
			// Refusals are policy decisions, not gateway faults: use a
			// non-retryable 4xx so SDKs do not retry them.
			return s.enqueueError(ctx, opCtx, lane, http.StatusForbidden, "invalid_request_error", "request_refused", "request refused by AI Gateway")
		}
	}
	var toolCallID *string
	if facts.CorrelatingToolCallID != "" {
		toolCallID = &facts.CorrelatingToolCallID
	}
	ic, err := s.startInterception(opCtx, lane, model, sentAt, toolCallID, func(ic *interception) {
		ic.prompt = facts.Prompt
		s.pending[lane] = append(s.pending[lane], ic)
	})
	if err != nil {
		if aborted := s.sendAborted(ctx); aborted != nil {
			return aborted
		}
		log.Warn(ctx, "failed to record interception", slog.Error(err))
		return s.enqueueError(ctx, opCtx, lane, http.StatusInternalServerError, intercept.OpenAIErrTypeAPI, intercept.OpenAIErrCodeServer, "failed to record interception")
	}
	// Check before forwarding: a write on an ended context may still
	// succeed, which would forward a create the caller abandoned.
	if aborted := s.sendAborted(ctx); aborted != nil {
		return s.endStarted(ctx, ic, aborted)
	}
	if err := s.upstream.Write(opCtx, frame); err != nil {
		return s.endStarted(ctx, ic, xerrors.Errorf("write response.create upstream: %w", err))
	}
	s.publish(ic)
	return nil
}

// publish makes a create whose write succeeded bindable. The transition
// happens under the lock, so a response.created handled before it never
// binds to the create. If such a response opened its own interception while
// the write was in flight, it may have been this create's answer, so the
// create ends without a response instead: binding it to a later response
// would shift every following response on the lane onto the wrong create.
// An error event that raced the write is its answer, so it becomes the end
// cause.
func (s *Session) publish(ic *interception) {
	s.mu.Lock()
	if ic.ended {
		s.mu.Unlock()
		return
	}
	if !ic.raced {
		ic.written = true
		s.mu.Unlock()
		return
	}
	racedErr, racedErrAt, racedAt := ic.racedErr, ic.racedErrAt, ic.racedAt
	s.mu.Unlock()
	if racedErr == nil {
		s.end(ic, nil, racedAt)
		return
	}
	// The create was never bound, so no other goroutine records for it.
	ext, rec := s.newExtraction(ic)
	rec.at = racedErrAt
	ext.OnEvent(eventError, racedErr)
	s.end(ic, ext.Outcome().Err, racedErrAt)
}

// endStarted ends ic, which a Send step started before failing with err, and
// returns what Send reports, classified as documented on Send.
func (s *Session) endStarted(ctx context.Context, ic *interception, err error) error {
	// Send observed the failure now.
	now := time.Now()
	switch {
	case s.isClosed():
		s.end(ic, context.Cause(s.ctx), s.sessionEndTime(now))
		return ErrClosed
	case ctx.Err() != nil:
		s.end(ic, ctx.Err(), now)
		return ctx.Err()
	default:
		s.end(ic, err, now)
		return err
	}
}

// startInterception records a new interception, started at startedAt, and
// runs register under the session lock so a concurrent shutdown cannot miss
// it.
func (s *Session) startInterception(ctx context.Context, lane, model string, startedAt time.Time, toolCallID *string, register func(*interception)) (*interception, error) {
	ic := &interception{id: uuid.NewString(), lane: lane, model: model, startedAt: startedAt, started: true}
	if err := s.opts.Recorder.RecordInterception(ctx, s.interceptionRecord(ic, toolCallID)); err != nil {
		return nil, xerrors.Errorf("record interception: %w", err)
	}
	s.mu.Lock()
	if s.closed {
		// The reader set the end time when it closed the session.
		ic.ended = true
		endedAt := s.endedAt
		s.mu.Unlock()
		s.recordEnded(ic, context.Cause(s.ctx), endedAt)
		return nil, ErrClosed
	}
	s.open[ic.id] = ic
	register(ic)
	s.mu.Unlock()
	return ic, nil
}

func (s *Session) interceptionRecord(ic *interception, toolCallID *string) *recorder.InterceptionRecord {
	return &recorder.InterceptionRecord{
		ID:                    ic.id,
		CorrelatingToolCallID: toolCallID,
		InitiatorID:           s.actor.ID,
		Metadata:              s.actor.Metadata,
		Model:                 ic.model,
		Provider:              s.opts.Provider.Type(),
		ProviderName:          s.opts.Provider.Name(),
		StartedAt:             ic.startedAt,
		ClientSessionID:       s.opts.ClientSessionID,
		Client:                s.opts.Client,
		UserAgent:             s.opts.UserAgent,
		CredentialKind:        s.opts.CredentialKind,
		CredentialHint:        s.opts.CredentialHint,
	}
}

// route binds a server frame, read at arrived, to an interception and
// queues the accounting the frame needs. It runs on the reader, never
// blocks, and makes no recorder calls: interceptions the server opens are
// registered here and recorded by the accountant, in queue order before
// their events. When the queue drops a terminal job, the interception is
// handed to the accountant for an overload end right away.
func (s *Session) route(frame []byte, arrived time.Time) {
	eventType := gjson.GetBytes(frame, "type").String()
	lane := gjson.GetBytes(frame, "stream_id").String()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	ev := job{kind: jobEvent, frame: frame, eventType: eventType, arrived: arrived}
	var ic *interception
	switch eventType {
	case eventCreated:
		s.bindCreatedLocked(lane, ev)
		return
	case eventCompleted, eventFailed, eventIncomplete:
		response := gjson.GetBytes(frame, "response")
		ic = s.responses[response.Get("id").String()]
		ev.terminal = true
		if ic == nil {
			// A response no interception owns still has its usage recorded.
			s.openLocked(lane, response.Get("model").String(), response.Get("id").String(), ev)
			return
		}
		if s.active[ic.lane] == ic {
			delete(s.active, ic.lane)
		}
		s.pushTerminalLocked(ic, ev)
		return
	case eventError:
		// Error events are request-scoped: they reject the oldest written
		// create on their lane. In-flight responses end via response.failed.
		q := s.pending[lane]
		switch {
		case len(q) > 0 && q[0].written:
			ic = q[0]
			setQueue(s.pending, lane, q[1:])
			ev.terminal = true
			s.pushTerminalLocked(ic, ev)
		case len(q) > 0:
			// The error may answer the create whose write is in flight. See
			// publish.
			q[0].markRaced(arrived)
			if q[0].racedErr == nil {
				// Copied, since the client owns frame.
				q[0].racedErr, q[0].racedErrAt = bytes.Clone(frame), arrived
			}
		}
		return
	default:
		ic = s.active[lane]
	}
	if ic != nil && respextract.Relevant(eventType) {
		ev.ic = ic
		s.pushLocked(ic, ev)
	}
}

// pushTerminalLocked queues ic's terminal job, or hands ic off for an
// overload end when the queue drops it.
func (s *Session) pushTerminalLocked(ic *interception, ev job) {
	ev.ic = ic
	if !s.pushLocked(ic, ev) {
		s.handOffLocked(ic, ev.arrived)
	}
}

// bindCreatedLocked binds a new response to the oldest create on its lane
// whose write succeeded. Otherwise the response opens its own interception
// so nothing goes unrecorded; admission is not applied to such responses.
func (s *Session) bindCreatedLocked(lane string, ev job) {
	responseID := gjson.GetBytes(ev.frame, "response.id").String()
	q := s.pending[lane]
	if len(q) > 0 && q[0].written {
		ic := q[0]
		setQueue(s.pending, lane, q[1:])
		ic.responseID = responseID
		s.bindLocked(ic, responseID)
		ev.ic = ic
		s.pushLocked(ic, ev)
		return
	}
	if len(q) > 0 {
		// Creates are written in order, so an unwritten head is the create
		// whose write is in flight. See publish.
		q[0].markRaced(ev.arrived)
	}
	if ic := s.openLocked(lane, gjson.GetBytes(ev.frame, "response.model").String(), responseID, ev); ic != nil {
		s.bindLocked(ic, responseID)
	}
}

// bindLocked routes the lane's events and responseID's terminal event to ic.
// ic.responseID is set before ic's jobs are queued, since the accountant
// reads it.
func (s *Session) bindLocked(ic *interception, responseID string) {
	s.active[ic.lane] = ic
	if responseID != "" {
		s.responses[responseID] = ic
	}
}

// openLocked registers an interception the server opened, starting when ev
// arrived, and queues its start record before ev. If the queue cannot take
// both, nothing is registered and the loss is logged.
func (s *Session) openLocked(lane, model, responseID string, ev job) *interception {
	ic := &interception{id: uuid.NewString(), lane: lane, model: model, responseID: responseID, startedAt: ev.arrived}
	ev.ic = ic
	if !s.pushLocked(ic, job{kind: jobStart, ic: ic}, ev) {
		return nil
	}
	s.open[ic.id] = ic
	return ic
}

// markRaced marks a create whose write was in flight when an event on its
// lane, which arrived at at, could not be attributed. Called with s.mu held.
func (ic *interception) markRaced(at time.Time) {
	if !ic.raced {
		ic.raced, ic.racedAt = true, at
	}
}

// end ends ic with err at endedAt exactly once: only the first caller
// records the end. An interception that lost an accounting job ends with
// errAccountingOverloaded instead, whichever path ends it.
func (s *Session) end(ic *interception, err error, endedAt time.Time) {
	s.mu.Lock()
	if ic.ended {
		s.mu.Unlock()
		return
	}
	ic.ended = true
	if ic.lossy {
		err = errAccountingOverloaded
	}
	s.forgetLocked(ic)
	s.mu.Unlock()
	s.recordEnded(ic, err, endedAt)
}

func (s *Session) forgetLocked(ic *interception) {
	delete(s.open, ic.id)
	if ic.responseID != "" && s.responses[ic.responseID] == ic {
		delete(s.responses, ic.responseID)
	}
	if s.active[ic.lane] == ic {
		delete(s.active, ic.lane)
	}
	isIC := func(other *interception) bool { return other == ic }
	setQueue(s.pending, ic.lane, slices.DeleteFunc(s.pending[ic.lane], isIC))
}

// setQueue stores the queue for lane, deleting the entry when it is empty so
// a client using a fresh stream_id per request does not grow the map.
func setQueue(m map[string][]*interception, lane string, q []*interception) {
	if len(q) == 0 {
		delete(m, lane)
		return
	}
	m[lane] = q
}

// recordEnded records the end of ic at endedAt with a fresh bounded
// context, so the time extraction spent recording does not shorten it.
func (s *Session) recordEnded(ic *interception, err error, endedAt time.Time) {
	ctx, cancel := s.recordContext()
	defer cancel()
	errType, message := interceptionerror.Categorize(s.opts.Provider, err)
	if err := s.opts.Recorder.RecordInterceptionEnded(ctx, &recorder.InterceptionRecordEnded{
		ID:             ic.id,
		EndedAt:        endedAt,
		CredentialHint: s.opts.CredentialHint,
		ErrorType:      errType,
		ErrorMessage:   message,
	}); err != nil {
		s.opts.Logger.Warn(ctx, "failed to record interception end", slog.Error(err), slog.F("interception_id", ic.id))
	}
}

// enqueueError delivers a synthesized error event to the client. The event
// carries both the Responses streaming error shape (top-level code and
// message) and the WebSocket mode shape (status and a nested error object),
// plus stream_id, so clients parsing either shape understand it. Synthesized
// errors never have a param, so it is omitted. ctx is the caller's context
// and opCtx the Send operation context, which bounds waiting for room.
func (s *Session) enqueueError(ctx, opCtx context.Context, lane string, status int, errType, code, message string) error {
	ev, err := json.Marshal(responses.ResponseErrorEvent{Code: code, Message: message})
	if err == nil {
		ev, err = sjson.DeleteBytes(ev, "param")
	}
	if err == nil {
		ev, err = sjson.SetBytes(ev, "status", status)
	}
	if err == nil {
		ev, err = sjson.SetBytes(ev, "error", map[string]string{"type": errType, "code": code, "message": message})
	}
	if err == nil && lane != "" {
		ev, err = sjson.SetBytes(ev, "stream_id", lane)
	}
	if err != nil {
		return xerrors.Errorf("encode error event: %w", err)
	}
	// Queue without waiting when there is room, so the outcome never
	// depends on select choosing between a free queue and an ended
	// operation. The event is queued exactly when nil is returned.
	select {
	case s.errors <- ev:
		return nil
	default:
	}
	select {
	case s.errors <- ev:
		return nil
	case <-opCtx.Done():
		// opCtx ends only with ctx or the session, so this is non-nil.
		if aborted := s.sendAborted(ctx); aborted != nil {
			return aborted
		}
		return opCtx.Err()
	}
}
