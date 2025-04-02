package wsjson

import (
	"cdr.dev/slog"
	"github.com/coder/websocket"
)

// Stream is a two-way messaging interface over a WebSocket connection.
// As an implementation detail, we cannot currently use Encoder to implement
// the writing side of things because it only supports sending one message, and
// then immediately closing the WebSocket.
type Stream[R any, W any] struct {
	conn *websocket.Conn
	r    *Decoder[R]
	w    *Encoder[W]
}

func NewStream[R any, W any](conn *websocket.Conn, readType, writeType websocket.MessageType, logger slog.Logger) *Stream[R, W] {
	return &Stream[R, W]{
		conn: conn,
		r:    NewDecoder[R](conn, readType, logger),
		// We intentionally don't call `NewEncoder` because it calls `CloseRead`.
		w: &Encoder[W]{conn: conn, typ: writeType},
	}
}

func (s *Stream[R, W]) Chan() <-chan R {
	return s.r.Chan()
}

func (s *Stream[R, W]) Send(v W) error {
	return s.w.Encode(v)
}

func (s *Stream[R, W]) Close(c websocket.StatusCode) error {
	return s.conn.Close(c, "")
}
