package codersdk

import (
	"context"

	"golang.org/x/xerrors"
	"nhooyr.io/websocket"
	"storj.io/drpc"
	"storj.io/drpc/drpcconn"

	"github.com/hashicorp/yamux"

	"github.com/coder/coder/provisionerd/proto"
)

// ProvisionerDaemonClient returns the gRPC service for a provisioner daemon implementation.
func (c *Client) ProvisionerDaemonClient(ctx context.Context) (proto.DRPCProvisionerDaemonClient, error) {
	serverURL, err := c.url.Parse("/api/v2/provisionerd")
	if err != nil {
		return nil, xerrors.Errorf("parse url: %w", err)
	}
	conn, res, err := websocket.Dial(ctx, serverURL.String(), &websocket.DialOptions{
		HTTPClient: c.httpClient,
	})
	if err != nil {
		if res == nil {
			return nil, err
		}
		return nil, readBodyAsError(res)
	}
	session, err := yamux.Client(websocket.NetConn(context.Background(), conn, websocket.MessageBinary), nil)
	if err != nil {
		return nil, xerrors.Errorf("multiplex client: %w", err)
	}
	return proto.NewDRPCProvisionerDaemonClient(&multiplexedDRPC{
		session: session,
	}), nil
}

// dRPC is a single-stream protocol by design. It's intended to operate
// a single HTTP-request per invocation. This multiplexes the WebSocket
// using yamux to enable multiple streams to function on a single connection.
//
// If this connection is too slow, we can create a WebSocket for each request.
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
