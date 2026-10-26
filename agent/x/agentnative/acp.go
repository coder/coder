package agentnative

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"sync"

	"github.com/google/uuid"
	"golang.org/x/xerrors"
)

const (
	acpMaxMessage     = 4 << 20
	acpMaxSubmissions = 32
)

type acpManager interface {
	List() []Session
	Session(string) (Session, error)
	Events(context.Context, string, uint64, bool) ([]Event, uint64, error)
	Create(context.Context, string, string) (Session, error)
	Submit(context.Context, string, Prompt) error
	SetMode(context.Context, string, string) error
	Cancel(context.Context, string, string) error
}

type acpRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type acpError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *acpError) Error() string { return e.Message }

func acpInvalid(message string) error { return &acpError{Code: -32602, Message: message} }

type acpEventSource uint8

const (
	acpRetainedEvent acpEventSource = iota
	acpLiveEvent
)

type acpCompletion struct {
	stop string
	err  error
}

type acpPrompt struct {
	id              string
	turnID          string
	submitted       bool
	cancelRequested bool
	cancelSent      bool
	settled         bool
	done            chan acpCompletion
}

type acpSession struct {
	mu       sync.Mutex
	id       string
	cursor   uint64
	changed  chan struct{}
	err      error
	pending  *acpPrompt
	streamed map[string]bool
}

type acpConnection struct {
	ctx         context.Context
	cancel      context.CancelFunc
	transport   io.ReadWriteCloser
	manager     acpManager
	writeMu     sync.Mutex
	mu          sync.Mutex
	sessions    map[string]*acpSession
	observers   sync.WaitGroup
	submissions chan struct{}
}

// ServeACP projects native session observations onto ACP v1 over a bounded
// newline-delimited JSON stream. Closing the stream only detaches this observer;
// it never cancels, resumes, or shuts down a native conversation.
func ServeACP(ctx context.Context, transport io.ReadWriteCloser, manager *Manager) error {
	return serveACPStream(ctx, transport, manager)
}

func serveACPStream(ctx context.Context, transport io.ReadWriteCloser, manager acpManager) error {
	ctx, cancel := context.WithCancel(ctx)
	c := &acpConnection{ctx: ctx, cancel: cancel, transport: transport, manager: manager, sessions: make(map[string]*acpSession), submissions: make(chan struct{}, acpMaxSubmissions)}
	closed := make(chan struct{})
	go func() {
		defer close(closed)
		<-ctx.Done()
		_ = transport.Close()
	}()
	var requests sync.WaitGroup
	defer func() {
		cancel()
		requests.Wait()
		c.observers.Wait()
		<-closed
	}()
	// Keep room for control requests while prompts await native completion.
	slots := make(chan struct{}, 128)
	scanner := bufio.NewScanner(transport)
	scanner.Buffer(make([]byte, 4096), acpMaxMessage)
	for scanner.Scan() {
		if len(bytes.TrimSpace(scanner.Bytes())) == 0 {
			continue
		}
		var req acpRequest
		if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
			_ = c.respond(json.RawMessage("null"), nil, &acpError{Code: -32700, Message: "Invalid JSON"})
			continue
		}
		if req.JSONRPC != "2.0" || req.Method == "" || !validACPID(req.ID) {
			_ = c.respond(json.RawMessage("null"), nil, &acpError{Code: -32600, Message: "Invalid JSON-RPC request"})
			continue
		}
		// ACP cancellation is the only supported client notification. In
		// particular, a notification must never create a session or prompt it.
		if len(req.ID) == 0 && req.Method != "session/cancel" {
			continue
		}
		select {
		case slots <- struct{}{}:
		default:
			_ = c.respond(req.ID, nil, xerrors.New("too many concurrent ACP requests"))
			continue
		}
		// Admit prompts and record cancellation in wire order. Completion is
		// asynchronous, but a following cancel must see the pending request.
		handle := func() (any, error) { return c.handle(req) }
		if req.Method == "session/prompt" {
			params, err := decodeACPParams(req.Params)
			if err == nil {
				handle, err = c.preparePrompt(params)
			}
			if err != nil {
				<-slots
				_ = c.respond(req.ID, nil, err)
				continue
			}
		} else if req.Method == "session/cancel" {
			result, err := handle()
			<-slots
			_ = c.respond(req.ID, result, err)
			continue
		}
		requests.Add(1)
		go func() {
			defer requests.Done()
			defer func() { <-slots }()
			result, err := handle()
			_ = c.respond(req.ID, result, err)
		}()
	}
	if err := scanner.Err(); err != nil && ctx.Err() == nil {
		return xerrors.Errorf("read native ACP stream: %w", err)
	}
	return nil
}

