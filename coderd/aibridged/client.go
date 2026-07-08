package aibridged

import (
	"context"

	"storj.io/drpc"

	"github.com/coder/coder/v2/coderd/aibridged/proto"
)

type Dialer func(ctx context.Context) (DRPCClient, error)

type ClientFunc func() (DRPCClient, error)

// DRPCClient is the union of various service interfaces the client must support.
type DRPCClient interface {
	proto.DRPCRecorderClient
	proto.DRPCMCPConfiguratorClient
	proto.DRPCAuthorizerClient
	proto.DRPCProviderConfiguratorClient
}

var _ DRPCClient = &Client{}

type Client struct {
	proto.DRPCRecorderClient
	proto.DRPCMCPConfiguratorClient
	proto.DRPCAuthorizerClient
	proto.DRPCProviderConfiguratorClient

	Conn drpc.Conn
}

func (c *Client) DRPCConn() drpc.Conn {
	return c.Conn
}
