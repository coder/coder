package agentacp

import (
	"context"
	"encoding/json"
	"maps"
	"slices"
	"strings"
	"sync"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	acp "github.com/coder/acp-go-sdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

type assistantText struct {
	seq  uint64
	text string
}

// ACPUpdate is a session notification or the result of an identified RPC.
// A result retains the request and turn it belongs to.
type ACPUpdate struct {
	Notification *acp.SessionNotification
	Prompt       *acp.PromptResponse
	RequestID    uuid.UUID
	Turn         uint64
	Error        error
}

// ACPUpdateProcessor serializes incoming ACP messages with session operations.
type ACPUpdateProcessor interface {
	ProcessACPUpdate(context.Context, ACPUpdate) error
}

var _ ACPUpdateProcessor = (*SessionFSM)(nil)

type sessionOperation string

const (
	operationMessage            sessionOperation = "message"
	operationInterrupt          sessionOperation = "interrupt"
	operationFinishRunning      sessionOperation = "finish_running"
	operationFinishInterruption sessionOperation = "finish_interruption"
	operationSetError           sessionOperation = "set_error"
)

var sessionTransitions = map[workspacesdk.ACPSessionStatus]map[sessionOperation]workspacesdk.ACPSessionStatus{
	workspacesdk.ACPSessionStatusIdle: {
		operationMessage:  workspacesdk.ACPSessionStatusRunning,
		operationSetError: workspacesdk.ACPSessionStatusError,
	},
	workspacesdk.ACPSessionStatusRunning: {
		operationInterrupt:     workspacesdk.ACPSessionStatusInterrupting,
		operationFinishRunning: workspacesdk.ACPSessionStatusIdle,
		operationSetError:      workspacesdk.ACPSessionStatusError,
	},
	workspacesdk.ACPSessionStatusInterrupting: {
		operationFinishInterruption: workspacesdk.ACPSessionStatusIdle,
		operationSetError:           workspacesdk.ACPSessionStatusError,
	},
}

func (s *SessionFSM) transitionLocked(operation sessionOperation) error {
	target, ok := sessionTransitions[s.info.Status][operation]
	if !ok {
		return xerrors.Errorf("%w: %s in %s", ErrConflict, operation, s.info.Status)
	}
	s.info.Status = target
	return nil
}

type sessionSubscriber struct {
	events chan workspacesdk.ACPEvent
	stop   func() bool
}

// SessionFSM owns operations and the ongoing transcript of one initialized
// adapter runtime. mu protects all mutable fields below it.
type SessionFSM struct {
	ctx     context.Context
	cancel  context.CancelFunc
	id      workspacesdk.ACPSessionID
	request workspacesdk.ACPCreateSessionRequest
	proc    *process
	run     func(func()) bool

	mu          sync.Mutex
	closed      bool
	info        workspacesdk.ACPSession
	events      []workspacesdk.ACPEvent
	assistant   []assistantText
	subscribers map[*sessionSubscriber]struct{}
	turn        uint64
}

func newSessionFSM(ctx context.Context, req workspacesdk.ACPCreateSessionRequest, info workspacesdk.ACPSession, p *process, run func(func()) bool) *SessionFSM {
	ctx, cancel := context.WithCancel(ctx)
	s := &SessionFSM{
		ctx: ctx, cancel: cancel, id: info.ID, request: req, info: info, proc: p, run: run,
		subscribers: make(map[*sessionSubscriber]struct{}),
	}
	context.AfterFunc(ctx, s.shutdown)
	return s
}

// lock acquires mu on success. Callers must unlock it after their operation.
// Cancellation is checked again after waiting for an earlier operation.
func (s *SessionFSM) lock(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	if err := ctx.Err(); err != nil {
		s.mu.Unlock()
		return err
	}
	if s.closed || s.ctx.Err() != nil {
		s.mu.Unlock()
		return ErrUnavailable
	}
	return nil
}

// shutdown cancels pending work before waiting for its critical section.
// The manager closes the transport separately to unblock synchronous writes.
func (s *SessionFSM) shutdown() {
	s.cancel()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	s.closed = true
	for subscriber := range s.subscribers {
		s.unsubscribeLocked(subscriber)
	}
}

// ProcessACPUpdate applies an incoming message only in running or interrupting.
func (s *SessionFSM) ProcessACPUpdate(ctx context.Context, update ACPUpdate) error {
	if err := s.lock(ctx); err != nil {
		return err
	}
	defer s.mu.Unlock()
	return s.processUpdateLocked(update)
}

func (s *SessionFSM) adapterError(ctx context.Context, err error) error {
	if lockErr := s.lock(ctx); lockErr != nil {
		return lockErr
	}
	defer s.mu.Unlock()
	return s.setErrorLocked(err)
}

// Read returns an immutable atomic snapshot after an optional cursor.
func (s *SessionFSM) Read(ctx context.Context, after *workspacesdk.ACPCursor) (workspacesdk.ACPSessionResponse, error) {
	if err := s.lock(ctx); err != nil {
		return workspacesdk.ACPSessionResponse{}, err
	}
	defer s.mu.Unlock()
	return s.snapshotLocked(after)
}

// Watch atomically returns a snapshot and a stream of subsequent events.
// The stream stays open across turns until ctx is canceled, the FSM stops,
// or its 64-event buffer fills. Callers must cancel ctx when finished.
func (s *SessionFSM) Watch(ctx context.Context, after *workspacesdk.ACPCursor) (workspacesdk.ACPSessionResponse, <-chan workspacesdk.ACPEvent, error) {
	if err := s.lock(ctx); err != nil {
		return workspacesdk.ACPSessionResponse{}, nil, err
	}
	defer s.mu.Unlock()
	r, err := s.snapshotLocked(after)
	if err != nil {
		return r, nil, err
	}
	subscriber := &sessionSubscriber{events: make(chan workspacesdk.ACPEvent, 64)}
	s.subscribers[subscriber] = struct{}{}
	subscriber.stop = context.AfterFunc(ctx, func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.unsubscribeLocked(subscriber)
	})
	return r, subscriber.events, nil
}

