//go:build !slim

package cli

import (
	"context"
	"path/filepath"

	"github.com/coder/coder/v2/enterprise/aiproxy"
	"github.com/coder/coder/v2/enterprise/coderd"
)

func newAIProxy(coderAPI *coderd.API) (*aiproxy.Server, error) {
	ctx := context.Background()
	coderAPI.Logger.Info(ctx, "starting in-memory AI proxy")

	logger := coderAPI.Logger.Named("aiproxy")

	// TODO: Make these configurable via deployment values
	// For now, expect certs in current working directory
	srv, err := aiproxy.New(ctx, logger, aiproxy.Options{
		ListenAddr:     ":8888",
		CertFile:       filepath.Join(".", "mitm.crt"),
		KeyFile:        filepath.Join(".", "mitm.key"),
		CoderAccessURL: coderAPI.AccessURL.String(),
	})
	if err != nil {
		return nil, err
	}

	return srv, nil
}
