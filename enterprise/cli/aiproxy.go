//go:build !slim

package cli

import (
	"context"
	"os"
	"path/filepath"

	"github.com/coder/coder/v2/enterprise/aiproxy"
	"github.com/coder/coder/v2/enterprise/coderd"
)

func newAIProxy(coderAPI *coderd.API) (*aiproxy.Server, error) {
	ctx := context.Background()
	coderAPI.Logger.Info(ctx, "starting in-memory AI proxy")

	logger := coderAPI.Logger.Named("aiproxy")

	// Load upstream proxy CA certificate if specified
	var upstreamCACert []byte
	if caPath := os.Getenv("CODER_AI_PROXY_UPSTREAM_CA"); caPath != "" {
		var err error
		upstreamCACert, err = os.ReadFile(caPath)
		if err != nil {
			return nil, err
		}
		logger.Info(ctx, "loaded upstream proxy CA certificate", "path", caPath)
	}

	// TODO: Make these configurable via deployment values
	// For now, expect certs in current working directory
	srv, err := aiproxy.New(ctx, logger, aiproxy.Options{
		ListenAddr:          ":8888",
		CertFile:            filepath.Join(".", "mitm.crt"), // This should be set to mitm-cross-signed.crt if CODER_AI_PROXY_UPSTREAM is set.
		KeyFile:             filepath.Join(".", "mitm.key"),
		CoderAccessURL:      coderAPI.AccessURL.String(),
		UpstreamProxy:       os.Getenv("CODER_AI_PROXY_UPSTREAM"),
		UpstreamProxyCACert: upstreamCACert,
	})
	if err != nil {
		return nil, err
	}

	return srv, nil
}
