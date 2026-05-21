package aibridged

import (
	"context"

	"storj.io/drpc"

	"github.com/coder/coder/v2/enterprise/aibridged/proto"
)

type Dialer func(ctx context.Context) (DRPCClient, error)

type ClientFunc func() (DRPCClient, error)

// DRPCClient is the union of various service interfaces the client must support.
type DRPCClient interface {
	proto.DRPCRecorderClient
	proto.DRPCMCPConfiguratorClient
	proto.DRPCAuthorizerClient
}

var _ DRPCClient = &Client{}

type Client struct {
	proto.DRPCRecorderClient
	proto.DRPCMCPConfiguratorClient
	proto.DRPCAuthorizerClient

	Conn drpc.Conn
}

func (c *Client) DRPCConn() drpc.Conn {
	return c.Conn
}