func validACPID(id json.RawMessage) bool {
	if len(id) == 0 {
		return true
	}
	var value any
	if json.Unmarshal(id, &value) != nil {
		return false
	}
	switch value.(type) {
	case string, float64:
		return true
	default:
		return false
	}
}

func (c *acpConnection) write(value any) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if err := c.ctx.Err(); err != nil {
		return err
	}
	if err := json.NewEncoder(c.transport).Encode(value); err != nil {
		c.cancel()
		return xerrors.Errorf("write native ACP stream: %w", err)
	}
	return nil
}

func (c *acpConnection) respond(id json.RawMessage, result any, err error) error {
	if len(id) == 0 {
		return nil
	}
	response := map[string]any{"jsonrpc": "2.0", "id": id}
	if err != nil {
		failure := &acpError{Code: -32000, Message: err.Error()}
		var protocolError *acpError
		if errors.As(err, &protocolError) {
			failure = protocolError
		}
		response["error"] = failure
	} else {
		response["result"] = result
	}
	return c.write(response)
}

func acpMetadata(s Session) map[string]any {
	meta := map[string]any{
		"harness": s.Harness, "nativeSessionId": s.NativeID,
		"startedAt": s.StartedAt, "archived": s.Archived, "ended": s.Archived,
		"resumePending": s.ResumePending, "resumable": s.Archived,
		"fault": s.Fault != "", "activeTurnId": s.ActiveTurnID,
		"pendingInput": s.PendingInput, "mode": s.Mode, "cursor": s.Cursor,
		"history": "retained-agent-events-only", "fullHistory": false,
		"permissions": "native-attach", "promptContent": []string{"text"}, "mcpServers": false,
	}
	if s.Fault != "" {
		meta["lastError"] = map[string]any{"message": s.Fault}
	}
	return map[string]any{"coder-acp": meta}
}

func acpDetails(s Session) map[string]any {
	return map[string]any{
		"modes": map[string]any{
			"currentModeId": s.Mode,
			"availableModes": []map[string]any{
				{"id": "human", "name": "Human", "description": "The native terminal controls prompts and permissions."},
				{"id": "automation", "name": "Automation", "description": "ACP can submit prompts. Permissions still require native attach."},
			},
		},
		"_meta": acpMetadata(s),
	}
}

type acpParams struct {
	SessionID             string            `json:"sessionId"`
	Cwd                   string            `json:"cwd"`
	ModeID                string            `json:"modeId"`
	Cursor                *string           `json:"cursor"`
	MCPServers            []json.RawMessage `json:"mcpServers"`
	AdditionalDirectories []string          `json:"additionalDirectories"`
	Prompt                []struct {
		Type string  `json:"type"`
		Text *string `json:"text"`
	} `json:"prompt"`
	Meta map[string]struct {
		Harness string `json:"harness"`
	} `json:"_meta"`
}

func decodeACPParams(raw json.RawMessage) (acpParams, error) {
	var p acpParams
	if len(raw) > 0 && json.Unmarshal(raw, &p) != nil {
		return p, acpInvalid("Invalid ACP parameters")
	}
	return p, nil
}

