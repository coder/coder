package agentacp

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"slices"
	"strings"
	"sync"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	acp "github.com/coder/acp-go-sdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

type assistantText struct {
	seq  uint64
	text string
}

// ACPUpdate is a session notification or prompt result.
// A result retains the turn it belongs to.
type ACPUpdate struct {
	Notification *acp.SessionNotification
	Prompt       *acp.PromptResponse
	Turn         uint64
	Error        error
}

type sessionOperation string

const (
	operationFinishInitialization sessionOperation = "finish_initialization"
	operationMessage              sessionOperation = "message"
	operationInterrupt            sessionOperation = "interrupt"
	operationFinishRunning        sessionOperation = "finish_running"
	operationFinishInterruption   sessionOperation = "finish_interruption"
	operationSetError             sessionOperation = "set_error"
)

var sessionTransitions = map[workspacesdk.ACPSessionStatus]map[sessionOperation]workspacesdk.ACPSessionStatus{
	workspacesdk.ACPSessionStatusInitializing: {
		operationFinishInitialization: workspacesdk.ACPSessionStatusIdle,
		operationSetError:             workspacesdk.ACPSessionStatusError,
	},
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

func (s *SessionFSM) updateStatusLocked(operation sessionOperation) error {
	target, ok := sessionTransitions[s.info.Status][operation]
	if !ok {
		return xerrors.Errorf("%w: %s in %s", ErrConflict, operation, s.info.Status)
	}
	if s.info.Status == target {
		return nil
	}
	s.info.Status = target
	// Disable permission grants after interruption, completion, or failure.
	// A new running turn gets a fresh context. Repeated cancellation on
	// transitions between other states is harmless.
	s.cancelPermissions()
	if target == workspacesdk.ACPSessionStatusRunning {
		s.permissionCtx, s.cancelPermissions = context.WithCancel(s.ctx)
	}
	return nil
}

type sessionSubscriber struct {
	events chan workspacesdk.ACPEvent
	done   chan struct{}
}

// SessionFSM owns the process, tasks, operations, and ongoing transcript of one
// adapter runtime. mu protects all mutable fields below it and the session ID
// assigned during initialization.
type SessionFSM struct {
	ctx       context.Context
	cancel    context.CancelFunc
	id        workspacesdk.ACPSessionID
	request   sessionInitRequest
	proc      *process
	logger    slog.Logger
	wg        sync.WaitGroup
	closeOnce sync.Once

	mu          sync.Mutex
	closed      bool
	info        workspacesdk.ACPSession
	events      []workspacesdk.ACPEvent
	assistant   []assistantText
	subscribers map[*sessionSubscriber]struct{}
	turn        uint64

	permissionCtx     context.Context
	cancelPermissions context.CancelFunc
}

func newSessionFSM(ctx context.Context, req sessionInitRequest, info workspacesdk.ACPSession) *SessionFSM {
	ctx, cancel := context.WithCancel(ctx)
	permissionCtx, cancelPermissions := context.WithCancel(ctx)
	return &SessionFSM{
		ctx: ctx, cancel: cancel, id: info.ID, request: req, info: info,
		permissionCtx: permissionCtx, cancelPermissions: cancelPermissions,
		subscribers: make(map[*sessionSubscriber]struct{}),
	}
}

// startProcess binds a launched process and starts lifecycle monitoring.
// Call it before publishing or closing the session.
func (s *SessionFSM) startProcess(p *process, logger slog.Logger) {
	s.proc = p
	s.logger = logger
	s.wg.Go(func() { s.watchProcess(logger) })
}

func (s *SessionFSM) finishInitialization(nativeID string, harness workspacesdk.ACPHarness) error {
	unlock, err := s.lock(s.ctx)
	if err != nil {
		return err
	}
	defer unlock()
	if err := s.updateStatusLocked(operationFinishInitialization); err != nil {
		return err
	}
	s.id.SessionID = nativeID
	s.info.ID = s.id
	s.info.Harness = harness
	return nil
}

// lock acquires mu on success and returns the function to release it.
// Cancellation is checked again after waiting for an earlier operation.
func (s *SessionFSM) lock(ctx context.Context) (unlock func(), err error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	if err := ctx.Err(); err != nil {
		s.mu.Unlock()
		return nil, err
	}
	if s.closed || s.ctx.Err() != nil {
		s.mu.Unlock()
		return nil, ErrUnavailable
	}
	return s.mu.Unlock, nil
}

// Close stops the process and subscriptions and waits for session tasks.
func (s *SessionFSM) Close() {
	s.closeOnce.Do(func() {
		s.cancel()
		// Writes hold mu, so close the transport before waiting for the lock.
		if s.proc != nil {
			s.proc.close()
		}
		s.mu.Lock()
		s.closed = true
		for subscriber := range s.subscribers {
			s.unsubscribeLocked(subscriber)
		}
		s.mu.Unlock()
		// Tasks are admitted under mu; none can start after closed is set.
		s.wg.Wait()
	})
}

func (s *SessionFSM) watchProcess(logger slog.Logger) {
	p := s.proc
	select {
	case <-p.done:
	case <-p.conn.Done():
	case <-s.ctx.Done():
		return
	}
	p.close()
	if s.ctx.Err() != nil {
		return
	}
	// CODAGT-1453: The ACP SDK closes Done before queued notifications finish
	// and exposes no signal for their completion. Disconnect can put the FSM
	// in error before final updates arrive, causing those updates to be rejected.
	err := p.err
	if err == nil {
		err = xerrors.New("harness disconnected")
	}
	failure := harnessFailure(s.ctx, logger, p, err, harnessConfig{slug: s.id.HarnessSlug})
	if err := s.adapterError(s.ctx, failure); err != nil {
		logger.Debug(s.ctx, "record acp harness failure", slog.Error(err))
	}
}

// ProcessACPUpdate applies replayed notifications while initializing and live
// updates while running or interrupting.
func (s *SessionFSM) ProcessACPUpdate(ctx context.Context, update ACPUpdate) error {
	unlock, err := s.lock(ctx)
	if err != nil {
		return err
	}
	defer unlock()
	return s.processUpdateLocked(update)
}

func (s *SessionFSM) adapterError(ctx context.Context, err error) error {
	unlock, lockErr := s.lock(ctx)
	if lockErr != nil {
		return lockErr
	}
	defer unlock()
	return s.setErrorLocked(err)
}

// Read returns an immutable atomic snapshot after an optional cursor.
func (s *SessionFSM) Read(ctx context.Context, after *workspacesdk.ACPCursor) (workspacesdk.ACPSessionResponse, error) {
	unlock, err := s.lock(ctx)
	if err != nil {
		return workspacesdk.ACPSessionResponse{}, err
	}
	defer unlock()
	return s.snapshotLocked(after)
}

// Watch atomically returns a snapshot and a stream of subsequent events.
// The stream stays open across turns until ctx is canceled, the FSM stops,
// or its 64-event buffer fills. Callers must cancel ctx when finished.
func (s *SessionFSM) Watch(ctx context.Context, after *workspacesdk.ACPCursor) (workspacesdk.ACPSessionResponse, <-chan workspacesdk.ACPEvent, error) {
	unlock, err := s.lock(ctx)
	if err != nil {
		return workspacesdk.ACPSessionResponse{}, nil, err
	}
	defer unlock()
	r, err := s.snapshotLocked(after)
	if err != nil {
		return r, nil, err
	}
	subscriber := &sessionSubscriber{
		// Bound the backlog so a slow subscriber cannot block the session
		// or accumulate an unbounded queue. On overflow, appendEventLocked
		// removes the subscriber and closes this channel. After draining
		// queued events, the caller can Watch again with its last cursor to
		// replay missed events from the current transcript.
		events: make(chan workspacesdk.ACPEvent, 64),
		done:   make(chan struct{}),
	}
	s.subscribers[subscriber] = struct{}{}
	s.wg.Go(func() {
		select {
		case <-ctx.Done():
			s.mu.Lock()
			defer s.mu.Unlock()
			s.unsubscribeLocked(subscriber)
		case <-subscriber.done:
		}
	})
	return r, subscriber.events, nil
}

func (s *SessionFSM) unsubscribeLocked(subscriber *sessionSubscriber) {
	if _, ok := s.subscribers[subscriber]; !ok {
		return
	}
	delete(s.subscribers, subscriber)
	close(subscriber.done)
	close(subscriber.events)
}

// Message writes a prompt while idle and returns after stdin admission.
// Later completion and failure are available through Read and Watch.
func (s *SessionFSM) Message(ctx context.Context, req workspacesdk.ACPMessageRequest) (workspacesdk.ACPMessageResponse, error) {
	if strings.TrimSpace(req.Text) == "" {
		return workspacesdk.ACPMessageResponse{}, ErrInvalid
	}
	unlock, err := s.lock(ctx)
	if err != nil {
		return workspacesdk.ACPMessageResponse{}, err
	}
	defer unlock()
	// Keep the transition and transcript changes atomic with the stdin write.
	if err := s.updateStatusLocked(operationMessage); err != nil {
		return workspacesdk.ACPMessageResponse{}, err
	}
	s.turn++
	turn := s.turn
	// s.proc.conn.Prompt returns only after the harness finishes the entire turn,
	// but Message must return once the prompt is written to the harness's stdin.
	// Watch the write before calling s.proc.conn.Prompt so we can return then.
	written := s.proc.watchNextWrite("session/prompt")
	s.wg.Go(func() {
		update := ACPUpdate{Turn: turn}
		// s.proc.conn.Prompt returns only once the harness finishes the turn.
		result, err := s.proc.conn.Prompt(s.ctx, acp.PromptRequest{SessionId: acp.SessionId(s.id.SessionID), Prompt: []acp.ContentBlock{acp.TextBlock(req.Text)}})
		update.Prompt, update.Error = &result, err
		if update.Error != nil {
			select {
			case <-s.proc.conn.Done():
				return
			default:
			}
		}
		if err := s.ProcessACPUpdate(s.ctx, update); err != nil {
			s.logger.Debug(s.ctx, "process acp prompt result", slog.Error(err))
		}
	})
	select {
	case err = <-written:
	case <-s.ctx.Done():
		err = ErrUnavailable
	}
	if err != nil {
		transitionErr := s.setErrorLocked(err)
		return workspacesdk.ACPMessageResponse{}, errors.Join(err, transitionErr)
	}
	s.events, s.assistant = nil, nil
	s.info.Cursor = workspacesdk.ACPCursor{GenerationID: uuid.New()}
	s.info.Error, s.info.StopReason = "", ""
	s.appendEventLocked(workspacesdk.ACPEventKindStatus, "", nil)
	return workspacesdk.ACPMessageResponse{Outcome: workspacesdk.ACPMessageOutcomePrompt}, nil
}

// Interrupt writes one cancellation notification, without retrying it.
func (s *SessionFSM) Interrupt(ctx context.Context) (workspacesdk.ACPSession, error) {
	unlock, err := s.lock(ctx)
	if err != nil {
		return workspacesdk.ACPSession{}, err
	}
	defer unlock()
	if err := s.updateStatusLocked(operationInterrupt); err != nil {
		return workspacesdk.ACPSession{}, err
	}
	err = s.proc.conn.Cancel(s.ctx, acp.CancelNotification{SessionId: acp.SessionId(s.id.SessionID)})
	if err != nil {
		transitionErr := s.setErrorLocked(err)
		return workspacesdk.ACPSession{}, errors.Join(err, transitionErr)
	}
	s.appendEventLocked(workspacesdk.ACPEventKindStatus, "", nil)
	return s.sessionInfoLocked(), nil
}

func (s *SessionFSM) processUpdateLocked(update ACPUpdate) error {
	initializing := s.info.Status == workspacesdk.ACPSessionStatusInitializing
	if !initializing && s.info.Status != workspacesdk.ACPSessionStatusRunning && s.info.Status != workspacesdk.ACPSessionStatusInterrupting {
		return xerrors.Errorf("%w: ACP update in %s", ErrConflict, s.info.Status)
	}
	switch {
	case update.Notification != nil:
		req := *update.Notification
		// Before initialization assigns a new session's identity, its setup
		// notifications do not represent conversation history.
		if initializing && s.id.SessionID == "" {
			return nil
		}
		if string(req.SessionId) != s.id.SessionID {
			return xerrors.Errorf("%w: notification session ID mismatch", ErrInvalid)
		}
		if req.Update.UserMessageChunk != nil {
			if initializing {
				// Expose only the latest turn from session/load history.
				s.events, s.assistant = nil, nil
				s.info.Cursor = workspacesdk.ACPCursor{GenerationID: uuid.New()}
			}
			return nil
		}
		return s.appendNotificationLocked(req)
	case update.Prompt != nil:
		if initializing {
			return xerrors.Errorf("%w: prompt result while initializing", ErrConflict)
		}
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
	if s.updateStatusLocked(operation) != nil {
		return
	}
	s.info.StopReason = stopReason
	s.appendEventLocked(workspacesdk.ACPEventKindStatus, "", nil)
}

func (s *SessionFSM) setErrorLocked(err error) error {
	if err := s.updateStatusLocked(operationSetError); err != nil {
		return err
	}
	// Keep the FSM readable while stopping its failed runtime.
	s.proc.close()
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
	if update := req.Update.ConfigOptionUpdate; update != nil {
		s.info.Harness.ConfigOptions = configOptions(update.ConfigOptions)
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

func (s *SessionFSM) appendEventLocked(kind workspacesdk.ACPEventKind, text string, raw json.RawMessage) {
	s.info.Cursor.Seq++
	e := workspacesdk.ACPEvent{Cursor: s.info.Cursor, Kind: kind, Text: text, Update: slices.Clone(raw)}
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
		if after.GenerationID != s.info.Cursor.GenerationID || after.Seq > s.info.Cursor.Seq {
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
