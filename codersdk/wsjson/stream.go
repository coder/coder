package wsjson

import (
	"context"
	"encoding/json"

	"cdr.dev/slog"
	"github.com/coder/websocket"
	"golang.org/x/xerrors"
)

// Stream is a two-way messaging interface over a WebSocket connection.
// As an implementation detail, we cannot currently use Encoder to implement
// the writing side of things because it only supports sending one message, and
// then immediately closing the WebSocket.
type Stream[R any, W any] struct {
	conn *websocket.Conn
	r    *Decoder[R]

	writeType websocket.MessageType
}

func NewStream[R any, W any](conn *websocket.Conn, readType, writeType websocket.MessageType, logger slog.Logger) *Stream[R, W] {
	return &Stream[R, W]{
		conn:      conn,
		r:         NewDecoder[R](conn, readType, logger),
		writeType: writeType,
	}
}

func (s *Stream[R, W]) Chan() <-chan R {
	return s.r.Chan()
}

func (s *Stream[R, W]) Send(v W) error {
	w, err := s.conn.Writer(context.Background(), s.writeType)
	if err != nil {
		return xerrors.Errorf("get websocket writer: %w", err)
	}
	j := json.NewEncoder(w)
	err = j.Encode(v)
	if err != nil {
		return xerrors.Errorf("encode json: %w", err)
	}
	return nil
}

func (s *Stream[R, W]) Close(c websocket.StatusCode) error {
	return s.conn.Close(c, "")
}