func (c *acpConnection) handle(req acpRequest) (any, error) {
	p, err := decodeACPParams(req.Params)
	if err != nil {
		return nil, err
	}
	switch req.Method {
	case "initialize":
		return map[string]any{
			"protocolVersion": 1,
			"agentInfo":       map[string]any{"name": "coder-native", "title": "Native agents", "version": "0.1.0"},
			"agentCapabilities": map[string]any{
				"loadSession":         true,
				"promptCapabilities":  map[string]bool{"image": false, "audio": false, "embeddedContext": false},
				"mcpCapabilities":     map[string]bool{"http": false, "sse": false},
				"sessionCapabilities": map[string]any{"list": map[string]any{}},
			},
			"authMethods": []any{},
			"_meta": map[string]any{"coder-acp": map[string]any{"harnesses": []map[string]string{
				{"id": "claude", "name": "claude-acp-bridge", "title": "Claude Code"},
				{"id": "codex", "name": "codex", "title": "Codex"},
				{"id": "opencode", "name": "opencode", "title": "OpenCode"},
			}}},
		}, nil
	case "session/list":
		if p.Cursor != nil {
			return nil, acpInvalid("Session list pagination is not supported")
		}
		sessions := make([]map[string]any, 0)
		for _, s := range c.manager.List() {
			if p.Cwd != "" && filepath.Clean(p.Cwd) != s.Cwd {
				continue
			}
			sessions = append(sessions, map[string]any{"sessionId": s.ID, "cwd": s.Cwd, "_meta": acpMetadata(s)})
		}
		return map[string]any{"sessions": sessions}, nil
	case "session/new", "session/load":
		if !filepath.IsAbs(p.Cwd) || len(p.MCPServers) != 0 || len(p.AdditionalDirectories) != 0 {
			return nil, acpInvalid("An absolute cwd is required; MCP injection and additional directories are unsupported")
		}
		var s Session
		var err error
		if req.Method == "session/new" {
			harness := p.Meta["coder-acp"].Harness
			if harness == "" {
				harness = "claude"
			}
			s, err = c.manager.Create(c.ctx, harness, filepath.Clean(p.Cwd))
		} else {
			s, err = c.manager.Session(p.SessionID)
		}
		if err != nil {
			return nil, err
		}
		if s.Cwd != filepath.Clean(p.Cwd) {
			return nil, acpInvalid("Native session cwd does not match requested cwd")
		}
		if err := c.attach(s); err != nil {
			return nil, err
		}
		s, err = c.manager.Session(s.ID)
		if err != nil {
			return nil, err
		}
		result := acpDetails(s)
		if req.Method == "session/new" {
			result["sessionId"] = s.ID
		}
		return result, nil
	case "session/set_mode":
		if _, err := c.session(p.SessionID); err != nil {
			return nil, err
		}
		if p.ModeID != "human" && p.ModeID != "automation" {
			return nil, acpInvalid("Unsupported native session mode")
		}
		return map[string]any{}, c.manager.SetMode(c.ctx, p.SessionID, p.ModeID)
	case "session/cancel":
		s, err := c.session(p.SessionID)
		if err != nil {
			return nil, err
		}
		s.mu.Lock()
		if s.pending != nil {
			s.pending.cancelRequested = true
			c.maybeCancel(s)
		}
		s.mu.Unlock()
		return map[string]any{}, nil
	default:
		return nil, &acpError{Code: -32601, Message: "ACP method is not supported"}
	}
}

func (c *acpConnection) session(id string) (*acpSession, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	s := c.sessions[id]
	if s == nil {
		return nil, acpInvalid("Session is not loaded on this ACP connection")
	}
	return s, nil
}

