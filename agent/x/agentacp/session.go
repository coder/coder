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

func (s *SessionFSM) updateStatusLocked(operation sessionOperation) error {
	target, ok := sessionTransitions[s.info.Status][operation]
	if !ok {
		return xerrors.Errorf("%w: %s in %s", ErrConflict, operation, s.info.Status)
	}
	s.info.Status = target
	return nil
}

// SessionFSM owns the process, tasks, operations, and ongoing transcript of one
// initialized adapter runtime. mu protects all mutable fields below it.
type SessionFSM struct {
	ctx       context.Context
	cancel    context.CancelFunc
	id        workspacesdk.ACPSessionID
	request   workspacesdk.ACPCreateSessionRequest
	proc      *process
	wg        sync.WaitGroup
	closeOnce sync.Once

	mu        sync.Mutex
	closed    bool
	info      workspacesdk.ACPSession
	events    []workspacesdk.ACPEvent
	assistant []assistantText
	turn      uint64
}

func newSessionFSM(ctx context.Context, req workspacesdk.ACPCreateSessionRequest, info workspacesdk.ACPSession, p *process, logger slog.Logger) *SessionFSM {
	ctx, cancel := context.WithCancel(ctx)
	s := &SessionFSM{
		ctx: ctx, cancel: cancel, id: info.ID, request: req, info: info, proc: p,
	}
	s.wg.Go(func() { s.watchProcess(logger) })
	context.AfterFunc(ctx, s.Close)
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

// Close stops the process and waits for session tasks.
func (s *SessionFSM) Close() {
	s.closeOnce.Do(func() {
		s.cancel()
		// Writes hold mu, so close the transport before waiting for the lock.
		s.proc.close()
		s.mu.Lock()
		s.closed = true
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
	// TODO: The ACP SDK closes Done before queued notifications finish and
	// exposes no signal for their completion. Disconnect can put the FSM in
	// error before final updates arrive, causing those updates to be rejected.
	err := p.err
	if err == nil {
		err = xerrors.New("harness disconnected")
	}
	_ = s.adapterError(s.ctx, harnessFailure(s.ctx, logger, p, err, harnessConfig{slug: s.id.HarnessSlug}))
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

// Message writes a prompt while idle and returns after stdin admission.
// Later completion and failure are available through Read.
func (s *SessionFSM) Message(ctx context.Context, req workspacesdk.ACPMessageRequest) (workspacesdk.ACPMessageResponse, error) {
	if req.ID == uuid.Nil || strings.TrimSpace(req.Text) == "" {
		return workspacesdk.ACPMessageResponse{}, ErrInvalid
	}
	if err := s.lock(ctx); err != nil {
		return workspacesdk.ACPMessageResponse{}, err
	}
	defer s.mu.Unlock()
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
	completed := make(chan error, 1)
	s.wg.Go(func() {
		update := ACPUpdate{Turn: turn}
		// s.proc.conn.Prompt returns only once the harness finishes the turn.
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
	select {
	case err = <-written:
	case err = <-completed:
	case <-s.ctx.Done():
		err = ErrUnavailable
	}
	if err != nil {
		_ = s.setErrorLocked(err)
		return workspacesdk.ACPMessageResponse{}, err
	}
	s.events, s.assistant = nil, nil
	s.info.Cursor = workspacesdk.ACPCursor{Epoch: uuid.New()}
	s.info.Error, s.info.StopReason = "", ""
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
	if err := s.updateStatusLocked(operationInterrupt); err != nil {
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
