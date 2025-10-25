//go:build !slim

package cli

import (
	"context"
	"os"

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
	openAIConfig := aibridge.NewProviderConfig(
		coderAPI.DeploymentValues.AI.BridgeConfig.OpenAI.BaseURL.String(),
		coderAPI.DeploymentValues.AI.BridgeConfig.OpenAI.Key.String(),
		os.TempDir(), // TODO: configurable?
	)
	anthropicConfig := aibridge.NewProviderConfig(
		coderAPI.DeploymentValues.AI.BridgeConfig.Anthropic.BaseURL.String(),
		coderAPI.DeploymentValues.AI.BridgeConfig.Anthropic.Key.String(),
		os.TempDir(), // TODO: configurable?
	)

	openAIProvider, err := aibridge.NewOpenAIProvider(openAIConfig)
	if err != nil {
		return nil, xerrors.Errorf("create openai provider: %w", err)
	}
	anthropicProvider, err := aibridge.NewAnthropicProvider(anthropicConfig)
	if err != nil {
		return nil, xerrors.Errorf("create anthropic provider: %w", err)
	}

	providers := []aibridge.Provider{
		openAIProvider,
		anthropicProvider,
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
