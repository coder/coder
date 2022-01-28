package codersdk

import (
	"context"

	"golang.org/x/xerrors"
	"nhooyr.io/websocket"
	"storj.io/drpc/drpcconn"

	"github.com/coder/coder/provisionerd/proto"
)

func (c *Client) ListenProvisionerDaemon(ctx context.Context) (proto.DRPCProvisionerDaemonClient, error) {
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
	return proto.NewDRPCProvisionerDaemonClient(drpcconn.New(websocket.NetConn(context.Background(), conn, websocket.MessageBinary))), nil
}
