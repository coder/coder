// Package agenttoolcall decides whether the workspace agent acts on a
// chat tool call request, and records what it did so that repeated
// requests for the same tool call get the same answer instead of a second
// side effect. chatd only sends requests for a chat's latest assistant
// message, so the agent keeps records only for each chat's latest message
// ID and refuses older messages; an agent that has run since before a tool
// call was committed and has no record of it therefore never received it.
package agenttoolcall

import (
	"context"
	"crypto/sha256"
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk/workspacesdk"
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

// Errors that answer a request without acting on the tool call.
var (
	ErrStaleToolCall             = xerrors.New("tool call is in an older message than the chat's latest")
	ErrAgentStartedAfterToolCall = xerrors.New("agent started after the tool call was committed")
	ErrInputMismatch             = xerrors.New("tool call input differs from the recorded input")
	ErrToolCallCanceled          = xerrors.New("tool call was canceled")
)

// errorResponses maps each error above to the HTTP 409 body that answers
// a request refused with it.
var errorResponses = []struct {
	err     error
	code    workspacesdk.ToolCallErrorCode
	message string
}{
	{ErrStaleToolCall, workspacesdk.ToolCallErrorStale, "The tool call is in an older message than the chat's latest message."},
	{ErrAgentStartedAfterToolCall, workspacesdk.ToolCallErrorAgentStartedAfterToolCall, "The workspace agent started after the tool call was committed."},
	{ErrInputMismatch, workspacesdk.ToolCallErrorInputMismatch, "The request differs from the recorded request for this tool call."},
	{ErrToolCallCanceled, workspacesdk.ToolCallErrorCanceled, "The tool call was canceled."},
}

// ErrorResponse returns the HTTP 409 body that answers a request refused
// with one of the errors above. ok is false for any other error.
func ErrorResponse(err error) (resp workspacesdk.ToolCallError, ok bool) {
	for _, r := range errorResponses {
		if errors.Is(err, r.err) {
			resp.Code = r.code
			resp.Message = r.message
			return resp, true
		}
	}
	return resp, false
}

// ageMargin covers request transit time after chatd measured the tool call age.
const ageMargin = 2 * time.Second

// Chats holds what every record table of one agent shares: the agent
// start and each chat's latest message ID. chatd sends requests only for a
// chat's latest message, so a request for a newer message through any
// table makes the older tool calls stale in every table. Per-chat state
// lives as long as Chats.
type Chats struct {
	clock     quartz.Clock
	startedAt time.Time

	mu     sync.Mutex
	latest map[uuid.UUID]int64
}

// NewChats returns Chats whose agent start, used by the
// ErrAgentStartedAfterToolCall rule, is clock.Now().
func NewChats(clock quartz.Clock) *Chats {
	return &Chats{
		clock:     clock,
		startedAt: clock.Now(),
		latest:    make(map[uuid.UUID]int64),
	}
}

// latestMessageID returns chatID's latest message ID, zero if the agent
// has received no request for the chat.
func (c *Chats) latestMessageID(chatID uuid.UUID) int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.latest[chatID]
}

// admit raises chatID's latest message ID to messageID if messageID is
// newer, and reports false when messageID is older. Checking and raising
// under one lock keeps two tables from both acting on different messages.
func (c *Chats) admit(chatID uuid.UUID, messageID int64) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if messageID < c.latest[chatID] {
		return false
	}
	c.latest[chatID] = messageID
	return true
}

// startedWithin reports whether the agent started no more than age plus
// ageMargin ago, so an earlier agent may have received a tool call of that
// age. age comes from a request header and can be as large as the maximum
// Duration, so age+ageMargin could overflow.
func (c *Chats) startedWithin(age time.Duration) bool {
	return c.clock.Since(c.startedAt)-ageMargin <= max(age, 0)
}

// Records holds the records of one kind of tool call, for example process
// starts, for each chat's latest message. V is what a start produced, for
// example a process. Tables built with the same Chats share each chat's
// latest message ID and the agent start.
type Records[V any] struct {
	chats *Chats

	mu       sync.Mutex
	messages map[uuid.UUID]*messageRecords[V]
}

// messageRecords holds a table's records of the tool calls of one
// message by provider tool call ID. A table keeps only the newest message
// it has records for; records of an older message are dropped when a
// newer one is inserted, and ignored once the chat's latest message ID
// has moved past them.
type messageRecords[V any] struct {
	messageID int64
	records   map[string]*record[V]
}

// record is the state of one tool call. input and canceled never change
// after the record is inserted. value and err are written once, before done
// is closed, and are read only after done is closed.
type record[V any] struct {
	input    [sha256.Size]byte
	canceled bool
	done     chan struct{}
	value    V
	err      error
}

// NewRecords returns empty Records that share chats with the other tables
// of the agent.
func NewRecords[V any](chats *Chats) *Records[V] {
	return &Records[V]{
		chats:    chats,
		messages: make(map[uuid.UUID]*messageRecords[V]),
	}
}