// Wait returns the retained output and completion or failure. Reader
// cancellation leaves the harness and transcript untouched.
func (s *SessionFSM) Wait(ctx context.Context) (workspacesdk.ACPSessionResponse, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	r, events, err := s.Watch(ctx, nil)
	if err != nil {
		return r, err
	}
	return waitSession(ctx, r, events)
}

func waitSession(ctx context.Context, r workspacesdk.ACPSessionResponse, events <-chan workspacesdk.ACPEvent) (workspacesdk.ACPSessionResponse, error) {
	var assistant strings.Builder
	_, _ = assistant.WriteString(r.AssistantResponse)
	for {
		switch r.Session.Status {
		case workspacesdk.ACPSessionStatusIdle:
			return r, nil
		case workspacesdk.ACPSessionStatusError:
			return r, xerrors.New(r.Session.Error)
		}
		select {
		case <-ctx.Done():
			return r, ctx.Err()
		case event, ok := <-events:
			if !ok {
				if err := ctx.Err(); err != nil {
					return r, err
				}
				return r, ErrUnavailable
			}
			r.Events = append(r.Events, event)
			if event.Session != nil {
				r.Session = *event.Session
			}
			r.Session.Cursor = event.Cursor
			if event.Kind == workspacesdk.ACPEventKindUpdate {
				var update acp.SessionUpdate
				if err := json.Unmarshal(event.Update, &update); err != nil {
					return r, xerrors.Errorf("decode ACP update: %w", err)
				}
				if chunk := update.AgentMessageChunk; chunk != nil && chunk.Content.Text != nil {
					_, _ = assistant.WriteString(chunk.Content.Text.Text)
					r.AssistantResponse = assistant.String()
				}
			}
		}
	}
}

