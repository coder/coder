//go:build !slim

package cli

import (
	"context"

	"golang.org/x/xerrors"

	"github.com/coder/aibridge"
	"github.com/coder/coder/v2/enterprise/coderd"
	aibridgepkg "github.com/coder/coder/v2/enterprise/coderd/aibridge"
	"github.com/coder/coder/v2/enterprise/x/aibridged"
)

func newAIBridgeDaemon(coderAPI *coderd.API) (*aibridged.Server, error) {
	ctx := context.Background()
	coderAPI.Logger.Debug(ctx, "starting in-memory aibridge daemon")

	logger := coderAPI.Logger.Named("aibridged")

	// Setup supported providers.
	openAIConfig := &aibridge.ProviderConfig{
		BaseURL: coderAPI.DeploymentValues.AI.BridgeConfig.OpenAI.BaseURL.String(),
		Key:     coderAPI.DeploymentValues.AI.BridgeConfig.OpenAI.Key.String(),
	}
	anthropicConfig := &aibridge.ProviderConfig{
		BaseURL: coderAPI.DeploymentValues.AI.BridgeConfig.Anthropic.BaseURL.String(),
		Key:     coderAPI.DeploymentValues.AI.BridgeConfig.Anthropic.Key.String(),
	}

	providers := []aibridge.Provider{
		aibridge.NewOpenAIProvider(openAIConfig),
		aibridge.NewAnthropicProvider(anthropicConfig),
	}

	// Store provider configs so we can update them when logging is toggled.
	aibridgepkg.SetProviderConfigs([]*aibridge.ProviderConfig{openAIConfig, anthropicConfig})

	// Create pool for reusable stateful [aibridge.RequestBridge] instances (one per user).
	pool, err := aibridged.NewCachedBridgePool(aibridged.DefaultPoolOptions, providers, logger.Named("pool")) // TODO: configurable.
	if err != nil {
		return nil, xerrors.Errorf("create request pool: %w", err)
	}

	// Create daemon.
	srv, err := aibridged.New(ctx, pool, func(dialCtx context.Context) (aibridged.DRPCClient, error) {
		return coderAPI.CreateInMemoryAIBridgeServer(dialCtx)
	}, logger)
	if err != nil {
		return nil, xerrors.Errorf("start in-memory aibridge daemon: %w", err)
	}
	return srv, nil
}
