package agentacp

import (
	"context"
	"encoding/json"
	"maps"
	"slices"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	acp "github.com/coder/acp-go-sdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

type assistantText struct {
	seq  uint64
	text string
}

// OperationResult reports stdin admission, followed by any later work failure.
// An unread result never prevents the event loop from making progress.
type OperationResult struct {
	Message workspacesdk.ACPMessageResponse
	Session workspacesdk.ACPSession
	Error   error
}

// ACPUpdate is a session notification or the result of an identified RPC.
// A result retains the request and turn it belongs to.
type ACPUpdate struct {
	Notification *acp.SessionNotification
	Prompt       *acp.PromptResponse
	Steering     *workspacesdk.ACPMessageResponse
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
		operationMessage:       workspacesdk.ACPSessionStatusRunning,
		operationInterrupt:     workspacesdk.ACPSessionStatusInterrupting,
		operationFinishRunning: workspacesdk.ACPSessionStatusIdle,
		operationSetError:      workspacesdk.ACPSessionStatusError,
	},
	workspacesdk.ACPSessionStatusInterrupting: {
		operationFinishInterruption: workspacesdk.ACPSessionStatusIdle,
		operationSetError:           workspacesdk.ACPSessionStatusError,
	},
}

func (s *SessionFSM) transition(operation sessionOperation) error {
	target, ok := sessionTransitions[s.info.Status][operation]
	if !ok {
		return xerrors.Errorf("%w: %s in %s", ErrConflict, operation, s.info.Status)
	}
	s.info.Status = target
	return nil
}

type sessionEventKind uint8

const (
	eventMessage sessionEventKind = iota
	eventInterrupt
	eventACPUpdate
	eventAdapterError
	eventRead
	eventWatch
)

type sessionEvent struct {
	kind      sessionEventKind
	message   workspacesdk.ACPMessageRequest
	update    ACPUpdate
	err       error
	after     *workspacesdk.ACPCursor
	reply     chan sessionReply
	operation chan OperationResult
}

type sessionReply struct {
	read    workspacesdk.ACPSessionResponse
	changed <-chan struct{}
	err     error
}

type steeringRequest struct {
	order       uint64
	adapterTurn uint64
}

// SessionFSM owns operations and the ongoing transcript of one initialized
// adapter runtime. All mutable fields below are accessed only by its loop.
type SessionFSM struct {
	ctx     context.Context
	cancel  context.CancelFunc
	id      workspacesdk.ACPSessionID
	request workspacesdk.ACPCreateSessionRequest
	proc    *process
	run     func(func()) bool
	queue   chan sessionEvent
	done    chan struct{}

	info               workspacesdk.ACPSession
	events             []workspacesdk.ACPEvent
	assistant          []assistantText
	changed            chan struct{}
	operations         []chan OperationResult
	turn               uint64
	detached           bool
	adapterStatus      string
	adapterTurn        uint64
	trackedAdapterTurn uint64
	steering           map[uuid.UUID]steeringRequest
	steeringOrder      uint64
	ownerOrder         uint64
	promptCompletion   *ACPUpdate
}

func newSessionFSM(ctx context.Context, req workspacesdk.ACPCreateSessionRequest, info workspacesdk.ACPSession, p *process, run func(func()) bool) *SessionFSM {
	return &SessionFSM{
		ctx: ctx, id: info.ID, request: req, info: info, proc: p, run: run,
		queue: make(chan sessionEvent), done: make(chan struct{}), changed: make(chan struct{}),
		steering: make(map[uuid.UUID]steeringRequest),
	}
}

func (s *SessionFSM) submit(ctx context.Context, e sessionEvent) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-s.done:
		return ErrUnavailable
	case <-s.ctx.Done():
		return ErrUnavailable
	case s.queue <- e:
		return nil
	}
}

func (s *SessionFSM) query(ctx context.Context, e sessionEvent) (sessionReply, error) {
	e.reply = make(chan sessionReply, 1)
	if err := s.submit(ctx, e); err != nil {
		return sessionReply{}, err
	}
	select {
	case <-ctx.Done():
		return sessionReply{}, ctx.Err()
	case <-s.done:
		return sessionReply{}, ErrUnavailable
	case r := <-e.reply:
		return r, r.err
	}
}

