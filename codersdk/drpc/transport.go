package drpc

import (
	"context"
	"net"
	"sync"

	"github.com/hashicorp/yamux"
	"github.com/valyala/fasthttp/fasthttputil"
	"storj.io/drpc"
	"storj.io/drpc/drpcconn"

	"github.com/coder/coder/v2/coderd/tracing"
)

const (
	// MaxMessageSize is the maximum payload size that can be
	// transported without error.
	MaxMessageSize = 4 << 20
)

// MultiplexedConn returns a multiplexed dRPC connection from a yamux Session.
func MultiplexedConn(session *yamux.Session) drpc.Conn {
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

func MemTransportPipe() (drpc.Conn, net.Listener) {
	m := &memDRPC{
		closed: make(chan struct{}),
		l:      fasthttputil.NewInmemoryListener(),
	}

	return m, m.l
}

type memDRPC struct {
	closeOnce sync.Once
	closed    chan struct{}
	l         *fasthttputil.InmemoryListener
}

func (m *memDRPC) Close() error {
	err := m.l.Close()
	m.closeOnce.Do(func() { close(m.closed) })
	return err
}

func (m *memDRPC) Closed() <-chan struct{} {
	return m.closed
}

func (m *memDRPC) Invoke(ctx context.Context, rpc string, enc drpc.Encoding, inMessage, outMessage drpc.Message) error {
	conn, err := m.l.Dial()
	if err != nil {
		return err
	}

	dConn := &tracing.DRPCConn{Conn: drpcconn.New(conn)}
	defer func() {
		_ = dConn.Close()
		_ = conn.Close()
	}()
	return dConn.Invoke(ctx, rpc, enc, inMessage, outMessage)
}

func (m *memDRPC) NewStream(ctx context.Context, rpc string, enc drpc.Encoding) (drpc.Stream, error) {
	conn, err := m.l.Dial()
	if err != nil {
		return nil, err
	}
	dConn := &tracing.DRPCConn{Conn: drpcconn.New(conn)}
	stream, err := dConn.NewStream(ctx, rpc, enc)
	if err != nil {
		_ = dConn.Close()
		_ = conn.Close()
		return nil, err
	}
	go func() {
		select {
		case <-stream.Context().Done():
		case <-m.closed:
		}
		_ = dConn.Close()
		_ = conn.Close()
	}()
	return stream, nil
}
