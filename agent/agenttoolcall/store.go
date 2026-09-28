// Package agenttoolcall decides whether the workspace agent acts on a chat
// tool call request, and records the response of the one run so that
// repeated requests for the same tool call get it back instead of a second
// side effect.
//
// The proof that the agent never received a tool call is the cutoff
// message ID. It starts at the last_chat_message_id of the agent
// instance's first manifest, which coderd draws from the chat message ID
// sequence after every message an earlier instance could have received a
// request for. Evicting a message's records raises it to that message ID.
// A tool call without a record in a message above the cutoff therefore
// never reached this agent instance or an earlier one, and a tool call
// without a record at or below the cutoff is unknown.
package agenttoolcall

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/coder/quartz"
)

// recordRetention is how long a message's records are kept after the last
// activity on any of them: a request, a cancel, a run in progress, or a
// cancel hook whose work is still running. chatd sends every request for a
// tool call within its agent answer timeout (1 minute) of the previous
// one, so an hour leaves room for outages before requests fail as unknown.
const recordRetention = time.Hour

// Key identifies a chat tool call.
type Key struct {
	ChatID uuid.UUID
	// MessageID is the chat message ID of the assistant message that
	// contains the tool call.
	MessageID int64
	ToolName  string
	// ToolCallID is the provider tool call ID.
	ToolCallID string
}

func (k Key) message() messageKey {
	return messageKey{chatID: k.ChatID, messageID: k.MessageID}
}

func (k Key) call() callKey {
	return callKey{toolName: k.ToolName, toolCallID: k.ToolCallID}
}

type messageKey struct {
	chatID    uuid.UUID
	messageID int64
}

type callKey struct {
	toolName   string
	toolCallID string
}

// Store holds one agent instance's tool call records, grouped by chat and
// message for retention, and the cutoff message ID.
type Store struct {
	clock quartz.Clock

	mu sync.Mutex
	// hasCutoff is false until SetLastChatMessageID; until then every
	// tool call is unknown.
	hasCutoff bool
	cutoff    int64
	messages  map[messageKey]*message
}

// message holds the records of one message's tool calls.
type message struct {
	lastActive time.Time
	records    map[callKey]*record
}

type recordState int

const (
	// statePending: the handler is running.
	statePending recordState = iota
	// stateRecorded: resp holds the recorded response.
	stateRecorded
	// stateDropped: the handler abandoned the run and the record was
	// removed, so waiters must start over.
	stateDropped
	// stateCanceledMarker: a cancel arrived before any run, or before an
	// abandoned run ended. Requests get tool_call_canceled.
	stateCanceledMarker
)

// record is the state of one tool call. All fields are guarded by
// Store.mu, except that state and resp are final once done is closed and
// may then be read without the lock.
type record struct {
	method   string
	path     string
	rawQuery string
	canceled bool
	state    recordState
	done     chan struct{}
	resp     response
	hook     *cancelHook
}

// response is a recorded response.
type response struct {
	status      int
	contentType string
	body        []byte
}

// cancelHook stops the work a tool call's handler started. stop runs at
// most once, outside Store.mu.
type cancelHook struct {
	stop    func() error
	stopped <-chan struct{}
	// started is guarded by Store.mu.
	started bool
	// ran is closed after stop returned; err is set before.
	ran chan struct{}
	err error
}

func (h *cancelHook) run() {
	h.err = h.stop()
	close(h.ran)
}

// running reports whether the hook's work has not stopped yet.
func (h *cancelHook) running() bool {
	select {
	case <-h.stopped:
		return false
	default:
		return true
	}
}

// NewStore returns an empty Store without a cutoff message ID.
func NewStore(clock quartz.Clock) *Store {
	return &Store{
		clock:    clock,
		messages: make(map[messageKey]*message),
	}
}

// SetLastChatMessageID sets the cutoff message ID from the agent
// instance's first manifest. Later calls have no effect: a later manifest
// is drawn after messages this instance may already have acted on.
func (s *Store) SetLastChatMessageID(id int64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.hasCutoff {
		return
	}
	s.hasCutoff = true
	s.cutoff = id
}

// Sweep evicts the records of every message without activity in the
// last recordRetention, and raises the cutoff message ID to the highest
// evicted message ID. A run in progress or a cancel hook whose work is
// running counts as activity at the time of the sweep.
func (s *Store) Sweep() {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.clock.Now()
	for mk, msg := range s.messages {
		if msg.active() {
			msg.lastActive = now
			continue
		}
		if now.Sub(msg.lastActive) < recordRetention {
			continue
		}
		delete(s.messages, mk)
		s.cutoff = max(s.cutoff, mk.messageID)
	}
}