func (s *SessionFSM) unsubscribeLocked(subscriber *sessionSubscriber) {
	if _, ok := s.subscribers[subscriber]; !ok {
		return
	}
	delete(s.subscribers, subscriber)
	subscriber.stop()
	close(subscriber.events)
}

// Message writes a prompt while idle and returns after stdin admission.
// Later completion and failure are available through Read, Watch, and Wait.
func (s *SessionFSM) Message(ctx context.Context, req workspacesdk.ACPMessageRequest) (workspacesdk.ACPMessageResponse, error) {
	if req.ID == uuid.Nil || strings.TrimSpace(req.Text) == "" {
		return workspacesdk.ACPMessageResponse{}, ErrInvalid
	}
	if err := s.lock(ctx); err != nil {
		return workspacesdk.ACPMessageResponse{}, err
	}
	defer s.mu.Unlock()
	// Keep the transition and transcript changes atomic with the stdin write.
	if err := s.transitionLocked(operationMessage); err != nil {
		return workspacesdk.ACPMessageResponse{}, err
	}
	s.turn++
	turn := s.turn
	written := s.proc.expectWrite("session/prompt")
	completed := make(chan error, 1)
	started := s.run(func() {
		update := ACPUpdate{RequestID: req.ID, Turn: turn}
		result, err := s.proc.conn.Prompt(s.ctx, acp.PromptRequest{SessionId: acp.SessionId(s.id.SessionID), Prompt: []acp.ContentBlock{acp.TextBlock(req.Text)}})
		update.Prompt, update.Error = &result, err
		completed <- update.Error
		if update.Error != nil {
			select {
			case <-s.proc.conn.Done():
				return
			default:
			}
		}
		_ = s.ProcessACPUpdate(s.ctx, update)
	})
	var err error
	if !started {
		err = ErrUnavailable
	} else {
		select {
		case err = <-written:
		case err = <-completed:
			select {
			case err = <-written:
			default:
			}
		case <-s.ctx.Done():
			err = ErrUnavailable
		}
	}
	if err != nil {
		_ = s.setErrorLocked(err)
		return workspacesdk.ACPMessageResponse{}, err
	}
	historyComplete := s.info.HistoryComplete && len(s.events) == 0
	s.events, s.assistant = nil, nil
	s.info.Cursor = workspacesdk.ACPCursor{Epoch: uuid.New()}
	s.info.Error, s.info.StopReason = "", ""
	s.info.HistoryComplete = historyComplete
	s.appendEventLocked(workspacesdk.ACPEventKindUserMessage, req.Text, nil, req.ID)
	s.appendEventLocked(workspacesdk.ACPEventKindStatus, "", nil)
	return workspacesdk.ACPMessageResponse{Outcome: workspacesdk.ACPMessageOutcomePrompt}, nil
}

// Interrupt writes one cancellation notification, without retrying it.
func (s *SessionFSM) Interrupt(ctx context.Context) (workspacesdk.ACPSession, error) {
	if err := s.lock(ctx); err != nil {
		return workspacesdk.ACPSession{}, err
	}
	defer s.mu.Unlock()
	if err := s.transitionLocked(operationInterrupt); err != nil {
		return workspacesdk.ACPSession{}, err
	}
	err := s.proc.conn.Cancel(s.ctx, acp.CancelNotification{SessionId: acp.SessionId(s.id.SessionID)})
	if err != nil {
		_ = s.setErrorLocked(err)
		return workspacesdk.ACPSession{}, err
	}
	s.appendEventLocked(workspacesdk.ACPEventKindStatus, "", nil)
	return s.sessionInfoLocked(), nil
}