// Start runs start at most once per key and returns its value and error,
// or the recorded value and error of an earlier Start. age is the tool
// call age, which is non-negative; a negative age counts as zero.
//
// Start returns ErrStaleToolCall for a message older than the chat's
// latest, ErrToolCallCanceled for a canceled record, ErrInputMismatch when
// input differs from the recorded input, and ErrAgentStartedAfterToolCall
// when there is no record and the agent has run for at most age plus
// ageMargin. Concurrent callers for key wait for the pending start or for
// their ctx. start runs regardless of ctx, and its error is recorded like a
// value. If start panics, the record gets an error and the panic continues.
func (r *Records[V]) Start(ctx context.Context, key Key, age time.Duration, input [sha256.Size]byte, start func() (V, error)) (V, error) {
	var zero V

	r.mu.Lock()
	rec, ok, err := r.lookup(key, age)
	if err != nil {
		r.mu.Unlock()
		return zero, err
	}
	if !ok {
		// Insert before calling start so that concurrent requests for key
		// wait for this start instead of running their own.
		rec = &record[V]{input: input, done: make(chan struct{})}
		if err := r.insert(key, rec); err != nil {
			r.mu.Unlock()
			return zero, err
		}
		r.mu.Unlock()
		return rec.run(start)
	}
	r.mu.Unlock()

	if rec.canceled {
		return zero, ErrToolCallCanceled
	}
	if rec.input != input {
		return zero, ErrInputMismatch
	}
	return rec.wait(ctx)
}

// Cancel returns the recorded value so the caller can stop it. started is
// false when the agent never received the tool call; Cancel then records
// it as canceled so a later Start returns ErrToolCallCanceled.
//
// Cancel returns ErrStaleToolCall and ErrAgentStartedAfterToolCall under
// the same rules as Start, with age treated the same way. For a started
// tool call it returns started=true with the recorded value and error,
// waiting for a pending start first. If ctx ends during that wait, Cancel
// returns started=true and ctx's error wrapped, on the same path as a
// recorded start error; callers tell them apart by checking ctx.Err().
func (r *Records[V]) Cancel(ctx context.Context, key Key, age time.Duration) (v V, started bool, err error) {
	r.mu.Lock()
	rec, ok, err := r.lookup(key, age)
	if err != nil {
		r.mu.Unlock()
		return v, false, err
	}
	if !ok {
		done := make(chan struct{})
		close(done)
		if err := r.insert(key, &record[V]{canceled: true, done: done}); err != nil {
			r.mu.Unlock()
			return v, false, err
		}
		r.mu.Unlock()
		return v, false, nil
	}
	r.mu.Unlock()

	if rec.canceled {
		return v, false, nil
	}
	v, err = rec.wait(ctx)
	return v, true, err
}

// Current reports whether key has a record in its chat's latest message.
func (r *Records[V]) Current(key Key) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	msg, ok := r.messages[key.ChatID]
	if !ok || msg.messageID != key.MessageID || r.chats.latestMessageID(key.ChatID) != key.MessageID {
		return false
	}
	_, ok = msg.records[key.ToolCallID]
	return ok
}

// lookup returns the record for key, ok=false when the agent can prove it
// never received the tool call, or the error that answers the request.
// r.mu must be held.
func (r *Records[V]) lookup(key Key, age time.Duration) (rec *record[V], ok bool, err error) {
	if key.MessageID < r.chats.latestMessageID(key.ChatID) {
		return nil, false, ErrStaleToolCall
	}
	if msg, found := r.messages[key.ChatID]; found && msg.messageID == key.MessageID {
		if rec, found := msg.records[key.ToolCallID]; found {
			return rec, true, nil
		}
	}
	// Records from before agent start are lost, so an agent that started
	// after the tool call was committed cannot tell whether an earlier
	// agent received it.
	if r.chats.startedWithin(age) {
		return nil, false, ErrAgentStartedAfterToolCall
	}
	return nil, false, nil
}

// insert records rec for key. A key in a newer message raises the chat's
// latest message ID for every table and drops this table's records of
// older messages, which chatd has resolved. It returns ErrStaleToolCall
// when another table raised the latest message ID past key since lookup.
// r.mu must be held.
func (r *Records[V]) insert(key Key, rec *record[V]) error {
	if !r.chats.admit(key.ChatID, key.MessageID) {
		return ErrStaleToolCall
	}
	msg, ok := r.messages[key.ChatID]
	if !ok || msg.messageID != key.MessageID {
		msg = &messageRecords[V]{
			messageID: key.MessageID,
			records:   make(map[string]*record[V]),
		}
		r.messages[key.ChatID] = msg
	}
	msg.records[key.ToolCallID] = rec
	return nil
}

// run calls start and publishes its result to rec's waiters.
func (rec *record[V]) run(start func() (V, error)) (V, error) {
	returned := false
	defer func() {
		if !returned {
			// start panicked. Without a result, waiters and later requests
			// would wait on this record forever.
			rec.err = xerrors.New("tool call start panicked")
			close(rec.done)
		}
	}()
	v, err := start()
	returned = true
	rec.value, rec.err = v, err
	close(rec.done)
	return v, err
}

// wait returns rec's result once it is published, or the context error if
// ctx ends first.
func (rec *record[V]) wait(ctx context.Context) (V, error) {
	// A published result wins over a done ctx.
	select {
	case <-rec.done:
		return rec.value, rec.err
	default:
	}
	select {
	case <-rec.done:
		return rec.value, rec.err
	case <-ctx.Done():
		var zero V
		return zero, xerrors.Errorf("wait for tool call start: %w", ctx.Err())
	}
}
