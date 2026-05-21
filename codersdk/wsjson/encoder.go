package wsjson

import (
	"context"
	"encoding/json"

	"golang.org/x/xerrors"

	"github.com/coder/websocket"
)

type Encoder[T any] struct {
	conn *websocket.Conn
	typ  websocket.MessageType
}

func (e *Encoder[T]) Encode(v T) error {
	w, err := e.conn.Writer(context.Background(), e.typ)
	if err != nil {
		return xerrors.Errorf("get websocket writer: %w", err)
	}
	defer w.Close()
	j := json.NewEncoder(w)
	err = j.Encode(v)
	if err != nil {
		return xerrors.Errorf("encode json: %w", err)
	}
	return nil
}

// nolint: revive // complains that Decoder has the same function name
func (e *Encoder[T]) Close(c websocket.StatusCode) error {
	return e.conn.Close(c, "")
}

// NewEncoder creates a JSON-over websocket encoder for the type T, which must be JSON-serializable.
// You may then call Encode() to send objects over the websocket. Creating an Encoder closes the
// websocket for reading, turning it into a unidirectional write stream of JSON-encoded objects.
func NewEncoder[T any](conn *websocket.Conn, typ websocket.MessageType) *Encoder[T] {
	// Here we close the websocket for reading, so that the websocket library will handle pings and
	// close frames.
	_ = conn.CloseRead(context.Background())
	return &Encoder[T]{conn: conn, typ: typ}
}