func (s *SessionFSM) processUpdateLocked(update ACPUpdate) error {
	if s.info.Status != workspacesdk.ACPSessionStatusRunning && s.info.Status != workspacesdk.ACPSessionStatusInterrupting {
		return xerrors.Errorf("%w: ACP update in %s", ErrConflict, s.info.Status)
	}
	switch {
	case update.Notification != nil:
		req := *update.Notification
		if string(req.SessionId) != s.id.SessionID {
			return xerrors.Errorf("%w: notification session ID mismatch", ErrInvalid)
		}
		if req.Update.UserMessageChunk != nil {
			return nil
		}
		return s.appendNotificationLocked(req)
	case update.Prompt != nil:
		if update.Turn != s.turn {
			return nil
		}
		if update.Error != nil {
			return s.setErrorLocked(update.Error)
		}
		s.finishLocked(string(update.Prompt.StopReason))
		return nil
	default:
		return xerrors.Errorf("%w: empty ACP update", ErrInvalid)
	}
}

func (s *SessionFSM) finishLocked(stopReason string) {
	operation := operationFinishRunning
	if s.info.Status == workspacesdk.ACPSessionStatusInterrupting {
		operation = operationFinishInterruption
	}
	if s.transitionLocked(operation) != nil {
		return
	}
	s.info.StopReason = stopReason
	s.appendEventLocked(workspacesdk.ACPEventKindStatus, "", nil)
}

func (s *SessionFSM) setErrorLocked(err error) error {
	if err := s.transitionLocked(operationSetError); err != nil {
		return err
	}
	s.info.Error = err.Error()
	s.appendEventLocked(workspacesdk.ACPEventKindError, err.Error(), nil)
	s.appendEventLocked(workspacesdk.ACPEventKindStatus, "", nil)
	return nil
}

func (s *SessionFSM) appendNotificationLocked(req acp.SessionNotification) error {
	raw, err := json.Marshal(req.Update)
	if err != nil {
		return err
	}
	s.appendEventLocked(workspacesdk.ACPEventKindUpdate, "", raw)
	if chunk := req.Update.AgentMessageChunk; chunk != nil && chunk.Content.Text != nil {
		s.assistant = append(s.assistant, assistantText{seq: s.info.Cursor.Seq, text: chunk.Content.Text.Text})
	}
	return nil
}

func (s *SessionFSM) sessionInfoLocked() workspacesdk.ACPSession {
	info := s.info
	info.Config = maps.Clone(info.Config)
	info.Harness = cloneHarness(info.Harness)
	return info
}

func (s *SessionFSM) appendEventLocked(kind workspacesdk.ACPEventKind, text string, raw json.RawMessage, ids ...uuid.UUID) {
	s.info.Cursor.Seq++
	e := workspacesdk.ACPEvent{Cursor: s.info.Cursor, Kind: kind, Text: text, Update: slices.Clone(raw)}
	if len(ids) != 0 {
		e.MessageID = new(ids[0])
	}
	if kind == workspacesdk.ACPEventKindStatus || kind == workspacesdk.ACPEventKindReset {
		e.Session = new(s.sessionInfoLocked())
	}
	s.events = append(s.events, e)
	for subscriber := range s.subscribers {
		select {
		case subscriber.events <- cloneEvent(e):
		default:
			s.unsubscribeLocked(subscriber)
		}
	}
}

func (s *SessionFSM) snapshotLocked(after *workspacesdk.ACPCursor) (workspacesdk.ACPSessionResponse, error) {
	start := uint64(0)
	if after != nil {
		if after.Epoch != s.info.Cursor.Epoch || after.Seq > s.info.Cursor.Seq {
			return workspacesdk.ACPSessionResponse{}, xerrors.Errorf("%w: transcript cursor requires reset", ErrConflict)
		}
		start = after.Seq
	}
	r := workspacesdk.ACPSessionResponse{Session: s.sessionInfoLocked(), Events: make([]workspacesdk.ACPEvent, 0, len(s.events[start:]))}
	for _, event := range s.events[start:] {
		r.Events = append(r.Events, cloneEvent(event))
	}
	var text strings.Builder
	for _, part := range s.assistant {
		if part.seq > start {
			_, _ = text.WriteString(part.text)
		}
	}
	r.AssistantResponse = text.String()
	return r, nil
}
