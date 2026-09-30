// Package responsesws records OpenAI Responses API WebSocket mode sessions
// while relaying every frame unchanged in both directions. It contains no
// transport code: callers supply the upstream text-message connection and
// relay client frames through a Session.
package responsesws

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"maps"
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

	eventSteerAccepted = "response.steer.accepted"
	eventSteerPending  = "response.steer.pending"
	eventSteerFailed   = "response.steer.failed"

	reasonSteered = "steered"
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
	// Observer receives frames bound to interceptions. Nil uses
	// NewRecordingObserver(Recorder, Logger).
	Observer Observer
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
// interception per response chain started by a client response.create. It
// is safe for one concurrent Send caller and one concurrent Recv caller.
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

	mu     sync.Mutex
	closed bool
	open   map[string]*interception
	// pending holds admitted creates per lane waiting for response.created.
	pending map[string][]*interception
	// continuations holds steered interceptions per lane waiting for the
	// server's automatic continuation response.
	continuations map[string][]*interception
	// active maps a lane to the interception owning its in-flight response.
	active map[string]*interception
	// responses maps response IDs to their owning interception.
	responses map[string]*interception
	// steered marks responses with an accepted steer not yet applied.
	steered map[string]bool
}

type interception struct {
	id          string
	lane        string
	responseIDs []string
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
	if opts.Observer == nil {
		opts.Observer = NewRecordingObserver(opts.Recorder, opts.Logger)
	}
	sessionCtx, cancel := context.WithCancelCause(ctx)
	s := &Session{
		opts:          opts,
		actor:         actor,
		upstream:      upstream,
		ctx:           sessionCtx,
		cancel:        cancel,
		frames:        make(chan []byte, frameBuffer),
		errors:        make(chan []byte, errorBuffer),
		readDone:      make(chan struct{}),
		open:          make(map[string]*interception),
		pending:       make(map[string][]*interception),
		continuations: make(map[string][]*interception),
		active:        make(map[string]*interception),
		responses:     make(map[string]*interception),
		steered:       make(map[string]bool),
	}
	go s.readLoop()
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
	if s.isClosed() {
		return ErrClosed
	}
	opCtx, cancelOp := context.WithCancel(ctx)
	defer cancelOp()
	stop := context.AfterFunc(s.ctx, cancelOp)
	defer stop()
	eventType := gjson.GetBytes(frame, "type").String()
	if eventType == eventCreate {
		return s.sendCreate(ctx, opCtx, frame)
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
		recordCtx, cancel := recordContext(ctx)
		defer cancel()
		s.opts.Observer.ClientEvent(recordCtx, steered.id, frame)
	}
	return nil
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
// with err, or ErrClosed when err is nil. It waits for the reader to exit.
func (s *Session) Close(err error) error {
	s.closeOnce.Do(func() {
		cause := err
		if cause == nil {
			cause = ErrClosed
		}
		s.cancel(cause)
		s.closeErr = s.upstream.Close()
		<-s.readDone
	})
	return s.closeErr
}

func (s *Session) readLoop() {
	defer close(s.readDone)
	for {
		frame, err := s.upstream.Read(s.ctx)
		if err == nil {
			frame = bytes.Clone(frame)
			s.handleServerEvent(frame)
			select {
			case s.frames <- frame:
				continue
			case <-s.ctx.Done():
			}
		}
		if s.ctx.Err() != nil {
			err = context.Cause(s.ctx)
		}
		s.readErr = err
		// Cancel the session so work bound to it, such as a Send waiting on
		// admission, stops when upstream ends. After Close this is a no-op
		// that keeps Close's cause.
		s.cancel(err)
		s.endAll(err)
		return
	}
}

// sendCreate implements Send for response.create. ctx is the caller's
// context and opCtx the Send operation context.
func (s *Session) sendCreate(ctx, opCtx context.Context, frame []byte) error {
	lane := gjson.GetBytes(frame, "stream_id").String()
	model := gjson.GetBytes(frame, "model").String()
	log := s.opts.Logger.With(slog.F("model", model), slog.F("stream_id", lane))
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
	ic, err := s.startInterception(opCtx, lane, model, func(ic *interception) {
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
	recordCtx, cancel := recordContext(ctx)
	defer cancel()
	s.opts.Observer.ClientEvent(recordCtx, ic.id, frame)
	if err := s.upstream.Write(opCtx, frame); err != nil {
		return s.endStarted(ctx, ic, xerrors.Errorf("write response.create upstream: %w", err))
	}
	// A client create continuing a response consumes any steer queued on it
	// (the steering guide's tool-output flow), so no automatic continuation
	// follows.
	if previous := gjson.GetBytes(frame, "previous_response_id").String(); previous != "" {
		s.cancelContinuation(previous)
	}
	return nil
}

// endStarted ends ic, which a Send step started before failing with err, and
// returns what Send reports, classified as documented on Send.
func (s *Session) endStarted(ctx context.Context, ic *interception, err error) error {
	switch {
	case s.isClosed():
		s.endInterception(ic, context.Cause(s.ctx))
		return ErrClosed
	case ctx.Err() != nil:
		s.endInterception(ic, ctx.Err())
		return ctx.Err()
	default:
		s.endInterception(ic, err)
		return err
	}
}

// startInterception records a new interception and runs register under the
// session lock so a concurrent shutdown cannot miss it.
func (s *Session) startInterception(ctx context.Context, lane, model string, register func(*interception)) (*interception, error) {
	ic := &interception{id: uuid.NewString(), lane: lane}
	if err := s.opts.Recorder.RecordInterception(ctx, &recorder.InterceptionRecord{
		ID:              ic.id,
		InitiatorID:     s.actor.ID,
		Metadata:        s.actor.Metadata,
		Model:           model,
		Provider:        s.opts.Provider.Type(),
		ProviderName:    s.opts.Provider.Name(),
		StartedAt:       time.Now(),
		ClientSessionID: s.opts.ClientSessionID,
		Client:          s.opts.Client,
		UserAgent:       s.opts.UserAgent,
		CredentialKind:  s.opts.CredentialKind,
		CredentialHint:  s.opts.CredentialHint,
	}); err != nil {
		return nil, xerrors.Errorf("record interception: %w", err)
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		s.recordEnded(ic, context.Cause(s.ctx))
		return nil, ErrClosed
	}
	s.open[ic.id] = ic
	register(ic)
	s.mu.Unlock()
	return ic, nil
}

func (s *Session) handleServerEvent(frame []byte) {
	ctx, cancel := recordContext(s.ctx)
	defer cancel()
	lane := gjson.GetBytes(frame, "stream_id").String()
	steerTarget := gjson.GetBytes(frame, "steer.previous_response_id").String()
	var ic *interception
	switch eventType := gjson.GetBytes(frame, "type").String(); eventType {
	case eventCreated:
		ic = s.bindCreated(ctx, lane, frame)
	case eventCompleted, eventFailed, eventIncomplete:
		s.finishResponse(ctx, eventType, frame)
		return
	case eventSteerAccepted, eventSteerPending, eventSteerFailed:
		s.mu.Lock()
		ic = s.responses[steerTarget]
		if ic != nil && eventType == eventSteerAccepted {
			s.steered[steerTarget] = true
		}
		s.mu.Unlock()
		if ic != nil {
			s.opts.Observer.ServerEvent(ctx, ic.id, frame)
		}
		if eventType != eventSteerAccepted {
			// The steer waits for client input or was dropped, so no
			// automatic continuation follows.
			s.cancelContinuation(steerTarget)
		}
		return
	case eventError:
		// Error events are request-scoped: they reject the oldest pending
		// create on their lane. In-flight responses end via response.failed.
		s.mu.Lock()
		if q := s.pending[lane]; len(q) > 0 {
			ic = q[0]
		}
		s.mu.Unlock()
		if ic == nil {
			return
		}
		s.opts.Observer.ServerEvent(ctx, ic.id, frame)
		errObj := gjson.GetBytes(frame, "error")
		code, message := errObj.Get("code").String(), errObj.Get("message").String()
		if !errObj.Exists() {
			code, message = gjson.GetBytes(frame, "code").String(), gjson.GetBytes(frame, "message").String()
		}
		s.endInterception(ic, upstreamError(int(gjson.GetBytes(frame, "status").Int()), errObj.Get("type").String(), code, message))
		return
	default:
		s.mu.Lock()
		ic = s.active[lane]
		s.mu.Unlock()
	}
	if ic != nil {
		s.opts.Observer.ServerEvent(ctx, ic.id, frame)
	}
}

// bindCreated binds a new response to the interception that explains it: the
// oldest pending create on the lane, else a steered interception awaiting its
// continuation. Otherwise the response opens its own interception so nothing
// goes unrecorded. Admission is not applied to such responses.
func (s *Session) bindCreated(ctx context.Context, lane string, frame []byte) *interception {
	responseID := gjson.GetBytes(frame, "response.id").String()
	bind := func(ic *interception) {
		s.active[lane] = ic
		if responseID != "" {
			s.responses[responseID] = ic
			ic.responseIDs = append(ic.responseIDs, responseID)
		}
	}
	s.mu.Lock()
	var ic *interception
	if q := s.pending[lane]; len(q) > 0 {
		ic = q[0]
		setQueue(s.pending, lane, q[1:])
	} else if q := s.continuations[lane]; len(q) > 0 {
		ic = q[0]
		setQueue(s.continuations, lane, q[1:])
	}
	if ic != nil {
		bind(ic)
		s.mu.Unlock()
		return ic
	}
	s.mu.Unlock()

	model := gjson.GetBytes(frame, "response.model").String()
	ic, err := s.startInterception(ctx, lane, model, bind)
	if err != nil {
		s.opts.Logger.Warn(ctx, "failed to record unexplained response", slog.Error(err), slog.F("response_id", responseID))
		return nil
	}
	return ic
}

func (s *Session) finishResponse(ctx context.Context, eventType string, frame []byte) {
	response := gjson.GetBytes(frame, "response")
	responseID := response.Get("id").String()
	s.mu.Lock()
	ic := s.responses[responseID]
	if ic != nil && s.active[ic.lane] == ic {
		delete(s.active, ic.lane)
	}
	s.mu.Unlock()
	if ic == nil {
		s.opts.Logger.Warn(ctx, "ignoring terminal event for unknown response", slog.F("type", eventType), slog.F("response_id", responseID))
		return
	}
	s.opts.Observer.ServerEvent(ctx, ic.id, frame)

	// Per the steering guide, the server starts a continuation response on
	// the same lane after a steer ends a response as incomplete with reason
	// "steered", or after a response with an accepted, unapplied steer
	// completes normally. The continuation stays in the steered interception.
	// Both linkages are unverified against a live trace.
	s.mu.Lock()
	continues := (eventType == eventIncomplete && response.Get("incomplete_details.reason").String() == reasonSteered) ||
		(eventType == eventCompleted && s.steered[responseID])
	delete(s.steered, responseID)
	if continues && s.open[ic.id] != nil {
		s.continuations[ic.lane] = append(s.continuations[ic.lane], ic)
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()
	var err error
	if eventType == eventFailed {
		code, message := response.Get("error.code").String(), response.Get("error.message").String()
		err = upstreamError(codeStatus(code), "", code, message)
	}
	s.endInterception(ic, err)
}

// cancelContinuation drops the steer state of responseID. If its interception
// was waiting for an automatic continuation, it ends successfully.
func (s *Session) cancelContinuation(responseID string) {
	s.mu.Lock()
	delete(s.steered, responseID)
	ic := s.responses[responseID]
	if ic == nil || !slices.Contains(s.continuations[ic.lane], ic) {
		s.mu.Unlock()
		return
	}
	s.forgetLocked(ic)
	s.mu.Unlock()
	s.recordEnded(ic, nil)
}

func (s *Session) endInterception(ic *interception, err error) {
	s.mu.Lock()
	if s.open[ic.id] == nil {
		s.mu.Unlock()
		return
	}
	s.forgetLocked(ic)
	s.mu.Unlock()
	s.recordEnded(ic, err)
}

func (s *Session) endAll(err error) {
	s.mu.Lock()
	s.closed = true
	ended := slices.Collect(maps.Values(s.open))
	clear(s.open)
	clear(s.pending)
	clear(s.continuations)
	clear(s.active)
	clear(s.responses)
	clear(s.steered)
	s.mu.Unlock()
	if err == nil {
		err = ErrClosed
	}
	for _, ic := range ended {
		s.recordEnded(ic, err)
	}
}

func (s *Session) forgetLocked(ic *interception) {
	delete(s.open, ic.id)
	for _, id := range ic.responseIDs {
		delete(s.responses, id)
		delete(s.steered, id)
	}
	if s.active[ic.lane] == ic {
		delete(s.active, ic.lane)
	}
	isIC := func(other *interception) bool { return other == ic }
	setQueue(s.pending, ic.lane, slices.DeleteFunc(s.pending[ic.lane], isIC))
	setQueue(s.continuations, ic.lane, slices.DeleteFunc(s.continuations[ic.lane], isIC))
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

func (s *Session) recordEnded(ic *interception, err error) {
	ctx, cancel := recordContext(s.ctx)
	defer cancel()
	s.opts.Observer.InterceptionEnded(ctx, ic.id)
	errType, message := interceptionerror.Categorize(s.opts.Provider, err)
	if err := s.opts.Recorder.RecordInterceptionEnded(ctx, &recorder.InterceptionRecordEnded{
		ID:             ic.id,
		EndedAt:        time.Now(),
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

// recordContext detaches recording from cancellation, bounded like async
// recordings, so a closing session or caller still persists what it saw.
func recordContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), recorder.DefaultAsyncTimeout)
}

// upstreamError wraps an upstream error event so provider categorizers see
// the OpenAI error envelope and status.
func upstreamError(status int, errType, code, message string) error {
	if message == "" {
		message = code
	}
	if message == "" {
		message = "upstream error"
	}
	return intercept.NewResponseError(message, errType, code, status, 0)
}

// codeStatus maps response.failed error codes, which carry no HTTP status, to
// the status the equivalent HTTP error would have.
func codeStatus(code string) int {
	switch code {
	case intercept.OpenAIErrCodeRateLimit:
		return http.StatusTooManyRequests
	case intercept.OpenAIErrCodeServer:
		return http.StatusInternalServerError
	case "":
		return 0
	default:
		return http.StatusBadRequest
	}
}