// Message submits a prompt or steering write. The channel receives admission
// and, if the work fails later, that failure before closing.
func (s *SessionFSM) Message(ctx context.Context, req workspacesdk.ACPMessageRequest) (<-chan OperationResult, error) {
	if req.ID == uuid.Nil || strings.TrimSpace(req.Text) == "" {
		return nil, ErrInvalid
	}
	reply := make(chan OperationResult, 2)
	if err := s.submit(ctx, sessionEvent{kind: eventMessage, message: req, operation: reply}); err != nil {
		return nil, err
	}
	return reply, nil
}

// Interrupt submits one cancellation notification, without retrying it.
func (s *SessionFSM) Interrupt(ctx context.Context) (<-chan OperationResult, error) {
	reply := make(chan OperationResult, 2)
	if err := s.submit(ctx, sessionEvent{kind: eventInterrupt, operation: reply}); err != nil {
		return nil, err
	}
	return reply, nil
}

// ProcessACPUpdate applies an incoming message only in running or interrupting.
func (s *SessionFSM) ProcessACPUpdate(ctx context.Context, update ACPUpdate) error {
	_, err := s.query(ctx, sessionEvent{kind: eventACPUpdate, update: update})
	return err
}

func (s *SessionFSM) adapterError(ctx context.Context, err error) error {
	_, result := s.query(ctx, sessionEvent{kind: eventAdapterError, err: err})
	return result
}

// Read returns an immutable atomic snapshot after an optional cursor.
func (s *SessionFSM) Read(ctx context.Context, after *workspacesdk.ACPCursor) (workspacesdk.ACPSessionResponse, error) {
	if after != nil {
		after = new(*after)
	}
	r, err := s.query(ctx, sessionEvent{kind: eventRead, after: after})
	return r.read, err
}

// Watch captures a snapshot and its wakeup channel in the same event.
func (s *SessionFSM) Watch(ctx context.Context, after *workspacesdk.ACPCursor) (workspacesdk.ACPSessionResponse, <-chan struct{}, error) {
	if after != nil {
		after = new(*after)
	}
	r, err := s.query(ctx, sessionEvent{kind: eventWatch, after: after})
	return r.read, r.changed, err
}

// Wait returns the retained output and completion or failure. Reader
// cancellation leaves the harness and transcript untouched.
func (s *SessionFSM) Wait(ctx context.Context) (workspacesdk.ACPSessionResponse, error) {
	for {
		r, changed, err := s.Watch(ctx, nil)
		if err != nil {
			return r, err
		}
		switch r.Session.Status {
		case workspacesdk.ACPSessionStatusIdle:
			return r, nil
		case workspacesdk.ACPSessionStatusError:
			return r, xerrors.New(r.Session.Error)
		}
		select {
		case <-ctx.Done():
			return r, ctx.Err()
		case <-s.done:
			return r, ErrUnavailable
		case <-changed:
		}
	}
}

func (s *SessionFSM) loop() {
	defer close(s.done)
	defer func() { close(s.changed) }()
	defer func() { s.finishOperations(ErrUnavailable) }()
	for {
		select {
		case <-s.ctx.Done():
			return
		case e := <-s.queue:
			r := sessionReply{}
			switch e.kind {
			case eventMessage:
				s.handleMessage(e)
			case eventInterrupt:
				s.handleInterrupt(e)
			case eventACPUpdate:
				r.err = s.processUpdate(e.update)
			case eventAdapterError:
				r.err = s.setError(e.err)
			case eventRead, eventWatch:
				r.read, r.err = s.snapshot(e.after)
				r.changed = s.changed
			}
			if e.reply != nil {
				e.reply <- r
			}
		}
	}
}

func (s *SessionFSM) reject(e sessionEvent, err error) {
	e.operation <- OperationResult{Session: s.sessionInfo(), Error: err}
	close(e.operation)
}

