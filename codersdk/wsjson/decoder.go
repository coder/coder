package wsjson

import (
	"context"
	"encoding/json"
	"sync/atomic"

	"nhooyr.io/websocket"

	"cdr.dev/slog"
)

type Decoder[T any] struct {
	conn       *websocket.Conn
	typ        websocket.MessageType
	ctx        context.Context
	cancel     context.CancelFunc
	chanCalled atomic.Bool
	logger     slog.Logger
}

// Chan starts the decoder reading from the websocket and returns a channel for reading the
// resulting values. The chan T is closed if the underlying websocket is closed, or we encounter an
// error. We also close the underlying websocket if we encounter an error reading or decoding.
func (d *Decoder[T]) Chan() <-chan T {
	if !d.chanCalled.CompareAndSwap(false, true) {
		panic("chan called more than once")
	}
	values := make(chan T, 1)
	go func() {
		defer close(values)
		defer d.conn.Close(websocket.StatusGoingAway, "")
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

// nolint: revive // complains that Encoder has the same function name
func (d *Decoder[T]) Close() error {
	err := d.conn.Close(websocket.StatusNormalClosure, "")
	d.cancel()
	return err
}

// NewDecoder creates a JSON-over-websocket decoder for type T, which must be deserializable from
// JSON.
func NewDecoder[T any](conn *websocket.Conn, typ websocket.MessageType, logger slog.Logger) *Decoder[T] {
	ctx, cancel := context.WithCancel(context.Background())
	return &Decoder[T]{conn: conn, ctx: ctx, cancel: cancel, typ: typ, logger: logger}
}
