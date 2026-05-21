package wsjson

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"

	"cdr.dev/slog/v3"
	"github.com/coder/websocket"
)

type Decoder[T any] struct {
	conn       *websocket.Conn
	typ        websocket.MessageType
	ctx        context.Context
	cancel     context.CancelFunc
	chanCalled atomic.Bool
	logger     slog.Logger
	closeOnce  sync.Once
	closeErr   error
}

// Chan returns a `chan` that you can read incoming messages from. The returned
// `chan` will be closed when the WebSocket connection is closed. If there is an
// error reading from the WebSocket or decoding a value the WebSocket will be
// closed.
//
// Safety: Chan must only be called once. Successive calls will panic.
func (d *Decoder[T]) Chan() <-chan T {
	if !d.chanCalled.CompareAndSwap(false, true) {
		panic("chan called more than once")
	}
	values := make(chan T, 1)
	go func() {
		defer close(values)
		defer func() { _ = d.closeConn(websocket.StatusGoingAway, "") }()
		for {
			// we don't use d.ctx here because it only gets canceled after closing the connection
			// and a "connection closed" type error is more clear than context canceled.
			typ, b, err := d.conn.Read(context.Background())
			if err != nil {
				// might be benign like EOF, so just log at debug
				d.logger.Debug(d.ctx, "error reading from websocket", slog.Error(err))
				return
			}
			if typ != d.typ {
				d.logger.Error(d.ctx, "websocket type mismatch while decoding")
				return
			}
			var value T
			err = json.Unmarshal(b, &value)
			if err != nil {
				d.logger.Error(d.ctx, "error unmarshalling", slog.Error(err))
				return
			}
			select {
			case values <- value:
				// OK
			case <-d.ctx.Done():
				return
			}
		}
	}()
	return values
}

// closeConn closes the underlying websocket connection and cancels the
// decoder context. It is safe to call multiple times or concurrently;
// only the first call sends a close frame and cancels the context.
// Subsequent calls return the result of that first close.
//
// The close code and reason are determined by whichever caller wins the
// race (Chan goroutine uses StatusGoingAway, Close uses
// StatusNormalClosure). After a server-initiated close the underlying
// TCP connection may already be torn down, so the close frame send is
// best-effort.
func (d *Decoder[T]) closeConn(code websocket.StatusCode, reason string) error {
	d.closeOnce.Do(func() {
		d.closeErr = d.conn.Close(code, reason)
		d.cancel()
	})
	// closeErr is safe to read without a mutex: sync.Once guarantees the
	// Do callback happens-before any subsequent Do call returns.
	return d.closeErr
}

// Close closes the decoder and the underlying websocket connection.
// It is safe to call multiple times or concurrently with the Chan
// goroutine.
//
// The returned error is from the first close attempt, which may have
// been initiated by the Chan goroutine (with StatusGoingAway) rather
// than by this call (StatusNormalClosure) depending on goroutine
// scheduling.
//
//nolint:revive // complains that Encoder has the same function name
func (d *Decoder[T]) Close() error {
	return d.closeConn(websocket.StatusNormalClosure, "")
}

// NewDecoder creates a JSON-over-websocket decoder for type T, which must be deserializable from
// JSON.
func NewDecoder[T any](conn *websocket.Conn, typ websocket.MessageType, logger slog.Logger) *Decoder[T] {
	ctx, cancel := context.WithCancel(context.Background())
	return &Decoder[T]{conn: conn, ctx: ctx, cancel: cancel, typ: typ, logger: logger}
}
