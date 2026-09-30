// Package responsesws records OpenAI Responses API WebSocket mode sessions
// while relaying every frame unchanged in both directions. It contains no
// transport code: callers supply the upstream text-message connection and
// relay client frames through a Session.
package responsesws

import (
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

	reasonSteered = "steered"
)

// ErrClosed is returned after the session has been closed.
var ErrClosed = xerrors.New("responses websocket session closed")

// MessageConn is the upstream text-message connection. Read and Write must
// honor ctx. Close must unblock pending Read and Write calls.
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
	interceptionerror.Categorizer
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
	CredentialKind  string
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
	}
	go s.readLoop()
	return s, nil
}

// Send forwards one client frame upstream unchanged. A refused or
// unrecordable response.create is not forwarded; the client receives an
// error event from Recv instead and the session stays open.
func (s *Session) Send(ctx context.Context, frame []byte) error {
	s.mu.Lock()
	closed := s.closed
	s.mu.Unlock()
	if closed || s.ctx.Err() != nil {
		return ErrClosed
	}
	switch gjson.GetBytes(frame, "type").String() {
	case eventCreate:
		return s.sendCreate(ctx, frame)
	case eventSteer:
		s.observeSteer(ctx, frame)
	}
	if err := s.upstream.Write(ctx, frame); err != nil {
		return xerrors.Errorf("write upstream: %w", err)
	}
	return nil
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
		s.endAll(err)
		return
	}
}

func (s *Session) sendCreate(ctx context.Context, frame []byte) error {
	lane := gjson.GetBytes(frame, "stream_id").String()
	model := gjson.GetBytes(frame, "model").String()
	log := s.opts.Logger.With(slog.F("model", model), slog.F("stream_id", lane))
	if s.opts.Admit != nil {
		if err := s.opts.Admit(ctx, model); err != nil {
			log.Info(ctx, "response.create refused by admission", slog.Error(err))
			if respErr, ok := errors.AsType[*intercept.ResponseError](err); ok && respErr.ErrorObject != nil {
				return s.enqueueError(ctx, lane, respErr.StatusCode, respErr.ErrorObject.Type, respErr.ErrorObject.Code, respErr.ErrorObject.Message)
			}
			// Refusals are policy decisions, not gateway faults: use a
			// non-retryable 4xx so SDKs do not retry them.
			return s.enqueueError(ctx, lane, http.StatusForbidden, "invalid_request_error", "request_refused", "request refused by AI Gateway")
		}
	}
	ic, err := s.startInterception(ctx, lane, model, func(ic *interception) {
		s.pending[lane] = append(s.pending[lane], ic)
	})
	if err != nil {
		log.Warn(ctx, "failed to record interception", slog.Error(err))
		if errors.Is(err, ErrClosed) {
			return ErrClosed
		}
		return s.enqueueError(ctx, lane, http.StatusInternalServerError, intercept.OpenAIErrTypeAPI, intercept.OpenAIErrCodeServer, "failed to record interception")
	}
	s.opts.Observer.ClientEvent(ctx, ic.id, frame)
	if err := s.upstream.Write(ctx, frame); err != nil {
		s.endInterception(ic, err)
		return xerrors.Errorf("write response.create upstream: %w", err)
	}
	return nil
}

