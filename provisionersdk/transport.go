package provisionersdk

import (
	"context"
	"io"

	"github.com/hashicorp/yamux"
	"storj.io/drpc"
	"storj.io/drpc/drpcconn"
)

// TransportPipe creates an in-memory pipe for dRPC transport.
func TransportPipe() (*yamux.Session, *yamux.Session) {
	clientReader, clientWriter := io.Pipe()
	serverReader, serverWriter := io.Pipe()
	yamuxConfig := yamux.DefaultConfig()
	yamuxConfig.LogOutput = io.Discard
	client, err := yamux.Client(&readWriteCloser{
		ReadCloser: clientReader,
		Writer:     serverWriter,
	}, yamuxConfig)
	if err != nil {
		panic(err)
	}

	server, err := yamux.Server(&readWriteCloser{
		ReadCloser: serverReader,
		Writer:     clientWriter,
	}, yamuxConfig)
	if err != nil {
		panic(err)
	}
	return client, server
}

// Conn returns a multiplexed dRPC connection from a yamux session.
func Conn(session *yamux.Session) drpc.Conn {
	return &multiplexedDRPC{session}
}

type readWriteCloser struct {
	io.ReadCloser
	io.Writer
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

func (m *multiplexedDRPC) Invoke(ctx context.Context, rpc string, enc drpc.Encoding, in, out drpc.Message) error {
	conn, err := m.session.Open()
	if err != nil {
		return err
	}
	return drpcconn.New(conn).Invoke(ctx, rpc, enc, in, out)
}

func (m *multiplexedDRPC) NewStream(ctx context.Context, rpc string, enc drpc.Encoding) (drpc.Stream, error) {
	conn, err := m.session.Open()
	if err != nil {
		return nil, err
	}
	return drpcconn.New(conn).NewStream(ctx, rpc, enc)
}