func (c *acpConnection) attach(snapshot Session) error {
	c.mu.Lock()
	s := c.sessions[snapshot.ID]
	if s != nil {
		c.mu.Unlock()
		// Loading twice refreshes metadata, without replaying history twice.
		return c.awaitCursor(s, snapshot.Cursor)
	}
	if len(c.sessions) >= 100 {
		c.mu.Unlock()
		return xerrors.New("ACP connection session limit reached")
	}
	s = &acpSession{id: snapshot.ID, cursor: snapshot.HistoryStartCursor, changed: make(chan struct{}), streamed: make(map[string]bool)}
	c.sessions[s.id] = s
	// A second load cannot pass the first replay and then start a second reader.
	s.mu.Lock()
	c.mu.Unlock()
	defer s.mu.Unlock()
	if err := c.readEvents(s, false, acpRetainedEvent); err != nil {
		c.fail(s, err)
		return err
	}
	c.observers.Add(1)
	go func() {
		defer c.observers.Done()
		for c.ctx.Err() == nil {
			s.mu.Lock()
			cursor := s.cursor
			s.mu.Unlock()
			events, through, err := c.manager.Events(c.ctx, s.id, cursor, true)
			s.mu.Lock()
			if err == nil {
				err = c.consumeBatch(s, events, through, acpLiveEvent)
			}
			if err != nil {
				c.fail(s, err)
			}
			s.mu.Unlock()
			if err != nil {
				return
			}
		}
	}()
	return nil
}

func (c *acpConnection) awaitCursor(s *acpSession, through uint64) error {
	for {
		s.mu.Lock()
		cursor, err, changed := s.cursor, s.err, s.changed
		s.mu.Unlock()
		if err != nil || cursor >= through {
			return err
		}
		select {
		case <-c.ctx.Done():
			return c.ctx.Err()
		case <-changed:
		}
	}
}

// readEvents and consumeBatch run with the session lock held. Validation of the
// entire batch precedes completion delivery, so a gap cannot look like success.
func (c *acpConnection) readEvents(s *acpSession, wait bool, source acpEventSource) error {
	events, through, err := c.manager.Events(c.ctx, s.id, s.cursor, wait)
	if err != nil {
		return err
	}
	return c.consumeBatch(s, events, through, source)
}

func (c *acpConnection) consumeBatch(s *acpSession, events []Event, through uint64, source acpEventSource) error {
	cursor := s.cursor
	for _, event := range events {
		if event.Seq != cursor+1 {
			return xerrors.New("native event history has a gap; reload the connection")
		}
		cursor = event.Seq
	}
	if cursor != through {
		return xerrors.New("native event cursor has a gap; reload the connection")
	}
	for _, event := range events {
		if err := c.consume(s, event, source); err != nil {
			return err
		}
		s.cursor = event.Seq
	}
	close(s.changed)
	s.changed = make(chan struct{})
	return nil
}

func (c *acpConnection) update(s *acpSession, value map[string]any, event Event) error {
	params := map[string]any{"sessionId": s.id, "update": value}
	if event.Seq > 0 {
		params["_meta"] = map[string]any{"coder-acp": map[string]any{"seq": event.Seq, "turnId": event.Data["turnId"], "requestId": event.Data["requestId"]}}
	}
	return c.write(map[string]any{"jsonrpc": "2.0", "method": "session/update", "params": params})
}

func (c *acpConnection) fail(s *acpSession, err error) {
	if s.err != nil {
		return
	}
	s.err = err
	if s.pending != nil {
		acpFinish(s.pending, acpCompletion{err: err})
	}
	close(s.changed)
	s.changed = make(chan struct{})
	_ = c.update(s, map[string]any{"sessionUpdate": "session_info_update", "_meta": map[string]any{"coder-acp": map[string]any{"error": err.Error(), "cursor": s.cursor}}}, Event{})
}

func acpFinish(p *acpPrompt, result acpCompletion) {
	if p.settled {
		return
	}
	p.settled = true
	select {
	case p.done <- result:
	default:
	}
}

