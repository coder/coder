package tailnet

import (
	"io"
	"net"

	"github.com/hashicorp/yamux"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk/drpc"
	"github.com/coder/coder/v2/tailnet/proto"
)

func NewDRPCClient(conn net.Conn) (proto.DRPCTailnetClient, error) {
	config := yamux.DefaultConfig()
	config.LogOutput = io.Discard
	session, err := yamux.Client(conn, config)
	if err != nil {
		return nil, xerrors.Errorf("multiplex client: %w", err)
	}
	return proto.NewDRPCTailnetClient(drpc.MultiplexedConn(session)), nil
}
