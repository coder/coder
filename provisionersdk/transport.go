package provisionersdk

import (
	"context"
	"io"
	"net"

	"github.com/hashicorp/yamux"
	"storj.io/drpc"
	"storj.io/drpc/drpcconn"
)

const (
	// MaxMessageSize is the maximum payload size that can be
	// transported without error.
	MaxMessageSize = 4 << 20
)

// TransportPipe creates an in-memory pipe for dRPC transport.
func TransportPipe() (*yamux.Session, *yamux.Session) {
	c1, c2 := net.Pipe()
	yamuxConfig := yamux.DefaultConfig()
	yamuxConfig.LogOutput = io.Discard
	client, err := yamux.Client(c1, yamuxConfig)
	if err != nil {
		panic(err)
	}
	server, err := yamux.Server(c2, yamuxConfig)
	if err != nil {
		panic(err)
	}
	return client, server
}

// Conn returns a multiplexed dRPC connection from a yamux session.
func Conn(session *yamux.Session) drpc.Conn {
	return &multiplexedDRPC{session}
}

// Allows concurrent requests on a single dRPC connection.
// Required for calling functions concurrently.
type multiplexedDRPC struct {
	session *yamux.Session
}

func (m *multiplexedDRPC) Close() error {
	return m.session.Close()
}

func (m *multiplexedDRPC) Closed() <-chan struct{} {
	return m.session.CloseChan()
}

func (m *multiplexedDRPC) Invoke(ctx context.Context, rpc string, enc drpc.Encoding, inMessage, outMessage drpc.Message) error {
	conn, err := m.session.Open()
	if err != nil {
		return err
	}
	dConn := drpcconn.New(conn)
	defer func() {
		_ = dConn.Close()
	}()
	return dConn.Invoke(ctx, rpc, enc, inMessage, outMessage)
}

func (m *multiplexedDRPC) NewStream(ctx context.Context, rpc string, enc drpc.Encoding) (drpc.Stream, error) {
	conn, err := m.session.Open()
	if err != nil {
		return nil, err
	}
	dConn := drpcconn.New(conn)
	stream, err := dConn.NewStream(ctx, rpc, enc)
	if err == nil {
		go func() {
			<-stream.Context().Done()
			_ = dConn.Close()
		}()
	}
	return stream, err
}
