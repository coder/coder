package wsjson

import (
	"cdr.dev/slog"
	"github.com/coder/websocket"
)

// Stream is a two-way messaging interface over a WebSocket connection.
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

// Chan returns a `chan` that you can read incoming messages from. The returned
// `chan` will be closed when the WebSocket connection is closed. If there is an
// error reading from the WebSocket or decoding a value the WebSocket will be
// closed.
//
// Safety: Chan must only be called once. Successive calls will panic.
func (s *Stream[R, W]) Chan() <-chan R {
	return s.r.Chan()
}

func (s *Stream[R, W]) Send(v W) error {
	return s.w.Encode(v)
}

func (s *Stream[R, W]) Close(c websocket.StatusCode) error {
	return s.conn.Close(c, "")
}

func (s *Stream[R, W]) Drop() {
	_ = s.conn.Close(websocket.StatusInternalError, "dropping connection")
}