func (c *acpConnection) maybeCancel(s *acpSession) {
	p := s.pending
	if p == nil || p.settled || !p.cancelRequested || !p.submitted || p.cancelSent {
		return
	}
	p.cancelSent = true
	c.observers.Add(1)
	go func() {
		defer c.observers.Done()
		if err := c.manager.Cancel(c.ctx, s.id, p.id); err != nil {
			s.mu.Lock()
			defer s.mu.Unlock()
			if s.pending == p {
				acpFinish(p, acpCompletion{err: err})
				_ = c.update(s, map[string]any{"sessionUpdate": "session_info_update", "_meta": map[string]any{"coder-acp": map[string]any{"error": err.Error()}}}, Event{})
			}
		}
	}()
}

func (c *acpConnection) preparePrompt(p acpParams) (func() (any, error), error) {
	s, err := c.session(p.SessionID)
	if err != nil {
		return nil, err
	}
	if len(p.Prompt) == 0 {
		return nil, acpInvalid("Only nonempty text prompts are supported")
	}
	parts := make([]string, len(p.Prompt))
	for i, part := range p.Prompt {
		if part.Type != "text" || part.Text == nil {
			return nil, acpInvalid("Only text prompt content is supported")
		}
		parts[i] = *part.Text
	}
	text := strings.Join(parts, "\n")
	if strings.TrimSpace(text) == "" {
		return nil, acpInvalid("Prompt text must not be empty")
	}
	pending := &acpPrompt{id: uuid.NewString(), done: make(chan acpCompletion, 1)}
	s.mu.Lock()
	switch {
	case s.err != nil:
		err = s.err
	case s.pending != nil:
		err = acpInvalid("A prompt is already pending for this session")
	default:
		// Native completion can precede its dispatch acknowledgement. Bound
		// those still-running submissions independently from ACP responses.
		select {
		case c.submissions <- struct{}{}:
			s.pending = pending
		default:
			err = xerrors.New("native submission limit reached; wait for pending acknowledgements")
		}
	}
	s.mu.Unlock()
	if err != nil {
		return nil, err
	}
	return func() (any, error) {
		defer func() {
			s.mu.Lock()
			defer s.mu.Unlock()
			if s.pending == pending {
				s.pending = nil
			}
		}()
		// A native terminal event can prove completion before the dispatch
		// acknowledgement arrives. Late transport errors cannot undo it.
		c.observers.Add(1)
		go func() {
			defer c.observers.Done()
			defer func() { <-c.submissions }()
			err := c.manager.Submit(c.ctx, s.id, Prompt{ID: pending.id, Text: text})
			if err != nil {
				// Drain observations that were persisted before the error. A
				// completed native root is stronger evidence than its lost ACK.
				if snapshot, snapshotErr := c.manager.Session(s.id); snapshotErr == nil {
					_ = c.awaitCursor(s, snapshot.Cursor)
				}
			}
			s.mu.Lock()
			defer s.mu.Unlock()
			if s.pending != pending || pending.settled {
				return
			}
			if err != nil {
				acpFinish(pending, acpCompletion{err: err})
				return
			}
			pending.submitted = true
			c.maybeCancel(s)
		}()
		select {
		case <-c.ctx.Done():
			return nil, c.ctx.Err()
		case result := <-pending.done:
			if result.err != nil {
				return nil, result.err
			}
			return map[string]any{"stopReason": result.stop}, nil
		}
	}, nil
}

