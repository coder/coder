package tailnet

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"sync/atomic"
	"time"

	"github.com/google/uuid"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/tailnet/proto"
)

const (
	// WriteTimeout is the amount of time we wait to write a node update to a connection before we
	// declare it hung. It is exported so that tests can use it.
	WriteTimeout = time.Second * 5
	// ResponseBufferSize is the max number of responses to buffer per connection before we start
	// dropping updates
	ResponseBufferSize = 512
	// RequestBufferSize is the max number of requests to buffer per connection
	RequestBufferSize = 32
)

type TrackedConn struct {
	ctx      context.Context
	cancel   func()
	kind     QueueKind
	conn     net.Conn
	updates  chan *proto.CoordinateResponse
	logger   slog.Logger
	lastData []byte

	// ID is an ephemeral UUID used to uniquely identify the owner of the
	// connection.
	id uuid.UUID

	name       string
	start      int64
	lastWrite  int64
	overwrites int64
}

func NewTrackedConn(ctx context.Context, cancel func(),
	conn net.Conn,
	id uuid.UUID,
	logger slog.Logger,
	name string,
	overwrites int64,
	kind QueueKind,
) *TrackedConn {
	// buffer updates so they don't block, since we hold the
	// coordinator mutex while queuing.  Node updates don't
	// come quickly, so 512 should be plenty for all but
	// the most pathological cases.
	updates := make(chan *proto.CoordinateResponse, ResponseBufferSize)
	now := time.Now().Unix()
	return &TrackedConn{
		ctx:        ctx,
		conn:       conn,
		cancel:     cancel,
		updates:    updates,
		logger:     logger,
		id:         id,
		start:      now,
		lastWrite:  now,
		name:       name,
		overwrites: overwrites,
		kind:       kind,
	}
}

func (t *TrackedConn) Enqueue(resp *proto.CoordinateResponse) (err error) {
	atomic.StoreInt64(&t.lastWrite, time.Now().Unix())
	select {
	case t.updates <- resp:
		return nil
	default:
		return ErrWouldBlock
	}
}

func (t *TrackedConn) UniqueID() uuid.UUID {
	return t.id
}

func (t *TrackedConn) Kind() QueueKind {
	return t.kind
}

func (t *TrackedConn) Name() string {
	return t.name
}

func (t *TrackedConn) Stats() (start, lastWrite int64) {
	return t.start, atomic.LoadInt64(&t.lastWrite)
}

func (t *TrackedConn) Overwrites() int64 {
	return t.overwrites
}

func (t *TrackedConn) CoordinatorClose() error {
	return t.Close()
}

func (t *TrackedConn) Done() <-chan struct{} {
	return t.ctx.Done()
}

// Close the connection and cancel the context for reading node updates from the queue
func (t *TrackedConn) Close() error {
	t.cancel()
	return t.conn.Close()
}

// SendUpdates reads node updates and writes them to the connection.  Ends when writes hit an error or context is
// canceled.
func (t *TrackedConn) SendUpdates() {
	for {
		select {
		case <-t.ctx.Done():
			t.logger.Debug(t.ctx, "done sending updates")
			return
		case resp := <-t.updates:
			nodes, err := OnlyNodeUpdates(resp)
			if err != nil {
				t.logger.Critical(t.ctx, "unable to parse response", slog.Error(err))
				return
			}
			if len(nodes) == 0 {
				t.logger.Debug(t.ctx, "skipping response with no nodes")
				continue
			}
			data, err := json.Marshal(nodes)
			if err != nil {
				t.logger.Error(t.ctx, "unable to marshal nodes update", slog.Error(err), slog.F("nodes", nodes))
				return
			}
			if bytes.Equal(t.lastData, data) {
				t.logger.Debug(t.ctx, "skipping duplicate update", slog.F("nodes", string(data)))
				continue
			}

			// Set a deadline so that hung connections don't put back pressure on the system.
			// Node updates are tiny, so even the dinkiest connection can handle them if it's not hung.
			err = t.conn.SetWriteDeadline(time.Now().Add(WriteTimeout))
			if err != nil {
				// often, this is just because the connection is closed/broken, so only log at debug.
				t.logger.Debug(t.ctx, "unable to set write deadline", slog.Error(err))
				_ = t.Close()
				return
			}
			_, err = t.conn.Write(data)
			if err != nil {
				// often, this is just because the connection is closed/broken, so only log at debug.
				t.logger.Debug(t.ctx, "could not write nodes to connection",
					slog.Error(err), slog.F("nodes", string(data)))
				_ = t.Close()
				return
			}
			t.logger.Debug(t.ctx, "wrote nodes", slog.F("nodes", string(data)))

			// nhooyr.io/websocket has a bugged implementation of deadlines on a websocket net.Conn.  What they are
			// *supposed* to do is set a deadline for any subsequent writes to complete, otherwise the call to Write()
			// fails.  What nhooyr.io/websocket does is set a timer, after which it expires the websocket write context.
			// If this timer fires, then the next write will fail *even if we set a new write deadline*.  So, after
			// our successful write, it is important that we reset the deadline before it fires.
			err = t.conn.SetWriteDeadline(time.Time{})
			if err != nil {
				// often, this is just because the connection is closed/broken, so only log at debug.
				t.logger.Debug(t.ctx, "unable to extend write deadline", slog.Error(err))
				_ = t.Close()
				return
			}
			t.lastData = data
		}
	}
}
