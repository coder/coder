//go:build !slim

package cli

import (
	"context"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/enterprise/aiproxyd"
	"github.com/coder/coder/v2/enterprise/coderd"
)

func newAIProxyDaemon(coderAPI *coderd.API) (*aiproxyd.Server, error) {
	ctx := context.Background()
	coderAPI.Logger.Debug(ctx, "starting in-memory aiproxy daemon")

	logger := coderAPI.Logger.Named("aiproxyd")

	srv, err := aiproxyd.New(ctx, logger, aiproxyd.Options{
		ListenAddr: coderAPI.DeploymentValues.AI.ProxyConfig.ListenAddr.String(),
		CertFile:   coderAPI.DeploymentValues.AI.ProxyConfig.CertFile.String(),
		KeyFile:    coderAPI.DeploymentValues.AI.ProxyConfig.KeyFile.String(),
	})
	if err != nil {
		return nil, xerrors.Errorf("failed to start in-memory aiproxy daemon: %w", err)
	}

	return srv, nil
}
