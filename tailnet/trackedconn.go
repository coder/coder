package tailnet

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/google/uuid"

	"cdr.dev/slog"
)

// WriteTimeout is the amount of time we wait to write a node update to a connection before we declare it hung.
// It is exported so that tests can use it.
const WriteTimeout = time.Second * 5

type TrackedConn struct {
	ctx      context.Context
	cancel   func()
	client   CoordinatorClient
	replies  chan CoordinatorReply
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

var _ Queue = &TrackedConn{}

func NewTrackedConn(ctx context.Context, cancel func(), client CoordinatorClient, id uuid.UUID, logger slog.Logger, name string, overwrites int64) *TrackedConn {
	// Buffer replies so they don't block, since we hold the coordinator mutex
	// while queuing. Node updates don't come quickly, so 512 should be plenty
	// for all but the most pathological cases.
	replies := make(chan CoordinatorReply, 512)
	now := time.Now().Unix()
	return &TrackedConn{
		ctx:        ctx,
		cancel:     cancel,
		client:     client,
		replies:    replies,
		logger:     logger,
		id:         id,
		start:      now,
		lastWrite:  now,
		name:       name,
		overwrites: overwrites,
	}
}

func (t *TrackedConn) Enqueue(reply CoordinatorReply) (err error) {
	atomic.StoreInt64(&t.lastWrite, time.Now().Unix())
	select {
	case t.replies <- reply:
		return nil
	default:
		return ErrWouldBlock
	}
}

func (t *TrackedConn) UniqueID() uuid.UUID {
	return t.id
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

// Close the connection and cancel the context for reading node updates from the queue
func (t *TrackedConn) Close() error {
	t.cancel()
	return t.client.Close()
}

// SendUpdates reads a reply and writes it to the connection. Ends when writes
// hit an error or context is canceled.
func (t *TrackedConn) SendUpdates() {
	for {
		select {
		case <-t.ctx.Done():
			t.logger.Debug(t.ctx, "done sending coordinator replies")
			return
		case reply := <-t.replies:
			err := t.client.WriteReply(reply)
			if err != nil {
				// often, this is just because the connection is closed/broken,
				// so only log at debug.
				t.logger.Debug(t.ctx, "could not write coordinator reply to connection",
					slog.Error(err), slog.F("reply", reply))
				_ = t.Close()
				return
			}
			t.logger.Debug(t.ctx, "wrote coordinator reply", slog.F("reply", reply))
		}
	}
}
