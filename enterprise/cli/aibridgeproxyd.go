//go:build !slim

package cli

import (
	"context"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/enterprise/aibridgeproxyd"
	"github.com/coder/coder/v2/enterprise/coderd"
)

func newAIBridgeProxyDaemon(coderAPI *coderd.API) (*aibridgeproxyd.Server, error) {
	ctx := context.Background()
	coderAPI.Logger.Debug(ctx, "starting in-memory aibridgeproxy daemon")

	logger := coderAPI.Logger.Named("aibridgeproxyd")

	srv, err := aibridgeproxyd.New(ctx, logger, aibridgeproxyd.Options{
		ListenAddr: coderAPI.DeploymentValues.AI.BridgeProxyConfig.ListenAddr.String(),
		CertFile:   coderAPI.DeploymentValues.AI.BridgeProxyConfig.CertFile.String(),
		KeyFile:    coderAPI.DeploymentValues.AI.BridgeProxyConfig.KeyFile.String(),
	})
	if err != nil {
		return nil, xerrors.Errorf("failed to start in-memory aibridgeproxy daemon: %w", err)
	}

	return srv, nil
}
