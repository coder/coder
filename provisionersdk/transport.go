package provisionersdk

import (
	"context"
	"io"

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