func (c *acpConnection) consume(s *acpSession, event Event, source acpEventSource) error {
	d := event.Data
	turnID, _ := d["turnId"].(string)
	requestID, _ := d["requestId"].(string)
	text, textOK := d["text"].(string)
	switch event.Type {
	case "turn.start", "text", "thought", "tool.start", "tool.end", "turn.complete":
		if turnID == "" {
			return xerrors.New("native turn event has no identity")
		}
	}
	switch event.Type {
	case "turn.start", "text", "thought":
		if !textOK {
			return xerrors.New("native text event has no text")
		}
	case "tool.start", "tool.end":
		if id, ok := d["toolCallId"].(string); !ok || id == "" {
			return xerrors.New("native tool event has no identity")
		}
	}
	pending := s.pending
	owns := source == acpLiveEvent && pending != nil && requestID == pending.id
	if owns && pending.turnID == "" && event.Type == "turn.start" {
		pending.turnID = turnID
	}
	owns = owns && turnID != "" && pending.turnID == turnID
	textUpdate := func(kind, text string) error {
		return c.update(s, map[string]any{"sessionUpdate": kind, "messageId": turnID + ":" + kind, "content": map[string]any{"type": "text", "text": text}}, event)
	}
	switch event.Type {
	case "turn.start":
		if !owns {
			return textUpdate("user_message_chunk", text)
		}
	case "text":
		if text != "" {
			s.streamed[turnID] = true
		}
		return textUpdate("agent_message_chunk", text)
	case "thought":
		return textUpdate("agent_thought_chunk", text)
	case "tool.start":
		return c.update(s, map[string]any{
			"sessionUpdate": "tool_call", "toolCallId": d["toolCallId"], "title": d["title"],
			"kind": "other", "status": "pending", "rawInput": d["input"],
		}, event)
	case "tool.end":
		status := "completed"
		isError, _ := d["isError"].(bool)
		if isError {
			status = "failed"
		}
		result, ok := d["result"].(string)
		if !ok {
			encoded, err := json.Marshal(d["result"])
			if err != nil {
				return xerrors.Errorf("encode native tool result: %w", err)
			}
			result = string(encoded)
		}
		update := map[string]any{
			"sessionUpdate": "tool_call_update", "toolCallId": d["toolCallId"], "status": status, "rawOutput": d["result"],
			"content": []any{map[string]any{"type": "content", "content": map[string]any{"type": "text", "text": result}}},
		}
		if input, ok := d["input"]; ok {
			update["rawInput"] = input
		}
		return c.update(s, update, event)
	case "turn.complete":
		answer, _ := d["answer"].(string)
		if answer != "" && !s.streamed[turnID] {
			if err := textUpdate("agent_message_chunk", answer); err != nil {
				return err
			}
		}
		delete(s.streamed, turnID)
		if owns {
			result := acpCompletion{stop: "end_turn"}
			switch d["reason"] {
			case "aborted":
				result.stop = "cancelled" //nolint:misspell // ACP v1 stopReason spelling.
			case "refusal":
				result.stop = "refusal"
			case "error":
				result.err = xerrors.New("native turn failed; inspect the native terminal")
			case "answer":
			default:
				return xerrors.New("invalid native turn completion")
			}
			acpFinish(pending, result)
		}
	default:
		// Historical controls describe old runtimes, not current readiness.
		if source == acpRetainedEvent {
			return nil
		}
		switch event.Type {
		case "prompt.accepted":
			if pending != nil && requestID == pending.id {
				pending.submitted = true
				c.maybeCancel(s)
			}
		case "prompt.rejected":
			if pending != nil && requestID == pending.id {
				result := acpCompletion{stop: "cancelled"} //nolint:misspell // ACP v1 stopReason spelling.
				canceled, _ := d["cancelled"].(bool)       //nolint:misspell // Native event field name.
				if !canceled {
					result.err = xerrors.New("native prompt rejected; inspect the native terminal")
				}
				acpFinish(pending, result)
			}
		case "mode":
			return c.update(s, map[string]any{"sessionUpdate": "current_mode_update", "currentModeId": d["mode"]}, event)
		case "error", "overflow", "session.end", "runtime.replaced":
			return xerrors.New("native observation stopped; inspect session diagnostics before continuing")
		case "needs.input", "session.resuming", "session.resume_failed", "runtime.resumed", "runtime.registered":
			snapshot, err := c.manager.Session(s.id)
			if err != nil {
				return err
			}
			return c.update(s, map[string]any{"sessionUpdate": "session_info_update", "_meta": acpMetadata(snapshot)}, event)
		}
	}
	return nil
}