func (s *SessionFSM) handleMessage(e sessionEvent) {
	req := e.message
	if s.info.Status != workspacesdk.ACPSessionStatusIdle && s.info.Status != workspacesdk.ACPSessionStatusRunning {
		s.reject(e, xerrors.Errorf("%w: message in %s", ErrConflict, s.info.Status))
		return
	}
	busy := s.info.Status == workspacesdk.ACPSessionStatusRunning
	if busy && !s.info.Harness.Steering {
		s.reject(e, xerrors.Errorf("%w: harness does not support steering", ErrConflict))
		return
	}
	// The loop publishes no snapshot until the synchronous write completes.
	if err := s.transition(operationMessage); err != nil {
		s.reject(e, err)
		return
	}
	method := "session/prompt"
	response := workspacesdk.ACPMessageResponse{Outcome: workspacesdk.ACPMessageOutcomePrompt}
	if busy {
		method = "_session/steering"
		response.Outcome = workspacesdk.ACPMessageOutcomeSteering
	} else {
		s.turn++
		s.detached = false
		s.trackedAdapterTurn = s.adapterTurn + 1
	}
	turn := s.turn
	written := s.proc.expectWrite(method)
	completed := make(chan error, 1)
	started := s.run(func() {
		update := ACPUpdate{RequestID: req.ID, Turn: turn}
		if busy {
			raw, err := s.proc.conn.CallExtension(s.ctx, "_session/steering", map[string]any{"sessionId": s.id.SessionID, "prompt": []acp.ContentBlock{acp.TextBlock(req.Text)}})
			result := workspacesdk.ACPMessageResponse{}
			if err == nil {
				err = json.Unmarshal(raw, &result)
			}
			if err == nil && result.Outcome != workspacesdk.ACPMessageOutcomeInjected && result.Outcome != workspacesdk.ACPMessageOutcomeStartedNewTurn {
				err = xerrors.New("steering was not accepted")
			}
			update.Steering, update.Error = &result, err
		} else {
			result, err := s.proc.conn.Prompt(s.ctx, acp.PromptRequest{SessionId: acp.SessionId(s.id.SessionID), Prompt: []acp.ContentBlock{acp.TextBlock(req.Text)}})
			update.Prompt, update.Error = &result, err
		}
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
		_ = s.setError(err)
		s.reject(e, err)
		return
	}
	if !busy {
		historyComplete := s.info.HistoryComplete && len(s.events) == 0
		s.events, s.assistant = nil, nil
		s.info.Cursor = workspacesdk.ACPCursor{Epoch: uuid.New()}
		s.info.Error, s.info.StopReason = "", ""
		s.info.HistoryComplete = historyComplete
		s.appendEvent(workspacesdk.ACPEventKindUserMessage, req.Text, nil, req.ID)
		s.appendEvent(workspacesdk.ACPEventKindStatus, "", nil)
	} else {
		s.appendEvent(workspacesdk.ACPEventKindUserMessage, req.Text, nil, req.ID)
		s.steeringOrder++
		s.steering[req.ID] = steeringRequest{order: s.steeringOrder, adapterTurn: s.adapterTurn}
	}
	s.operations = append(s.operations, e.operation)
	e.operation <- OperationResult{Message: response, Session: s.sessionInfo()}
}

func (s *SessionFSM) handleInterrupt(e sessionEvent) {
	if err := s.transition(operationInterrupt); err != nil {
		s.reject(e, err)
		return
	}
	err := s.proc.conn.Cancel(s.ctx, acp.CancelNotification{SessionId: acp.SessionId(s.id.SessionID)})
	if err != nil {
		_ = s.setError(err)
		s.reject(e, err)
		return
	}
	s.appendEvent(workspacesdk.ACPEventKindStatus, "", nil)
	s.operations = append(s.operations, e.operation)
	e.operation <- OperationResult{Session: s.sessionInfo()}
}

func (s *SessionFSM) processUpdate(update ACPUpdate) error {
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
		if err := s.appendNotification(req); err != nil {
			return err
		}
		if req.Update.SessionInfoUpdate != nil {
			if s.adapterStatus == "systemError" {
				return s.setError(ErrUnavailable)
			}
			if s.detached && len(s.steering) == 0 {
				s.completeDetached()
			}
		}
	case update.Steering != nil:
		request, ok := s.steering[update.RequestID]
		if !ok {
			return xerrors.Errorf("%w: unknown steering request", ErrInvalid)
		}
		delete(s.steering, update.RequestID)
		if update.Error != nil {
			return s.setError(update.Error)
		}
		if update.Steering.Outcome == workspacesdk.ACPMessageOutcomeStartedNewTurn {
			// Count every newly started adapter turn, including acknowledgements
			// that arrive after a newer steering request's response.
			s.trackedAdapterTurn = max(s.trackedAdapterTurn+1, request.adapterTurn+1)
			if request.order > s.ownerOrder {
				s.ownerOrder = request.order
				s.turn++
				s.detached = true
			}
		}
		if len(s.steering) == 0 {
			if s.detached {
				s.completeDetached()
			}
			if completion := s.promptCompletion; completion != nil {
				s.promptCompletion = nil
				if completion.Turn == s.turn {
					return s.completePrompt(*completion)
				}
			}
		}
	case update.Prompt != nil:
		if update.Turn != s.turn {
			return nil
		}
		if update.Error != nil {
			return s.setError(update.Error)
		}
		if len(s.steering) != 0 {
			s.promptCompletion = &update
			return nil
		}
		return s.completePrompt(update)
	default:
		return xerrors.Errorf("%w: empty ACP update", ErrInvalid)
	}
	return nil
}

