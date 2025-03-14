package tailnet
import (
	"fmt"
	"errors"
	"context"
	"net"
	"github.com/hashicorp/yamux"
	"cdr.dev/slog"
	"github.com/coder/coder/v2/codersdk/drpc"
	"github.com/coder/coder/v2/tailnet/proto"
)
func NewDRPCClient(conn net.Conn, logger slog.Logger) (proto.DRPCTailnetClient, error) {
	config := yamux.DefaultConfig()
	config.LogOutput = nil
	config.Logger = slog.Stdlib(context.Background(), logger, slog.LevelInfo)
	session, err := yamux.Client(conn, config)
	if err != nil {
		return nil, fmt.Errorf("multiplex client: %w", err)
	}
	return proto.NewDRPCTailnetClient(drpc.MultiplexedConn(session)), nil
}