func (s *Session) observeSteer(ctx context.Context, frame []byte) {
	previous := gjson.GetBytes(frame, "previous_response_id").String()
	s.mu.Lock()
	ic := s.responses[previous]
	s.mu.Unlock()
	if ic != nil {
		s.opts.Observer.ClientEvent(ctx, ic.id, frame)
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
	ctx := s.ctx
	lane := gjson.GetBytes(frame, "stream_id").String()
	var ic *interception
	switch gjson.GetBytes(frame, "type").String() {
	case eventCreated:
		ic = s.bindCreated(lane, frame)
	case eventCompleted, eventFailed, eventIncomplete:
		s.finishResponse(lane, gjson.GetBytes(frame, "type").String(), frame)
		return
	case eventError:
		s.mu.Lock()
		ic = s.active[lane]
		if ic == nil && len(s.pending[lane]) > 0 {
			ic = s.pending[lane][0]
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

// bindCreated binds a new response to the interception that explains it: a
// steered continuation first, then the oldest pending create on the lane.
// Otherwise the response opens its own interception so nothing goes
// unrecorded. Admission is not applied to such responses.
func (s *Session) bindCreated(lane string, frame []byte) *interception {
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
	if q := s.continuations[lane]; len(q) > 0 {
		ic, s.continuations[lane] = q[0], q[1:]
	} else if q := s.pending[lane]; len(q) > 0 {
		ic, s.pending[lane] = q[0], q[1:]
	}
	if ic != nil {
		bind(ic)
		s.mu.Unlock()
		return ic
	}
	s.mu.Unlock()

	model := gjson.GetBytes(frame, "response.model").String()
	ic, err := s.startInterception(s.ctx, lane, model, bind)
	if err != nil {
		s.opts.Logger.Warn(s.ctx, "failed to record unexplained response", slog.Error(err), slog.F("response_id", responseID))
		return nil
	}
	return ic
}

func (s *Session) finishResponse(lane, eventType string, frame []byte) {
	response := gjson.GetBytes(frame, "response")
	s.mu.Lock()
	ic := s.responses[response.Get("id").String()]
	if ic == nil {
		ic = s.active[lane]
	}
	if ic != nil && s.active[lane] == ic {
		delete(s.active, lane)
	}
	s.mu.Unlock()
	if ic == nil {
		return
	}
	s.opts.Observer.ServerEvent(s.ctx, ic.id, frame)

	// A steer that interrupts a response ends it as incomplete with reason
	// "steered", and the server then starts a continuation response on the
	// same lane. The continuation stays in the steered interception. This
	// linkage follows the steering guide and is unverified against a live
	// trace. If no continuation follows (for example the steer waits for a
	// tool result), the next response on the lane binds to this interception.
	if eventType == eventIncomplete && response.Get("incomplete_details.reason").String() == reasonSteered {
		s.mu.Lock()
		if s.open[ic.id] != nil {
			s.continuations[lane] = append(s.continuations[lane], ic)
		}
		s.mu.Unlock()
		return
	}
	var err error
	if eventType == eventFailed {
		code, message := response.Get("error.code").String(), response.Get("error.message").String()
		err = upstreamError(codeStatus(code), "", code, message)
	}
	s.endInterception(ic, err)
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
	}
	if s.active[ic.lane] == ic {
		delete(s.active, ic.lane)
	}
	isIC := func(other *interception) bool { return other == ic }
	s.pending[ic.lane] = slices.DeleteFunc(s.pending[ic.lane], isIC)
	s.continuations[ic.lane] = slices.DeleteFunc(s.continuations[ic.lane], isIC)
}

func (s *Session) recordEnded(ic *interception, err error) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(s.ctx), recorder.DefaultAsyncTimeout)
	defer cancel()
	s.opts.Observer.InterceptionEnded(ctx, ic.id)
	errType, message := interceptionerror.Categorize(s.opts.Provider, err, 0)
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
// carries both the Responses streaming error shape (top-level code, message,
// param) and the WebSocket mode shape (status and a nested error object),
// plus stream_id, so clients parsing either shape understand it.
func (s *Session) enqueueError(ctx context.Context, lane string, status int, errType, code, message string) error {
	ev, err := json.Marshal(responses.ResponseErrorEvent{Code: code, Message: message})
	if err == nil {
		ev, err = sjson.SetBytes(ev, "status", status)
	}
	if err == nil {
		ev, err = sjson.SetBytes(ev, "error", map[string]string{"type": errType, "code": code, "message": message, "param": ""})
	}
	if err == nil && lane != "" {
		ev, err = sjson.SetBytes(ev, "stream_id", lane)
	}
	if err != nil {
		return xerrors.Errorf("encode error event: %w", err)
	}
	select {
	case s.errors <- ev:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-s.ctx.Done():
		return ErrClosed
	}
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