func (s *SessionFSM) completePrompt(update ACPUpdate) error {
	if update.Error != nil {
		return s.setError(update.Error)
	}
	s.finish(string(update.Prompt.StopReason))
	return nil
}

func (s *SessionFSM) completeDetached() {
	if s.adapterTurn < s.trackedAdapterTurn || s.adapterStatus != "idle" {
		return
	}
	stopReason := string(acp.StopReasonEndTurn)
	s.finish(stopReason)
}

func (s *SessionFSM) finish(stopReason string) {
	operation := operationFinishRunning
	if s.info.Status == workspacesdk.ACPSessionStatusInterrupting {
		operation = operationFinishInterruption
	}
	if s.transition(operation) != nil {
		return
	}
	s.info.StopReason = stopReason
	s.detached = false
	s.appendEvent(workspacesdk.ACPEventKindStatus, "", nil)
	s.finishOperations(nil)
}

func (s *SessionFSM) setError(err error) error {
	if err := s.transition(operationSetError); err != nil {
		return err
	}
	s.info.Error = err.Error()
	s.appendEvent(workspacesdk.ACPEventKindError, err.Error(), nil)
	s.appendEvent(workspacesdk.ACPEventKindStatus, "", nil)
	s.finishOperations(err)
	return nil
}

func (s *SessionFSM) finishOperations(err error) {
	for _, reply := range s.operations {
		if err != nil {
			reply <- OperationResult{Error: err}
		}
		close(reply)
	}
	s.operations = nil
}

func (s *SessionFSM) appendNotification(req acp.SessionNotification) error {
	raw, err := json.Marshal(req.Update)
	if err != nil {
		return err
	}
	s.appendEvent(workspacesdk.ACPEventKindUpdate, "", raw)
	if chunk := req.Update.AgentMessageChunk; chunk != nil && chunk.Content.Text != nil {
		s.assistant = append(s.assistant, assistantText{seq: s.info.Cursor.Seq, text: chunk.Content.Text.Text})
	}
	if update := req.Update.SessionInfoUpdate; update != nil {
		codex, _ := update.Meta["codex"].(map[string]any)
		status, _ := codex["threadStatus"].(map[string]any)
		kind, _ := status["type"].(string)
		switch kind {
		case "active", "idle", "systemError":
			if kind == "active" && s.adapterStatus != "active" {
				s.adapterTurn++
			}
			s.adapterStatus = kind
		}
	}
	return nil
}

func (s *SessionFSM) sessionInfo() workspacesdk.ACPSession {
	info := s.info
	info.Config = maps.Clone(info.Config)
	info.Harness = cloneHarness(info.Harness)
	return info
}

func (s *SessionFSM) appendEvent(kind workspacesdk.ACPEventKind, text string, raw json.RawMessage, ids ...uuid.UUID) {
	s.info.Cursor.Seq++
	e := workspacesdk.ACPEvent{Cursor: s.info.Cursor, Kind: kind, Text: text, Update: slices.Clone(raw)}
	if len(ids) != 0 {
		e.MessageID = new(ids[0])
	}
	if kind == workspacesdk.ACPEventKindStatus || kind == workspacesdk.ACPEventKindReset {
		e.Session = new(s.sessionInfo())
	}
	s.events = append(s.events, e)
	close(s.changed)
	s.changed = make(chan struct{})
}

func (s *SessionFSM) snapshot(after *workspacesdk.ACPCursor) (workspacesdk.ACPSessionResponse, error) {
	start := uint64(0)
	if after != nil {
		if after.Epoch != s.info.Cursor.Epoch || after.Seq > s.info.Cursor.Seq {
			return workspacesdk.ACPSessionResponse{}, xerrors.Errorf("%w: transcript cursor requires reset", ErrConflict)
		}
		start = after.Seq
	}
	r := workspacesdk.ACPSessionResponse{Session: s.sessionInfo(), Events: make([]workspacesdk.ACPEvent, 0, len(s.events[start:]))}
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
