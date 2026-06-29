package aibridged

import "github.com/coder/coder/v2/coderd/aibridged/proto"

type DRPCServer interface {
	proto.DRPCRecorderServer
	proto.DRPCMCPConfiguratorServer
	proto.DRPCAuthorizerServer
	proto.DRPCProviderConfiguratorServer
}