func (m *message) active() bool {
	for _, rec := range m.records {
		if rec.state == statePending {
			return true
		}
		if rec.hook != nil && rec.hook.running() {
			return true
		}
	}
	return false
}

// Retained reports whether the store has a record for key. Work a tool
// call started, such as an exited process, can be discarded once its
// record is gone.
func (s *Store) Retained(key Key) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.lookup(key) != nil
}

// lookup returns the record for key, or nil. s.mu must be held.
func (s *Store) lookup(key Key) *record {
	msg, ok := s.messages[key.message()]
	if !ok {
		return nil
	}
	return msg.records[key.call()]
}

// provenNew reports whether the agent can prove it never received a tool
// call in messageID that has no record. s.mu must be held.
func (s *Store) provenNew(messageID int64) bool {
	return s.hasCutoff && messageID > s.cutoff
}

// touch records activity on key's message. s.mu must be held.
func (s *Store) touch(key Key) {
	if msg, ok := s.messages[key.message()]; ok {
		msg.lastActive = s.clock.Now()
	}
}

// insert stores rec for key. s.mu must be held.
func (s *Store) insert(key Key, rec *record) {
	msg, ok := s.messages[key.message()]
	if !ok {
		msg = &message{records: make(map[callKey]*record)}
		s.messages[key.message()] = msg
	}
	msg.lastActive = s.clock.Now()
	msg.records[key.call()] = rec
}

// begin applies the request decision table. It returns the existing
// record, or a new pending record (created) that the caller must run and
// then finish or drop.
func (s *Store) begin(key Key, r *http.Request) (rec *record, created bool, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if rec := s.lookup(key); rec != nil {
		s.touch(key)
		if rec.state == stateCanceledMarker {
			return nil, false, errToolCallCanceled
		}
		if rec.method != r.Method || rec.path != r.URL.Path || rec.rawQuery != r.URL.RawQuery {
			return nil, false, errRequestMismatch
		}
		return rec, false, nil
	}
	if !s.provenNew(key.MessageID) {
		return nil, false, errToolCallUnknown
	}
	rec = &record{
		method:   r.Method,
		path:     r.URL.Path,
		rawQuery: r.URL.RawQuery,
		state:    statePending,
		done:     make(chan struct{}),
	}
	s.insert(key, rec)
	return rec, true, nil
}

// finish records resp for rec's run and wakes its waiters.
func (s *Store) finish(key Key, rec *record, resp response) {
	s.mu.Lock()
	defer s.mu.Unlock()

	rec.resp = resp
	rec.state = stateRecorded
	s.touch(key)
	close(rec.done)
}

// drop ends rec's abandoned run without a recorded response. A record a
// cancel already marked stays as a canceled marker; any other record is
// removed so that the next request runs.
func (s *Store) drop(key Key, rec *record) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if rec.canceled {
		rec.state = stateCanceledMarker
		s.touch(key)
		close(rec.done)
		return
	}
	rec.state = stateDropped
	if msg, ok := s.messages[key.message()]; ok {
		delete(msg.records, key.call())
		if len(msg.records) == 0 {
			delete(s.messages, key.message())
		}
	}
	close(rec.done)
}

// cancel applies the cancel decision table. It returns the canceled
// record, or nil when there is nothing to wait for.
func (s *Store) cancel(key Key) *record {
	s.mu.Lock()
	defer s.mu.Unlock()

	if rec := s.lookup(key); rec != nil {
		s.touch(key)
		rec.canceled = true
		return rec
	}
	if !s.provenNew(key.MessageID) {
		return nil
	}
	done := make(chan struct{})
	close(done)
	s.insert(key, &record{canceled: true, state: stateCanceledMarker, done: done})
	return nil
}

// setHook registers rec's cancel hook. It reports whether the caller must
// run it because rec is already canceled. Only the first hook is kept.
func (s *Store) setHook(rec *record, hook *cancelHook) (mustRun bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if rec.hook != nil {
		return false
	}
	rec.hook = hook
	if !rec.canceled {
		return false
	}
	hook.started = true
	return true
}

// startHook returns rec's cancel hook and whether the caller must run it.
func (s *Store) startHook(rec *record) (hook *cancelHook, mustRun bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	hook = rec.hook
	if hook == nil || hook.started {
		return hook, false
	}
	hook.started = true
	return hook, true
}

// waitDone reports whether rec's run ended before ctx did.
func waitDone(ctx context.Context, rec *record) bool {
	// An ended run wins over a done ctx.
	select {
	case <-rec.done:
		return true
	default:
	}
	select {
	case <-rec.done:
		return true
	case <-ctx.Done():
		return false
	}
}
