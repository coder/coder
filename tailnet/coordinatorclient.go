package tailnet

import (
	"encoding/json"
	"io"
	"net"
	"time"

	"golang.org/x/xerrors"
)

// CoordinatorClient is the interface for clients accepted by the Coordinator.
// There are two implementations available, the deprecated CoordinatorClientV1
// and the new CoordinatorClientV2.
//
// The Coder API handles both clients, but the deprecated client is only
// supported to allow older clients/agents to connect and will be removed
// eventually.
//
// The deprecated endpoint isn't used by any clients in the current codebase, so
// up-to-date versions of clients will always use the new endpoint.
type CoordinatorClient interface {
	io.Closer
	ReadRequest() (CoordinatorRequest, error)
	WriteReply(reply CoordinatorReply) error
}

// CoordinatorClientV1 is the deprecated interface for clients accepted by the
// old coordinator protocol.
//
// This protocol was deprecated in favor of the new protocol, which uses JSON
// objects instead of arrays to allow for adding new fields without breaking
// compatibility.
type CoordinatorClientV1 struct {
	net.Conn
	// The decoder needs to be persisted in case it reads slightly more than a
	// single JSON message, otherwise the buffered data will be lost.
	dec *json.Decoder
}

var _ CoordinatorClient = &CoordinatorClientV1{}

// NewCoordinatorClientV1 creates a new CoordinatorClientV1 from a net.Conn.
func NewCoordinatorClientV1(conn net.Conn) *CoordinatorClientV1 {
	return &CoordinatorClientV1{
		Conn: &writeTimeoutConn{
			Conn:         conn,
			writeTimeout: WriteTimeout,
		},
		dec: json.NewDecoder(conn),
	}
}

// ReadRequest implements CoordinatorClient. The returned request is constructed
// from the single JSON node sent by the client.
func (c *CoordinatorClientV1) ReadRequest() (CoordinatorRequest, error) {
	var node Node
	err := c.dec.Decode(&node)
	if err != nil {
		return CoordinatorRequest{}, xerrors.Errorf("decode coordinator protocol V1 request: %w", err)
	}
	return CoordinatorRequest{Node: &node}, nil
}

// WriteReply implements CoordinatorClient. The reply is sent as a single JSON
// array with the value of reply.AddNodes. If reply.AddNodes is nil or empty,
// no reply is sent.
func (c *CoordinatorClientV1) WriteReply(reply CoordinatorReply) error {
	if len(reply.AddNodes) == 0 {
		return nil
	}
	err := json.NewEncoder(c.Conn).Encode(reply.AddNodes)
	if err != nil {
		return xerrors.Errorf("encode coordinator protocol V1 reply: %w", err)
	}
	return nil
}

// CoordinatorClientV2 is the interface for clients accepted by the new
// coordinator protocol.
type CoordinatorClientV2 struct {
	net.Conn
	// The decoder needs to be persisted in case it reads slightly more than a
	// single JSON message, otherwise the buffered data will be lost.
	dec *json.Decoder
}

var _ CoordinatorClient = &CoordinatorClientV2{}

// NewCoordinatorClientV2 creates a new CoordinatorClientV2 from a net.Conn.
func NewCoordinatorClientV2(conn net.Conn) *CoordinatorClientV2 {
	return &CoordinatorClientV2{
		Conn: &writeTimeoutConn{
			Conn:         conn,
			writeTimeout: WriteTimeout,
		},
		dec: json.NewDecoder(conn),
	}
}

// ReadRequest implements CoordinatorClient.
func (c *CoordinatorClientV2) ReadRequest() (CoordinatorRequest, error) {
	var req CoordinatorRequest
	err := c.dec.Decode(&req)
	if err != nil {
		return CoordinatorRequest{}, xerrors.Errorf("decode coordinator protocol V2 request: %w", err)
	}
	return req, nil
}

// WriteReply implements CoordinatorClient.
func (c *CoordinatorClientV2) WriteReply(reply CoordinatorReply) error {
	err := json.NewEncoder(c.Conn).Encode(reply)
	if err != nil {
		return xerrors.Errorf("encode coordinator protocol V2 reply: %w", err)
	}
	return nil
}

type writeTimeoutConn struct {
	net.Conn
	writeTimeout time.Duration
}

func (c *writeTimeoutConn) Write(p []byte) (int, error) {
	// Set a deadline so that hung connections don't put back pressure on the
	// system. Coordinator packets are usually tiny, so even the dinkiest
	// connection can handle them if it's not hung.
	if c.writeTimeout > 0 {
		err := c.Conn.SetWriteDeadline(time.Now().Add(c.writeTimeout))
		if err != nil {
			return 0, xerrors.Errorf("set write deadline: %w", err)
		}
	}

	n, err := c.Conn.Write(p)
	if err != nil {
		return n, xerrors.Errorf("write: %w", err)
	}

	// nhooyr.io/websocket has a bugged implementation of deadlines on a
	// websocket net.Conn. What they are *supposed* to do is set a deadline for
	// any subsequent writes to complete, otherwise the call to Write() fails.
	// What nhooyr.io/websocket does is set a timer, after which it expires the
	// websocket write context. If this timer fires, then the next write will
	// fail *even if we set a new write deadline*. So, after our successful
	// write, it is important that we reset the deadline before it fires.
	if c.writeTimeout > 0 {
		err := c.Conn.SetWriteDeadline(time.Time{})
		if err != nil {
			return n, xerrors.Errorf("reset write deadline: %w", err)
		}
	}

	return n, nil
}
