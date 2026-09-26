// Package agenttoolcall decides whether the workspace agent acts on a
// chat tool call request, and records the response of the one run so that
// repeated requests for the same tool call get it back instead of a second
// side effect. chatd only sends requests for a chat's latest assistant
// message, so the agent keeps records only for each chat's latest message
// ID and refuses older messages; an agent that has run since before a tool
// call was committed and has no record of it therefore never received it.
package agenttoolcall

import (
	"context"
	"crypto/sha256"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/quartz"
)

// Key identifies a chat tool call.
type Key struct {
	ChatID uuid.UUID
	// MessageID is the chat message ID of the assistant message that
	// contains the tool call.
	MessageID int64
	// ToolCallID is the provider tool call ID.
	ToolCallID string
}

// Errors that answer a request without acting on the tool call. Each has
// a code and message in toolCallErrors.
var (
	errStaleToolCall             = xerrors.New("tool call is in an older message than the chat's latest")
	errAgentStartedAfterToolCall = xerrors.New("agent started after the tool call was committed")
	errInputMismatch             = xerrors.New("tool call request differs from the recorded request")
	errToolCallCanceled          = xerrors.New("tool call was canceled")
)

// ageMargin covers request transit time after chatd measured the tool call age.
const ageMargin = 2 * time.Second

// Store holds one agent's tool call records: per chat, the latest message
// ID and the records of that message's tool calls. Per-chat state lives as
// long as the Store.
type Store struct {
	clock     quartz.Clock
	startedAt time.Time

	mu    sync.Mutex
	chats map[uuid.UUID]*chatRecords
}

type chatRecords struct {
	latestMessageID int64
	// records holds the tool calls of latestMessageID by provider tool
	// call ID.
	records map[string]*record
}

// record is the state of one tool call. input, canceled, and createdAt
// never change after the record is inserted. resp is written once, before
// done is closed, and is read only after done is closed.
type record struct {
	input     [sha256.Size]byte
	canceled  bool
	createdAt time.Time
	done      chan struct{}
	resp      response
}

// response is a recorded response.
type response struct {
	status      int
	contentType string
	body        []byte
}

// NewStore returns an empty Store. The agent start that the
// agent-started-after-tool-call rule compares against is clock.Now().
func NewStore(clock quartz.Clock) *Store {
	return &Store{
		clock:     clock,
		startedAt: clock.Now(),
		chats:     make(map[uuid.UUID]*chatRecords),
	}
}

// Current reports whether key has a record in its chat's latest message.
func (s *Store) Current(key Key) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	chat, ok := s.chats[key.ChatID]
	if !ok || chat.latestMessageID != key.MessageID {
		return false
	}
	_, ok = chat.records[key.ToolCallID]
	return ok
}

// begin applies the decision rules to a request that runs the tool call.
// It returns the existing record, or a new pending record (created) that
// the caller must run and publish. Concurrent requests for key wait on the
// pending record instead of running their own.
func (s *Store) begin(key Key, age time.Duration, input [sha256.Size]byte) (rec *record, created bool, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	rec, ok, err := s.lookup(key, age)
	if err != nil {
		return nil, false, err
	}
	if ok {
		if rec.canceled {
			return nil, false, errToolCallCanceled
		}
		if rec.input != input {
			return nil, false, errInputMismatch
		}
		return rec, false, nil
	}
	rec = &record{input: input, createdAt: s.clock.Now(), done: make(chan struct{})}
	s.insert(key, rec)
	return rec, true, nil
}

// cancel applies the decision rules to a cancel. It returns the existing
// record, which may be canceled or pending, or received=false after
// recording a tool call the agent never received as canceled.
func (s *Store) cancel(key Key, age time.Duration) (rec *record, received bool, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	rec, ok, err := s.lookup(key, age)
	if err != nil {
		return nil, false, err
	}
	if ok {
		return rec, true, nil
	}
	done := make(chan struct{})
	close(done)
	s.insert(key, &record{canceled: true, createdAt: s.clock.Now(), done: done})
	return nil, false, nil
}

// lookup returns the record for key, ok=false when the agent can prove it
// never received the tool call, or the error that answers the request.
// age is non-negative; a negative age counts as zero. s.mu must be held.
func (s *Store) lookup(key Key, age time.Duration) (rec *record, ok bool, err error) {
	if chat, found := s.chats[key.ChatID]; found {
		if key.MessageID < chat.latestMessageID {
			return nil, false, errStaleToolCall
		}
		if rec, found := chat.records[key.ToolCallID]; found && key.MessageID == chat.latestMessageID {
			return rec, true, nil
		}
	}
	// Records from before agent start are lost, so an agent that started
	// after the tool call was committed cannot tell whether an earlier
	// agent received it. age comes from a request header and can be as
	// large as the maximum Duration, so age+ageMargin could overflow.
	if s.clock.Since(s.startedAt)-ageMargin <= max(age, 0) {
		return nil, false, errAgentStartedAfterToolCall
	}
	return nil, false, nil
}

// insert records rec for key. A key in a newer message raises the chat's
// latest message ID and drops the records of older messages, which chatd
// has resolved. s.mu must be held.
func (s *Store) insert(key Key, rec *record) {
	chat, ok := s.chats[key.ChatID]
	if !ok || key.MessageID > chat.latestMessageID {
		chat = &chatRecords{
			latestMessageID: key.MessageID,
			records:         make(map[string]*record),
		}
		s.chats[key.ChatID] = chat
	}
	chat.records[key.ToolCallID] = rec
}

// runAge returns the time since rec was created.
func (s *Store) runAge(rec *record) time.Duration {
	return s.clock.Since(rec.createdAt)
}

// publish records resp and wakes rec's waiters.
func (rec *record) publish(resp response) {
	rec.resp = resp
	close(rec.done)
}

// wait returns rec's response once it is published, or false if ctx ends
// first.
func (rec *record) wait(ctx context.Context) (response, bool) {
	// A published response wins over a done ctx.
	select {
	case <-rec.done:
		return rec.resp, true
	default:
	}
	select {
	case <-rec.done:
		return rec.resp, true
	case <-ctx.Done():
		return response{}, false
	}
}
