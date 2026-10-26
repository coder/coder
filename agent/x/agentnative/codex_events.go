package agentnative

import (
	"encoding/json"
	"strings"

	"golang.org/x/xerrors"

	"github.com/coder/quartz"
)

type codexMessage struct {
	ID     json.RawMessage `json:"id,omitempty"`
	Method string          `json:"method,omitempty"`
	Params json.RawMessage `json:"params,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  json.RawMessage `json:"error,omitempty"`
}

func (m codexMessage) threadID() string {
	var p struct {
		ThreadID string `json:"threadId"`
		Thread   struct {
			ID string `json:"id"`
		} `json:"thread"`
	}
	if json.Unmarshal(m.Params, &p) != nil {
		return ""
	}
	if p.ThreadID != "" {
		return p.ThreadID
	}
	if m.Method == "thread/started" {
		return p.Thread.ID
	}
	return ""
}

func (m codexMessage) size() int {
	return len(m.ID) + len(m.Method) + len(m.Params) + len(m.Result) + len(m.Error) + 100
}

type codexStatus struct {
	Type string `json:"type"`
}
type codexThread struct {
	ID     string      `json:"id"`
	Cwd    string      `json:"cwd"`
	Status codexStatus `json:"status"`
}

func (t codexThread) validate(expected Session) error {
	if !codexValidID(t.ID) || t.Cwd != expected.Cwd || (expected.NativeID != "" && t.ID != expected.NativeID) || !codexThreadStatus(t.Status.Type) {
		return xerrors.New("invalid codex thread identity or status")
	}
	return nil
}

func codexThreadStatus(status string) bool {
	return status == "idle" || status == "active" || status == "notLoaded" || status == "systemError"
}

func codexTurnStatus(status string) bool {
	return status == "inProgress" || status == "completed" || status == "interrupted" || status == "failed"
}

func codexSafeID(id string) bool { return len(id) > 0 && len(id) <= 200 }

type codexNativeTurn struct {
	ID     string            `json:"id"`
	Items  []codexNativeItem `json:"items"`
	Status string            `json:"status"`
	Error  struct {
		Info json.RawMessage `json:"codexErrorInfo"`
	} `json:"error"`
}

type codexNativeItem struct {
	ID           string          `json:"id"`
	Type         string          `json:"type"`
	ClientID     string          `json:"clientId"`
	Content      json.RawMessage `json:"content"`
	Text         *string         `json:"text"`
	Summary      []string        `json:"summary"`
	Command      any             `json:"command"`
	Changes      any             `json:"changes"`
	Arguments    any             `json:"arguments"`
	Status       string          `json:"status"`
	Success      *bool           `json:"success"`
	Output       any             `json:"aggregatedOutput"`
	Result       any             `json:"result"`
	ContentItems any             `json:"contentItems"`
}

type codexTurn struct {
	id              string
	requestID       string
	identified      bool
	human           bool
	nativeCompleted bool
	completed       bool
	pending         []Event
	items           map[string]*codexItem
	timer           *quartz.Timer
}

type codexItem struct {
	streamed bool
	started  bool
	done     bool
}
type codexApproval struct {
	turnID string
	kind   string
}

func (c *Codex) receive(generation uint64, message codexMessage) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || generation != c.generation {
		return
	}
	id := message.threadID()
	s := c.sessions[id]
	if s == nil {
		if c.creating && codexValidID(id) {
			if len(c.early) >= 128 || c.earlyBytes+message.size() > codexBufferLimit {
				c.earlyOverflow = true
				return
			}
			c.early = append(c.early, message)
			c.earlyBytes += message.size()
		}
		return
	}
	if s.fault {
		return
	}
	if !s.registered {
		if len(s.buffer) >= 128 || s.bufferBytes+message.size() > codexBufferLimit {
			c.fault(s, "Codex startup event buffer exhausted")
			return
		}
		s.buffer = append(s.buffer, message)
		s.bufferBytes += message.size()
		return
	}
	if err := c.ingest(s, message); err != nil {
		c.fault(s, "Invalid or unbounded Codex event stream; native work state is unknown")
	}
}

func (c *Codex) human(s *codexSession) error {
	if s.mode == "human" {
		return nil
	}
	s.mode = "human"
	return c.publish(s, Event{Type: "mode", Data: map[string]any{"mode": "human"}})
}

func (*Codex) turn(s *codexSession, id string) (*codexTurn, error) {
	if !codexSafeID(id) {
		return nil, xerrors.New("invalid codex turn ID")
	}
	if t := s.turns[id]; t != nil {
		return t, nil
	}
	if len(s.turns) >= 1000 {
		return nil, xerrors.New("codex turn budget exhausted")
	}
	t := &codexTurn{id: id, items: make(map[string]*codexItem)}
	s.turns[id] = t
	return t, nil
}

func (c *Codex) emit(s *codexSession, t *codexTurn, event Event) error {
	if t.completed || s.fault {
		return nil
	}
	if !t.identified {
		encoded, err := json.Marshal(event)
		if err != nil {
			return err
		}
		if len(t.pending) >= 128 || s.raceBytes+len(encoded) > codexBufferLimit {
			return xerrors.New("codex attribution buffer exhausted")
		}
		s.raceBytes += len(encoded)
		t.pending = append(t.pending, event)
		return nil
	}
	event.Data["turnId"] = t.id
	if t.requestID != "" {
		event.Data["requestId"] = t.requestID
	}
	if event.Type == "turn.complete" {
		t.completed = true
		if t.timer != nil {
			t.timer.Stop()
			t.timer = nil
		}
		if r := s.state.Requests[t.requestID]; r != nil {
			r.Status = "completed"
		}
		s.itemCount -= len(t.items)
		clear(t.items)
	}
	return c.publish(s, event)
}

func (c *Codex) identify(s *codexSession, t *codexTurn, requestID, text string) error {
	if t.identified {
		if requestID != "" && t.requestID != requestID {
			return xerrors.New("codex turn ownership changed")
		}
		return nil
	}
	t.identified = true
	t.requestID = requestID
	if t.timer != nil {
		t.timer.Stop()
		t.timer = nil
	}
	if requestID != "" {
		r := s.state.Requests[requestID]
		if r == nil || (r.TurnID != "" && r.TurnID != t.id) {
			return xerrors.New("codex prompt identified multiple turns")
		}
		r.TurnID = t.id
		if r.Status != "unknown" {
			r.Status = "running"
		}
	} else {
		t.human = true
	}
	if err := c.emit(s, t, Event{Type: "turn.start", Data: map[string]any{"text": text}}); err != nil {
		return err
	}
	pending := t.pending
	t.pending = nil
	for _, event := range pending {
		encoded, _ := json.Marshal(event)
		s.raceBytes -= len(encoded)
		if err := c.emit(s, t, event); err != nil {
			return err
		}
	}
	return nil
}

func (*Codex) itemState(s *codexSession, t *codexTurn, id string) (*codexItem, error) {
	if !codexSafeID(id) {
		return nil, xerrors.New("invalid codex item ID")
	}
	if i := t.items[id]; i != nil {
		return i, nil
	}
	if s.itemCount >= 2000 {
		return nil, xerrors.New("codex item budget exhausted")
	}
	i := &codexItem{}
	t.items[id] = i
	s.itemCount++
	return i, nil
}

//nolint:revive // complete is the native item lifecycle state, not a separate operation.
func (c *Codex) item(s *codexSession, t *codexTurn, native codexNativeItem, complete bool) error {
	if !codexSafeID(native.ID) || native.Type == "" {
		return xerrors.New("invalid codex item")
	}
	if t.completed || s.fault {
		return nil
	}
	i, err := c.itemState(s, t, native.ID)
	if err != nil {
		return err
	}
	if i.done {
		return nil
	}
	switch native.Type {
	case "userMessage":
		var content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}
		if len(native.Content) == 0 || native.Content[0] != '[' || json.Unmarshal(native.Content, &content) != nil {
			return xerrors.New("invalid codex user message")
		}
		requestID := ""
		for id, r := range s.state.Requests {
			if r.ClientID == native.ClientID {
				requestID = id
				break
			}
		}
		if requestID == "" {
			t.human = true
			if err := c.human(s); err != nil {
				return err
			}
		}
		texts := make([]string, 0, len(content))
		for _, p := range content {
			if p.Type == "text" {
				texts = append(texts, p.Text)
			}
		}
		if err := c.identify(s, t, requestID, strings.Join(texts, "\n")); err != nil {
			return err
		}
	case "agentMessage", "plan":
		if complete && !i.streamed {
			if native.Text == nil {
				return xerrors.New("missing codex message text")
			}
			kind := "text"
			if native.Type == "plan" {
				kind = "thought"
			}
			if err := c.emit(s, t, Event{Type: kind, Data: map[string]any{"text": *native.Text}}); err != nil {
				return err
			}
		}
	case "reasoning":
		if complete && !i.streamed {
			var content []string
			if native.Summary == nil || len(native.Content) == 0 || native.Content[0] != '[' || json.Unmarshal(native.Content, &content) != nil {
				return xerrors.New("invalid codex reasoning")
			}
			if err := c.emit(s, t, Event{Type: "thought", Data: map[string]any{"text": strings.Join(append(native.Summary, content...), "\n")}}); err != nil {
				return err
			}
		}
	case "commandExecution", "fileChange", "mcpToolCall", "dynamicToolCall", "collabAgentToolCall", "webSearch":
		input := native.Arguments
		result := native.Result
		if result == nil {
			result = native.ContentItems
		}
		if native.Type == "commandExecution" {
			input = native.Command
			result = native.Output
		}
		if native.Type == "fileChange" {
			input = native.Changes
			result = native.Changes
		}
		if !i.started {
			i.started = true
			if err := c.emit(s, t, Event{Type: "tool.start", Data: map[string]any{"toolCallId": native.ID, "title": native.Type, "input": input}}); err != nil {
				return err
			}
		}
		if complete {
			failed := native.Status == "failed" || native.Status == "declined" || (native.Success != nil && !*native.Success)
			if err := c.emit(s, t, Event{Type: "tool.end", Data: map[string]any{"toolCallId": native.ID, "isError": failed, "result": result}}); err != nil {
				return err
			}
		}
	}
	if complete {
		i.done = true
	}
	return nil
}

func (c *Codex) ingest(s *codexSession, message codexMessage) error {
	var p struct {
		TurnID         string          `json:"turnId"`
		Turn           codexNativeTurn `json:"turn"`
		Item           codexNativeItem `json:"item"`
		ItemID         string          `json:"itemId"`
		Delta          *string         `json:"delta"`
		Status         codexStatus     `json:"status"`
		ThreadSettings struct {
			Cwd string `json:"cwd"`
		} `json:"threadSettings"`
		Goal struct {
			Status string `json:"status"`
		} `json:"goal"`
		RequestID json.RawMessage `json:"requestId"`
	}
	if json.Unmarshal(message.Params, &p) != nil {
		return xerrors.New("invalid codex notification")
	}
	if len(message.ID) > 0 {
		kind := ""
		switch message.Method {
		case "item/commandExecution/requestApproval":
			kind = "command"
		case "item/fileChange/requestApproval":
			kind = "file-change"
		case "item/permissions/requestApproval":
			kind = "permissions"
		case "item/tool/requestUserInput":
			kind = "user-input"
		case "mcpServer/elicitation/request":
			kind = "elicitation"
		case "item/tool/call":
			kind = "tool"
		default:
			return nil // Never answer native requests, known or unknown.
		}
		if p.TurnID == "" && kind != "elicitation" {
			return xerrors.New("missing codex approval turn")
		}
		key := string(message.ID)
		if _, ok := s.approvals[key]; ok {
			return nil
		}
		if len(s.approvals) >= 128 {
			return xerrors.New("codex approval budget exhausted")
		}
		s.approvals[key] = codexApproval{turnID: p.TurnID, kind: kind}
		return c.approval(s, p.TurnID, kind, true)
	}
	switch message.Method {
	case "serverRequest/resolved":
		approval, ok := s.approvals[string(p.RequestID)]
		if !ok {
			return nil
		}
		delete(s.approvals, string(p.RequestID))
		return c.approval(s, approval.turnID, approval.kind, len(s.approvals) > 0)
	case "thread/closed", "thread/archived", "thread/deleted":
		c.fault(s, "Native Codex thread closed")
		return nil
	case "thread/settings/updated":
		if p.ThreadSettings.Cwd != s.identity.Cwd {
			c.fault(s, "Native Codex working directory changed; automation is disabled")
		}
		return nil
	case "thread/queue/changed":
		s.workRevision++
		return c.human(s)
	case "thread/goal/updated":
		if p.Goal.Status != "complete" {
			s.workRevision++
		}
		return c.human(s)
	case "thread/goal/cleared":
		// Resume emits the current goal snapshot, including this notification
		// when no goal exists. Removing work cannot create an idle dispatch.
		return c.human(s)
	case "thread/status/changed":
		if !codexThreadStatus(p.Status.Type) {
			return xerrors.New("invalid codex status")
		}
		s.status = p.Status.Type
		s.revision++
		if s.status == "systemError" {
			c.fault(s, "Native Codex thread failed")
		}
		return nil
	case "turn/started", "turn/completed":
		if p.Turn.Items == nil {
			return xerrors.New("missing codex turn items")
		}
		t, err := c.turn(s, p.Turn.ID)
		if err != nil {
			return err
		}
		if t.completed {
			return nil
		}
		completing := message.Method == "turn/completed" && !t.nativeCompleted
		if completing && (p.Turn.Status == "inProgress" || !codexTurnStatus(p.Turn.Status)) {
			return xerrors.New("invalid codex completion")
		}
		if !t.nativeCompleted {
			s.revision++
			if message.Method == "turn/started" {
				s.active = t.id
				s.status = "active"
			} else if s.active == "" || s.active == t.id {
				s.active = ""
				s.status = "idle"
			}
		}
		if completing {
			t.nativeCompleted = true
		}
		for _, native := range p.Turn.Items {
			if err := c.item(s, t, native, message.Method == "turn/completed"); err != nil {
				return err
			}
		}
		if completing {
			reason := "answer"
			if p.Turn.Status == "interrupted" {
				reason = "aborted"
			}
			if p.Turn.Status == "failed" {
				reason = "error"
				if string(p.Turn.Error.Info) == `"cyberPolicy"` || string(p.Turn.Error.Info) == `"misalignmentPolicyViolation"` {
					reason = "refusal"
				}
			}
			return c.emit(s, t, Event{Type: "turn.complete", Data: map[string]any{"reason": reason, "answer": ""}})
		}
		return c.maybeCancel(s, t.requestID)
	case "item/started", "item/completed":
		t, err := c.turn(s, p.TurnID)
		if err != nil {
			return err
		}
		if err := c.item(s, t, p.Item, message.Method == "item/completed"); err != nil {
			return err
		}
		return c.maybeCancel(s, t.requestID)
	case "item/agentMessage/delta", "item/plan/delta", "item/reasoning/summaryTextDelta", "item/reasoning/textDelta":
		if p.Delta == nil {
			return xerrors.New("missing codex delta")
		}
		t, err := c.turn(s, p.TurnID)
		if err != nil {
			return err
		}
		if t.completed {
			return nil
		}
		i, err := c.itemState(s, t, p.ItemID)
		if err != nil {
			return err
		}
		if i.done {
			return nil
		}
		i.streamed = true
		kind := "thought"
		if message.Method == "item/agentMessage/delta" {
			kind = "text"
		}
		return c.emit(s, t, Event{Type: kind, Data: map[string]any{"text": *p.Delta}})
	}
	return nil
}

func (c *Codex) approval(s *codexSession, turnID, kind string, pending bool) error {
	event := Event{Type: "needs.input", Data: map[string]any{"kind": kind, "pending": pending}}
	if turnID == "" {
		return c.publish(s, event)
	}
	t, err := c.turn(s, turnID)
	if err != nil {
		return xerrors.Errorf("codex approval: %w", err)
	}
	return c.emit(s, t, event)
}
